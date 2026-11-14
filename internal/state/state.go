// Package state keeps what the program knows about one destination folder:
// what Drive held at the last look, what has been copied, what the user
// selected, the SKIP list and the trash. It is a SQLite database inside the
// destination, so the knowledge travels with the drive.
//
// Every change is a transaction: a drive pulled out in the middle of a sync
// leaves the database as it was before the interrupted change.
package state

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // SQLite without cgo: the build needs no C compiler
)

// Dir is the folder inside a destination that holds the state.
const Dir = ".sync-states"

const schema = 1

// Store is the open state of one destination.
type Store struct {
	db *sql.DB
}

// Item is one thing on Drive as the program last saw it and, for a file,
// what is known about its copy on the drive.
type Item struct {
	ID     string
	Parent string // the Drive folder it is in; empty at the top level
	Name   string // as on Drive
	// Path is where the item lives on the drive, relative to the
	// destination, with forward slashes. It is decided once and kept.
	Path     string
	Folder   bool
	Google   string // "document", "spreadsheet"…; empty for an ordinary file
	Size     int64
	MD5      string
	Created  time.Time
	Modified time.Time
	Seen     bool  // it was in the last listing
	Copy     *Copy // nil until the file has been copied
}

// Copy describes the file on the drive right after it was copied.
type Copy struct {
	MD5      string    // the version of the content that was copied
	Modified time.Time // the Drive date of that version; what tells Google documents apart
	Size     int64     // the size of the file on the drive
	MTime    time.Time // its modification time on the drive
	At       time.Time
}

// Kept is a file that stays on the drive outside the copy: on the SKIP
// list or in the trash.
type Kept struct {
	ID       int64
	Path     string // where it was in the copy
	Stored   string // trash only: where it lies now, relative to the destination
	Size     int64
	Created  time.Time
	Modified time.Time
	Added    time.Time
	Reason   string // trash only: why it is there
}

// Open opens the state of a destination, creating it on first use.
func Open(destination string) (*Store, error) {
	dir := filepath.Join(destination, Dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("the state folder cannot be created: %w", err)
	}
	// FULL waits for the drive to confirm each transaction: slower, but a
	// flash drive may be pulled out at any moment.
	dsn := "file:" + filepath.ToSlash(filepath.Join(dir, "state.db")) + "?_pragma=synchronous(FULL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // one writer; the program is the only user of the file
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the database file.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("the state database cannot be read: %w", err)
	}
	switch {
	case version > schema:
		return fmt.Errorf("the state on this drive was written by a newer version of the program (format %d, this one knows %d)", version, schema)
	case version == schema:
		return nil
	}
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS items (
	id TEXT PRIMARY KEY,
	parent TEXT NOT NULL,
	name TEXT NOT NULL,
	path TEXT NOT NULL,
	folder INTEGER NOT NULL,
	google TEXT NOT NULL,
	size INTEGER NOT NULL,
	md5 TEXT NOT NULL,
	created INTEGER NOT NULL,
	modified INTEGER NOT NULL,
	seen INTEGER NOT NULL,
	copied INTEGER NOT NULL DEFAULT 0,
	copy_md5 TEXT NOT NULL DEFAULT '',
	copy_modified INTEGER NOT NULL DEFAULT 0,
	copy_size INTEGER NOT NULL DEFAULT 0,
	copy_mtime INTEGER NOT NULL DEFAULT 0,
	copy_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS items_parent ON items(parent);
CREATE TABLE IF NOT EXISTS checked (id TEXT PRIMARY KEY, yes INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS take_new (folder TEXT PRIMARY KEY, yes INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS skip (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	path TEXT NOT NULL UNIQUE,
	size INTEGER NOT NULL, created INTEGER NOT NULL, modified INTEGER NOT NULL, added INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS trash (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	path TEXT NOT NULL,
	stored TEXT NOT NULL UNIQUE,
	size INTEGER NOT NULL, created INTEGER NOT NULL, modified INTEGER NOT NULL, added INTEGER NOT NULL,
	reason TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
PRAGMA user_version = 1;`)
	if err != nil {
		return fmt.Errorf("the state database cannot be set up: %w", err)
	}
	return nil
}

// Times are stored as nanoseconds since 1970; zero stands for "unknown".
func stamp(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

func moment(n int64) time.Time {
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n).UTC()
}

// tx runs a change as one transaction.
func (s *Store) tx(change func(*sql.Tx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if err := change(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Items returns everything known about Drive, in no particular order.
func (s *Store) Items() ([]Item, error) {
	rows, err := s.db.Query(`SELECT id, parent, name, path, folder, google, size, md5, created, modified, seen,
		copied, copy_md5, copy_modified, copy_size, copy_mtime, copy_at FROM items`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		var it Item
		var c Copy
		var created, modified, copyModified, copyMTime, copyAt int64
		var copied bool
		if err := rows.Scan(&it.ID, &it.Parent, &it.Name, &it.Path, &it.Folder, &it.Google, &it.Size, &it.MD5,
			&created, &modified, &it.Seen, &copied, &c.MD5, &copyModified, &c.Size, &copyMTime, &copyAt); err != nil {
			return nil, err
		}
		it.Created, it.Modified = moment(created), moment(modified)
		if copied {
			c.Modified, c.MTime, c.At = moment(copyModified), moment(copyMTime), moment(copyAt)
			it.Copy = &c
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// SaveListing records what Drive holds now. Items in the list are added or
// refreshed; what is known about their copies stays. Every other known item
// is marked as not seen: it is gone from Drive, or the account lost access.
func (s *Store) SaveListing(items []Item) error {
	return s.tx(func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE items SET seen = 0"); err != nil {
			return err
		}
		put, err := tx.Prepare(`INSERT INTO items (id, parent, name, path, folder, google, size, md5, created, modified, seen)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
			ON CONFLICT(id) DO UPDATE SET parent = excluded.parent, name = excluded.name, path = excluded.path,
				folder = excluded.folder, google = excluded.google, size = excluded.size, md5 = excluded.md5,
				created = excluded.created, modified = excluded.modified, seen = 1`)
		if err != nil {
			return err
		}
		defer put.Close()
		for _, it := range items {
			if _, err := put.Exec(it.ID, it.Parent, it.Name, it.Path, it.Folder, it.Google, it.Size, it.MD5,
				stamp(it.Created), stamp(it.Modified)); err != nil {
				return err
			}
		}
		return nil
	})
}

// SetCopied records that a file has been copied, and what the copy is like.
func (s *Store) SetCopied(id string, c Copy) error {
	res, err := s.db.Exec(`UPDATE items SET copied = 1, copy_md5 = ?, copy_modified = ?, copy_size = ?, copy_mtime = ?, copy_at = ?
		WHERE id = ?`, c.MD5, stamp(c.Modified), c.Size, stamp(c.MTime), stamp(c.At), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("the copied file is not in the state")
	}
	return nil
}

// ClearCopied records that a file is no longer on the drive as a copy: it
// went to the trash or onto the SKIP list, or was deleted.
func (s *Store) ClearCopied(ids ...string) error {
	return s.tx(func(tx *sql.Tx) error {
		for _, id := range ids {
			if _, err := tx.Exec("UPDATE items SET copied = 0, copy_md5 = '', copy_modified = 0, copy_size = 0, copy_mtime = 0, copy_at = 0 WHERE id = ?", id); err != nil {
				return err
			}
		}
		return nil
	})
}

// Forget removes items that are gone from Drive and dealt with.
func (s *Store) Forget(ids ...string) error {
	return s.tx(func(tx *sql.Tx) error {
		for _, id := range ids {
			for _, q := range []string{"DELETE FROM items WHERE id = ?", "DELETE FROM checked WHERE id = ?", "DELETE FROM take_new WHERE folder = ?"} {
				if _, err := tx.Exec(q, id); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Checked returns which files are selected. A file that is not in the map
// has not been decided yet.
func (s *Store) Checked() (map[string]bool, error) { return s.flags("SELECT id, yes FROM checked") }

// SetChecked records the selection of files.
func (s *Store) SetChecked(checked map[string]bool) error {
	return s.setFlags("INSERT INTO checked (id, yes) VALUES (?, ?) ON CONFLICT(id) DO UPDATE SET yes = excluded.yes", checked)
}

// TakeNew returns, per folder, whether files and folders that appear in it
// later are synced while the folder is selected only in part. The empty
// identifier stands for the top level. A folder that is not in the map
// takes new items.
func (s *Store) TakeNew() (map[string]bool, error) {
	return s.flags("SELECT folder, yes FROM take_new")
}

// SetTakeNew records the switches "new files and folders here".
func (s *Store) SetTakeNew(folders map[string]bool) error {
	return s.setFlags("INSERT INTO take_new (folder, yes) VALUES (?, ?) ON CONFLICT(folder) DO UPDATE SET yes = excluded.yes", folders)
}

func (s *Store) flags(query string) (map[string]bool, error) {
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		var yes bool
		if err := rows.Scan(&id, &yes); err != nil {
			return nil, err
		}
		out[id] = yes
	}
	return out, rows.Err()
}

func (s *Store) setFlags(query string, flags map[string]bool) error {
	return s.tx(func(tx *sql.Tx) error {
		put, err := tx.Prepare(query)
		if err != nil {
			return err
		}
		defer put.Close()
		for id, yes := range flags {
			if _, err := put.Exec(id, yes); err != nil {
				return err
			}
		}
		return nil
	})
}

// Skipped returns the SKIP list.
func (s *Store) Skipped() ([]Kept, error) {
	return s.kept("SELECT id, path, '', size, created, modified, added, '' FROM skip ORDER BY id")
}

// AddSkip puts a file on the SKIP list. A file that is already there stays
// as it was.
func (s *Store) AddSkip(k Kept) error {
	_, err := s.db.Exec("INSERT OR IGNORE INTO skip (path, size, created, modified, added) VALUES (?, ?, ?, ?, ?)",
		k.Path, k.Size, stamp(k.Created), stamp(k.Modified), stamp(k.Added))
	return err
}

// RemoveSkip takes files off the SKIP list.
func (s *Store) RemoveSkip(ids ...int64) error { return s.remove("DELETE FROM skip WHERE id = ?", ids) }

// Trashed returns what is in the trash.
func (s *Store) Trashed() ([]Kept, error) {
	return s.kept("SELECT id, path, stored, size, created, modified, added, reason FROM trash ORDER BY id")
}

// AddTrash records a file that was moved into the trash.
func (s *Store) AddTrash(k Kept) error {
	_, err := s.db.Exec("INSERT INTO trash (path, stored, size, created, modified, added, reason) VALUES (?, ?, ?, ?, ?, ?, ?)",
		k.Path, k.Stored, k.Size, stamp(k.Created), stamp(k.Modified), stamp(k.Added), k.Reason)
	return err
}

// RemoveTrash drops the records of files that left the trash.
func (s *Store) RemoveTrash(ids ...int64) error {
	return s.remove("DELETE FROM trash WHERE id = ?", ids)
}

func (s *Store) kept(query string) ([]Kept, error) {
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Kept
	for rows.Next() {
		var k Kept
		var created, modified, added int64
		if err := rows.Scan(&k.ID, &k.Path, &k.Stored, &k.Size, &created, &modified, &added, &k.Reason); err != nil {
			return nil, err
		}
		k.Created, k.Modified, k.Added = moment(created), moment(modified), moment(added)
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Store) remove(query string, ids []int64) error {
	return s.tx(func(tx *sql.Tx) error {
		for _, id := range ids {
			if _, err := tx.Exec(query, id); err != nil {
				return err
			}
		}
		return nil
	})
}

// Meta returns a stored value, such as the time of the last sync; empty if
// there is none.
func (s *Store) Meta(key string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM meta WHERE key = ?", key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

// SetMeta stores a value.
func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec("INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", key, value)
	return err
}
