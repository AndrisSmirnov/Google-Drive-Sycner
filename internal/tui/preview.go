package tui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"syncer-gdrive/internal/demo"
	"syncer-gdrive/internal/format"
)

// viewer is an open preview window.
type viewer struct {
	close   func()      // closes the window
	running func() bool // false once the window is gone
}

// previewState remembers which file is being previewed. The model is copied
// on every update, so it holds this by pointer.
type previewState struct {
	path string
	v    *viewer
}

func (p *previewState) open() bool { return p.v != nil && p.v.running() }

func (p *previewState) stop() {
	if p.open() {
		p.v.close()
	}
	p.v = nil
}

var errNoViewer = errors.New("no system viewer")

// openPreview shows a file in the system viewer. Tests replace it so that
// they never open a window.
var openPreview = quickLook

// quickLook opens the macOS Quick Look window, the one Finder shows on
// Space. The prototype has no real files, so it previews a stand-in text
// file that is removed when the window closes.
func quickLook(name, text string) (*viewer, error) {
	if runtime.GOOS != "darwin" {
		return nil, errNoViewer
	}
	bin, err := exec.LookPath("qlmanage")
	if err != nil {
		return nil, errNoViewer
	}
	dir, err := os.MkdirTemp("", "syncer-preview-")
	if err != nil {
		return nil, err
	}
	file := filepath.Join(dir, name+".txt")
	if err := os.WriteFile(file, []byte(text), 0o600); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	cmd := exec.Command(bin, "-p", file) // its output is discarded
	if err := cmd.Start(); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		cmd.Wait()
		os.RemoveAll(dir)
		close(done)
	}()
	return &viewer{
		close: func() { cmd.Process.Kill() },
		running: func() bool {
			select {
			case <-done:
				return false
			default:
				return true
			}
		},
	}, nil
}

// togglePreview opens the preview of a file, or closes it when Space is
// pressed on the file that is already shown.
func (m *Model) togglePreview(it demo.Entry) {
	tr := m.tr
	same := m.pv.open() && m.pv.path == it.Path
	m.pv.stop()
	if same {
		m.flash = tr.T("preview.closed")
		return
	}
	v, err := openPreview(it.Name(), tr.T("preview.demo", it.Path, format.Size(it.Size), it.Modified))
	switch {
	case errors.Is(err, errNoViewer):
		m.flash = tr.T("preview.na")
	case err != nil:
		m.flash = tr.T("preview.failed", err.Error())
	default:
		m.pv.path, m.pv.v = it.Path, v
		m.flash = tr.T("preview.opened", it.Path)
	}
}

// Close releases what the interface holds outside the terminal.
func (m Model) Close() {
	if m.pv != nil {
		m.pv.stop()
	}
}
