package engine

import (
	"context"
	"os"
	"testing"

	"syncer-gdrive/internal/drive"
	"syncer-gdrive/internal/state"
)

// realistic builds a shared folder with the awkward things a real one has.
func realistic() *fakeDrive {
	d := newFakeDrive()
	d.folder("root", "owner-folder-the-account-cannot-see", "Syncer")
	d.folder("docs", "root", "Документи")
	d.folder("empty", "root", "Порожня")
	d.file("a", "root", "нотатки.md", "нотатки", 3)
	d.file("q", "root", "що?.webm", "video", 4)
	d.file("old", "root", "5. DevOps.pdf", "one", 5)
	d.file("new", "root", "5. DevOps.pdf", "two", 9)
	d.file("deep", "docs", "договір.pdf", "contract", 6)
	d.google("sheet", "docs", "Бюджет", "spreadsheet", "PK-xlsx-content")
	d.google("form", "docs", "Опитування", "form", "")
	d.file("icon", "docs", "Icon\r", "", 2)
	d.file("store", "root", ".DS_Store", "x", 2)
	d.files["short"] = &fakeFile{File: drive.File{ID: "short", Parent: "root", Name: "ярлик", MimeType: "application/vnd.google-apps.shortcut", CanDownload: true}}
	locked := d.file("locked", "root", "закрите.mov", "secret", 7)
	locked.CanDownload = false
	return d
}

func TestFirstLookPlacesEverything(t *testing.T) {
	d := realistic()
	e, dest := open(t, d)
	s, err := e.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		"root":  "Syncer",
		"docs":  "Syncer/Документи",
		"a":     "Syncer/нотатки.md",
		"q":     "Syncer/що_.webm",      // a character no drive takes
		"old":   "Syncer/5. DevOps.pdf", // the older of two keeps the name
		"new":   "Syncer/5. DevOps (2).pdf",
		"deep":  "Syncer/Документи/договір.pdf",
		"sheet": "Syncer/Документи/Бюджет.xlsx", // a Google document gets its export extension
	}
	for id, path := range want {
		if s.Wanted[id] != path {
			t.Errorf("%s belongs at %q, want %q", id, s.Wanted[id], path)
		}
	}
	// The shared folder stands at the top although it has a parent on
	// Drive: that parent is not something the account can see.
	if len(s.Top) != 1 || s.Top[0] != "root" {
		t.Errorf("top level: %v", s.Top)
	}
	// What is never copied is counted, not listed.
	for _, id := range []string{"form", "icon", "store", "short", "locked"} {
		if _, listed := s.Items[id]; listed {
			t.Errorf("%s is listed for copying", id)
		}
	}
	counts := map[string]int{NotUnsupported: 1, NotService: 2, NotShortcut: 1, NotForbidden: 1}
	for reason, n := range counts {
		if s.NotCopied[reason].Files != n {
			t.Errorf("not copied as %s: %d, want %d", reason, s.NotCopied[reason].Files, n)
		}
	}
	// On a first run everything is selected.
	for id, it := range s.Items {
		if !it.Folder && !s.Checked[id] {
			t.Errorf("%s is not selected on a first run", id)
		}
	}
	// The two files with one name are flagged, with the name each gets.
	if len(s.Clash) != 2 || s.Clash["old"] != "5. DevOps.pdf" || s.Clash["new"] != "5. DevOps (2).pdf" {
		t.Errorf("clashes: %v", s.Clash)
	}
	// Folders first, then by name.
	if got := s.Children["root"]; len(got) != 6 || got[0] != "docs" || got[1] != "empty" {
		t.Errorf("order inside the shared folder: %v", got)
	}
	// Looking changes nothing in the copy.
	if files := entries(t, dest); len(files) != 0 {
		t.Errorf("a look at Drive wrote to the copy: %v", files)
	}

	// The plan: every selected file is new.
	p, by := planned(t, e)
	if len(p.Downloads) != 6 || len(p.Moves)+len(p.Leftovers)+len(p.Lost) != 0 || p.Clashes != 2 {
		t.Fatalf("plan: %d downloads, %d moves, %d leftovers, %d clashes", len(p.Downloads), len(p.Moves), len(p.Leftovers), p.Clashes)
	}
	for target, dl := range by {
		if dl.Why != ForNew {
			t.Errorf("%s: %s", target, dl.Why)
		}
	}
	// Room for a Google document is reserved generously.
	if got := by["Syncer/Документи/Бюджет.xlsx"].Size; got != 1<<20 {
		t.Errorf("reserved %d bytes for the spreadsheet", got)
	}
	if !p.Writable || p.Volume.Free == 0 || !p.Fits() {
		t.Errorf("destination: writable %v, free %d, fits %v", p.Writable, p.Volume.Free, p.Fits())
	}
}

func TestNewFilesFollowTheirFolder(t *testing.T) {
	d := newFakeDrive()
	d.folder("root", "", "Syncer")
	d.folder("whole", "root", "Ціла")
	d.folder("part", "root", "Частково")
	d.folder("none", "root", "Жодного")
	d.folder("empty", "root", "Порожня")
	d.file("w1", "whole", "w1", "x", 1)
	d.file("p1", "part", "p1", "x", 1)
	d.file("p2", "part", "p2", "x", 1)
	d.file("n1", "none", "n1", "x", 1)
	e, _ := open(t, d)
	ctx := context.Background()
	if _, err := e.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	// The user clears one file of "part" and everything in "none".
	if err := e.SetSelection(map[string]bool{"p2": false, "n1": false}, nil); err != nil {
		t.Fatal(err)
	}

	d.file("w2", "whole", "w2", "x", 2) // a folder selected as a whole takes it
	d.file("p3", "part", "p3", "x", 2)  // selected in part: its switch is on by default
	d.file("n2", "none", "n2", "x", 2)  // not selected: stays out
	d.file("e1", "empty", "e1", "x", 2) // no files of its own: the folder above decides
	d.folder("fresh", "part", "Нова")   // a folder that is new itself
	d.file("f1", "fresh", "f1", "x", 2) // … follows the nearest known folder
	d.file("r1", "root", "r1", "x", 2)  // directly in the shared folder
	s, _ := e.Refresh(ctx)
	for id, want := range map[string]bool{"w2": true, "p3": true, "n2": false, "e1": true, "f1": true, "r1": true} {
		if s.Checked[id] != want {
			t.Errorf("%s selected = %v, want %v", id, s.Checked[id], want)
		}
	}
	if s.NewUnselected.Files != 1 {
		t.Errorf("%d new files left out, want 1", s.NewUnselected.Files)
	}

	// With the switch of "part" off, its new files stay out — also those
	// in a folder that appears inside it.
	e.SetSelection(nil, map[string]bool{"part": false})
	d.file("p4", "part", "p4", "x", 3)
	d.folder("fresh2", "part", "Ще нова")
	d.file("f2", "fresh2", "f2", "x", 3)
	// With the switch of the shared folder off, new files directly in it
	// stay out, and so does what has no folder of its own to ask.
	e.SetSelection(nil, map[string]bool{"root": false})
	d.file("r2", "root", "r2", "x", 3)
	d.file("e2", "empty", "e2", "x", 3)
	d.file("w3", "whole", "w3", "x", 3) // still whole: still taken
	s, _ = e.Refresh(ctx)
	for id, want := range map[string]bool{"p4": false, "f2": false, "r2": false, "w3": true} {
		if s.Checked[id] != want {
			t.Errorf("%s selected = %v, want %v", id, s.Checked[id], want)
		}
	}
	// "empty" got a selected file meanwhile, so it now decides for itself.
	if !s.Checked["e2"] {
		t.Error("a folder selected as a whole did not take a new file")
	}
	// A decision once made is kept across looks.
	if s.Checked["p2"] || s.Checked["n1"] || !s.Checked["p1"] {
		t.Error("the user's own choice changed")
	}
	// A second shared folder follows the top-level switch.
	e.SetSelection(nil, map[string]bool{"": false})
	d.folder("second", "", "Друга")
	d.file("s1", "second", "s1", "x", 4)
	s, _ = e.Refresh(ctx)
	if s.Checked["s1"] {
		t.Error("a newly shared folder was taken with the top-level switch off")
	}
}

func TestNamesStayPutAcrossLooks(t *testing.T) {
	d := newFakeDrive()
	d.folder("root", "", "Syncer")
	d.file("old", "root", "звіт.pdf", "1", 1)
	d.file("new", "root", "звіт.pdf", "2", 5)
	e, _ := open(t, d)
	ctx := context.Background()
	s, _ := e.Refresh(ctx)
	if s.Wanted["new"] != "Syncer/звіт (2).pdf" {
		t.Fatalf("the newer twin belongs at %q", s.Wanted["new"])
	}
	// The older twin goes away. The newer one keeps its numbered name: a
	// copy must not be renamed because of what happened to another file.
	delete(d.files, "old")
	s, _ = e.Refresh(ctx)
	if s.Wanted["new"] != "Syncer/звіт (2).pdf" {
		t.Errorf("after its twin was deleted the file moved to %q", s.Wanted["new"])
	}
	// A third file of that name takes the free one.
	d.file("third", "root", "звіт.pdf", "3", 9)
	s, _ = e.Refresh(ctx)
	if s.Wanted["third"] != "Syncer/звіт.pdf" || s.Wanted["new"] != "Syncer/звіт (2).pdf" {
		t.Errorf("third at %q, second at %q", s.Wanted["third"], s.Wanted["new"])
	}
	// Renamed on Drive, a file is named afresh.
	d.files["new"].Name = "підсумок.pdf"
	s, _ = e.Refresh(ctx)
	if s.Wanted["new"] != "Syncer/підсумок.pdf" {
		t.Errorf("after a rename on Drive the file belongs at %q", s.Wanted["new"])
	}
}

func TestPlanTellsWhatHappenedToEachCopy(t *testing.T) {
	d := newFakeDrive()
	d.folder("root", "", "Syncer")
	d.folder("docs", "root", "Документи")
	for _, id := range []string{"same", "changed", "missing", "edited", "renamed", "unselected", "deleted"} {
		d.file(id, "docs", id+".txt", "content of "+id, 2)
	}
	d.google("doc", "docs", "Звіт", "document", "exported")
	e, dest := open(t, d)
	ctx := context.Background()
	if _, err := e.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	for id := range d.files {
		if !d.files[id].IsFolder() {
			placeCopy(t, e, d, id)
		}
	}
	// Nothing changed: nothing to do.
	p, _ := planned(t, e)
	if len(p.Downloads)+len(p.Moves)+len(p.Leftovers) != 0 {
		t.Fatalf("an up-to-date copy needs %d downloads, %d moves, %d leftovers", len(p.Downloads), len(p.Moves), len(p.Leftovers))
	}

	d.write("changed", "new content on Drive")
	os.Remove(e.local("Syncer/Документи/missing.txt"))
	os.WriteFile(e.local("Syncer/Документи/edited.txt"), []byte("edited by hand"), 0o644)
	d.files["renamed"].Name = "нова назва.txt"
	e.SetSelection(map[string]bool{"unselected": false}, nil)
	delete(d.files, "deleted")
	d.files["doc"].Modified = day(15) // a Google document tells a change by its date only

	p, by := planned(t, e)
	why := map[string]string{
		"Syncer/Документи/changed.txt": ForUpdated,
		"Syncer/Документи/missing.txt": ForMissing,
		"Syncer/Документи/edited.txt":  ForEdited,
		"Syncer/Документи/Звіт.docx":   ForUpdated,
	}
	if len(p.Downloads) != len(why) {
		t.Errorf("%d downloads, want %d: %v", len(p.Downloads), len(why), by)
	}
	for target, want := range why {
		if by[target].Why != want {
			t.Errorf("%s is downloaded as %q, want %q", target, by[target].Why, want)
		}
	}
	if len(p.Moves) != 1 || p.Moves[0].From != "Syncer/Документи/renamed.txt" || p.Moves[0].To != "Syncer/Документи/нова назва.txt" {
		t.Errorf("moves: %+v", p.Moves)
	}
	left := map[string]string{}
	for _, l := range p.Leftovers {
		left[l.Item.ID] = l.Why
	}
	if len(left) != 2 || left["unselected"] != LeftUnselected || left["deleted"] != LeftDeleted {
		t.Errorf("leftovers: %v", left)
	}
	// Planning touched nothing.
	if got := entries(t, dest)["Syncer/Документи/edited.txt"]; got != "edited by hand" {
		t.Errorf("the plan changed a file: %q", got)
	}
}

func TestLostAccessIsNotDeletion(t *testing.T) {
	d := newFakeDrive()
	d.folder("root", "", "Syncer")
	d.folder("shared", "", "Спільні звіти")
	d.folder("inner", "shared", "2026")
	d.file("a", "root", "a.txt", "a", 1)
	d.file("r1", "shared", "r1.txt", "r1", 1)
	d.file("r2", "inner", "r2.txt", "r2", 1)
	e, dest := open(t, d)
	ctx := context.Background()
	e.Refresh(ctx)
	for _, id := range []string{"a", "r1", "r2"} {
		placeCopy(t, e, d, id)
	}

	// The second folder is no longer shared: everything in it vanishes.
	for _, id := range []string{"shared", "inner", "r1", "r2"} {
		delete(d.files, id)
	}
	p, _ := planned(t, e)
	if len(p.Leftovers) != 0 {
		t.Errorf("files of a folder the account lost are offered for deletion: %+v", p.Leftovers)
	}
	if len(p.Lost) != 1 || p.Lost[0].Name != "Спільні звіти" || p.LostFiles.Files != 2 {
		t.Errorf("lost: %+v with %d files", p.Lost, p.LostFiles.Files)
	}
	if len(entries(t, dest)) != 3 {
		t.Error("the copy changed")
	}

	// One file deleted inside a folder that is still shared is a deletion.
	delete(d.files, "a")
	p, _ = planned(t, e)
	if len(p.Leftovers) != 1 || p.Leftovers[0].Why != LeftDeleted || p.Leftovers[0].Item.ID != "a" {
		t.Errorf("leftovers: %+v", p.Leftovers)
	}
}

func TestFilesTheDriveCannotHoldAreLeftOut(t *testing.T) {
	e, _ := open(t, newFakeDrive())
	p := &Plan{NotCopied: map[string]Tally{}}
	big := state.Item{ID: "big", Size: 5 << 30}
	e.download(p, big, "big.mov", ForNew, 4<<30-1)
	e.download(p, state.Item{ID: "ok", Size: 1 << 30}, "ok.mov", ForNew, 4<<30-1)
	e.download(p, big, "big.mov", ForNew, 0) // a drive without the limit takes it
	if len(p.Downloads) != 2 || p.NotCopied[NotTooLarge].Files != 1 || p.NotCopied[NotTooLarge].Bytes != 5<<30 {
		t.Errorf("%d downloads, too large: %+v", len(p.Downloads), p.NotCopied[NotTooLarge])
	}
}

func TestFailedLookChangesNothing(t *testing.T) {
	d := newFakeDrive()
	d.folder("root", "", "Syncer")
	d.file("a", "root", "a.txt", "a", 1)
	e, _ := open(t, d)
	ctx := context.Background()
	e.Refresh(ctx)
	d.listErr = context.DeadlineExceeded
	if _, err := e.Plan(ctx); err == nil {
		t.Fatal("a plan was made without seeing Drive")
	}
	items, _ := e.store.Items()
	if len(items) != 2 || !items[0].Seen || !items[1].Seen {
		t.Errorf("a failed look marked items as gone: %+v", items)
	}
}
