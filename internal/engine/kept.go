package engine

import (
	"errors"

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

// Unskip takes a file off the SKIP list. The file stays on the drive.
func (e *Engine) Unskip(k state.Kept) error { return e.store.RemoveSkip(k.ID) }
