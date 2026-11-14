package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func open(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func at(day int) time.Time { return time.Date(2026, 11, day, 12, 30, 0, 123, time.UTC) }

func byID(t *testing.T, s *Store) map[string]Item {
	t.Helper()
	items, err := s.Items()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]Item{}
	for _, it := range items {
		out[it.ID] = it
	}
	return out
}

func TestListingAndCopies(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	if _, err := os.Stat(filepath.Join(dir, Dir, "state.db")); err != nil {
		t.Fatalf("the database is not inside the destination: %v", err)
	}

	first := []Item{
		{ID: "root", Name: "Syncer", Path: "Syncer", Folder: true, Created: at(1), Modified: at(1)},
		{ID: "a", Parent: "root", Name: "звіт?.pdf", Path: "Syncer/звіт_.pdf", Size: 100, MD5: "aa", Created: at(2), Modified: at(3)},
		{ID: "g", Parent: "root", Name: "Бюджет", Path: "Syncer/Бюджет.xlsx", Google: "spreadsheet", Size: 7317, Modified: at(4)},
	}
	if err := s.SaveListing(first); err != nil {
		t.Fatal(err)
	}
	got := byID(t, s)
	if a := got["a"]; a.Name != "звіт?.pdf" || a.Path != "Syncer/звіт_.pdf" || a.Size != 100 || !a.Modified.Equal(at(3)) || !a.Seen || a.Copy != nil {
		t.Errorf("the file as stored: %+v", a)
	}
	if g := got["g"]; g.Google != "spreadsheet" || !g.Created.IsZero() {
		t.Errorf("the Google document as stored: %+v", g)
	}
	if !got["root"].Folder {
		t.Error("the folder lost its kind")
	}

	copied := Copy{MD5: "aa", Modified: at(3), Size: 100, MTime: at(3), At: at(5)}
	if err := s.SetCopied("a", copied); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCopied("nobody", copied); err == nil {
		t.Error("a copy was recorded for a file that is not known")
	}

	// A second listing: "a" changed on Drive, "g" is gone, "b" is new.
	second := []Item{
		first[0],
		{ID: "a", Parent: "root", Name: "звіт?.pdf", Path: "Syncer/звіт_.pdf", Size: 150, MD5: "bb", Created: at(2), Modified: at(6)},
		{ID: "b", Parent: "root", Name: "нове.txt", Path: "Syncer/нове.txt", Size: 1, MD5: "cc", Created: at(6), Modified: at(6)},
	}
	if err := s.SaveListing(second); err != nil {
		t.Fatal(err)
	}
	got = byID(t, s)
	a := got["a"]
	if a.MD5 != "bb" || a.Size != 150 || a.Copy == nil || a.Copy.MD5 != "aa" || !a.Copy.At.Equal(at(5)) || a.Copy.Size != 100 {
		t.Errorf("after a new listing the file is %+v with copy %+v; the copy must stay as recorded", a, a.Copy)
	}
	if g := got["g"]; g.Seen || g.Path != "Syncer/Бюджет.xlsx" {
		t.Errorf("an item gone from Drive should stay known and be marked unseen: %+v", g)
	}
	if !got["b"].Seen || len(got) != 4 {
		t.Errorf("%d items known, want 4 with the new one seen", len(got))
	}

	if err := s.ClearCopied("a"); err != nil {
		t.Fatal(err)
	}
	if err := s.Forget("g"); err != nil {
		t.Fatal(err)
	}
	got = byID(t, s)
	if got["a"].Copy != nil || len(got) != 3 {
		t.Errorf("after clearing and forgetting: copy %v, %d items", got["a"].Copy, len(got))
	}
}

func TestSelection(t *testing.T) {
	s := open(t, t.TempDir())
	if err := s.SetChecked(map[string]bool{"a": true, "b": false}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetChecked(map[string]bool{"b": true}); err != nil {
		t.Fatal(err)
	}
	checked, err := s.Checked()
	if err != nil || len(checked) != 2 || !checked["a"] || !checked["b"] {
		t.Errorf("checked = %v, %v", checked, err)
	}
	if _, decided := checked["c"]; decided {
		t.Error("a file nobody decided on is in the selection")
	}

	if err := s.SetTakeNew(map[string]bool{"": false, "folder": true}); err != nil {
		t.Fatal(err)
	}
	takeNew, err := s.TakeNew()
	if err != nil || takeNew[""] || !takeNew["folder"] || len(takeNew) != 2 {
		t.Errorf("takeNew = %v, %v", takeNew, err)
	}
	// Forgetting an item drops its selection too.
	s.SaveListing([]Item{{ID: "a", Name: "a", Path: "a"}})
	s.Forget("a", "folder")
	checked, _ = s.Checked()
	takeNew, _ = s.TakeNew()
	if _, ok := checked["a"]; ok || len(takeNew) != 1 {
		t.Errorf("after forgetting: checked %v, takeNew %v", checked, takeNew)
	}
}

func TestSkipAndTrash(t *testing.T) {
	s := open(t, t.TempDir())
	skip := Kept{Path: "Syncer/старе.zip", Size: 9, Created: at(1), Modified: at(2), Added: at(3)}
	if err := s.AddSkip(skip); err != nil {
		t.Fatal(err)
	}
	if err := s.AddSkip(Kept{Path: skip.Path, Size: 1}); err != nil { // already there
		t.Fatal(err)
	}
	skipped, _ := s.Skipped()
	if len(skipped) != 1 || skipped[0].Size != 9 || !skipped[0].Added.Equal(at(3)) {
		t.Fatalf("skipped = %+v", skipped)
	}

	// The same path may be in the trash twice: an older and a newer copy.
	for i, stored := range []string{".bin/Syncer/1.doc", ".bin/Syncer/1 (2026-11-14 10-00-00).doc"} {
		if err := s.AddTrash(Kept{Path: "Syncer/1.doc", Stored: stored, Size: int64(i + 1), Added: at(4 + i), Reason: "drive"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AddTrash(Kept{Path: "Syncer/1.doc", Stored: ".bin/Syncer/1.doc"}); err == nil {
		t.Error("two trash records point at one stored file")
	}
	trashed, _ := s.Trashed()
	if len(trashed) != 2 || trashed[0].Reason != "drive" || trashed[1].Size != 2 {
		t.Fatalf("trashed = %+v", trashed)
	}
	if err := s.RemoveTrash(trashed[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveSkip(skipped[0].ID); err != nil {
		t.Fatal(err)
	}
	trashed, _ = s.Trashed()
	skipped, _ = s.Skipped()
	if len(trashed) != 1 || len(skipped) != 0 {
		t.Errorf("after removing: %d in the trash, %d skipped", len(trashed), len(skipped))
	}
}

func TestStateSurvivesReopening(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.SaveListing([]Item{{ID: "a", Name: "a", Path: "a", Modified: at(1)}})
	s.SetCopied("a", Copy{MD5: "x", At: at(2)})
	s.SetMeta("last-sync", "2026-11-14T10:00:00Z")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	again := open(t, dir)
	got := byID(t, again)
	if got["a"].Copy == nil || got["a"].Copy.MD5 != "x" {
		t.Errorf("after reopening the copy is %+v", got["a"].Copy)
	}
	if v, _ := again.Meta("last-sync"); v != "2026-11-14T10:00:00Z" {
		t.Errorf("meta = %q", v)
	}
	if v, err := again.Meta("never-set"); v != "" || err != nil {
		t.Errorf("a value that was never set: %q, %v", v, err)
	}
}

func TestNewerFormatIsRefused(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.db.Exec("PRAGMA user_version = 99")
	s.Close()
	if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "newer version") {
		t.Errorf("a state written by a newer program was opened: %v", err)
	}
}

func TestFailedChangeLeavesNothingBehind(t *testing.T) {
	s := open(t, t.TempDir())
	s.SaveListing([]Item{{ID: "a", Name: "a", Path: "a"}})
	// The second item breaks the listing (a NULL where none is allowed):
	// the whole change must be undone, including the "unseen" marks.
	s.db.Exec("CREATE TRIGGER broken BEFORE INSERT ON items WHEN NEW.id = 'bad' BEGIN SELECT RAISE(ABORT, 'broken'); END")
	if err := s.SaveListing([]Item{{ID: "b", Name: "b", Path: "b"}, {ID: "bad", Name: "x", Path: "x"}}); err == nil {
		t.Fatal("a broken listing was saved")
	}
	got := byID(t, s)
	if len(got) != 1 || !got["a"].Seen {
		t.Errorf("after a failed listing: %+v", got)
	}
}
