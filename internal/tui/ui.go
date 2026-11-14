package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// cell is one styled piece of a row.
type cell struct {
	text  string
	fg    lipgloss.TerminalColor
	bg    lipgloss.TerminalColor // nil: the row background
	bold  bool
	width int // >0 fixed, 0 as wide as the text, -1 takes the space left over
	right bool
}

func t(text string, fg lipgloss.TerminalColor) cell { return cell{text: text, fg: fg} }

func (c cell) b() cell                           { c.bold = true; return c }
func (c cell) w(n int) cell                      { c.width = n; return c }
func (c cell) r() cell                           { c.right = true; return c }
func (c cell) flex() cell                        { c.width = -1; return c }
func (c cell) on(bg lipgloss.TerminalColor) cell { c.bg = bg; return c }

// renderRow lays cells out left to right and pads the row to width. Every
// piece carries the background itself, so a painted background has no holes.
func renderRow(bg lipgloss.TerminalColor, width int, cells []cell) string {
	taken, longest, shrink := 0, 0, -1
	for i, c := range cells {
		switch {
		case c.width > 0:
			taken += c.width
		case c.width == 0:
			w := runewidth.StringWidth(c.text)
			taken += w
			if w > longest {
				longest, shrink = w, i
			}
		}
	}
	rest := max(width-taken, 0)
	// A row that does not fit shortens its longest text; if that is not
	// enough, whatever runs past the edge is cut.
	over := taken - width
	if over <= 0 || longest <= over {
		shrink = -1
	}

	var out strings.Builder
	left := width
	for i, c := range cells {
		text := c.text
		textW := runewidth.StringWidth(text)
		cellW := c.width
		switch {
		case i == shrink:
			cellW = textW - over
		case cellW == 0:
			cellW = textW
		case cellW < 0:
			cellW, rest = rest, 0
		}
		cellW = min(cellW, left)
		if cellW <= 0 {
			continue
		}
		if textW > cellW {
			text = runewidth.Truncate(text, cellW, "…")
			textW = runewidth.StringWidth(text)
		}
		pad := strings.Repeat(" ", cellW-textW)
		if c.right {
			text = pad + text
		} else {
			text += pad
		}
		cellBg := bg
		if c.bg != nil {
			cellBg = c.bg
		}
		style := lipgloss.NewStyle().Foreground(c.fg).Background(cellBg).Bold(c.bold)
		out.WriteString(style.Render(text))
		left -= cellW
	}
	if left > 0 {
		out.WriteString(lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", left)))
	}
	return out.String()
}

// The interface needs at least this much room.
const minW, minH = 40, 10

// cw is the width of the content column.
func (m Model) cw() int {
	return min(m.w-4, 100)
}

func (m Model) line(cells ...cell) string { return renderRow(m.th.Bg, m.cw(), cells) }
func (m Model) sel(cells ...cell) string  { return renderRow(m.th.Panel, m.cw(), cells) }
func (m Model) blank() string             { return renderRow(m.th.Bg, m.cw(), nil) }
func (m Model) gap() cell                 { return t("", m.th.Dim).flex() }

func (m Model) rule() string {
	return m.line(t(strings.Repeat("─", m.cw()), m.th.Line))
}

// head is the screen title with a note on the right and a rule below.
func (m Model) head(title, note string) []string {
	return []string{
		m.line(t(title, m.th.Accent).b(), m.gap(), t(note, m.th.Dim)),
		m.rule(),
	}
}

// pick renders a menu row: highlighted with a marker when active.
func (m Model) pick(active bool, label string, rest ...cell) string {
	if active {
		cells := append([]cell{t(" › ", m.th.Accent).b(), t(label, m.th.Accent).b()}, rest...)
		return m.sel(cells...)
	}
	cells := append([]cell{t("   ", m.th.Fg), t(label, m.th.Fg)}, rest...)
	return m.line(cells...)
}

// para wraps text to the given width.
func (m Model) para(text string, fg lipgloss.TerminalColor, width int) []string {
	style := lipgloss.NewStyle().Foreground(fg).Background(m.th.Bg).Width(width)
	return strings.Split(style.Render(text), "\n")
}

// box draws a rounded border around lines that are cw()-4 wide.
func (m Model) box(border lipgloss.TerminalColor, inner []string) []string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		BorderBackground(m.th.Bg).
		Background(m.th.Bg).
		Padding(0, 1)
	return strings.Split(style.Render(strings.Join(inner, "\n")), "\n")
}

// note is a bordered message: a statement and a quieter explanation.
func (m Model) note(border lipgloss.TerminalColor, text, detail string) []string {
	inner := m.para(text, m.th.Fg, m.cw()-4)
	inner = append(inner, m.para(detail, m.th.Dim, m.cw()-4)...)
	return m.box(border, inner)
}

// buttons renders a row of buttons with the active one filled.
func (m Model) buttons(active int, labels ...string) string {
	cells := []cell{t(" ", m.th.Fg)}
	for i, label := range labels {
		if i == active {
			cells = append(cells, t("  "+label+"  ", m.th.On).on(m.th.Accent).b())
		} else {
			cells = append(cells, t("  "+label+"  ", m.th.Fg).on(m.th.Panel))
		}
		cells = append(cells, t("  ", m.th.Fg))
	}
	return m.line(cells...)
}

// hints renders "key what · key what" from alternating key and description.
func (m Model) hints(pairs ...string) string {
	var cells []cell
	for i := 0; i+1 < len(pairs); i += 2 {
		if i > 0 {
			cells = append(cells, t(" · ", m.th.Dim))
		}
		cells = append(cells, t(pairs[i], m.th.Second), t(" "+pairs[i+1], m.th.Dim))
	}
	return m.line(cells...)
}

// move shifts a list cursor with the arrow keys or j/k.
func move(idx, n int, key string) int {
	switch key {
	case "up", "k":
		if idx > 0 {
			idx--
		}
	case "down", "j":
		if idx < n-1 {
			idx++
		}
	}
	return idx
}

// window picks which rows of a long list to show so the focused one is visible.
func window(focus, n, size int) (first, last int) {
	size = max(size, 3)
	if n <= size {
		return 0, n
	}
	first = min(max(focus-size/2, 0), n-size)
	return first, first + size
}

func baseName(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}
