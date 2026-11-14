package tui

import (
	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	"syncer-gdrive/internal/demo"
	"syncer-gdrive/internal/format"
)

// The question "where does this sync go" comes before the checks. Its rows:
// the known folders, another folder, and the switch that keeps the question
// coming. The same switch sits in the destination settings.

func (m Model) whereRows() int { return len(demo.Destinations) + 2 }

func (m Model) viewWhere() ([]string, string) {
	th, tr := m.th, m.tr
	rows := []string{m.line(t(" "+tr.T("where.text"), th.Fg)), m.blank()}
	n := len(demo.Destinations)
	focus := 0
	// add appends a row the cursor can stand on.
	add := func(row int, label string, rest ...cell) {
		if row == m.where {
			focus = len(rows)
		}
		rows = append(rows, m.pick(row == m.where, label, rest...))
	}
	for i, d := range demo.Destinations {
		mark := "( ) "
		if i == m.dest {
			mark = "(•) "
		}
		state := t("● "+tr.T("status.connected"), th.Ok)
		if m.opts.Demo == "no-drive" {
			state = t("○ "+tr.T("status.disconnected"), th.Err)
		}
		last := tr.T("where.never")
		if d.LastSync != "" {
			last = tr.T("where.last", d.LastSync)
		}
		free := tr.T("where.free", format.Size(d.Free), format.Size(d.Capacity))
		add(i, strconv.Itoa(i+1)+"  "+mark+d.Path, t("  ", th.Dim), state)
		rows = append(rows, m.line(t("", th.Dim).w(10), t(d.Volume+" · "+free+" · "+last, th.Dim)))
	}
	add(n, strconv.Itoa(n+1)+"  "+tr.T("where.other"))
	rows = append(rows, m.blank())
	add(n+1, strconv.Itoa(n+2)+"  "+checkbox(m.askDest)+tr.T("dest.ask"))
	rows = append(rows, m.blank())
	rows = append(rows, m.para(tr.T("where.note"), th.Dim, m.cw())...)
	// On a short terminal the list scrolls to keep the cursor in sight.
	first, last := window(focus, len(rows), m.h-6)
	out := append(m.head(tr.T("where.title"), tr.T("menu.sync")), rows[first:last]...)
	return out, m.hints("↑↓", tr.T("hint.move"), "Enter", tr.T("hint.next"),
		"1–"+strconv.Itoa(m.whereRows()), tr.T("hint.direct"), "Esc", tr.T("hint.back"))
}

func (m Model) keyWhere(key string) (tea.Model, tea.Cmd) {
	confirm := key == "enter" || key == " "
	if row, ok := digit(key, m.whereRows()); ok {
		m.where, confirm = row, true
	}
	n := len(demo.Destinations)
	switch {
	case key == "esc":
		m.screen = scrMenu
	case !confirm:
		m.where = move(m.where, m.whereRows(), key)
	case m.where < n:
		m.dest = m.where
		m.screen = scrPreflight
	case m.where == n:
		m.flash = m.tr.T("demo.na")
	default:
		m.askDest = !m.askDest
	}
	return m, nil
}
