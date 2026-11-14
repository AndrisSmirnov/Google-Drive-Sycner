package drive

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"io"
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

// It downloads the smallest ordinary file and exports one Google document of
// each kind into memory, and checks the checksum of the download.
func TestRealDownloadAndExport(t *testing.T) {
	c := realClient(t)
	ctx := context.Background()
	files, err := c.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var smallest *File
	exported := map[string]bool{}
	for i, f := range files {
		switch kind := f.GoogleType(); {
		case kind != "":
			if _, ok := ExportExt(kind); !ok || exported[kind] {
				continue
			}
			exported[kind] = true
			var buf bytes.Buffer
			n, err := c.Export(ctx, f.ID, kind, &buf)
			if err != nil {
				t.Errorf("export of a %s: %v", kind, err)
				continue
			}
			t.Logf("%s: %d bytes on Drive, %d bytes exported, a zip archive: %v", kind, f.Size, n, bytes.HasPrefix(buf.Bytes(), []byte("PK")))
		case !f.IsFolder() && !f.IsShortcut() && f.Size > 64 && (smallest == nil || f.Size < smallest.Size):
			smallest = &files[i]
		}
	}
	if smallest == nil {
		t.Skip("no ordinary file to download")
	}
	// In two halves, the way a broken download is continued.
	var buf bytes.Buffer
	half := &limitedWriter{w: &buf, room: smallest.Size / 2}
	n, _ := c.Download(ctx, smallest.ID, 0, half)
	if _, err := c.Download(ctx, smallest.ID, n, &buf); err != nil {
		t.Fatal(err)
	}
	sum := md5.Sum(buf.Bytes())
	t.Logf("downloaded %d bytes in two parts (first %d), checksum matches: %v", buf.Len(), n, hex.EncodeToString(sum[:]) == smallest.MD5)
	if hex.EncodeToString(sum[:]) != smallest.MD5 || int64(buf.Len()) != smallest.Size {
		t.Error("the downloaded content differs from what Drive describes")
	}
}

// limitedWriter stops accepting bytes after a while, to break a download.
type limitedWriter struct {
	w    io.Writer
	room int64
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > l.room {
		n, _ := l.w.Write(p[:l.room])
		l.room = 0
		return n, io.ErrShortWrite
	}
	l.room -= int64(len(p))
	return l.w.Write(p)
}
