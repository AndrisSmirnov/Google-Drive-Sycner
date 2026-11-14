package engine

import (
	"context"
	"errors"
	"os"

	"syncer-gdrive/internal/state"
)

// The trash and the SKIP list: what the interface lets the user do with
// files that stay on the drive outside the copy.

// Restore puts a file from the trash back where it was. If something lies
// there now, the file is restored next to it under a numbered name. It
// returns the place the file ended up at.
func (e *Engine) Restore(k state.Kept) (string, error) {
	if !e.exists(k.Stored) {
		// The file was removed from the drive by hand: only the record is left.
		return "", errors.Join(errors.New("the file is no longer in the trash"), e.store.RemoveTrash(k.ID))
	}
	target := e.free(k.Path)
	if err := e.move(k.Stored, target); err != nil {
		return "", err
	}
	return target, e.store.RemoveTrash(k.ID)
}

// DeleteForever removes a file of the trash from the drive.
func (e *Engine) DeleteForever(k state.Kept) error {
	if err := e.remove(k.Stored); err != nil {
		return err
	}
	return e.store.RemoveTrash(k.ID)
}

// EmptyTrash removes everything in the trash from the drive.
func (e *Engine) EmptyTrash() error {
	trashed, err := e.store.Trashed()
	if err != nil {
		return err
	}
	for _, k := range trashed {
		if err := e.DeleteForever(k); err != nil {
			return err
		}
	}
	return nil
}

// Checked is what a full check of the copy found.
type Checked struct {
	Good    Tally // files whose content matches what was copied
	Damaged int   // files whose content differs: bit rot, or an edit that kept size and time
	Missing int   // files that are gone from the drive
}

// Verify reads every copied file and compares its checksum with the one
// recorded when it was copied. A sync only looks at sizes and times; this
// is the slow, thorough check. Files that fail it are forgotten as copies,
// so the next sync fetches them again; the damaged file itself is not
// deleted — that sync moves it into the trash. Google documents have no
// checksum and are not checked.
func (e *Engine) Verify(ctx context.Context) (Checked, error) {
	var found Checked
	items, err := e.store.Items()
	if err != nil {
		return found, err
	}
	var redo []string
	for _, it := range items {
		if it.Copy == nil || it.Google != "" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return found, err
		}
		sum, err := checksum(e.local(it.Path))
		switch {
		case errors.Is(err, os.ErrNotExist):
			found.Missing++
			redo = append(redo, it.ID)
		case err != nil:
			return found, err
		case sum != it.Copy.MD5:
			found.Damaged++
			redo = append(redo, it.ID)
		default:
			found.Good.add(it.Copy.Size)
		}
	}
	return found, e.store.ClearCopied(redo...)
}

// Unskip takes a file off the SKIP list. The file stays on the drive.
func (e *Engine) Unskip(k state.Kept) error { return e.store.RemoveSkip(k.ID) }
