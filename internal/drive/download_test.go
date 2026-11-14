package drive

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// serveContent answers a download the way Drive does: the whole content, or
// the rest of it when a range is asked for.
func serveContent(content string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("alt") != "media" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		from := 0
		if rng := r.Header.Get("Range"); rng != "" {
			from, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rng, "bytes="), "-"))
			if from >= len(content) {
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return
			}
			w.WriteHeader(http.StatusPartialContent)
		}
		io.WriteString(w, content[from:])
	}
}

// brokenWriter fails after taking a number of bytes, like a connection or a
// drive that goes away in the middle of a file.
type brokenWriter struct {
	got  bytes.Buffer // a field, not embedded: io.Copy must go through Write
	room int
}

func (b *brokenWriter) Write(p []byte) (int, error) {
	if len(p) > b.room {
		n, _ := b.got.Write(p[:b.room])
		b.room = 0
		return n, errors.New("the drive is gone")
	}
	b.room -= len(p)
	return b.got.Write(p)
}

func TestDownloadCanBeContinued(t *testing.T) {
	const content = "0123456789абвгґдеєжз"
	f := newFake(t)
	f.files = serveContent(content)
	c := f.client()
	ctx := context.Background()

	var whole bytes.Buffer
	if n, err := c.Download(ctx, "f1", 0, &whole); err != nil || whole.String() != content || int(n) != len(content) {
		t.Fatalf("whole file: %d bytes, %q, %v", n, whole.String(), err)
	}

	// A download that breaks off reports how far it got; a second call
	// brings the rest.
	part := &brokenWriter{room: 7}
	n, err := c.Download(ctx, "f1", 0, part)
	if err == nil || n != 7 {
		t.Fatalf("broken download: %d bytes, %v; want 7 and an error", n, err)
	}
	var rest bytes.Buffer
	if _, err := c.Download(ctx, "f1", n, &rest); err != nil {
		t.Fatal(err)
	}
	if got := part.got.String() + rest.String(); got != content {
		t.Errorf("continued download gave %q", got)
	}

	// Nothing is left of a file that is already complete.
	var none bytes.Buffer
	if n, err := c.Download(ctx, "f1", int64(len(content)), &none); err != nil || n != 0 || none.Len() != 0 {
		t.Errorf("complete file: %d bytes, %v", n, err)
	}
}

func TestDownloadThatIgnoresTheOffsetIsRejected(t *testing.T) {
	f := newFake(t)
	f.files = func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "whole content") }
	var buf bytes.Buffer
	n, err := f.client().Download(context.Background(), "f1", 5, &buf)
	if !errors.Is(err, ErrNoResume) || n != 0 || buf.Len() != 0 {
		t.Fatalf("%d bytes, %v; want ErrNoResume and nothing written", n, err)
	}
}

func TestExport(t *testing.T) {
	f := newFake(t)
	f.files = func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/doc1/export"):
			if got := r.URL.Query().Get("mimeType"); !strings.Contains(got, "wordprocessingml") {
				t.Errorf("document exported as %q", got)
			}
			io.WriteString(w, "PK-docx")
		case strings.HasSuffix(r.URL.Path, "/big/export"):
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":{"message":"This file is too large to be exported.","errors":[{"reason":"exportSizeLimitExceeded"}]}}`)
		case strings.HasSuffix(r.URL.Path, "/doc1"):
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":{"message":"Only files with binary content can be downloaded.","errors":[{"reason":"fileNotDownloadable"}]}}`)
		}
	}
	c := f.client()
	ctx := context.Background()

	var buf bytes.Buffer
	if n, err := c.Export(ctx, "doc1", "document", &buf); err != nil || buf.String() != "PK-docx" || n != 7 {
		t.Fatalf("export: %d bytes, %q, %v", n, buf.String(), err)
	}
	if _, err := c.Export(ctx, "big", "spreadsheet", io.Discard); !Refused(err, ReasonExportTooLarge) {
		t.Errorf("an export over the limit: %v", err)
	}
	if _, err := c.Download(ctx, "doc1", 0, io.Discard); !Refused(err, ReasonNotDownloadable) {
		t.Errorf("downloading a Google document: %v", err)
	}
	if _, err := c.Export(ctx, "form1", "form", io.Discard); err == nil {
		t.Error("a form was exported")
	}

	for googleType, want := range map[string]string{"document": ".docx", "spreadsheet": ".xlsx", "presentation": ".pptx"} {
		if ext, ok := ExportExt(googleType); !ok || ext != want {
			t.Errorf("ExportExt(%q) = %q, %v", googleType, ext, ok)
		}
	}
	if _, ok := ExportExt("form"); ok {
		t.Error("a form has an export format")
	}
}
