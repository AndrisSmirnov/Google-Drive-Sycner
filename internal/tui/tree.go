package tui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"syncer-gdrive/internal/demo"
	"syncer-gdrive/internal/format"
	"syncer-gdrive/internal/names"
)

type treeState struct {
	root   []*demo.Node
	cursor int
	// newTop is the switch of the top level: whether folders and files
	// shared with the account later are synced while the selection is partial.
	newTop bool
	saved  treeSaved // the choice when the screen was opened
}

// treeSaved is what Esc brings back.
type treeSaved struct {
	checked map[*demo.Node]bool
	takeNew map[*demo.Node]bool
	newTop  bool
}

// treeRow is a file, a folder, or the switch "new files and folders here"
// of a folder selected in part.
type treeRow struct {
	node  *demo.Node // nil for a switch
	owner *demo.Node // the folder a switch belongs to
	top   bool       // the switch of the top level
	depth int
}

func (r treeRow) isSwitch() bool { return r.node == nil }

func newTree() treeState {
	s := treeState{root: demo.Tree(), newTop: true}
	// The cursor starts on the first file or folder, not on a switch.
	for i, r := range s.rows() {
		if !r.isSwitch() {
			s.cursor = i
			break
		}
	}
	return s
}

// topState tells whether all, some or none of everything shared is selected.
func (s treeState) topState() int {
	all, none := true, true
	for _, n := range s.root {
		switch n.State() {
		case demo.All:
			none = false
		case demo.None:
			all = false
		default:
			all, none = false, false
		}
	}
	switch {
	case none:
		return demo.None
	case all:
		return demo.All
	default:
		return demo.Partial
	}
}

// rows lists what is currently visible, top to bottom. A folder selected in
// part shows its switch as the first line inside it.
func (s treeState) rows() []treeRow {
	var out []treeRow
	if s.topState() == demo.Partial {
		out = append(out, treeRow{top: true})
	}
	var walk func(nodes []*demo.Node, depth int)
	walk = func(nodes []*demo.Node, depth int) {
		for _, n := range nodes {
			out = append(out, treeRow{node: n, depth: depth})
			if n.Dir && n.Open {
				if n.State() == demo.Partial {
					out = append(out, treeRow{owner: n, depth: depth + 1})
				}
				walk(n.Children, depth+1)
			}
		}
	}
	walk(s.root, 0)
	return out
}

// keep puts the cursor back on a row after rows have appeared or gone above
// it. If the row itself is gone, the cursor stays within the list.
func (s *treeState) keep(row treeRow) {
	rows := s.rows()
	for i, r := range rows {
		if r.node == row.node && r.owner == row.owner && r.top == row.top {
			s.cursor = i
			return
		}
	}
	s.cursor = max(min(s.cursor, len(rows)-1), 0)
}

func (s treeState) selected() (files int, bytes int64) {
	for _, n := range s.root {
		f, b := n.Selected()
		files += f
		bytes += b
	}
	return files, bytes
}

// takesNew tells whether a file that has just appeared on Drive is synced.
// The folder it is in decides: selected as a whole — yes; in part — as its
// switch says; not at all — no. For a folder that is new itself, the nearest
// known folder above it decides.
func (s treeState) takesNew(path string) bool {
	nodes, state, takeNew := s.root, s.topState(), s.newTop
	parts := strings.Split(path, "/")
	for _, part := range parts[:len(parts)-1] {
		var next *demo.Node
		for _, n := range nodes {
			if n.Dir && n.Name == part {
				next = n
				break
			}
		}
		if next == nil {
			break
		}
		nodes, state, takeNew = next.Children, next.State(), next.New
	}
	switch state {
	case demo.All:
		return true
	case demo.Partial:
		return takeNew
	default:
		return false
	}
}

// clashes finds selected files that share a name with another selected file
// in the same folder. Drive allows that; the file system of a drive does
// not, and most of them ignore case too. So every such file after the oldest
// gets a number in its name there. The result holds, for each file involved,
// the name it will have on the drive.
func (s treeState) clashes() map[*demo.Node]string {
	out := map[*demo.Node]string{}
	var walk func(nodes []*demo.Node)
	walk = func(nodes []*demo.Node) {
		same := map[string][]*demo.Node{}
		for _, n := range nodes {
			switch {
			case n.Dir:
				walk(n.Children)
			case n.Checked:
				key := strings.ToLower(n.Name)
				same[key] = append(same[key], n)
			}
		}
		for _, group := range same {
			if len(group) < 2 {
				continue
			}
			sort.SliceStable(group, func(a, b int) bool { return group[a].Modified < group[b].Modified })
			for i, n := range group {
				out[n] = names.Numbered(n.Name, i+1)
			}
		}
	}
	walk(s.root)
	return out
}

func eachNode(nodes []*demo.Node, fn func(*demo.Node)) {
	for _, n := range nodes {
		fn(n)
		eachNode(n.Children, fn)
	}
}

// remember stores the choice so Esc can bring it back.
func (s *treeState) remember() {
	s.saved = treeSaved{checked: map[*demo.Node]bool{}, takeNew: map[*demo.Node]bool{}, newTop: s.newTop}
	eachNode(s.root, func(n *demo.Node) {
		if n.Dir {
			s.saved.takeNew[n] = n.New
		} else {
			s.saved.checked[n] = n.Checked
		}
	})
}

func (s *treeState) revert() {
	eachNode(s.root, func(n *demo.Node) {
		if was, ok := s.saved.checked[n]; ok {
			n.Checked = was
		}
		if was, ok := s.saved.takeNew[n]; ok {
			n.New = was
		}
	})
	s.newTop = s.saved.newTop
}

func (m Model) viewTree() ([]string, string) {
	th, tr := m.th, m.tr
	out := m.head(tr.T("tree.title"), "Drive → "+m.destination().Path)
	out = append(out, m.line(
		t("", th.Dim).w(5), t(tr.T("col.name"), th.Dim), m.gap(),
		t(tr.T("col.size"), th.Dim).w(9).r(), t(tr.T("col.modified"), th.Dim).w(12).r(), t(" ", th.Dim),
	))

	rows := m.tree.rows()
	clash := m.tree.clashes()
	room := m.h - 10
	if len(clash) > 0 {
		room-- // the warning below the list takes a line
	}
	first, last := window(m.tree.cursor, len(rows), room)
	for i := first; i < last; i++ {
		r := rows[i]
		active := i == m.tree.cursor
		indent := strings.Repeat("  ", r.depth)

		var cells []cell
		if r.isSwitch() {
			on, label := m.tree.newTop, tr.T("tree.new.top")
			if !r.top {
				on, label = r.owner.New, tr.T("tree.new.here")
			}
			box, boxFg := "[ ]", th.Dim
			if on {
				box, boxFg = "[x]", th.Accent
			}
			name := t(indent+"+ "+label, th.Dim)
			if active {
				name = t(name.text, th.Accent).b()
			}
			cells = []cell{t(" "+box+" ", boxFg), name}
		} else {
			n := r.node
			box, boxFg := "[ ]", th.Dim
			switch n.State() {
			case demo.All:
				box, boxFg = "[x]", th.Accent
			case demo.Partial:
				box, boxFg = "[-]", th.Second
			}
			name := t(indent+"  "+n.Name, th.Fg)
			switch {
			case n.Dir && n.Open:
				name = t(indent+"▾ "+n.Name, th.Second).b()
			case n.Dir:
				name = t(indent+"▸ "+n.Name, th.Second).b()
			}
			if active {
				name = t(name.text, th.Accent).b()
			}
			size := format.Size(n.Total())
			cells = []cell{t(" "+box+" ", boxFg), name}
			if n.Export != "" {
				size = "~" + size
				cells = append(cells, t(" · "+n.Export, th.Dim))
			}
			if saved, ok := clash[n]; ok {
				text := tr.T("tree.clash")
				if saved != n.Name {
					text = tr.T("tree.clash.as", saved)
				}
				cells = append(cells, t("  ≈ "+text, th.Warn))
			}
			cells = append(cells, m.gap(),
				t(size, th.Fg).w(9).r(), t(n.LastModified(), th.Dim).w(12).r(), t(" ", th.Dim))
		}
		if active {
			out = append(out, m.sel(cells...))
		} else {
			out = append(out, m.line(cells...))
		}
	}

	files, bytes := m.tree.selected()
	free := m.destination().Free
	fit := t("✓ "+tr.T("tree.fits")+" ", th.Ok)
	if bytes > free {
		fit = t("× "+tr.T("tree.nofit")+" ", th.Err)
	}
	if len(clash) > 0 {
		out = append(out, m.line(t(" ! "+tr.T("tree.clash.note", tr.N("n.files", len(clash))), th.Warn)))
	}
	out = append(out, m.rule(), m.line(
		t(" "+tr.T("tree.selected")+" ", th.Dim), t(tr.N("n.files", files)+" · ", th.Fg), t(format.Size(bytes), th.Fg).b(),
		m.gap(), t(tr.T("tree.free")+" ", th.Dim), t(format.Size(free)+" ", th.Fg), fit,
	))
	return out, m.hints("↑↓", tr.T("hint.move"), "Space", tr.T("hint.toggle"), "← →", tr.T("hint.fold"),
		"Enter", tr.T("hint.save"), "Esc", tr.T("hint.cancel"))
}

func (m Model) keyTree(key string) (tea.Model, tea.Cmd) {
	rows := m.tree.rows()
	cur := rows[m.tree.cursor]
	switch key {
	case "up", "k", "down", "j":
		m.tree.cursor = move(m.tree.cursor, len(rows), key)
		return m, nil
	case " ":
		switch {
		case cur.top:
			m.tree.newTop = !m.tree.newTop
		case cur.isSwitch():
			cur.owner.New = !cur.owner.New
		default:
			cur.node.SetChecked(cur.node.State() != demo.All)
		}
	case "right", "l":
		if !cur.isSwitch() && cur.node.Dir {
			cur.node.Open = true
		}
	case "left", "h":
		if !cur.isSwitch() && cur.node.Dir {
			cur.node.Open = false
		}
	case "enter":
		files, bytes := m.tree.selected()
		m.flash = m.tr.T("tree.saved", m.tr.N("n.files", files), format.Size(bytes))
		m.screen = scrMenu
	case "esc":
		m.tree.revert()
		m.screen = scrMenu
	}
	// A switch may have appeared or gone above the cursor.
	m.tree.keep(cur)
	return m, nil
}
