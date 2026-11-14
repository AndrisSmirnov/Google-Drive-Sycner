package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"syncer-gdrive/internal/drive"
	"syncer-gdrive/internal/state"
)

// What to do with a leftover: a file on the drive that is gone from Drive
// or no longer selected.
const (
	ToTrash  = iota // move it into the trash; the default
	ToSkip          // leave it where it is and stop tracking it
	ToDelete        // remove it from the drive
)

// Why a version of a file is in the trash.
const (
	TrashDeleted    = "drive"      // it was deleted on Drive
	TrashUnselected = "unselected" // the user stopped selecting it
	TrashEdited     = "local"      // it was changed on the drive and replaced by the Drive version
)

const (
	workers = 4 // files fetched at the same time
	tries   = 3 // attempts per file within one run
)

// Event tells what a run is doing. Events come from several goroutines at
// once: whoever receives them has to be ready for that.
type Event struct {
	Kind   int
	Target string
	// Size is the expected size of a file that starts; Bytes is how many
	// bytes were just written, negative when a download starts over.
	Size  int64
	Bytes int64
	Err   error  // of a failure
	Why   string // of a file left out: one of the Not… reasons
}

// Kinds of events.
const (
	Started = iota
	Advanced
	Finished
	Failed
	LeftOut
)

// Failure is a file that could not be fetched in this run. The next run
// tries it again.
type Failure struct {
	Target string
	Err    error
}

// Result is what a run did.
type Result struct {
	Downloaded Tally // files that were not on the drive before
	Updated    Tally // files fetched again: changed, missing or edited
	Moved      int   // copies carried to a new place
	Trashed    int   // leftovers and edited versions moved into the trash
	Skipped    int   // leftovers put on the SKIP list
	Deleted    int   // leftovers removed from the drive
	Renamed    int   // SKIP files renamed because Drive has a file of that name again

	NotCopied map[string]Tally // found out while fetching: forbidden, export too large
	Failures  []Failure
	Stopped   bool  // the run was cancelled
	Left      Tally // what was planned and not done
	Took      time.Duration
}

// Run carries out a plan. decide gives the action for each leftover; nil
// puts them all into the trash. report, if not nil, is told what happens.
// Cancelling the context stops the run: what is done stays done, and files
// that were on their way are continued by the next run.
func (e *Engine) Run(ctx context.Context, p *Plan, decide func(Leftover) int, report func(Event)) (*Result, error) {
	if report == nil {
		report = func(Event) {}
	}
	began := e.now()
	r := &Result{NotCopied: map[string]Tally{}}

	if err := e.store.Forget(p.forget...); err != nil {
		return r, err
	}
	if err := e.store.ClearCopied(p.forgetCopy...); err != nil {
		return r, err
	}
	e.dropStaleParts(p)

	// Leftovers first: they may free the names and the room the downloads need.
	for _, l := range p.Leftovers {
		action := ToTrash
		if decide != nil {
			action = decide(l)
		}
		if err := e.settle(l, action, r); err != nil {
			r.Failures = append(r.Failures, Failure{l.Item.Path, err})
		}
	}

	for _, m := range p.Moves {
		if err := e.carry(m, r); err != nil {
			r.Failures = append(r.Failures, Failure{m.From, err})
		}
	}

	var mu sync.Mutex
	jobs := make(chan Download)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for d := range jobs {
				err := e.fetch(ctx, d, report)
				mu.Lock()
				var left *notCopied
				switch {
				case err == nil:
					tally := &r.Updated
					if d.Why == ForNew {
						tally = &r.Downloaded
					}
					tally.add(d.Item.Size)
					if d.Why == ForEdited {
						r.Trashed++
					}
				case errors.As(err, &left):
					t := r.NotCopied[left.why]
					t.add(d.Item.Size)
					r.NotCopied[left.why] = t
					report(Event{Kind: LeftOut, Target: d.Target, Why: left.why})
				case ctx.Err() != nil:
					r.Left.add(d.Size)
				default:
					r.Failures = append(r.Failures, Failure{d.Target, err})
					report(Event{Kind: Failed, Target: d.Target, Err: err})
				}
				mu.Unlock()
			}
		}()
	}
	for i, d := range p.Downloads {
		if ctx.Err() != nil {
			mu.Lock()
			for _, rest := range p.Downloads[i:] {
				r.Left.add(rest.Size)
			}
			mu.Unlock()
			break
		}
		jobs <- d
	}
	close(jobs)
	wg.Wait()

	r.Stopped = ctx.Err() != nil
	r.Took = e.now().Sub(began)
	if !r.Stopped {
		if err := e.store.SetMeta(MetaLastSync, e.now().UTC().Format(time.RFC3339)); err != nil {
			return r, err
		}
	}
	return r, nil
}

// MetaLastSync is the key under which the time of the last finished sync
// is kept.
const MetaLastSync = "last-sync"

// settle does to a leftover what the user chose.
func (e *Engine) settle(l Leftover, action int, r *Result) error {
	it := l.Item
	switch action {
	case ToDelete:
		if err := e.remove(it.Path); err != nil {
			return err
		}
		r.Deleted++
	case ToSkip:
		if err := e.store.AddSkip(state.Kept{Path: it.Path, Size: it.Copy.Size, Created: it.Created, Modified: it.Modified, Added: e.now()}); err != nil {
			return err
		}
		r.Skipped++
	default:
		reason := TrashDeleted
		if l.Why == LeftUnselected {
			reason = TrashUnselected
		}
		if err := e.toTrash(it.Path, reason, it.Created, it.Modified); err != nil {
			return err
		}
		r.Trashed++
	}
	if l.Why == LeftUnselected {
		return e.store.ClearCopied(it.ID) // still on Drive, just not copied
	}
	return e.store.Forget(it.ID)
}

// carry moves a copy to the place its file now has on Drive.
func (e *Engine) carry(m Move, r *Result) error {
	if err := e.makeRoom(m.To); err != nil {
		return err
	}
	if err := e.move(m.From, m.To); err != nil {
		return err
	}
	r.Moved++
	return e.store.SetPath(m.ID, m.To)
}

// makeRoom clears a place for a file of the copy without ever destroying
// what lies there. A file on the SKIP list is renamed, as agreed with the
// user for the case "Drive has a file of that name again"; anything else
// goes into the trash.
func (e *Engine) makeRoom(target string) error {
	if !e.exists(target) {
		return nil
	}
	skipped, err := e.store.Skipped()
	if err != nil {
		return err
	}
	for _, k := range skipped {
		if k.Path != target {
			continue
		}
		ext := path.Ext(target)
		if ext == path.Base(target) {
			ext = ""
		}
		renamed := e.free(strings.TrimSuffix(target, ext) + "_skipped_" + k.Added.Format("2006-01-02") + ext)
		if err := e.move(target, renamed); err != nil {
			return err
		}
		return e.store.RemoveSkip(k.ID)
	}
	return e.toTrash(target, TrashEdited, time.Time{}, time.Time{})
}

// notCopied is a file Drive will not give, now or ever in this form.
type notCopied struct{ why string }

func (n *notCopied) Error() string { return "not copied: " + n.why }

// partFor names the unfinished download of a file. The checksum is part of
// the name, so a file that changed on Drive never continues an old part.
func partFor(it state.Item) string {
	if it.Google != "" {
		return path.Join(partsDir, it.ID+".export.part")
	}
	return path.Join(partsDir, it.ID+"-"+it.MD5+".part")
}

// dropStaleParts removes unfinished downloads that this plan will not
// continue: of files that changed, were deleted or are no longer wanted.
func (e *Engine) dropStaleParts(p *Plan) {
	keep := map[string]bool{}
	for _, d := range p.Downloads {
		keep[path.Base(partFor(d.Item))] = true
	}
	entries, _ := os.ReadDir(e.local(partsDir))
	for _, entry := range entries {
		if !keep[entry.Name()] {
			os.Remove(filepath.Join(e.local(partsDir), entry.Name()))
		}
	}
}

// fetch brings one file onto the drive: into a part file first, then,
// checked, to its place.
func (e *Engine) fetch(ctx context.Context, d Download, report func(Event)) error {
	it := d.Item
	part := partFor(it)
	full := e.local(part)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	report(Event{Kind: Started, Target: d.Target, Size: d.Size})

	var err error
	for range tries {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err = e.fill(ctx, it, full, d.Target, report); err == nil {
			break
		}
		var refused *drive.APIError
		switch {
		case drive.Refused(err, drive.ReasonExportTooLarge):
			os.Remove(full)
			return &notCopied{NotExportLimit}
		case drive.Refused(err, drive.ReasonCannotDownload):
			os.Remove(full)
			return &notCopied{NotForbidden}
		case errors.As(err, &refused) && refused.Status < 500 && refused.Status != 429:
			return err // Drive said no; asking again will not help
		}
	}
	if err != nil {
		return err
	}

	// An ordinary file must match the checksum Drive gave for it.
	if it.Google == "" {
		sum, err := checksum(full)
		if err != nil {
			return err
		}
		if sum != it.MD5 {
			os.Remove(full)
			return errors.New("the downloaded content does not match the checksum from Drive")
		}
	}

	// Into place. The version edited by hand goes to the trash; an older
	// copy at another place (the file was renamed on Drive too) goes away.
	if d.Why == ForEdited {
		if err := e.toTrash(it.Path, TrashEdited, it.Created, time.Time{}); err != nil {
			return err
		}
	} else if it.Copy != nil && it.Path != d.Target {
		if err := e.remove(it.Path); err != nil {
			return err
		}
	}
	if it.Copy == nil || it.Path != d.Target || d.Why == ForMissing {
		if err := e.makeRoom(d.Target); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(e.local(d.Target)), 0o755); err != nil {
		return err
	}
	if err := os.Rename(full, e.local(d.Target)); err != nil {
		return err
	}
	// The file carries the date it has on Drive. What the drive then
	// reports is recorded as is: file systems round times differently.
	os.Chtimes(e.local(d.Target), it.Modified, it.Modified)
	info, err := os.Stat(e.local(d.Target))
	if err != nil {
		return err
	}
	if err := e.store.SetPath(it.ID, d.Target); err != nil {
		return err
	}
	if err := e.store.SetCopied(it.ID, state.Copy{MD5: it.MD5, Modified: it.Modified, Size: info.Size(), MTime: info.ModTime(), At: e.now()}); err != nil {
		return err
	}
	report(Event{Kind: Finished, Target: d.Target, Size: info.Size()})
	return nil
}

// fill writes the content into the part file, continuing what is there.
func (e *Engine) fill(ctx context.Context, it state.Item, full, target string, report func(Event)) error {
	if it.Google != "" {
		// An export cannot be continued: it starts over each time.
		file, err := os.Create(full)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = e.remote.Export(ctx, it.ID, it.Google, &counter{file: file, target: target, report: report})
		return err
	}

	var offset int64
	if info, err := os.Stat(full); err == nil {
		offset = info.Size()
	}
	if offset > it.Size { // cannot be the beginning of this file
		offset = 0
	}
	if offset == it.Size && offset > 0 {
		// Complete from an earlier run; the checksum decides.
		report(Event{Kind: Advanced, Target: target, Bytes: offset})
		return nil
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if offset == 0 {
		flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}
	file, err := os.OpenFile(full, flags, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if offset > 0 {
		report(Event{Kind: Advanced, Target: target, Bytes: offset}) // already on the drive
	}
	_, err = e.remote.Download(ctx, it.ID, offset, &counter{file: file, target: target, report: report})
	if errors.Is(err, drive.ErrNoResume) {
		// Drive sent the file from its start: the part is of no use.
		report(Event{Kind: Advanced, Target: target, Bytes: -offset})
		file.Close()
		os.Remove(full)
		return fmt.Errorf("starting over: %w", err)
	}
	return err
}

// counter passes bytes on to the part file and reports how many went by.
type counter struct {
	file   *os.File
	target string
	report func(Event)
}

func (c *counter) Write(p []byte) (int, error) {
	n, err := c.file.Write(p)
	if n > 0 {
		c.report(Event{Kind: Advanced, Target: c.target, Bytes: int64(n)})
	}
	return n, err
}
