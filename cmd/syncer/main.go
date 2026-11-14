// Command syncer is the terminal prototype of the Google Drive backup tool.
// It runs on demo data: nothing is read from Drive, and the only file it
// writes is a temporary stand-in for the trash preview.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"syncer-gdrive/internal/theme"
	"syncer-gdrive/internal/tui"
)

func main() {
	var opts tui.Options
	flag.BoolVar(&opts.FirstRun, "first-run", false, "start with the first-run wizard")
	flag.StringVar(&opts.Demo, "demo", "", "show a problem state: no-drive or lost-access")
	flag.StringVar(&opts.Lang, "lang", "uk", "interface language: uk or en")
	flag.StringVar(&opts.Theme, "theme", "auto", "light or dark variant: auto, dark or light")
	var ids []string
	for _, s := range theme.Schemes {
		ids = append(ids, s.ID)
	}
	flag.StringVar(&opts.Palette, "palette", "", "colour scheme: "+strings.Join(ids, ", "))
	flag.Parse()

	model, err := tui.New(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "syncer:", err)
		os.Exit(1)
	}
	final, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	if m, ok := final.(tui.Model); ok {
		m.Close()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "syncer:", err)
		os.Exit(1)
	}
}
