package engine

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"syncer-gdrive/internal/drive"
	"syncer-gdrive/internal/state"
)

// fakeDrive is a made-up Google Drive: files with content, in memory.
type fakeDrive struct {
	mu      sync.Mutex
	files   map[string]*fakeFile
	listErr error
	// breakAt makes the next download of a file stop after that many bytes.
	breakAt map[string]int
	// refuse makes fetching a file fail with a Drive refusal.
	refuse    map[string]string
	downloads map[string]int // how many times each file was asked for
	offsets   map[string][]int64
}

type fakeFile struct {
	drive.File
	content []byte
}

func newFakeDrive() *fakeDrive {
	return &fakeDrive{files: map[string]*fakeFile{}, breakAt: map[string]int{}, refuse: map[string]string{},
		downloads: map[string]int{}, offsets: map[string][]int64{}}
}

func day(n int) time.Time { return time.Date(2026, 11, n, 10, 0, 0, 0, time.UTC) }

// folder adds a folder; an empty parent makes it one shared with the account.
func (d *fakeDrive) folder(id, parent, name string) {
	d.files[id] = &fakeFile{File: drive.File{ID: id, Parent: parent, Name: name, MimeType: "application/vnd.google-apps.folder",
		Created: day(1), Modified: day(1), CanDownload: true}}
}

// file adds an ordinary file created on the given day of the month.
func (d *fakeDrive) file(id, parent, name, content string, created int) *fakeFile {
	f := &fakeFile{File: drive.File{ID: id, Parent: parent, Name: name, MimeType: "application/octet-stream",
		Created: day(created), Modified: day(created), CanDownload: true}}
	d.files[id] = f
	d.write(id, content)
	return f
}

// google adds a Google document: it has no checksum and its size on Drive
// says nothing about its export.
func (d *fakeDrive) google(id, parent, name, kind, exported string) {
	d.files[id] = &fakeFile{File: drive.File{ID: id, Parent: parent, Name: name, MimeType: "application/vnd.google-apps." + kind,
		Size: 7, Created: day(2), Modified: day(2), CanDownload: true}, content: []byte(exported)}
}

// write changes the content of a file, as an edit on Drive would.
func (d *fakeDrive) write(id, content string) {
	f := d.files[id]
	f.content = []byte(content)
	sum := md5.Sum(f.content)
	f.MD5, f.Size = hex.EncodeToString(sum[:]), int64(len(content))
}

func (d *fakeDrive) List(ctx context.Context) ([]drive.File, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.listErr != nil {
		return nil, d.listErr
	}
	var out []drive.File
	for _, f := range d.files {
		out = append(out, f.File)
	}
	return out, nil
}

func (d *fakeDrive) Download(ctx context.Context, id string, offset int64, w io.Writer) (int64, error) {
	d.mu.Lock()
	f, ok := d.files[id]
	d.downloads[id]++
	d.offsets[id] = append(d.offsets[id], offset)
	reason := d.refuse[id]
	stop, breaks := d.breakAt[id]
	delete(d.breakAt, id)
	d.mu.Unlock()
	if !ok {
		return 0, &drive.APIError{Status: 404, Reason: "notFound", Message: "File not found"}
	}
	if reason != "" {
		return 0, &drive.APIError{Status: 403, Reason: reason, Message: "refused"}
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	rest := f.content[min(offset, int64(len(f.content))):]
	if breaks && stop < len(rest) {
		n, _ := w.Write(rest[:stop])
		return int64(n), errors.New("connection reset")
	}
	n, err := w.Write(rest)
	return int64(n), err
}

func (d *fakeDrive) Export(ctx context.Context, id, googleType string, w io.Writer) (int64, error) {
	d.mu.Lock()
	f, ok := d.files[id]
	d.downloads[id]++
	reason := d.refuse[id]
	d.mu.Unlock()
	if !ok {
		return 0, &drive.APIError{Status: 404, Reason: "notFound", Message: "File not found"}
	}
	if reason != "" {
		return 0, &drive.APIError{Status: 403, Reason: reason, Message: "refused"}
	}
	n, err := w.Write(f.content)
	return int64(n), err
}

// open makes an engine over a made-up Drive and a temporary destination.
func open(t *testing.T, d *fakeDrive) (*Engine, string) {
	t.Helper()
	dest := t.TempDir()
	e, err := Open(dest, d)
	if err != nil {
		t.Fatal(err)
	}
	e.now = func() time.Time { return day(20) }
	t.Cleanup(func() { e.Close() })
	return e, dest
}

// planned looks at Drive and returns the plan, the downloads by target.
func planned(t *testing.T, e *Engine) (*Plan, map[string]Download) {
	t.Helper()
	p, err := e.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]Download{}
	for _, d := range p.Downloads {
		by[d.Target] = d
	}
	return p, by
}

// placeCopy puts a file on the drive and records it as copied, the way a
// finished download does. It lets the plan be tested apart from the run.
func placeCopy(t *testing.T, e *Engine, d *fakeDrive, id string) {
	t.Helper()
	it := e.last.Items[id]
	full := e.local(e.last.Wanted[id])
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, d.files[id].content, 0o644); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(full, it.Modified, it.Modified)
	info, _ := os.Stat(full)
	if err := e.store.SetPath(id, e.last.Wanted[id]); err != nil {
		t.Fatal(err)
	}
	if err := e.store.SetCopied(id, state.Copy{MD5: it.MD5, Modified: it.Modified, Size: info.Size(), MTime: info.ModTime(), At: day(20)}); err != nil {
		t.Fatal(err)
	}
}

// entries lists what is in the copy, leaving the program's own folders out.
func entries(t *testing.T, dest string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.WalkDir(dest, func(path string, entry os.DirEntry, err error) error {
		rel, _ := filepath.Rel(dest, path)
		if entry.IsDir() {
			if rel == state.Dir || rel == TrashDir {
				return filepath.SkipDir
			}
			return nil
		}
		data, _ := os.ReadFile(path)
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	return out
}
