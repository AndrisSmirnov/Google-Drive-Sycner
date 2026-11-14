// Package tui is the terminal interface. A live session works on Google
// Drive and on the destination through the engine. With demo data nothing
// is touched: no Drive access, and no file operations apart from the
// temporary stand-in file of the preview.
package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"syncer-gdrive/internal/demo"
	"syncer-gdrive/internal/i18n"
	"syncer-gdrive/internal/theme"
)

// Options come from the command line.
type Options struct {
	FirstRun bool
	Version  string // shown in the head of the menu
	Demo     string // "", "no-drive" or "lost-access"
	Lang     string // "uk" or "en"
	Theme    string // "auto", "dark" or "light"
	Palette  string // a colour scheme identifier, "" for the default
}

type screen int

const (
	scrMenu  screen = iota
	scrWhere        // where this sync goes; shown before the checks
	scrPreflight
	scrDeleted
	scrProgress
	scrSummary
	scrTree
	scrSkip
	scrTrash
	scrSettings
	scrAccount
	scrDest
	scrLang
	scrTheme
	scrWizard
)

// Model is the whole interface state.
type Model struct {
	opts Options
	th   theme.Theme
	tr   *i18n.Catalog
	w, h int

	screen screen
	flash  string // one-off message shown above the key hints

	scheme    int    // index into theme.Schemes
	themeMode string // auto | dark | light
	paintBg   bool
	darkTerm  bool // what the terminal reported at start

	menuIdx int
	dest    int  // index into demo.Destinations
	askDest bool // ask where to sync before every sync
	where   int  // row on the question screen
	tree    treeState
	run     runState
	skip    browser
	trash   browser
	set     settingsState
	wiz     int
	pv      *previewState

	// lv is the work on real data; nil when the interface runs on demo data.
	lv       *live
	whereFor int     // what the destination question was asked for
	prompt   *prompt // a question answered by typing, shown instead of the screen
	busy     string  // what the program is waiting for; keys wait too
	cancel   func()  // stops what busy waits for
	failure  string  // an error to show before going back to the menu
}

// New builds the model. It asks the terminal for its background colour, so
// it must run before the Bubble Tea program starts.
func New(o Options) (Model, error) {
	code := o.Lang
	if code == "" {
		code = "uk"
	}
	tr, err := i18n.Load(code)
	if err != nil {
		return Model{}, err
	}
	mode := o.Theme
	if mode != "dark" && mode != "light" {
		mode = "auto"
	}
	scheme := 0
	if o.Palette != "" {
		var ok bool
		if scheme, ok = theme.Index(o.Palette); !ok {
			var ids []string
			for _, s := range theme.Schemes {
				ids = append(ids, s.ID)
			}
			return Model{}, fmt.Errorf("unknown palette %q, choose one of: %s", o.Palette, strings.Join(ids, ", "))
		}
	}
	m := Model{
		opts:      o,
		tr:        tr,
		scheme:    scheme,
		themeMode: mode,
		paintBg:   true,
		askDest:   true,
		darkTerm:  lipgloss.HasDarkBackground(),
		tree:      newTree(),
		skip:      newBrowser("skip", demo.SkipList()),
		trash:     newBrowser("trash", demo.Trash()),
		pv:        &previewState{},
	}
	m.applyTheme()
	if o.FirstRun {
		m.screen = scrWizard
	}
	return m, nil
}

func (m *Model) applyTheme() {
	dark := m.themeMode == "dark" || (m.themeMode == "auto" && m.darkTerm)
	m.th = theme.New(theme.Schemes[m.scheme], dark, m.paintBg)
}

// destination is the folder the copy goes to.
func (m Model) destination() demo.Destination {
	if m.lv != nil {
		return m.lv.info
	}
	d := demo.Destinations[m.dest]
	d.Missing = m.opts.Demo == "no-drive"
	return d
}

// dests are the folders the question "where to sync" offers.
func (m Model) dests() []demo.Destination {
	if m.lv != nil {
		return m.lv.dests
	}
	out := slices.Clone(demo.Destinations)
	for i := range out {
		out[i].Missing = m.opts.Demo == "no-drive"
	}
	return out
}

// account is the email of the Service Account, empty if there is no key.
func (m Model) account() string {
	switch {
	case m.lv == nil:
		return demo.Account
	case m.lv.key == nil:
		return ""
	}
	return m.lv.key.Email
}

// langs lists the codes of the interface languages.
func langs() []string {
	var out []string
	for _, l := range i18n.Langs {
		out = append(out, l.Code)
	}
	return out
}

// Close releases what the interface holds outside the terminal: an open
// preview and the state of the destination.
func (m Model) Close() {
	if m.pv != nil {
		m.pv.stop()
	}
	if m.lv != nil {
		if m.lv.stopRun != nil {
			m.lv.stopRun()
		}
		m.lv.closeEngine()
	}
}

// lastSync is when the destination was last synced, in words if never.
func (m Model) lastSync() string {
	if last := m.destination().LastSync; last != "" {
		return last
	}
	return m.tr.T("status.never")
}

func (m Model) langName() string {
	for _, l := range i18n.Langs {
		if l.Code == m.tr.Code {
			return l.Name
		}
	}
	return m.tr.Code
}

type tickMsg struct{}

func tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tickMsg:
		return m.onTick()
	case listedMsg:
		return m.onListed(msg)
	case plannedMsg:
		return m.onPlanned(msg)
	case ranMsg:
		return m.onRan(msg)
	case accessMsg:
		return m.onAccess(msg)
	case verifiedMsg:
		return m.onVerified(msg)
	case tea.KeyMsg:
		key := msg.String()
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		switch {
		case m.busy != "":
			// The program is waiting for Drive: Esc gives up, the rest waits.
			if key == "esc" && m.cancel != nil {
				m.cancel()
			}
			return m, nil
		case m.prompt != nil:
			return m.keyPrompt(msg)
		case m.failure != "":
			if key == "enter" || key == "esc" {
				m.failure = ""
				if m.screen != scrWizard && m.screen != scrWhere {
					m.screen = scrMenu
				}
			}
			return m, nil
		}
		m.flash = ""
		switch m.screen {
		case scrMenu:
			return m.keyMenu(key)
		case scrWhere:
			return m.keyWhere(key)
		case scrPreflight:
			return m.keyPreflight(key)
		case scrDeleted:
			return m.keyDeleted(key)
		case scrProgress:
			return m.keyProgress(key)
		case scrSummary:
			return m.keySummary(key)
		case scrTree:
			return m.keyTree(key)
		case scrSkip:
			return m.keySkip(key)
		case scrTrash:
			return m.keyTrash(key)
		case scrWizard:
			return m.keyWizard(key)
		default:
			return m.keySettings(key)
		}
	}
	return m, nil
}

func (m Model) body() ([]string, string) {
	th, tr := m.th, m.tr
	switch {
	case m.busy != "":
		return []string{m.blank(), m.line(t(" "+m.busy, th.Fg))}, m.hints("Esc", tr.T("hint.cancel"))
	case m.prompt != nil:
		return m.viewPrompt()
	case m.failure != "":
		out := append(m.head(tr.T("fail.title"), ""), m.note(th.Err, m.failure, tr.T("fail.note"))...)
		return append(out, m.blank(), m.buttons(0, tr.T("pre.back"))), m.hints("Enter", tr.T("hint.confirm"))
	}
	switch m.screen {
	case scrWhere:
		return m.viewWhere()
	case scrPreflight:
		return m.viewPreflight()
	case scrDeleted:
		return m.viewDeleted()
	case scrProgress:
		return m.viewProgress()
	case scrSummary:
		return m.viewSummary()
	case scrTree:
		return m.viewTree()
	case scrSkip:
		return m.viewSkip()
	case scrTrash:
		return m.viewTrash()
	case scrSettings:
		return m.viewSettings()
	case scrAccount:
		return m.viewAccount()
	case scrDest:
		return m.viewDest()
	case scrLang:
		return m.viewLang()
	case scrTheme:
		return m.viewTheme()
	case scrWizard:
		return m.viewWizard()
	default:
		return m.viewMenu()
	}
}

// View fills the whole terminal: a top margin, the screen, then the flash
// message and the key hints pinned to the bottom.
func (m Model) View() string {
	if m.w == 0 || m.h == 0 {
		return ""
	}
	if m.w < minW || m.h < minH {
		return m.tr.T("app.small", minW, minH)
	}
	body, hints := m.body()
	bg := lipgloss.NewStyle().Background(m.th.Bg)
	left := bg.Render("  ")
	right := bg.Render(strings.Repeat(" ", max(m.w-m.cw()-2, 0)))
	empty := bg.Render(strings.Repeat(" ", m.w))
	wrap := func(s string) string { return left + s + right }

	room := max(m.h-4, 0)
	if len(body) > room {
		body = body[:room]
	}
	lines := make([]string, 0, m.h)
	lines = append(lines, empty)
	for _, l := range body {
		lines = append(lines, wrap(l))
	}
	for len(lines) < m.h-3 {
		lines = append(lines, empty)
	}
	flash := m.blank()
	if m.flash != "" {
		flash = m.line(t(m.flash, m.th.Ok))
	}
	lines = append(lines, wrap(flash), wrap(hints), empty)
	return strings.Join(lines, "\n")
}
