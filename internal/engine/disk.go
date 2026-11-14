package engine

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"syncer-gdrive/internal/names"
	"syncer-gdrive/internal/state"
)

// Everything the engine does to files on the drive goes through this file.
// Paths are relative to the destination and use forward slashes. A later
// version that encrypts the copy changes this layer and nothing above it.

// partsDir holds downloads that are not finished yet. It is inside the
// destination, so a finished file is moved into place within one volume.
var partsDir = path.Join(state.Dir, "parts")

// exists tells whether a file is at the place.
func (e *Engine) exists(rel string) bool {
	info, err := os.Stat(e.local(rel))
	return err == nil && !info.IsDir()
}

// move carries a file to another place, creating the folders on the way.
func (e *Engine) move(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(e.local(to)), 0o755); err != nil {
		return err
	}
	if err := os.Rename(e.local(from), e.local(to)); err != nil {
		return err
	}
	e.prune(path.Dir(from))
	return nil
}

// remove deletes a file. One that is already gone is not an error.
func (e *Engine) remove(rel string) error {
	err := os.Remove(e.local(rel))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err == nil {
		e.prune(path.Dir(rel))
	}
	return err
}

// prune removes folders that became empty, up to the destination itself.
// The program's own folders are left alone.
func (e *Engine) prune(dir string) {
	for dir != "." && dir != "/" && dir != "" && dir != state.Dir && dir != TrashDir {
		if os.Remove(e.local(dir)) != nil { // not empty, or gone
			return
		}
		dir = path.Dir(dir)
	}
}

// free finds a place next to rel that nothing occupies, by numbering.
func (e *Engine) free(rel string) string {
	dir, name := path.Split(rel)
	candidate := rel
	for n := 2; e.exists(candidate) || e.isDir(candidate); n++ {
		candidate = dir + names.Numbered(name, n)
	}
	return candidate
}

func (e *Engine) isDir(rel string) bool {
	info, err := os.Stat(e.local(rel))
	return err == nil && info.IsDir()
}

// toTrash moves a file of the copy into the trash, keeping its folders. A
// file already there under the same path is not overwritten: the newcomer
// gets the moment in its name.
func (e *Engine) toTrash(rel, reason string, created, modified time.Time) error {
	info, err := os.Stat(e.local(rel))
	if err != nil {
		return err
	}
	stored := path.Join(TrashDir, rel)
	if e.exists(stored) || e.isDir(stored) {
		dir, name := path.Split(stored)
		ext := path.Ext(name)
		if ext == name {
			ext = ""
		}
		stamp := " (" + e.now().Format("2006-01-02 15-04-05") + ")"
		stored = e.free(dir + strings.TrimSuffix(name, ext) + stamp + ext)
	}
	if err := e.move(rel, stored); err != nil {
		return err
	}
	if modified.IsZero() {
		modified = info.ModTime()
	}
	return e.store.AddTrash(state.Kept{
		Path: rel, Stored: stored, Size: info.Size(), Created: created, Modified: modified, Added: e.now(), Reason: reason,
	})
}

// checksum reads a file and returns its MD5, the checksum Drive reports.
func checksum(full string) (string, error) {
	file, err := os.Open(full)
	if err != nil {
		return "", err
	}
	defer file.Close()
	sum := md5.New()
	if _, err := io.Copy(sum, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}
