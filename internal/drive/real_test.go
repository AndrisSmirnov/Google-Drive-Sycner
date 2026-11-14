package drive

import (
	"context"
	"os"
	"testing"
	"time"
)

// realClient gives a client for a real account when SYNCER_TEST_KEY names a
// Service Account key file. Without it the test is skipped, so the ordinary
// test run never needs the network or a key.
func realClient(t *testing.T) *Client {
	t.Helper()
	path := os.Getenv("SYNCER_TEST_KEY")
	if path == "" {
		t.Skip("SYNCER_TEST_KEY is not set")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ParseKey(data)
	if err != nil {
		t.Fatal(err)
	}
	return New(key)
}

// It prints numbers only: the names of real files stay out of the log.
func TestRealList(t *testing.T) {
	c := realClient(t)
	began := time.Now()
	files, err := c.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var folders, google, shortcuts, locked int
	var bytes int64
	for _, f := range files {
		switch {
		case f.IsFolder():
			folders++
		case f.IsShortcut():
			shortcuts++
		case f.GoogleType() != "":
			google++
		default:
			bytes += f.Size
		}
		if !f.CanDownload {
			locked++
		}
	}
	t.Logf("%d items in %s: %d folders, %d Google documents, %d shortcuts, %d not downloadable, %d bytes in files",
		len(files), time.Since(began).Round(time.Millisecond), folders, google, shortcuts, locked, bytes)
	if len(files) == 0 {
		t.Error("the account sees nothing: is a folder shared with it?")
	}
}
