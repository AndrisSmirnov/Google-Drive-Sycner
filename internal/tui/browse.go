package tui

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"syncer-gdrive/internal/demo"
	"syncer-gdrive/internal/filekind"
	"syncer-gdrive/internal/format"
)

// Sort orders of a file list, in the order the choice list shows them.
const (
	byName = iota
	byType
	bySize
	byAdded // when the file went into the trash or onto the SKIP list
	byModified
	byCreated
)

var listOrders = []string{"name", "type", "size", "added", "modified", "created"}

// Groupings of a file list.
const (
	groupFolder = iota // a tree of the folders the files were in on Drive
	groupKind
	groupNone
)

var listGroups = []string{"folder", "kind", "none"}

// What is open on top of a file list.
const (
	menuNone = iota
	menuSort
	menuGroup
	menuFilter
	menuConfirm // the question before files are deleted for good
)

// alphabet orders file and folder names by the alphabet rather than by
// character code: і sits after и, not after я. Case is ignored and numbers
// go by value, so IMG_2 comes before IMG_10.
var alphabet = collate.New(language.Ukrainian, collate.IgnoreCase, collate.Numeric)

// browser is a list of files kept on the drive outside the copy — the trash
// or the SKIP list — that can be sorted, grouped, filtered and folded.
type browser struct {
	kind   string // "trash" or "skip": which texts the list uses
	items  []demo.Entry
	cursor int             // position among the rows that can be selected
	order  int             // byName … byCreated
	desc   bool            // descending: Z to A, largest or newest first
	filter int             // 0 all, then filekind.All
	group  int             // groupFolder, groupKind or groupNone
	closed map[string]bool // folded folders, by path
	menu   int             // what is open on top of the list
	choice int             // highlighted option of the open list
	doomed listRow         // what the delete question is about
}

func newBrowser(kind string, items []demo.Entry) browser {
	return browser{kind: kind, items: items, order: bySize, desc: true, group: groupFolder, closed: map[string]bool{}}
}

// descByDefault tells which way an order naturally runs: names and types
// from A, sizes from the largest, dates from the newest.
func descByDefault(order int) bool { return order >= bySize }

func (s browser) totals() (files int, bytes int64) {
	for _, it := range s.items {
		bytes += it.Size
	}
	return len(s.items), bytes
}

// date is the one the list shows: the date the files are sorted by, or the
// day the file went into the list.
func (s browser) date(it demo.Entry) string {
	switch s.order {
	case byModified:
		return it.Modified
	case byCreated:
		return it.Created
	default:
		return it.Added
	}
}

// sorted returns the files that pass the filter, in the chosen order.
func (s browser) sorted() []int {
	var idx []int
	for i, it := range s.items {
		if s.filter == 0 || filekind.Of(it.Name()) == filekind.All[s.filter-1] {
			idx = append(idx, i)
		}
	}
	slices.SortStableFunc(idx, func(a, b int) int {
		x, y := s.items[a], s.items[b]
		var c int
		switch s.order {
		case byName:
			c = alphabet.CompareString(x.Name(), y.Name())
		case byType:
			c = cmp.Compare(filekind.Ext(x.Name()), filekind.Ext(y.Name()))
		case bySize:
			c = cmp.Compare(x.Size, y.Size)
		default:
			c = cmp.Compare(s.date(x), s.date(y))
		}
		if s.desc {
			c = -c
		}
		if c == 0 {
			c = alphabet.CompareString(x.Path, y.Path)
		}
		return c
	})
	return idx
}

// listRow is one line of a file list.
type listRow struct {
	item  int    // index into items; -1 for a folder or a heading
	name  string // a file or folder name, or the kind a heading stands for
	dir   string // folder rows: the folder's path
	depth int
	head  bool  // a heading of the grouping by kind; cannot be selected
	files []int // folder and heading rows: every file under them
}

func (r listRow) folder() bool { return r.item < 0 && !r.head }

// folderNode is a folder of the tree built from the paths of the files.
type folderNode struct {
	name, path string
	sub        []*folderNode
	files      []int // directly inside, in list order
}

// all lists every file under the folder.
func (f *folderNode) all() []int {
	out := slices.Clone(f.files)
	for _, sub := range f.sub {
		out = append(out, sub.all()...)
	}
	return out
}

// compareFolders orders folders the way the files are ordered: by name, by
// what they hold in total, or by the latest date inside.
func (s browser) compareFolders(a, b *folderNode) int {
	var c int
	switch s.order {
	case byName:
		c = alphabet.CompareString(a.name, b.name)
	case byType: // folders have no type: by name, always from A
	case bySize:
		size := func(f *folderNode) (sum int64) {
			for _, i := range f.all() {
				sum += s.items[i].Size
			}
			return sum
		}
		c = cmp.Compare(size(a), size(b))
	default:
		c = cmp.Compare(s.latest(a.all()), s.latest(b.all()))
	}
	if s.desc {
		c = -c
	}
	if c == 0 {
		c = alphabet.CompareString(a.name, b.name)
	}
	return c
}

// latest is the newest shown date among the files.
func (s browser) latest(files []int) string {
	out := ""
	for _, i := range files {
		if d := s.date(s.items[i]); d > out {
			out = d
		}
	}
	return out
}

// rows lists what the list shows, top to bottom: a tree of folders, kinds
// under their headings, or the bare files.
func (s browser) rows() []listRow {
	files := s.sorted()
	var out []listRow
	switch s.group {
	case groupNone:
		for _, i := range files {
			out = append(out, listRow{item: i, name: s.items[i].Name()})
		}
	case groupKind:
		for _, kind := range filekind.All {
			var of []int
			for _, i := range files {
				if filekind.Of(s.items[i].Name()) == kind {
					of = append(of, i)
				}
			}
			if len(of) == 0 {
				continue
			}
			out = append(out, listRow{item: -1, name: kind, head: true, files: of})
			for _, i := range of {
				out = append(out, listRow{item: i, name: s.items[i].Name()})
			}
		}
	default:
		root := &folderNode{}
		for _, i := range files {
			at := root
			if dir := s.items[i].Dir(); dir != "" {
				path := ""
				for _, part := range strings.Split(dir, "/") {
					path = strings.TrimPrefix(path+"/"+part, "/")
					n := slices.IndexFunc(at.sub, func(f *folderNode) bool { return f.name == part })
					if n < 0 {
						at.sub = append(at.sub, &folderNode{name: part, path: path})
						n = len(at.sub) - 1
					}
					at = at.sub[n]
				}
			}
			at.files = append(at.files, i)
		}
		var walk func(f *folderNode, depth int)
		walk = func(f *folderNode, depth int) {
			slices.SortStableFunc(f.sub, s.compareFolders)
			for _, sub := range f.sub {
				out = append(out, listRow{item: -1, name: sub.name, dir: sub.path, depth: depth, files: sub.all()})
				if !s.closed[sub.path] {
					walk(sub, depth+1)
				}
			}
			for _, i := range f.files {
				out = append(out, listRow{item: i, name: s.items[i].Name(), depth: depth})
			}
		}
		walk(root, 0)
	}
	return out
}

// selectable lists the rows the cursor can stand on.
func (s browser) selectable() []listRow {
	var out []listRow
	for _, r := range s.rows() {
		if !r.head {
			out = append(out, r)
		}
	}
	return out
}

// visible returns the files on screen, top to bottom.
func (s browser) visible() []int {
	var out []int
	for _, r := range s.rows() {
		if r.item >= 0 {
			out = append(out, r.item)
		}
	}
	return out
}

// current is the row under the cursor.
func (s browser) current() (listRow, bool) {
	rows := s.selectable()
	if s.cursor < 0 || s.cursor >= len(rows) {
		return listRow{}, false
	}
	return rows[s.cursor], true
}

// targets lists the files a row stands for: itself, or all inside a folder.
func (r listRow) targets() []int {
	if r.item >= 0 {
		return []int{r.item}
	}
	return r.files
}

// remove takes files out of the list.
func (s *browser) remove(files []int) {
	var kept []demo.Entry
	for i, it := range s.items {
		if !slices.Contains(files, i) {
			kept = append(kept, it)
		}
	}
	s.items = kept
}

// describe names what a row stands for in a message: the path of a file, or
// the folder with the number of files in it.
func (m Model) describe(s browser, r listRow) string {
	if r.item >= 0 {
		return s.items[r.item].Path
	}
	return r.dir + " · " + m.tr.N("n.files", len(r.files))
}

// open shows a choice list with the current value highlighted.
func (s *browser) open(menu int) {
	s.menu = menu
	switch menu {
	case menuSort:
		s.choice = s.order
	case menuGroup:
		s.choice = s.group
	case menuFilter:
		s.choice = s.filter
	case menuConfirm:
		s.choice = 0 // "no" comes first
	}
}

// options is how many options the open list has. The sort list ends with
// the two directions.
func (s browser) options() int {
	switch s.menu {
	case menuSort:
		return len(listOrders) + 2
	case menuGroup:
		return len(listGroups)
	case menuConfirm:
		return 2
	default:
		return len(filekind.All) + 1
	}
}

// choose applies the highlighted option and closes the list. It reports
// whether the delete question was answered with yes.
func (s *browser) choose() (confirmed bool) {
	switch s.menu {
	case menuSort:
		switch {
		case s.choice >= len(listOrders):
			s.desc = s.choice == len(listOrders)+1
		case s.choice != s.order:
			s.order, s.desc = s.choice, descByDefault(s.choice)
		}
	case menuGroup:
		s.group = s.choice
	case menuFilter:
		s.filter = s.choice
	case menuConfirm:
		confirmed = s.choice == 1
	}
	s.menu = menuNone
	return confirmed
}

// clamp keeps the cursor on a row after the list has changed.
func (s *browser) clamp() {
	s.cursor = max(min(s.cursor, len(s.selectable())-1), 0)
}

// key handles what every file list shares: the open choice list, moving,
// folding and the keys that open the lists. It reports whether the key was
// used, and whether a delete question was just answered with yes.
func (s *browser) key(key string) (used, confirmed bool) {
	defer s.clamp()
	if s.menu != menuNone {
		switch key {
		case "up", "k", "down", "j":
			s.choice = move(s.choice, s.options(), key)
		case "enter", " ":
			confirmed = s.choose()
		case "esc":
			s.menu = menuNone
		default:
			switch {
			case s.menu != menuConfirm && s.openMenu(key):
			case len(key) == 1 && key[0] >= '1' && int(key[0]-'0') <= s.options():
				s.choice = int(key[0] - '1')
				confirmed = s.choose()
			}
		}
		return true, confirmed
	}
	switch key {
	case "up", "k", "down", "j":
		s.cursor = move(s.cursor, len(s.selectable()), key)
	case "left", "h", "right", "l":
		if row, ok := s.current(); ok && row.folder() {
			s.closed[row.dir] = key == "left" || key == "h"
		}
	default:
		return s.openMenu(key), false
	}
	return true, false
}

// openMenu opens the choice list that belongs to the key, if there is one.
func (s *browser) openMenu(key string) bool {
	switch key {
	case "s":
		s.open(menuSort)
	case "g":
		s.open(menuGroup)
	case "f":
		s.open(menuFilter)
	default:
		return false
	}
	return true
}

// option is one line of a choice list.
type option struct {
	label string
	info  string
	on    bool   // the option is in effect
	head  string // a line above the option, where a new set of options starts
}

// listOptions describes what is open on top of a file list.
func (m Model) listOptions(s browser) (title string, opts []option) {
	tr := m.tr
	switch s.menu {
	case menuSort:
		for i, key := range listOrders {
			if key == "added" {
				key += "." + s.kind
			}
			opts = append(opts, option{label: tr.T("sort." + key + ".long"), on: i == s.order})
		}
		// The directions are worded for the order in effect.
		class := "date"
		switch s.order {
		case byName, byType:
			class = "alpha"
		case bySize:
			class = "size"
		}
		opts = append(opts,
			option{label: "↑ " + tr.T("dir."+class+".asc"), on: !s.desc, head: tr.T("list.menu.dir")},
			option{label: "↓ " + tr.T("dir."+class+".desc"), on: s.desc})
		return tr.T("list.menu.sort"), opts
	case menuGroup:
		for i, key := range listGroups {
			opts = append(opts, option{label: tr.T("group." + key + ".long"), on: i == s.group})
		}
		return tr.T("list.menu.group"), opts
	case menuConfirm:
		var bytes int64
		for _, i := range s.doomed.targets() {
			bytes += s.items[i].Size
		}
		return tr.T("trash.confirm"), []option{
			{label: tr.T("trash.confirm.no"), head: m.describe(s, s.doomed) + " · " + format.Size(bytes)},
			{label: tr.T("trash.confirm.yes")},
		}
	default:
		size := func(kind string) string {
			n, sum := 0, int64(0)
			for _, it := range s.items {
				if kind == "" || filekind.Of(it.Name()) == kind {
					n++
					sum += it.Size
				}
			}
			return tr.N("n.files", n) + " · " + format.Size(sum)
		}
		opts = []option{{label: tr.T("kind.all.long"), info: size(""), on: s.filter == 0}}
		for i, kind := range filekind.All {
			opts = append(opts, option{label: tr.T("kind." + kind + ".long"), info: size(kind), on: s.filter == i+1})
		}
		return tr.T("list.menu.filter"), opts
	}
}

// choices renders an open choice list as a panel: a title, then one numbered
// row per option, the highlighted one filled. With marks on, the options in
// effect carry a mark; a yes-or-no question has none.
func (m Model) choices(title string, opts []option, cursor, room int, marks bool) []string {
	th := m.th
	width := min(m.cw(), 68)
	// The right-hand column exists only where there is something to put in it.
	infoW := 0
	if width >= 60 && slices.ContainsFunc(opts, func(o option) bool { return o.info != "" }) {
		infoW = 20
	}
	labelW := width - 3 - infoW - 1
	var rows []string
	focus := 0
	for i, o := range opts {
		if o.head != "" {
			fg := th.Dim
			if !marks {
				fg = th.Fg // what the question is about
			}
			rows = append(rows, m.line(t("   "+o.head, fg).w(width).on(th.Panel)))
		}
		mark := ""
		if marks {
			mark = "( ) "
			if o.on {
				mark = "(•) "
			}
		}
		label := strconv.Itoa(i+1) + "  " + mark + o.label
		var bg lipgloss.TerminalColor = th.Panel
		lead, text, info := t("   ", th.Fg), t(label, th.Fg), t(o.info, th.Dim)
		if i == cursor {
			bg = th.Accent
			lead, text, info = t(" › ", th.On).b(), t(label, th.On).b(), t(o.info, th.On)
			focus = len(rows)
		}
		cells := []cell{lead.on(bg), text.w(labelW).on(bg)}
		if infoW > 0 {
			cells = append(cells, info.w(infoW).r().on(bg))
		}
		rows = append(rows, m.line(append(cells, t(" ", th.Fg).on(bg))...))
	}
	first, last := window(focus, len(rows), room)
	head := m.line(t(" "+title, th.Accent).b().w(width).on(th.Panel))
	return append([]string{head}, rows[first:last]...)
}

// viewList draws a file list: the header with its three controls, the open
// choice list if any, then the rows.
func (m Model) viewList(s browser) []string {
	th, tr := m.th, m.tr
	files, bytes := s.totals()

	// The last column holds the file count of a folder and, in the trash,
	// the reason a file is there. It is as wide as its longest text.
	noteW := runewidth.StringWidth(tr.N("n.files", files))
	if s.kind == "trash" {
		for _, key := range []string{"col.reason", "reason.drive", "reason.local", "reason.unselected"} {
			noteW = max(noteW, runewidth.StringWidth(tr.T(key)))
		}
	}
	noteW += 2

	arrow := " ↑"
	if s.desc {
		arrow = " ↓"
	}
	order := listOrders[s.order]
	if order == "added" {
		order += "." + s.kind
	}
	filter := tr.T("kind.all")
	if s.filter > 0 {
		filter = tr.T("kind." + filekind.All[s.filter-1])
	}
	// Each control shows its key and value; the one whose list is open stands out.
	control := func(key, label, value string, menu int) []cell {
		v := t(value, th.Fg)
		if s.menu == menu {
			v = t(value, th.Accent).b()
		}
		return []cell{t(key, th.Second), t(" "+label+" ", th.Dim), v}
	}
	controls := control("s", tr.T("list.sort"), tr.T("sort."+order)+arrow, menuSort)
	controls = append(controls, t(" · ", th.Dim))
	controls = append(controls, control("g", tr.T("list.group"), tr.T("group."+listGroups[s.group]), menuGroup)...)
	controls = append(controls, t(" · ", th.Dim))
	controls = append(controls, control("f", tr.T("list.filter"), filter, menuFilter)...)
	// The totals give way when the title and the controls leave no room for them.
	title := tr.T(s.kind + ".title")
	totals := " · " + tr.N("n.files", files) + " · " + format.Size(bytes)
	taken := runewidth.StringWidth(title+totals) + 2
	for _, c := range controls {
		taken += runewidth.StringWidth(c.text)
	}
	header := []cell{t(title, th.Accent).b()}
	if taken <= m.cw() {
		header = append(header, t(totals, th.Dim))
	}
	header = append(header, m.gap(), t("  ", th.Dim))
	out := []string{m.line(append(header, controls...)...), m.rule()}

	if s.menu != menuNone {
		name, opts := m.listOptions(s)
		out = append(out, m.choices(name, opts, s.choice, m.h-7, s.menu != menuConfirm)...)
		out = append(out, m.blank())
	}

	rows := s.rows()
	if len(rows) == 0 {
		return append(out, m.line(t(" "+tr.T(s.kind+".empty"), th.Dim)))
	}

	// The date column follows the sort order.
	dateCol := "col.added." + s.kind
	switch s.order {
	case byModified:
		dateCol = "col.modified"
	case byCreated:
		dateCol = "col.created"
	}
	noteCol := ""
	if s.kind == "trash" {
		noteCol = tr.T("col.reason")
	}
	out = append(out, m.line(t("", th.Dim).w(3), t(tr.T("col.name"), th.Dim), m.gap(),
		t(tr.T("col.size"), th.Dim).w(9).r(), t(tr.T(dateCol), th.Dim).w(13).r(),
		t("  "+noteCol, th.Dim).w(noteW)))

	var lines []string
	focus, pos := 0, 0
	for _, r := range rows {
		var size int64
		for _, i := range r.targets() {
			size += s.items[i].Size
		}
		if r.head {
			lines = append(lines, m.line(t(" "+tr.T("kind."+r.name), th.Second).b(),
				t(" · "+tr.N("n.files", len(r.files))+" · "+format.Size(size), th.Dim)))
			continue
		}
		active := pos == s.cursor && s.menu == menuNone
		if pos == s.cursor {
			focus = len(lines)
		}
		pos++

		indent := strings.Repeat("  ", r.depth)
		var name cell
		var rest []cell
		switch {
		case r.folder():
			fold := "▾ "
			if s.closed[r.dir] {
				fold = "▸ "
			}
			name = t(indent+fold+r.name, th.Second).b()
			rest = []cell{m.gap(), t(format.Size(size), th.Fg).w(9).r(), t(s.latest(r.files), th.Dim).w(13).r(),
				t("  "+tr.N("n.files", len(r.files)), th.Dim).w(noteW)}
		default:
			it := s.items[r.item]
			name = t(r.name, th.Fg)
			if s.group == groupFolder {
				name = t(indent+"  "+r.name, th.Fg) // in line with the folder names
			} else if it.Dir() != "" {
				// Outside the tree every row says where the file was: two
				// files may share a name.
				rest = append(rest, t("  "+it.Dir(), th.Dim))
			}
			note := t("", th.Dim).w(noteW)
			switch it.Reason {
			case "drive":
				note = t("  "+tr.T("reason.drive"), th.Dim).w(noteW)
			case "local":
				note = t("  "+tr.T("reason.local"), th.Warn).w(noteW)
			case "unselected":
				note = t("  "+tr.T("reason.unselected"), th.Dim).w(noteW)
			}
			rest = append(rest, m.gap(), t(format.Size(it.Size), th.Fg).w(9).r(), t(s.date(it), th.Dim).w(13).r(), note)
		}
		if active {
			name = t(name.text, th.Accent).b()
			lines = append(lines, m.sel(append([]cell{t(" › ", th.Accent).b(), name}, rest...)...))
		} else {
			lines = append(lines, m.line(append([]cell{t("   ", th.Fg), name}, rest...)...))
		}
	}
	first, last := window(focus, len(lines), m.h-7)
	return append(out, lines[first:last]...)
}

// listHints are the key hints of a file list: the screen's own keys, or
// those of the open choice list.
func (m Model) listHints(s browser, own ...string) string {
	tr := m.tr
	if s.menu != menuNone {
		return m.hints("↑↓", tr.T("hint.move"), "Enter", tr.T("hint.choose"),
			"1–"+strconv.Itoa(s.options()), tr.T("hint.direct"), "Esc", tr.T("hint.cancel"))
	}
	if s.group == groupFolder {
		own = append(own, "← →", tr.T("hint.fold"))
	}
	return m.hints(append(own, "Esc", tr.T("hint.back"))...)
}
