package tui

import (
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// prompt is a question answered by typing: a path or a password. While one
// is open it takes the place of the screen.
type prompt struct {
	title, text, label string
	value              []rune
	secret             bool   // show dots instead of what is typed
	err                string // why the last answer was not accepted
	// done receives the answer. To close the prompt it clears m.prompt; to
	// keep it open it calls m.refuse with the reason.
	done func(m Model, value string) (Model, tea.Cmd)
}

// ask opens a prompt.
func (m Model) ask(p prompt) Model {
	m.prompt = &p
	return m
}

// refuse keeps the prompt open and says why the answer did not do.
func (m Model) refuse(why string) Model {
	p := *m.prompt
	p.err = why
	if p.secret {
		p.value = nil
	}
	m.prompt = &p
	return m
}

func (m Model) viewPrompt() ([]string, string) {
	th, tr, p := m.th, m.tr, m.prompt
	out := m.head(p.title, "")
	out = append(out, m.para(p.text, th.Fg, m.cw())...)
	shown := string(p.value)
	if p.secret {
		shown = strings.Repeat("•", len(p.value))
	}
	out = append(out, m.blank(), m.line(t(" "+p.label, th.Dim)), m.sel(t(" "+shown, th.Fg), t("▏", th.Accent)), m.blank())
	if p.err != "" {
		out = append(out, m.para(p.err, th.Err, m.cw())...)
	}
	return out, m.hints("Enter", tr.T("hint.confirm"), "Esc", tr.T("hint.cancel"))
}

func (m Model) keyPrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := *m.prompt // the model is copied on every update: never share the prompt
	switch msg.Type {
	case tea.KeyEsc:
		m.prompt = nil
		return m, nil
	case tea.KeyEnter:
		value := string(p.value)
		if !p.secret {
			value = strings.TrimSpace(value)
		}
		if value == "" {
			return m, nil
		}
		return p.done(m, value)
	case tea.KeyBackspace:
		if len(p.value) > 0 {
			p.value = p.value[:len(p.value)-1]
		}
	case tea.KeyCtrlU:
		p.value = nil
	case tea.KeyRunes, tea.KeySpace:
		runes := msg.Runes
		if msg.Type == tea.KeySpace && len(runes) == 0 {
			runes = []rune{' '}
		}
		for _, r := range runes {
			if r >= 0x20 && r != 0x7f { // pasted text may bring line ends along
				p.value = append(p.value, r)
			}
		}
	}
	p.err = ""
	m.prompt = &p
	return m, nil
}

// cleanPath tidies a path the way it tends to arrive: typed with ~, or
// dragged into the terminal, which quotes it or escapes its spaces.
func cleanPath(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '\'' || s[0] == '"') && s[len(s)-1] == s[0] {
		s = s[1 : len(s)-1]
	} else if filepath.Separator == '/' {
		s = strings.ReplaceAll(s, `\ `, " ")
	}
	if s == "~" || strings.HasPrefix(s, "~/") || strings.HasPrefix(s, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			s = filepath.Join(home, s[1:])
		}
	}
	if s == "" {
		return ""
	}
	return filepath.Clean(s)
}
