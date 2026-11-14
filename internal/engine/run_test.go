package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"syncer-gdrive/internal/state"
)

// run plans and carries out a sync, failing the test on an error.
func run(t *testing.T, e *Engine, decide func(Leftover) int) *Result {
	t.Helper()
	p, err := e.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.Run(context.Background(), p, decide, nil)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestFirstSyncCopiesEverythingSelected(t *testing.T) {
	d := realistic()
	e, dest := open(t, d)
	var mu sync.Mutex
	bytes := map[string]int64{}
	finished := 0
	p, _ := planned(t, e)
	r, err := e.Run(context.Background(), p, nil, func(ev Event) {
		mu.Lock()
		defer mu.Unlock()
		switch ev.Kind {
		case Advanced:
			bytes[ev.Target] += ev.Bytes
		case Finished:
			finished++
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"Syncer/нотатки.md":            "нотатки",
		"Syncer/що_.webm":              "video",
		"Syncer/5. DevOps.pdf":         "one",
		"Syncer/5. DevOps (2).pdf":     "two",
		"Syncer/Документи/договір.pdf": "contract",
		"Syncer/Документи/Бюджет.xlsx": "PK-xlsx-content",
	}
	got := entries(t, dest)
	if len(got) != len(want) {
		t.Errorf("the copy holds %d files, want %d: %v", len(got), len(want), got)
	}
	for path, content := range want {
		if got[path] != content {
			t.Errorf("%s holds %q, want %q", path, got[path], content)
		}
		if bytes[path] != int64(len(content)) {
			t.Errorf("%s: %d bytes reported, want %d", path, bytes[path], len(content))
		}
	}
	if r.Downloaded.Files != 6 || finished != 6 || len(r.Failures) != 0 || r.Stopped {
		t.Errorf("result: %+v, %d finished", r, finished)
	}
	// A file carries the date it has on Drive.
	info, _ := os.Stat(filepath.Join(dest, "Syncer", "нотатки.md"))
	if !info.ModTime().Equal(day(3)) {
		t.Errorf("the copy is dated %v, want %v", info.ModTime(), day(3))
	}
	// No unfinished downloads are left behind.
	if parts, _ := os.ReadDir(e.local(partsDir)); len(parts) != 0 {
		t.Errorf("%d part files left", len(parts))
	}
	if last, _ := e.store.Meta(MetaLastSync); last == "" {
		t.Error("the time of the sync was not recorded")
	}

	// A second sync finds nothing to do and fetches nothing.
	before := len(d.downloads)
	calls := 0
	for _, n := range d.downloads {
		calls += n
	}
	p, _ = planned(t, e)
	if len(p.Downloads)+len(p.Moves)+len(p.Leftovers) != 0 {
		t.Fatalf("a second sync wants %d downloads", len(p.Downloads))
	}
	again := 0
	for _, n := range d.downloads {
		again += n
	}
	if again != calls || len(d.downloads) != before {
		t.Error("the second look fetched content")
	}
}

func TestBrokenDownloadIsContinued(t *testing.T) {
	d := newFakeDrive()
	d.folder("root", "", "Syncer")
	content := strings.Repeat("0123456789", 100)
	d.file("big", "root", "big.bin", content, 1)
	d.breakAt["big"] = 300 // the connection drops after 300 bytes
	e, dest := open(t, d)
	r := run(t, e, nil)
	if got := entries(t, dest)["Syncer/big.bin"]; got != content {
		t.Fatalf("the file holds %d bytes, want %d", len(got), len(content))
	}
	if len(r.Failures) != 0 || d.downloads["big"] != 2 {
		t.Errorf("failures %v after %d requests", r.Failures, d.downloads["big"])
	}
	// The second request asked only for what was missing.
	if offsets := d.offsets["big"]; len(offsets) != 2 || offsets[0] != 0 || offsets[1] != 300 {
		t.Errorf("requested from offsets %v", offsets)
	}
}

func TestStoppedSyncGoesOnNextTime(t *testing.T) {
	d := newFakeDrive()
	d.folder("root", "", "Syncer")
	content := strings.Repeat("abcdefghij", 50)
	d.file("big", "root", "big.bin", content, 1)
	d.file("small", "root", "small.txt", "s", 1)
	e, dest := open(t, d)
	p, _ := planned(t, e)

	// The run is stopped before it fetches anything.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, err := e.Run(ctx, p, nil, nil)
	if err != nil || !r.Stopped || r.Left.Files != 2 || len(entries(t, dest)) != 0 {
		t.Fatalf("stopped run: %+v, %v", r, err)
	}
	if last, _ := e.store.Meta(MetaLastSync); last != "" {
		t.Error("a stopped sync was recorded as finished")
	}

	// Part of the big file is on the drive from an interrupted run.
	item := e.last.Items["big"]
	os.MkdirAll(e.local(partsDir), 0o755)
	os.WriteFile(e.local(partFor(item)), []byte(content[:200]), 0o644)
	// … and so is a part of a file that has changed since: it is dropped.
	os.WriteFile(filepath.Join(e.local(partsDir), "big-0000.part"), []byte("stale"), 0o644)

	run(t, e, nil)
	if got := entries(t, dest); got["Syncer/big.bin"] != content || got["Syncer/small.txt"] != "s" {
		t.Fatalf("after continuing: %v", got)
	}
	if offsets := d.offsets["big"]; offsets[len(offsets)-1] != 200 {
		t.Errorf("the download did not continue from the saved part: offsets %v", offsets)
	}
	if parts, _ := os.ReadDir(e.local(partsDir)); len(parts) != 0 {
		t.Errorf("%d part files left", len(parts))
	}
}

func TestWrongContentIsNotAccepted(t *testing.T) {
	d := newFakeDrive()
	d.folder("root", "", "Syncer")
	f := d.file("bad", "root", "bad.bin", "what Drive really sends", 1)
	f.MD5 = "00000000000000000000000000000000" // Drive describes another content
	d.file("good", "root", "good.txt", "fine", 1)
	e, dest := open(t, d)
	r := run(t, e, nil)
	got := entries(t, dest)
	if _, landed := got["Syncer/bad.bin"]; landed || got["Syncer/good.txt"] != "fine" {
		t.Errorf("the copy holds %v", got)
	}
	if len(r.Failures) != 1 || r.Failures[0].Target != "Syncer/bad.bin" || r.Downloaded.Files != 1 {
		t.Errorf("result: %+v", r)
	}
	// It is tried again by the next sync.
	if p, _ := planned(t, e); len(p.Downloads) != 1 {
		t.Errorf("the failed file is not planned again: %+v", p.Downloads)
	}
}

func TestWhatDriveRefusesIsLeftOutNotFailed(t *testing.T) {
	d := newFakeDrive()
	d.folder("root", "", "Syncer")
	d.google("huge", "root", "Великий звіт", "document", "x")
	d.file("locked", "root", "locked.mov", "x", 1)
	d.file("gone", "root", "gone.txt", "x", 1)
	d.refuse["huge"] = "exportSizeLimitExceeded"
	d.refuse["locked"] = "cannotDownloadFile"
	d.refuse["gone"] = "notFound"
	e, dest := open(t, d)
	r := run(t, e, nil)
	if r.NotCopied[NotExportLimit].Files != 1 || r.NotCopied[NotForbidden].Files != 1 {
		t.Errorf("left out: %+v", r.NotCopied)
	}
	// A plain refusal is a failure, reported once and not retried in this run.
	if len(r.Failures) != 1 || r.Failures[0].Target != "Syncer/gone.txt" || d.downloads["gone"] != 1 {
		t.Errorf("failures: %+v after %d requests", r.Failures, d.downloads["gone"])
	}
	if len(entries(t, dest)) != 0 {
		t.Error("something was written")
	}
}

func TestEditedCopyGoesToTheTrash(t *testing.T) {
	d := newFakeDrive()
	d.folder("root", "", "Syncer")
	d.file("a", "root", "звіт.docx", "from Drive", 1)
	e, dest := open(t, d)
	run(t, e, nil)
	os.WriteFile(filepath.Join(dest, "Syncer", "звіт.docx"), []byte("my own edits"), 0o644)

	r := run(t, e, nil)
	if got := entries(t, dest)["Syncer/звіт.docx"]; got != "from Drive" {
		t.Errorf("the copy holds %q", got)
	}
	trashed, _ := e.store.Trashed()
	if len(trashed) != 1 || trashed[0].Reason != TrashEdited || trashed[0].Path != "Syncer/звіт.docx" {
		t.Fatalf("trash: %+v", trashed)
	}
	if data, _ := os.ReadFile(e.local(trashed[0].Stored)); string(data) != "my own edits" {
		t.Errorf("the trash holds %q", data)
	}
	if r.Updated.Files != 1 || r.Trashed != 1 {
		t.Errorf("result: %+v", r)
	}
	// The same happens again: the trash keeps both versions.
	os.WriteFile(filepath.Join(dest, "Syncer", "звіт.docx"), []byte("more edits"), 0o644)
	run(t, e, nil)
	trashed, _ = e.store.Trashed()
	if len(trashed) != 2 || trashed[0].Stored == trashed[1].Stored {
		t.Fatalf("after a second edit the trash holds %+v", trashed)
	}
	for i, want := range []string{"my own edits", "more edits"} {
		if data, _ := os.ReadFile(e.local(trashed[i].Stored)); string(data) != want {
			t.Errorf("trash entry %d holds %q, want %q", i, data, want)
		}
	}
}

func TestLeftoversDoWhatTheUserChose(t *testing.T) {
	d := newFakeDrive()
	d.folder("root", "", "Syncer")
	d.folder("docs", "root", "Документи")
	for _, id := range []string{"del", "skip", "trash", "off"} {
		d.file(id, "docs", id+".txt", id, 1)
	}
	e, dest := open(t, d)
	run(t, e, nil)

	for _, id := range []string{"del", "skip", "trash"} {
		delete(d.files, id)
	}
	e.SetSelection(map[string]bool{"off": false}, nil)
	r := run(t, e, func(l Leftover) int {
		switch l.Item.ID {
		case "del":
			return ToDelete
		case "skip":
			return ToSkip
		}
		return ToTrash
	})
	if r.Deleted != 1 || r.Skipped != 1 || r.Trashed != 2 {
		t.Errorf("result: %+v", r)
	}
	// Only the skipped file is still in the copy.
	got := entries(t, dest)
	if len(got) != 1 || got["Syncer/Документи/skip.txt"] != "skip" {
		t.Errorf("the copy holds %v", got)
	}
	skipped, _ := e.store.Skipped()
	if len(skipped) != 1 || skipped[0].Path != "Syncer/Документи/skip.txt" || !skipped[0].Added.Equal(day(20)) {
		t.Errorf("SKIP list: %+v", skipped)
	}
	trashed, _ := e.store.Trashed()
	reasons := map[string]string{}
	for _, k := range trashed {
		reasons[k.Path] = k.Reason
		if !e.exists(k.Stored) {
			t.Errorf("%s is recorded in the trash but not there", k.Stored)
		}
	}
	if len(trashed) != 2 || reasons["Syncer/Документи/trash.txt"] != TrashDeleted || reasons["Syncer/Документи/off.txt"] != TrashUnselected {
		t.Errorf("trash: %v", reasons)
	}
	// Nothing is asked twice.
	if p, _ := planned(t, e); len(p.Leftovers)+len(p.Downloads) != 0 {
		t.Errorf("after settling: %d leftovers, %d downloads", len(p.Leftovers), len(p.Downloads))
	}
	// The unselected file is still known: selecting it again fetches it.
	e.SetSelection(map[string]bool{"off": true}, nil)
	run(t, e, nil)
	if got := entries(t, dest)["Syncer/Документи/off.txt"]; got != "off" {
		t.Errorf("a file selected again holds %q", got)
	}
}

func TestRenamedOnDriveIsMovedNotFetched(t *testing.T) {
	d := newFakeDrive()
	d.folder("root", "", "Syncer")
	d.folder("old", "root", "Стара папка")
	d.file("a", "old", "a.txt", "a", 1)
	d.file("b", "old", "b.txt", "b", 1)
	e, dest := open(t, d)
	run(t, e, nil)
	fetched := d.downloads["a"] + d.downloads["b"]

	d.files["old"].Name = "Нова папка"
	d.files["a"].Name = "перейменований.txt"
	r := run(t, e, nil)
	got := entries(t, dest)
	if len(got) != 2 || got["Syncer/Нова папка/перейменований.txt"] != "a" || got["Syncer/Нова папка/b.txt"] != "b" {
		t.Errorf("the copy holds %v", got)
	}
	if r.Moved != 2 || d.downloads["a"]+d.downloads["b"] != fetched {
		t.Errorf("moved %d, fetched %d more times", r.Moved, d.downloads["a"]+d.downloads["b"]-fetched)
	}
	// The folder that became empty is gone.
	if _, err := os.Stat(filepath.Join(dest, "Syncer", "Стара папка")); err == nil {
		t.Error("the empty old folder is still there")
	}
	if p, _ := planned(t, e); len(p.Moves)+len(p.Downloads) != 0 {
		t.Error("the move is planned again")
	}

	// Renamed and changed at once: fetched to the new place, the old copy removed.
	d.files["b"].Name = "b-нове.txt"
	d.write("b", "b changed")
	run(t, e, nil)
	got = entries(t, dest)
	if len(got) != 2 || got["Syncer/Нова папка/b-нове.txt"] != "b changed" {
		t.Errorf("after rename and change the copy holds %v", got)
	}
}

func TestSkippedFileMakesWayForItsReturn(t *testing.T) {
	d := newFakeDrive()
	d.folder("root", "", "Syncer")
	d.file("first", "root", "file_1.md", "the old one", 1)
	e, dest := open(t, d)
	run(t, e, nil)
	delete(d.files, "first")
	run(t, e, func(Leftover) int { return ToSkip })

	// A file of that name appears on Drive again.
	d.file("second", "root", "file_1.md", "the new one", 5)
	r := run(t, e, nil)
	got := entries(t, dest)
	if got["Syncer/file_1.md"] != "the new one" || got["Syncer/file_1_skipped_2026-11-20.md"] != "the old one" || len(got) != 2 {
		t.Errorf("the copy holds %v", got)
	}
	if skipped, _ := e.store.Skipped(); len(skipped) != 0 {
		t.Errorf("SKIP list: %+v", skipped)
	}
	if r.Downloaded.Files != 1 {
		t.Errorf("result: %+v", r)
	}
}

func TestAStrangerAtTheTargetIsNotOverwritten(t *testing.T) {
	d := newFakeDrive()
	d.folder("root", "", "Syncer")
	d.file("a", "root", "a.txt", "from Drive", 1)
	e, dest := open(t, d)
	// A file the program knows nothing about lies where the copy belongs.
	os.MkdirAll(filepath.Join(dest, "Syncer"), 0o755)
	os.WriteFile(filepath.Join(dest, "Syncer", "a.txt"), []byte("put here by hand"), 0o644)
	run(t, e, nil)
	if got := entries(t, dest)["Syncer/a.txt"]; got != "from Drive" {
		t.Errorf("the copy holds %q", got)
	}
	trashed, _ := e.store.Trashed()
	if len(trashed) != 1 {
		t.Fatalf("trash: %+v", trashed)
	}
	if data, _ := os.ReadFile(e.local(trashed[0].Stored)); string(data) != "put here by hand" {
		t.Errorf("the stranger was lost: the trash holds %q", data)
	}
}

func TestTrashAndSkipActions(t *testing.T) {
	d := newFakeDrive()
	d.folder("root", "", "Syncer")
	d.folder("docs", "root", "Документи")
	d.file("a", "docs", "a.txt", "a", 1)
	d.file("b", "docs", "b.txt", "b", 1)
	d.file("c", "docs", "c.txt", "c", 1)
	e, dest := open(t, d)
	run(t, e, nil)
	for _, id := range []string{"a", "b", "c"} {
		delete(d.files, id)
	}
	run(t, e, nil) // all three go to the trash
	if len(entries(t, dest)) != 0 {
		t.Fatal("the copy is not empty")
	}
	trashed, _ := e.store.Trashed()
	by := map[string]state.Kept{}
	for _, k := range trashed {
		by[k.Path] = k
	}

	// Restored to where it was.
	place, err := e.Restore(by["Syncer/Документи/a.txt"])
	if err != nil || place != "Syncer/Документи/a.txt" || entries(t, dest)[place] != "a" {
		t.Errorf("restore: %q, %v", place, err)
	}
	// Something lies there now: restored next to it.
	os.WriteFile(filepath.Join(dest, "Syncer", "Документи", "b.txt"), []byte("another b"), 0o644)
	place, err = e.Restore(by["Syncer/Документи/b.txt"])
	if err != nil || place != "Syncer/Документи/b (2).txt" || entries(t, dest)[place] != "b" {
		t.Errorf("restore next to an occupied place: %q, %v", place, err)
	}
	if got := entries(t, dest)["Syncer/Документи/b.txt"]; got != "another b" {
		t.Errorf("restoring overwrote a file: %q", got)
	}
	// Deleted for good.
	stored := by["Syncer/Документи/c.txt"].Stored
	if err := e.DeleteForever(by["Syncer/Документи/c.txt"]); err != nil || e.exists(stored) {
		t.Errorf("delete forever: %v", err)
	}
	if left, _ := e.store.Trashed(); len(left) != 0 {
		t.Errorf("trash records left: %+v", left)
	}
	// The trash folder holds no empty folders afterwards.
	if _, err := os.Stat(filepath.Join(dest, TrashDir, "Syncer")); err == nil {
		t.Error("empty folders are left in the trash")
	}

	// Emptying the trash, and taking a file off the SKIP list.
	e.toTrash("Syncer/Документи/a.txt", TrashDeleted, day(1), day(1))
	e.toTrash("Syncer/Документи/b.txt", TrashDeleted, day(1), day(1))
	if err := e.EmptyTrash(); err != nil {
		t.Fatal(err)
	}
	if left, _ := e.store.Trashed(); len(left) != 0 {
		t.Errorf("after emptying: %+v", left)
	}
	e.store.AddSkip(state.Kept{Path: "Syncer/Документи/b (2).txt", Added: day(20)})
	skipped, _ := e.store.Skipped()
	if err := e.Unskip(skipped[0]); err != nil {
		t.Fatal(err)
	}
	if left, _ := e.store.Skipped(); len(left) != 0 || entries(t, dest)["Syncer/Документи/b (2).txt"] != "b" {
		t.Error("taking a file off the SKIP list touched the file or left the record")
	}
}
