package tui

import (
	tea "github.com/charmbracelet/bubbletea"
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
			s.remove(row.targets())
		}
	case " ":
		if ok && row.item >= 0 {
			m.togglePreview(s.items[row.item])
		}
	case "esc":
		m.pv.stop()
		m.screen = scrMenu
	}
	s.clamp()
	return m, nil
}

// --- trash ---

func (m Model) viewTrash() ([]string, string) {
	tr := m.tr
	return m.viewList(m.trash), m.listHints(m.trash, "Enter", tr.T("hint.restore"), "d", tr.T("hint.delete"),
		"Space", tr.T("hint.preview"))
}

func (m Model) keyTrash(key string) (tea.Model, tea.Cmd) {
	s := &m.trash
	used, confirmed := s.key(key)
	if confirmed {
		m.flash = m.tr.T("trash.deleted", m.describe(*s, s.doomed))
		s.remove(s.doomed.targets())
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
			s.remove(row.targets())
		}
	case "d":
		// Deleting is for good, so it asks first and says what goes.
		if ok {
			s.doomed = row
			s.open(menuConfirm)
		}
	case " ":
		if ok && row.item >= 0 {
			m.togglePreview(s.items[row.item])
		}
	case "esc":
		m.pv.stop()
		m.screen = scrMenu
	}
	s.clamp()
	return m, nil
}
