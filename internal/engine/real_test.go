package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"syncer-gdrive/internal/drive"
)

// TestRealSync syncs a real shared folder into a temporary one. It runs
// only when SYNCER_TEST_KEY names a Service Account key file. To stay
// quick it selects files of up to 5 Mb and Google documents; the names of
// real files are never printed.
func TestRealSync(t *testing.T) {
	path := os.Getenv("SYNCER_TEST_KEY")
	if path == "" {
		t.Skip("SYNCER_TEST_KEY is not set")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	key, err := drive.ParseKey(data)
	if err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	e, err := Open(dest, drive.New(key))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	ctx := context.Background()

	s, err := e.Refresh(ctx)
	if err != nil {
		t.Fatal(err)
	}
	checked := map[string]bool{}
	small := 0
	for id, it := range s.Items {
		if it.Folder {
			continue
		}
		checked[id] = it.Size <= 5<<20
		if checked[id] {
			small++
		}
	}
	if err := e.SetSelection(checked, nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("Drive: %d items, %d top-level, %d selected for the test, clashes %d, not copied %v",
		len(s.Items), len(s.Top), small, len(s.Clash), s.NotCopied)

	p, err := e.Plan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	began := time.Now()
	r, err := e.Run(ctx, p, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("first sync: %d files, %d bytes in %s; failures %d, left out %v",
		r.Downloaded.Files, r.Downloaded.Bytes, time.Since(began).Round(time.Millisecond), len(r.Failures), r.NotCopied)
	for _, f := range r.Failures {
		t.Errorf("a file failed: %v", f.Err)
	}

	// Every ordinary file on the drive matches the checksum Drive gave.
	items, _ := e.Store().Items()
	checkedFiles := 0
	for _, it := range items {
		if it.Copy == nil {
			continue
		}
		full := filepath.Join(dest, filepath.FromSlash(it.Path))
		info, err := os.Stat(full)
		if err != nil {
			t.Errorf("a copied file is missing")
			continue
		}
		if it.Google == "" {
			if sum, _ := checksum(full); sum != it.MD5 || info.Size() != it.Size {
				t.Errorf("a copied file differs from Drive")
			}
			checkedFiles++
		}
	}
	t.Logf("%d copies checked against their checksums", checkedFiles)

	// A second sync has nothing to do.
	p, err = e.Plan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Downloads)+len(p.Moves)+len(p.Leftovers) != 0 {
		t.Errorf("a second sync wants %d downloads, %d moves, %d leftovers", len(p.Downloads), len(p.Moves), len(p.Leftovers))
	}
}
