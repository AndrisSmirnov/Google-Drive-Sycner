package tui

import (
	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	"syncer-gdrive/internal/format"
)

type menuItem struct {
	label  string
	info   string
	target screen
}

const quitItem screen = -1

func (m Model) menuItems() []menuItem {
	tr := m.tr
	_, selBytes := m.tree.selected()
	trashN, trashBytes := m.trash.totals()
	// Choosing comes first: it is what a sync acts on.
	return []menuItem{
		{tr.T("menu.files"), tr.T("menu.files.info", format.Size(selBytes)), scrTree},
		{tr.T("menu.sync"), tr.T("menu.sync.info", m.lastSync()), scrPreflight},
		{tr.T("menu.skip"), tr.N("n.files", len(m.skip.items)), scrSkip},
		{tr.T("menu.trash"), tr.N("n.files", trashN) + " · " + format.Size(trashBytes), scrTrash},
		{tr.T("menu.settings"), tr.T("menu.settings.info", m.langName()), scrSettings},
		{tr.T("menu.quit"), "", quitItem},
	}
}

func (m Model) viewMenu() ([]string, string) {
	th, tr := m.th, m.tr
	files, bytes := m.tree.selected()
	dest := m.destination()

	drive := t("● "+tr.T("status.connected"), th.Ok)
	if dest.Missing {
		drive = t("○ "+tr.T("status.disconnected"), th.Err)
	}
	account := m.account()
	if account == "" {
		account = tr.T("acc.none")
	}
	label := func(key string) cell { return t(tr.T(key), th.Dim).w(12) }

	out := []string{
		m.line(t("SYNCER", th.Accent).b(), t("  "+tr.T("app.tagline"), th.Dim), m.gap(), t(tr.T("app.version"), th.Dim)),
		m.rule(),
		m.line(label("status.account"), t(account, th.Fg)),
		m.line(label("status.drive"), t(dest.Path+"  ", th.Fg), drive),
		m.line(label("status.free"), t(tr.T("status.free.val", format.Size(dest.Free), format.Size(dest.Capacity)), th.Fg)),
		m.line(label("status.last"), t(m.lastSync()+" · "+tr.N("n.files", files)+" · "+format.Size(bytes), th.Fg)),
		m.blank(),
	}
	for i, it := range m.menuItems() {
		num := strconv.Itoa(i+1) + "  "
		out = append(out, m.pick(i == m.menuIdx, num+it.label, m.gap(), t(it.info+" ", th.Dim)))
	}
	return out, m.hints("↑↓", tr.T("hint.move"), "Enter", tr.T("hint.open"), "1–6", tr.T("hint.jump"), "q", tr.T("hint.quit"))
}

func (m Model) keyMenu(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k", "down", "j":
		m.menuIdx = move(m.menuIdx, len(m.menuItems()), key)
	case "q":
		return m, tea.Quit
	case "enter":
		return m.openMenu()
	case "1", "2", "3", "4", "5", "6":
		m.menuIdx = int(key[0] - '1')
		return m.openMenu()
	}
	return m, nil
}

func (m Model) openMenu() (tea.Model, tea.Cmd) {
	target := m.menuItems()[m.menuIdx].target
	switch target {
	case quitItem:
		return m, tea.Quit
	case scrPreflight:
		if m.askDest {
			return m.askWhere(whereSync), nil
		}
		if m.lv != nil {
			return m.planSync()
		}
		m.run = m.newRun()
	case scrTree:
		if m.lv != nil {
			return m.listThenTree()
		}
		m.tree.remember()
	case scrSkip:
		if m.lv != nil {
			m.reload()
		}
		m.flash = m.tr.T("skip.note")
	case scrTrash:
		if m.lv != nil {
			m.reload()
		}
	case scrSettings:
		m.set = settingsState{}
	}
	m.screen = target
	return m, nil
}
