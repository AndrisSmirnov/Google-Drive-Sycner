package tui

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"syncer-gdrive/internal/demo"
	"syncer-gdrive/internal/format"
	"syncer-gdrive/internal/i18n"
	"syncer-gdrive/internal/theme"
)

type settingsState struct {
	idx int // row on the settings screen
	sub int // row on the opened sub-screen
}

var settingsScreens = []screen{scrAccount, scrDest, scrLang, scrTheme}

func (m Model) themeName() string {
	scheme := theme.Schemes[m.scheme]
	if !scheme.HasLight() {
		return scheme.Name
	}
	name := scheme.Name + " · " + m.tr.T("theme."+m.themeMode)
	if m.themeMode != "auto" {
		return name
	}
	now := m.tr.T("theme.light")
	if m.th.IsDark {
		now = m.tr.T("theme.dark")
	}
	return name + " (" + now + ")"
}

func (m Model) sharedFolders() []string {
	var names []string
	for _, n := range m.tree.root {
		if n.Dir {
			names = append(names, n.Name)
		}
	}
	return names
}

func (m Model) viewSettings() ([]string, string) {
	th, tr := m.th, m.tr
	out := append(m.head(tr.T("set.title"), ""), m.blank())
	rows := []struct{ label, value string }{
		{tr.T("set.account"), demo.Account},
		{tr.T("set.dest"), m.destination().Path},
		{tr.T("set.lang"), m.langName()},
		{tr.T("set.theme"), m.themeName()},
	}
	for i, r := range rows {
		label := strconv.Itoa(i+1) + "  " + r.label + strings.Repeat(" ", max(24-len([]rune(r.label)), 1))
		out = append(out, m.pick(i == m.set.idx, label, t(r.value, th.Dim)))
	}
	return out, m.hints("↑↓", tr.T("hint.move"), "Enter", tr.T("hint.open"),
		"1–"+strconv.Itoa(len(rows)), tr.T("hint.jump"), "Esc", tr.T("hint.back"))
}

// digit turns a key into the row it stands for among n numbered rows.
func digit(key string, n int) (row int, ok bool) {
	if len(key) != 1 || key[0] < '1' || int(key[0]-'0') > n {
		return 0, false
	}
	return int(key[0] - '1'), true
}

// checkbox draws a checkbox.
func checkbox(on bool) string {
	if on {
		return "[x] "
	}
	return "[ ] "
}

var accountActions = []string{"acc.check", "acc.copy", "acc.replace", "acc.remove"}

func (m Model) viewAccount() ([]string, string) {
	th, tr := m.th, m.tr
	out := m.head(tr.T("set.account"), tr.T("set.title"))
	label := func(key string) cell { return t(" "+tr.T(key), th.Dim).w(12) }
	out = append(out,
		m.line(label("acc.email"), t(demo.Account, th.Fg)),
		m.line(label("acc.key"), t(tr.T("acc.key.val", demo.KeyDate), th.Fg)),
		m.line(label("acc.access"), t("✓ ", th.Ok), t(tr.T("acc.access.val", tr.N("n.folders", len(m.sharedFolders()))), th.Fg)),
		m.blank(),
	)
	for i, key := range accountActions {
		out = append(out, m.pick(i == m.set.sub, strconv.Itoa(i+1)+"  "+tr.T(key)))
	}
	return out, m.hints("↑↓", tr.T("hint.move"), "Enter", tr.T("hint.run"),
		"1–"+strconv.Itoa(len(accountActions)), tr.T("hint.direct"), "Esc", tr.T("hint.back"))
}

func (m Model) viewDest() ([]string, string) {
	th, tr := m.th, m.tr
	files, bytes := m.tree.selected()
	trashN, trashBytes := m.trash.totals()
	dest := m.destination()
	out := m.head(tr.T("set.dest"), tr.T("set.title"))
	label := func(key string) cell { return t(" "+tr.T(key), th.Dim).w(12) }
	out = append(out,
		m.line(label("dest.path"), t(dest.Path, th.Fg)),
		m.line(label("dest.volume"), t(dest.Volume+"  ", th.Fg), t("● "+tr.T("status.connected"), th.Ok)),
		m.line(label("dest.free"), t(tr.T("status.free.val", format.Size(dest.Free), format.Size(dest.Capacity)), th.Fg)),
		m.line(label("dest.copy"), t(tr.N("n.files", files)+" · "+format.Size(bytes), th.Fg)),
		m.line(label("dest.trash"), t(tr.N("n.files", trashN)+" · "+format.Size(trashBytes), th.Fg)),
		m.blank(),
		m.pick(m.set.sub == 0, "1  "+tr.T("dest.change")),
		m.pick(m.set.sub == 1, "2  "+checkbox(m.askDest)+tr.T("dest.ask")),
		m.blank(),
	)
	out = append(out, m.para(tr.T("dest.note"), th.Dim, m.cw())...)
	return out, m.hints("↑↓", tr.T("hint.move"), "Enter", tr.T("hint.run"), "1–2", tr.T("hint.direct"), "Esc", tr.T("hint.back"))
}

// langRows lists the languages as radio buttons; shared with the wizard,
// which shows them without numbers.
func (m Model) langRows(numbered bool) []string {
	th := m.th
	var out []string
	for i, l := range i18n.Langs {
		mark := "( ) "
		if l.Code == m.tr.Code {
			mark = "(•) "
		}
		if numbered {
			mark = strconv.Itoa(i+1) + "  " + mark
		}
		var rest []cell
		if !l.Ready {
			rest = []cell{t("  "+m.tr.T("lang.soon"), th.Dim)}
		}
		out = append(out, m.pick(i == m.set.sub, mark+l.Name, rest...))
	}
	return out
}

func (m Model) viewLang() ([]string, string) {
	tr := m.tr
	out := append(m.head(tr.T("set.lang"), tr.T("set.title")), m.blank())
	out = append(out, m.langRows(true)...)
	out = append(out, m.blank(), m.line(t(" "+tr.T("lang.note"), m.th.Dim)))
	return out, m.hints("↑↓", tr.T("hint.move"), "Enter", tr.T("hint.choose"),
		"1–"+strconv.Itoa(len(i18n.Langs)), tr.T("hint.direct"), "Esc", tr.T("hint.back"))
}

// setLang switches the interface language; it reports whether it did.
func (m *Model) setLang(i int) bool {
	l := i18n.Langs[i]
	if !l.Ready {
		m.flash = m.tr.T("lang.later", l.Name)
		return false
	}
	catalog, err := i18n.Load(l.Code)
	if err != nil {
		m.flash = err.Error()
		return false
	}
	m.tr = catalog
	return true
}

var themeModes = []string{"auto", "dark", "light"}

// viewTheme has three parts on one list: the colour scheme, the light or
// dark variant, and whether the background is painted. Every scheme shows a
// sample of its own colours.
func (m Model) viewTheme() ([]string, string) {
	th, tr := m.th, m.tr
	var rows []string
	focus, row := 0, 0
	add := func(label string, rest ...cell) {
		if row == m.set.sub {
			focus = len(rows)
		}
		rows = append(rows, m.pick(row == m.set.sub, label, rest...))
		row++
	}
	radio := func(on bool) string {
		if on {
			return "(•) "
		}
		return "( ) "
	}

	rows = append(rows, m.line(t(" "+tr.T("theme.scheme"), th.Dim)))
	for i, s := range theme.Schemes {
		p := s.Variant(th.IsDark)
		bg := lipgloss.Color(p.Bg)
		label := s.Key + "  " + radio(i == m.scheme) + s.Name +
			strings.Repeat(" ", max(16-runewidth.StringWidth(s.Name), 2))
		sample := []cell{t(" Aa ", lipgloss.Color(p.Fg)).on(bg)}
		for _, c := range []string{p.Accent, p.Second, p.Ok, p.Warn, p.Err} {
			sample = append(sample, t("● ", lipgloss.Color(c)).on(bg))
		}
		add(label, sample...)
	}
	mode := []cell{t(" "+tr.T("theme.mode"), th.Dim)}
	if !theme.Schemes[m.scheme].HasLight() {
		mode = append(mode, t(" — "+tr.T("theme.darkonly"), th.Warn))
	}
	rows = append(rows, m.blank(), m.line(mode...))
	for _, mode := range themeModes {
		add("   " + radio(m.themeMode == mode) + tr.T("theme."+mode+".long"))
	}
	rows = append(rows, m.blank())
	add("   " + checkbox(m.paintBg) + tr.T("theme.paint"))
	rows = append(rows, m.blank(), m.line(t(" "+tr.T("theme.paint.note"), th.Dim)))

	first, last := window(focus, len(rows), m.h-6)
	out := append(m.head(tr.T("set.theme"), tr.T("set.title")), rows[first:last]...)
	// The keys of the palettes: the digits as a range, the letters one by one.
	keys := "1–9, 0"
	for _, s := range theme.Schemes {
		if s.Key < "0" || s.Key > "9" {
			keys += ", " + s.Key
		}
	}
	return out, m.hints("↑↓", tr.T("hint.move"), "Enter", tr.T("hint.choose"), keys, tr.T("hint.palette"), "Esc", tr.T("hint.back"))
}

func (m Model) keySettings(key string) (tea.Model, tea.Cmd) {
	if key == "esc" {
		if m.screen == scrSettings {
			m.screen = scrMenu
		} else {
			m.screen = scrSettings
		}
		return m, nil
	}
	confirm := key == "enter" || key == " "
	switch m.screen {
	case scrSettings:
		m.set.idx = move(m.set.idx, len(settingsScreens), key)
		if row, ok := digit(key, len(settingsScreens)); ok {
			m.set.idx, key = row, "enter"
		}
		if key == "enter" {
			m.screen = settingsScreens[m.set.idx]
			m.set.sub = 0
			switch m.screen {
			case scrLang:
				for i, l := range i18n.Langs {
					if l.Code == m.tr.Code {
						m.set.sub = i
					}
				}
			case scrTheme:
				m.set.sub = m.scheme
			}
		}
	case scrAccount:
		m.set.sub = move(m.set.sub, len(accountActions), key)
		if row, ok := digit(key, len(accountActions)); ok {
			m.set.sub, confirm = row, true
		}
		if confirm {
			m.flash = m.tr.T("demo.na")
		}
	case scrDest:
		m.set.sub = move(m.set.sub, 2, key)
		if row, ok := digit(key, 2); ok {
			m.set.sub, confirm = row, true
		}
		switch {
		case confirm && m.set.sub == 1:
			m.askDest = !m.askDest
		case confirm:
			m.flash = m.tr.T("demo.na")
		}
	case scrLang:
		m.set.sub = move(m.set.sub, len(i18n.Langs), key)
		if row, ok := digit(key, len(i18n.Langs)); ok {
			m.set.sub, confirm = row, true
		}
		if confirm && m.setLang(m.set.sub) {
			m.flash = m.tr.T("lang.changed")
		}
	case scrTheme:
		schemes, modes := len(theme.Schemes), len(themeModes)
		m.set.sub = move(m.set.sub, schemes+modes+1, key)
		// Every palette has a key of its own: a digit or a letter.
		for row, s := range theme.Schemes {
			if key == s.Key {
				m.set.sub, confirm = row, true
			}
		}
		if confirm {
			switch {
			case m.set.sub < schemes:
				m.scheme = m.set.sub
			case m.set.sub < schemes+modes:
				m.themeMode = themeModes[m.set.sub-schemes]
			default:
				m.paintBg = !m.paintBg
			}
			m.applyTheme()
		}
	}
	return m, nil
}

// --- first-run wizard ---

const wizardSteps = 4

func (m Model) viewWizard() ([]string, string) {
	th, tr := m.th, m.tr
	step := tr.T("wiz.step", m.wiz+1, wizardSteps)
	field := func(value string) string {
		return m.sel(t(" "+value, th.Fg), t("▏", th.Accent))
	}
	label := func(key string) cell { return t(" "+tr.T(key), th.Dim).w(12) }
	back := []string{"Esc", tr.T("hint.back")}

	switch m.wiz {
	case 0:
		out := append(m.head(tr.T("wiz.title"), step), m.line(t(tr.T("wiz.lang"), th.Fg)), m.blank())
		out = append(out, m.langRows(false)...)
		return out, m.hints("↑↓", tr.T("hint.move"), "Enter", tr.T("hint.next"))
	case 1:
		out := append(m.head(tr.T("wiz.key.title"), step), m.para(tr.T("wiz.key.text"), th.Fg, m.cw())...)
		out = append(out, m.blank(), m.line(t(" "+tr.T("wiz.key.label"), th.Dim)), field(demo.KeyPath), m.blank(),
			m.line(t(" ✓ "+tr.T("wiz.key.ok"), th.Ok), t(" · "+demo.Account, th.Dim)), m.blank())
		out = append(out, m.para(tr.T("wiz.key.note"), th.Dim, m.cw())...)
		return out, m.hints(append([]string{"Enter", tr.T("hint.import")}, back...)...)
	case 2:
		folders := m.sharedFolders()
		out := append(m.head(tr.T("wiz.share.title"), step), m.para(tr.T("wiz.share.text"), th.Fg, m.cw())...)
		out = append(out, m.blank(), m.sel(t(" "+demo.Account, th.Accent).b()), m.blank(),
			m.line(t(" ✓ "+tr.T("wiz.share.ok", len(folders)), th.Ok)),
			m.line(t("   "+strings.Join(folders, " · "), th.Second).b()), m.blank())
		out = append(out, m.para(tr.T("wiz.share.note"), th.Dim, m.cw())...)
		return out, m.hints(append([]string{"Enter", tr.T("hint.next")}, back...)...)
	default:
		out := append(m.head(tr.T("wiz.dest.title"), step), m.para(tr.T("wiz.dest.text"), th.Fg, m.cw())...)
		dest := m.destination()
		out = append(out, m.blank(), m.line(t(" "+tr.T("dest.path"), th.Dim)), field(dest.Path), m.blank(),
			m.line(label("dest.volume"), t(dest.Volume, th.Fg)),
			m.line(label("dest.free"), t(tr.T("status.free.val", format.Size(dest.Free), format.Size(dest.Capacity)), th.Fg)),
			m.blank(), m.line(t(" ✓ "+tr.T("wiz.dest.ok"), th.Ok)))
		return out, m.hints(append([]string{"Enter", tr.T("hint.finish")}, back...)...)
	}
}

func (m Model) keyWizard(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k", "down", "j":
		if m.wiz == 0 {
			m.set.sub = move(m.set.sub, len(i18n.Langs), key)
		}
	case "esc":
		if m.wiz > 0 {
			m.wiz--
		}
	case "enter":
		if m.wiz == 0 && !m.setLang(m.set.sub) {
			return m, nil
		}
		m.wiz++
		if m.wiz == wizardSteps {
			m.wiz = 0
			m.tree.remember()
			m.screen = scrTree
			m.flash = m.tr.T("wiz.done")
		}
	}
	return m, nil
}
