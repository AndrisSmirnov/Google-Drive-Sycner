package engine

import (
	"context"
	"os"
	"path/filepath"
	"sort"

	"syncer-gdrive/internal/state"
	"syncer-gdrive/internal/volume"
)

// Why a file is downloaded.
const (
	ForNew     = "new"     // selected and never copied
	ForUpdated = "updated" // changed on Drive since it was copied
	ForMissing = "missing" // its copy is gone from the drive
	ForEdited  = "edited"  // its copy was changed by hand: that version goes to the trash first
)

// Why a file on the drive has no counterpart to sync with any more.
const (
	LeftDeleted    = "deleted"    // it is gone from Drive
	LeftUnselected = "unselected" // the user no longer selects it
)

// Download is one file to fetch.
type Download struct {
	Item   state.Item
	Target string // where it goes, relative to the destination
	Why    string
	// Size is how many bytes are expected. For a Google document it is a
	// guess: the real size is known only once it has been exported.
	Size int64
}

// Move is a copy to carry to another place because the file was renamed or
// moved on Drive.
type Move struct {
	ID       string
	From, To string
}

// Leftover is a file on the drive that the sync no longer keeps up to date.
// What happens to it is the user's decision.
type Leftover struct {
	Item state.Item
	Why  string
}

// Plan is what a sync would do. Making it changes nothing on the drive.
type Plan struct {
	Downloads []Download
	Moves     []Move
	Leftovers []Leftover

	// Lost are folders shared with the account that it can no longer see.
	// That is not a deletion: their copies stay untouched.
	Lost      []state.Item
	LostFiles Tally

	// forget are items gone from Drive with nothing left on the drive;
	// forgetCopy are unselected files whose copy is already gone; keep are
	// the copies under lost folders.
	forget, forgetCopy, keep []string

	NotCopied     map[string]Tally // by reason
	NewUnselected Tally            // appeared on Drive, not selected
	Clashes       int              // selected files sharing a name in their folder

	Bytes    int64       // to download, with the guesses for Google documents
	Volume   volume.Info // the volume of the destination
	Writable bool
}

// Fits tells whether the downloads fit on the drive.
func (p *Plan) Fits() bool { return p.Bytes <= p.Volume.Free }

// A Google document may export to many times the size Drive reports for
// it: 14 Kb became 230 Kb on a real folder. Room is reserved generously.
func exportGuess(size int64) int64 {
	return max(size*20, 1<<20)
}

// local is the full path of a place in the copy.
func (e *Engine) local(rel string) string {
	return filepath.Join(e.dest, filepath.FromSlash(rel))
}

// Plan looks at Drive, at the state and at the drive, and works out what a
// sync has to do.
func (e *Engine) Plan(ctx context.Context) (*Plan, error) {
	s, err := e.Refresh(ctx)
	if err != nil {
		return nil, err
	}
	p := &Plan{NotCopied: map[string]Tally{}, NewUnselected: s.NewUnselected, Clashes: len(s.Clash)}
	for reason, t := range s.NotCopied {
		p.NotCopied[reason] = t
	}
	if info, err := volume.Stat(e.dest); err == nil {
		p.Volume = info
	}
	p.Writable = writable(e.dest)
	limit := p.Volume.MaxFileSize()

	for id, it := range s.Items {
		if it.Folder {
			continue
		}
		selected := s.Checked[id]
		if it.Copy == nil {
			if !selected {
				continue
			}
			e.download(p, it, s.Wanted[id], ForNew, limit)
			continue
		}

		info, err := os.Stat(e.local(it.Path))
		present := err == nil && !info.IsDir()
		if !selected {
			// Still on the drive, no longer wanted: the user decides.
			if present {
				p.Leftovers = append(p.Leftovers, Leftover{it, LeftUnselected})
			} else {
				p.forgetCopy = append(p.forgetCopy, id)
			}
			continue
		}
		changedOnDrive := it.MD5 != it.Copy.MD5
		if it.Google != "" {
			changedOnDrive = !it.Modified.Equal(it.Copy.Modified)
		}
		switch {
		case !present:
			e.download(p, it, s.Wanted[id], ForMissing, limit)
		case info.Size() != it.Copy.Size || !info.ModTime().Equal(it.Copy.MTime):
			e.download(p, it, s.Wanted[id], ForEdited, limit)
		case changedOnDrive:
			e.download(p, it, s.Wanted[id], ForUpdated, limit)
		case it.Path != s.Wanted[id]:
			p.Moves = append(p.Moves, Move{ID: id, From: it.Path, To: s.Wanted[id]})
		}
	}

	// Items gone from Drive. If a folder that was shared with the account
	// is gone as a whole, the account has most likely lost access to it:
	// to the program that looks exactly like everything in it being
	// deleted, and treating it so would empty the copy.
	gone := map[string]state.Item{}
	for _, it := range s.gone {
		gone[it.ID] = it
	}
	lostRoot := func(it state.Item) (state.Item, bool) {
		for {
			parent, ok := gone[it.Parent]
			if !ok {
				return it, it.Parent == "" && it.Folder
			}
			it = parent
		}
	}
	lost := map[string]bool{}
	for _, it := range s.gone {
		if it.Folder || it.Copy == nil {
			p.forget = append(p.forget, it.ID)
			continue
		}
		if _, err := os.Stat(e.local(it.Path)); err != nil {
			p.forget = append(p.forget, it.ID)
			continue
		}
		if root, ok := lostRoot(it); ok {
			if !lost[root.ID] {
				lost[root.ID] = true
				p.Lost = append(p.Lost, root)
			}
			p.LostFiles.add(it.Copy.Size)
			p.keep = append(p.keep, it.ID)
			continue
		}
		p.Leftovers = append(p.Leftovers, Leftover{it, LeftDeleted})
	}
	// What belongs to a lost folder is remembered, not forgotten: if the
	// folder is shared again, its copies are still known.
	kept := map[string]bool{}
	for _, id := range p.keep {
		kept[id] = true
	}
	for _, it := range s.gone {
		if root, ok := lostRoot(it); ok && lost[root.ID] {
			kept[it.ID] = true
		}
	}
	var forget []string
	for _, id := range p.forget {
		if !kept[id] {
			forget = append(forget, id)
		}
	}
	p.forget = forget

	sort.Slice(p.Downloads, func(a, b int) bool { return p.Downloads[a].Target < p.Downloads[b].Target })
	sort.Slice(p.Moves, func(a, b int) bool { return p.Moves[a].From < p.Moves[b].From })
	sort.Slice(p.Leftovers, func(a, b int) bool { return p.Leftovers[a].Item.Path < p.Leftovers[b].Item.Path })
	sort.Slice(p.Lost, func(a, b int) bool { return p.Lost[a].Path < p.Lost[b].Path })
	return p, nil
}

// download adds a file to the plan, unless the drive cannot hold it.
func (e *Engine) download(p *Plan, it state.Item, target, why string, limit int64) {
	if limit > 0 && it.Size > limit && it.Google == "" {
		t := p.NotCopied[NotTooLarge]
		t.add(it.Size)
		p.NotCopied[NotTooLarge] = t
		return
	}
	size := it.Size
	if it.Google != "" {
		size = exportGuess(it.Size)
	}
	p.Downloads = append(p.Downloads, Download{Item: it, Target: target, Why: why, Size: size})
	p.Bytes += size
}

// writable tells whether files can be created in the destination.
func writable(dest string) bool {
	if err := os.MkdirAll(filepath.Join(dest, state.Dir), 0o755); err != nil {
		return false
	}
	probe, err := os.CreateTemp(filepath.Join(dest, state.Dir), "probe-*")
	if err != nil {
		return false
	}
	probe.Close()
	os.Remove(probe.Name())
	return true
}
