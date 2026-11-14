// Package engine is the sync itself. It compares what Google Drive holds
// with what it remembers and with what is on the drive, works out what has
// to be done, and does it.
//
// The work is split in three steps a caller goes through in order:
// Refresh looks at Drive, Plan decides, Run acts. Nothing is written to the
// copy before Run.
package engine

import (
	"context"
	"io"
	"time"

	"syncer-gdrive/internal/drive"
	"syncer-gdrive/internal/state"
)

// Remote is the part of Google Drive the engine needs. *drive.Client is
// one; the tests use a made-up one.
type Remote interface {
	List(ctx context.Context) ([]drive.File, error)
	Download(ctx context.Context, id string, offset int64, w io.Writer) (int64, error)
	Export(ctx context.Context, id, googleType string, w io.Writer) (int64, error)
}

// Engine works on one destination folder.
type Engine struct {
	remote Remote
	store  *state.Store
	dest   string
	now    func() time.Time

	// last is what the latest Refresh found; Plan and Run build on it.
	last *Snapshot
}

// Open prepares the engine for a destination, creating its state on first
// use. The caller closes it.
func Open(destination string, remote Remote) (*Engine, error) {
	store, err := state.Open(destination)
	if err != nil {
		return nil, err
	}
	return &Engine{remote: remote, store: store, dest: destination, now: time.Now}, nil
}

// Close releases the state.
func (e *Engine) Close() error { return e.store.Close() }

// Store gives access to the state, for the lists the interface shows.
func (e *Engine) Store() *state.Store { return e.store }

// TrashDir is the folder inside a destination that holds the trash.
const TrashDir = ".bin"

// Why an item on Drive is not copied.
const (
	NotShortcut    = "shortcut"     // a pointer to a file that lives elsewhere
	NotUnsupported = "unsupported"  // a Google type with no export: a form, a site…
	NotService     = "service"      // a file macOS keeps for itself
	NotForbidden   = "forbidden"    // the owner does not allow downloading
	NotTooLarge    = "too-large"    // over what the file system of the drive can hold
	NotExportLimit = "export-limit" // a Google document whose export Drive refuses as too big
)

// Tally counts files and their bytes.
type Tally struct {
	Files int
	Bytes int64
}

func (t *Tally) add(bytes int64) {
	t.Files++
	t.Bytes += bytes
}
