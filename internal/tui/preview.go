package tui

import (
	"encoding/base64"
	"errors"
	"fmt"
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

// openFile shows a real file in the system viewer. Tests replace it.
var openFile = showFile

// showFile opens a file the way the system previews it: Quick Look on
// macOS, the default program elsewhere.
func showFile(file string) (*viewer, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("qlmanage", "-p", file)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", file)
	default:
		cmd = exec.Command("xdg-open", file)
	}
	if _, err := exec.LookPath(cmd.Args[0]); err != nil {
		return nil, errNoViewer
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		cmd.Wait()
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

// osc52 asks the terminal to put text on the clipboard. Terminals that do
// not know the request ignore it.
func osc52(text string) {
	fmt.Fprintf(os.Stdout, "\x1b]52;c;%s\x07", base64.StdEncoding.EncodeToString([]byte(text)))
}

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
// pressed on the file that is already shown. file is where the file lies
// on the drive; with demo data there is no file and a stand-in is shown.
func (m *Model) togglePreview(it demo.Entry, file string) {
	tr := m.tr
	same := m.pv.open() && m.pv.path == it.Path
	m.pv.stop()
	if same {
		m.flash = tr.T("preview.closed")
		return
	}
	var v *viewer
	var err error
	if file != "" {
		v, err = openFile(file)
	} else {
		v, err = openPreview(it.Name(), tr.T("preview.demo", it.Path, format.Size(it.Size), it.Modified))
	}
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
