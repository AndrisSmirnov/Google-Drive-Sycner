package tui

import (
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"syncer-gdrive/internal/demo"
)

// The trash and the SKIP list are the same kind of file list (browse.go);
// what differs is what can be done with a file.

// --- SKIP files ---

func (m Model) viewSkip() ([]string, string) {
	tr := m.tr
	return m.viewList(m.skip), m.listHints(m.skip, "r", tr.T("hint.unskip"), "Space", tr.T("hint.preview"))
}

func (m Model) keySkip(key string) (tea.Model, tea.Cmd) {
	s := &m.skip
	if used, _ := s.key(key); used {
		return m, nil
	}
	row, ok := s.current()
	switch key {
	case "r":
		if ok {
			m.flash = m.tr.T("skip.removed", m.describe(*s, row))
			m.act("unskip", s, row)
		}
	case " ":
		if ok && row.item >= 0 {
			m.togglePreview(s.items[row.item], m.fileOf(s.items[row.item].Path))
		}
	case "esc":
		m.pv.stop()
		m.screen = scrMenu
	}
	s.clamp()
	return m, nil
}

// act takes the files of a row out of a list. On real data it first does
// to the files what was asked; if that fails, the message says so.
func (m *Model) act(action string, s *browser, row listRow) {
	if m.lv == nil {
		s.remove(row.targets())
		return
	}
	var entries []demo.Entry
	for _, i := range row.targets() {
		entries = append(entries, s.items[i])
	}
	if err := m.keptAct(action, entries); err != nil {
		m.flash = err.Error()
	}
}

// fileOf is where a file of the destination lies on this computer; empty
// with demo data, which has no files.
func (m Model) fileOf(rel string) string {
	if m.lv == nil || rel == "" {
		return ""
	}
	return filepath.Join(m.lv.dest, filepath.FromSlash(rel))
}

// --- trash ---

func (m Model) viewTrash() ([]string, string) {
	tr := m.tr
	return m.viewList(m.trash), m.listHints(m.trash, "Enter", tr.T("hint.restore"), "d", tr.T("hint.delete"),
		"e", tr.T("hint.empty"), "Space", tr.T("hint.preview"))
}

func (m Model) keyTrash(key string) (tea.Model, tea.Cmd) {
	s := &m.trash
	used, confirmed := s.key(key)
	if confirmed {
		m.flash = m.tr.T("trash.deleted", m.describe(*s, s.doomed))
		m.act("delete", s, s.doomed)
		s.clamp()
	}
	if used {
		return m, nil
	}
	row, ok := s.current()
	switch key {
	case "enter":
		if ok {
			m.flash = m.tr.T("trash.restored", m.describe(*s, row))
			m.act("restore", s, row)
		}
	case "d":
		// Deleting is for good, so it asks first and says what goes.
		if ok {
			s.doomed = row
			s.open(menuConfirm)
		}
	case "e":
		// Emptying the trash is the same question about everything in it.
		if len(s.items) > 0 {
			all := make([]int, len(s.items))
			for i := range all {
				all[i] = i
			}
			s.doomed = listRow{item: -1, dir: m.tr.T("trash.title"), files: all}
			s.open(menuConfirm)
		}
	case " ":
		if ok && row.item >= 0 {
			m.togglePreview(s.items[row.item], m.fileOf(s.items[row.item].Stored))
		}
	case "esc":
		m.pv.stop()
		m.screen = scrMenu
	}
	s.clamp()
	return m, nil
}
