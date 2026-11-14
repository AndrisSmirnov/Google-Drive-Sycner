package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"

	"syncer-gdrive/internal/demo"
	"syncer-gdrive/internal/filekind"
	"syncer-gdrive/internal/theme"
)

const testW, testH = 100, 30

// previews records what the tests asked to preview.
var previews []string

// Tests never open a real preview window and never touch the clipboard.
func init() {
	stub := func(name string) (*viewer, error) {
		previews = append(previews, name)
		open := true
		return &viewer{close: func() { open = false }, running: func() bool { return open }}, nil
	}
	openPreview = func(name, text string) (*viewer, error) { return stub(name) }
	openFile = func(file string) (*viewer, error) { return stub(file) }
	copyToClipboard = func(text string) { copied = text }
}

// copied is what the tests put on the clipboard.
var copied string

func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	}
}

func start(t *testing.T, o Options) Model {
	t.Helper()
	m, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: testW, Height: testH})
	return next.(Model)
}

// press sends keys one by one and checks the frame after each.
func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		next, _ := m.Update(keyMsg(k))
		m = next.(Model)
		checkFrame(t, m, k)
	}
	return m
}

// checkFrame makes sure the view fills the terminal exactly.
func checkFrame(t *testing.T, m Model, after string) {
	t.Helper()
	lines := strings.Split(m.View(), "\n")
	if len(lines) != testH {
		t.Fatalf("screen %d after %q: %d lines, want %d", m.screen, after, len(lines), testH)
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w != testW {
			t.Fatalf("screen %d after %q: line %d is %d wide, want %d: %q", m.screen, after, i, w, testW, l)
		}
	}
}

func wantScreen(t *testing.T, m Model, want screen) {
	t.Helper()
	if m.screen != want {
		t.Fatalf("screen = %d, want %d", m.screen, want)
	}
}

func TestEveryScreenFillsTheTerminal(t *testing.T) {
	for _, lang := range langs() {
		for _, theme := range []string{"dark", "light"} {
			m := start(t, Options{Lang: lang, Theme: theme})
			checkFrame(t, m, "start")

			m = press(t, m, "1", "down", "down", " ", "right", "left", "down", " ", "enter")
			wantScreen(t, m, scrMenu)
			m = press(t, m, "3", "down", "s", "2", "s", "8", "g", "2", "g", "3", "g", "1", "f", "1", "left", "right", " ", "r", "esc")
			m = press(t, m, "4", " ", "down", " ", "s", "down", "enter", "s", "7", "g", "2", "g", "3", "g", "1",
				"f", "down", "down", "down", "down", "enter", "f", "1", "left", "right", "enter", "d", "esc", "d", "2", "esc")
			m = press(t, m, "5", "enter", "down", "enter", "esc")
			m = press(t, m, "down", "enter", "enter", "esc")
			m = press(t, m, "down", "enter", "down", "down", "enter", "esc")
			m = press(t, m, "down", "enter", "down", "enter", "down", "down", "enter", "enter", "esc", "esc")
			wantScreen(t, m, scrMenu)

			if missing := m.tr.Missing(); len(missing) > 0 {
				t.Errorf("%s: texts missing from the catalog: %v", lang, missing)
			}
		}
	}
}

func TestSyncRunReachesTheSummary(t *testing.T) {
	m := start(t, Options{Lang: "uk"})
	skipBefore, trashBefore := len(m.skip.items), len(m.trash.items)
	m = press(t, m, "2")
	wantScreen(t, m, scrWhere)
	m = press(t, m, "enter")
	wantScreen(t, m, scrPreflight)
	m = press(t, m, "enter")
	wantScreen(t, m, scrDeleted)
	m = press(t, m, "1", "down", "2", "down", "3", "enter")
	wantScreen(t, m, scrProgress)

	if len(m.skip.items) != skipBefore+1 {
		t.Errorf("SKIP list has %d files, want %d", len(m.skip.items), skipBefore+1)
	}
	if len(m.trash.items) != trashBefore+2 {
		t.Errorf("trash has %d files, want %d", len(m.trash.items), trashBefore+2)
	}

	for i := 0; i < 1000 && m.screen == scrProgress; i++ {
		checkFrame(t, m, "tick")
		next, _ := m.Update(tickMsg{})
		m = next.(Model)
	}
	wantScreen(t, m, scrSummary)
	checkFrame(t, m, "summary")
	if m.run.done != m.run.total {
		t.Errorf("downloaded %d bytes of %d", m.run.done, m.run.total)
	}
	// The hand-edited file's old version joins the trash once it is replaced.
	if len(m.trash.items) != trashBefore+3 {
		t.Errorf("trash has %d files after the run, want %d", len(m.trash.items), trashBefore+3)
	}
	m = press(t, m, "enter")
	wantScreen(t, m, scrMenu)
	if missing := m.tr.Missing(); len(missing) > 0 {
		t.Errorf("texts missing from the catalog: %v", missing)
	}
}

func TestStoppedRunShowsWhatIsLeft(t *testing.T) {
	m := start(t, Options{})
	m = press(t, m, "2", "enter", "enter", "enter")
	wantScreen(t, m, scrProgress)
	next, _ := m.Update(tickMsg{})
	m = press(t, next.(Model), "esc")
	wantScreen(t, m, scrSummary)
	if !m.run.stopped {
		t.Error("run is not marked as stopped")
	}
}

func TestProblemStatesAndWizard(t *testing.T) {
	m := start(t, Options{Demo: "no-drive"})
	m = press(t, m, "2", "enter", "right", "enter")
	wantScreen(t, m, scrMenu)

	m = start(t, Options{Demo: "lost-access"})
	m = press(t, m, "2", "enter", "right", "left", "enter")
	wantScreen(t, m, scrDeleted)

	m = start(t, Options{FirstRun: true})
	checkFrame(t, m, "start")
	m = press(t, m, "down", "down", "enter") // the third language
	wantScreen(t, m, scrWizard)
	if m.wiz != 1 || m.tr.Code != "pl" {
		t.Errorf("after choosing Polski: step %d, language %q", m.wiz, m.tr.Code)
	}
	m = press(t, m, "esc", "up", "enter", "enter", "esc", "enter", "enter", "enter")
	wantScreen(t, m, scrTree)
	if m.tr.Code != "en" {
		t.Errorf("language = %q, want en", m.tr.Code)
	}
	if missing := m.tr.Missing(); len(missing) > 0 {
		t.Errorf("texts missing from the catalog: %v", missing)
	}
}

// sortedBy reports whether the visible files run in the given order.
func sortedBy(t *testing.T, s browser, order int, desc bool) {
	t.Helper()
	key := func(it demo.Entry) string {
		switch order {
		case byType:
			return filekind.Ext(it.Name())
		case bySize:
			return fmt.Sprintf("%020d", it.Size)
		case byModified:
			return it.Modified
		case byCreated:
			return it.Created
		default:
			return it.Added
		}
	}
	shown := s.visible()
	for i := 1; i < len(shown); i++ {
		before, after := s.items[shown[i-1]], s.items[shown[i]]
		c := strings.Compare(key(before), key(after))
		if order == byName {
			c = alphabet.CompareString(before.Name(), after.Name()) // by the alphabet, not by character code
		}
		if (!desc && c > 0) || (desc && c < 0) {
			t.Errorf("%s by %s, desc %v: %s comes before %s", s.kind, listOrders[order], desc, before.Path, after.Path)
		}
	}
}

// findRow returns the position of a row among those the cursor can stand on.
func findRow(t *testing.T, s browser, match func(listRow) bool) int {
	t.Helper()
	for i, r := range s.selectable() {
		if match(r) {
			return i
		}
	}
	t.Fatalf("%s: the row is not in the list", s.kind)
	return -1
}

func isFolder(dir string) func(listRow) bool {
	return func(r listRow) bool { return r.folder() && r.dir == dir }
}

// goToRow moves the cursor of the trash (screen 4) or the SKIP list to a row.
func goToRow(t *testing.T, m Model, list func(Model) browser, match func(listRow) bool) Model {
	t.Helper()
	for range 200 {
		want := findRow(t, list(m), match)
		switch at := list(m).cursor; {
		case at < want:
			m = press(t, m, "down")
		case at > want:
			m = press(t, m, "up")
		default:
			return m
		}
	}
	t.Fatal("the cursor never reached the row")
	return m
}

func trashOf(m Model) browser { return m.trash }
func skipOf(m Model) browser  { return m.skip }

// Both lists offer the same choice lists; each is checked the same way.
func TestListChoiceLists(t *testing.T) {
	lists := []struct {
		open string
		of   func(Model) browser
	}{{"4", trashOf}, {"3", skipOf}}
	for _, lang := range langs() {
		for _, list := range lists {
			m := start(t, Options{Lang: lang})
			m = press(t, m, list.open, "f")
			if list.of(m).menu != menuFilter {
				t.Fatalf("f did not open the filter list")
			}
			// Every option is on screen, so nobody has to guess what the key cycles through.
			for _, menu := range []string{"f", "s", "g"} {
				m = press(t, m, menu)
				_, opts := m.listOptions(list.of(m))
				if len(opts) != list.of(m).options() {
					t.Fatalf("list %q has %d options, want %d", menu, len(opts), list.of(m).options())
				}
				for _, o := range opts {
					if !strings.Contains(m.View(), o.label) {
						t.Errorf("%s: option %q is not on screen", lang, o.label)
					}
				}
			}

			// A kind can be picked by its digit and shows only its own files.
			for n, kind := range filekind.All {
				want := 0
				for _, it := range list.of(m).items {
					if filekind.Of(it.Name()) == kind {
						want++
					}
				}
				m = press(t, m, "f", strconv.Itoa(n+2))
				if list.of(m).menu != menuNone {
					t.Fatal("a digit did not close the list")
				}
				shown := list.of(m).visible()
				if len(shown) != want {
					t.Errorf("%s filter shows %d files, want %d", kind, len(shown), want)
				}
				for _, i := range shown {
					if got := filekind.Of(list.of(m).items[i].Name()); got != kind {
						t.Errorf("%s filter shows a %s file: %s", kind, got, list.of(m).items[i].Path)
					}
				}
				// A filter leaves no empty folders behind.
				for _, r := range list.of(m).rows() {
					if r.folder() && len(r.files) == 0 {
						t.Errorf("%s filter: folder %q is empty", kind, r.dir)
					}
				}
			}
			m = press(t, m, "f", "up", "up", "up", "up", "up", "enter")
			if s := list.of(m); s.filter != 0 || len(s.visible()) != len(s.items) {
				t.Errorf("filter = %d after choosing all types", s.filter)
			}

			// Each order starts in its natural direction and runs both ways.
			m = press(t, m, "g", "3")
			if list.of(m).group != groupNone {
				t.Fatal("grouping is still on")
			}
			asc, desc := strconv.Itoa(len(listOrders)+1), strconv.Itoa(len(listOrders)+2)
			for order := range listOrders {
				m = press(t, m, "s", strconv.Itoa(order+1))
				if s := list.of(m); s.order != order || s.desc != descByDefault(order) {
					t.Fatalf("order = %d desc = %v, want %d in its natural direction", s.order, s.desc, order)
				}
				sortedBy(t, list.of(m), order, list.of(m).desc)
				m = press(t, m, "s", asc)
				if s := list.of(m); s.desc || s.order != order {
					t.Fatalf("ascending was not applied to order %d", order)
				}
				sortedBy(t, list.of(m), order, false)
				m = press(t, m, "s", desc)
				if !list.of(m).desc {
					t.Fatalf("descending was not applied to order %d", order)
				}
				sortedBy(t, list.of(m), order, true)
				// Choosing the order in effect again keeps the direction.
				m = press(t, m, "s", "enter")
				if !list.of(m).desc {
					t.Errorf("order %d lost its direction when chosen again", order)
				}
			}

			m = press(t, m, "g", "up", "esc")
			if list.of(m).group != groupNone {
				t.Error("Esc applied the highlighted grouping")
			}
			m = press(t, m, "g", "up", "enter")
			if list.of(m).group != groupKind {
				t.Error("Enter did not apply the highlighted grouping")
			}
			// An open list swallows the file keys: nothing is removed behind it.
			before := len(list.of(m).items)
			m = press(t, m, "f", "d", "r", "9", "esc")
			if s := list.of(m); len(s.items) != before || s.filter != 0 {
				t.Errorf("keys leaked through the open list: %d files, filter %d", len(s.items), s.filter)
			}
			m = press(t, m, "esc")
			wantScreen(t, m, scrMenu)
			if missing := m.tr.Missing(); len(missing) > 0 {
				t.Errorf("%s: texts missing from the catalog: %v", lang, missing)
			}
		}
	}
}

// The trash opens as a tree: a folder inside a folder sits one level deeper,
// the way the files-and-folders screen shows Drive.
func TestTrashIsATreeOfFolders(t *testing.T) {
	m := start(t, Options{})
	m = press(t, m, "4")
	if m.trash.group != groupFolder {
		t.Fatalf("the trash opens with grouping %d, want the folder tree", m.trash.group)
	}
	depth := func(match func(listRow) bool) int {
		rows := m.trash.selectable()
		return rows[findRow(t, m.trash, match)].depth
	}
	file := func(path string) func(listRow) bool {
		return func(r listRow) bool { return r.item >= 0 && m.trash.items[r.item].Path == path }
	}
	// Everything hangs off the one shared folder, which is the first row.
	if rows := m.trash.selectable(); !rows[0].folder() || rows[0].dir != demo.Root || len(rows[0].files) != len(m.trash.items) {
		t.Errorf("the first row is %q with %d files, want the shared folder with all %d",
			rows[0].dir, len(rows[0].files), len(m.trash.items))
	}
	outer, inner := findRow(t, m.trash, isFolder("Syncer/Відео")), findRow(t, m.trash, isFolder("Syncer/Відео/Інтерв'ю"))
	if inner <= outer || depth(isFolder("Syncer/Відео")) != 1 || depth(isFolder("Syncer/Відео/Інтерв'ю")) != 2 {
		t.Errorf("Відео at row %d, Відео/Інтерв'ю at row %d: the inner folder is not nested", outer, inner)
	}
	if d := depth(file("Syncer/Відео/Інтерв'ю/чернетка.mov")); d != 3 {
		t.Errorf("a file two folders deep in the shared folder is at depth %d", d)
	}
	if d := depth(file("Syncer/Відео/тест-камери.mp4")); d != 2 {
		t.Errorf("a file one folder deep in the shared folder is at depth %d", d)
	}
	if d := depth(file("Syncer/нотатки-старі.md")); d != 1 {
		t.Errorf("a file directly in the shared folder is at depth %d", d)
	}
	// The inner folder shows its own name, not the whole path.
	if view := m.View(); !strings.Contains(view, "▾ Відео") || !strings.Contains(view, "  ▾ Інтерв'ю") ||
		strings.Contains(view, "Відео/Інтерв'ю") {
		t.Error("the folders are not drawn as nested levels")
	}
	// Two files named 1.doc sit under different folders.
	for _, dir := range []string{"Syncer/Важливе/Документи/Мої", "Syncer/Ігри/NFSU2/stats"} {
		if d := depth(file(dir + "/1.doc")); d != 4 {
			t.Errorf("%s/1.doc is at depth %d, want 4", dir, d)
		}
		findRow(t, m.trash, isFolder(dir))
	}

	// A folder row counts everything inside it, folded or not.
	m = goToRow(t, m, trashOf, isFolder("Syncer/Відео"))
	row, _ := m.trash.current()
	if len(row.files) != 3 {
		t.Errorf("Відео holds %d files, want 3", len(row.files))
	}
	shown := len(m.trash.selectable())
	m = press(t, m, "left")
	if got := len(m.trash.selectable()); got != shown-4 || !strings.Contains(m.View(), "▸ Відео") {
		t.Errorf("folding Відео left %d rows of %d", got, shown)
	}
	if row, _ := m.trash.current(); len(row.files) != 3 {
		t.Errorf("the folded folder holds %d files, want 3", len(row.files))
	}
	m = press(t, m, "right")
	if got := len(m.trash.selectable()); got != shown {
		t.Errorf("unfolding Відео gave %d rows, want %d", got, shown)
	}

	// Enter on a folder restores everything in it, and the message says so.
	before := len(m.trash.items)
	m = goToRow(t, m, trashOf, isFolder("Syncer/Відео/Інтерв'ю"))
	m = press(t, m, "enter")
	if len(m.trash.items) != before-2 || !strings.Contains(m.flash, "Syncer/Відео/Інтерв'ю") {
		t.Errorf("restoring a folder left %d files of %d, message %q", len(m.trash.items), before, m.flash)
	}
	if missing := m.tr.Missing(); len(missing) > 0 {
		t.Errorf("texts missing from the catalog: %v", missing)
	}
}

// Deleting from the trash is for good, so it asks first.
func TestDeletingAsksFirst(t *testing.T) {
	m := start(t, Options{})
	m = press(t, m, "4")
	path := "Syncer/Важливе/Документи/Мої/1.doc"
	isFile := func(r listRow) bool { return r.item >= 0 && m.trash.items[r.item].Path == path }
	m = goToRow(t, m, trashOf, isFile)
	before := len(m.trash.items)

	m = press(t, m, "d")
	if m.trash.menu != menuConfirm || len(m.trash.items) != before {
		t.Fatalf("d deleted at once: menu %d, %d files of %d", m.trash.menu, len(m.trash.items), before)
	}
	if !strings.Contains(m.View(), path) {
		t.Error("the question does not name the file")
	}
	m = press(t, m, "enter") // the first answer is no
	if m.trash.menu != menuNone || len(m.trash.items) != before {
		t.Fatal("Enter on the first answer deleted the file")
	}
	m = press(t, m, "d", "esc")
	if len(m.trash.items) != before {
		t.Fatal("Esc deleted the file")
	}
	m = press(t, m, "d", "s") // the keys of the lists do not answer the question
	if m.trash.menu != menuConfirm {
		t.Fatal("a list key closed the question")
	}
	m = press(t, m, "down", "enter")
	if len(m.trash.items) != before-1 || !strings.Contains(m.flash, path) {
		t.Fatalf("yes left %d files of %d, message %q", len(m.trash.items), before, m.flash)
	}
	for _, it := range m.trash.items {
		if it.Path == path {
			t.Error("the file is still in the trash")
		}
	}
	// The other 1.doc is untouched.
	findRow(t, m.trash, func(r listRow) bool {
		return r.item >= 0 && m.trash.items[r.item].Path == "Syncer/Ігри/NFSU2/stats/1.doc"
	})

	// A folder goes with everything in it.
	before = len(m.trash.items)
	m = goToRow(t, m, trashOf, isFolder("Syncer/Фото"))
	m = press(t, m, "d", "2")
	if len(m.trash.items) != before-3 {
		t.Errorf("deleting the folder left %d files of %d", len(m.trash.items), before)
	}
	if missing := m.tr.Missing(); len(missing) > 0 {
		t.Errorf("texts missing from the catalog: %v", missing)
	}
}

// Outside the tree two files with one name are told apart by the folder
// shown next to each.
func TestFlatListsShowTheFolder(t *testing.T) {
	m := start(t, Options{})
	m = press(t, m, "4", "f", strconv.Itoa(slices.Index(filekind.All, filekind.Docs)+2))
	for _, group := range []string{"2", "3"} {
		m = press(t, m, "g", group)
		for _, dir := range []string{"Важливе/Документи/Мої", "Ігри/NFSU2/stats"} {
			if !strings.Contains(m.View(), dir) {
				t.Errorf("grouping %s: folder %q is not on screen", group, dir)
			}
		}
	}
}

func TestSkipList(t *testing.T) {
	m := start(t, Options{})
	m = press(t, m, "3")
	wantScreen(t, m, scrSkip)
	if m.skip.group != groupFolder {
		t.Fatalf("the SKIP list opens with grouping %d, want the folder tree", m.skip.group)
	}
	// Its date is the day a file went onto the list, and the sort list says so.
	m = press(t, m, "s")
	if view := m.View(); !strings.Contains(view, m.tr.T("sort.added.skip.long")) ||
		strings.Contains(view, m.tr.T("sort.added.trash.long")) {
		t.Error("the sort list of SKIP does not name its own date")
	}
	m = press(t, m, "esc")

	// r takes one file off the list, or a folder with everything in it.
	before := len(m.skip.items)
	isFile := func(r listRow) bool {
		return r.item >= 0 && m.skip.items[r.item].Path == "Syncer/нотатки-2024.md"
	}
	m = goToRow(t, m, skipOf, isFile)
	m = press(t, m, "r")
	if len(m.skip.items) != before-1 || !strings.Contains(m.flash, "нотатки-2024.md") {
		t.Errorf("r left %d files of %d, message %q", len(m.skip.items), before, m.flash)
	}
	m = goToRow(t, m, skipOf, isFolder("Syncer/Фото"))
	m = press(t, m, "left", "right", "r")
	if len(m.skip.items) != before-3 {
		t.Errorf("r on a folder left %d files of %d", len(m.skip.items), before)
	}
	// Enter and d belong to the trash and do nothing here.
	m = press(t, m, "enter", "d")
	if len(m.skip.items) != before-3 || m.skip.menu != menuNone {
		t.Error("a trash key changed the SKIP list")
	}
	m = press(t, m, "esc")
	wantScreen(t, m, scrMenu)
	if missing := m.tr.Missing(); len(missing) > 0 {
		t.Errorf("texts missing from the catalog: %v", missing)
	}
}

// A sync starts by asking where it goes, until the question is switched off;
// the destination settings switch it back on.
func TestSyncAsksWhereToGo(t *testing.T) {
	for _, lang := range langs() {
		m := start(t, Options{Lang: lang})
		if !m.askDest {
			t.Fatal("the question should be on from the start")
		}
		m = press(t, m, "2")
		wantScreen(t, m, scrWhere)
		for _, d := range demo.Destinations {
			if !strings.Contains(m.View(), d.Path) {
				t.Errorf("%s: folder %s is not offered", lang, d.Path)
			}
		}
		if !strings.Contains(m.View(), "[x] "+m.tr.T("dest.ask")) {
			t.Errorf("%s: the question screen does not show its switch", lang)
		}

		// Another folder is picked by its digit and the sync goes there.
		other := demo.Destinations[1]
		m = press(t, m, "2")
		wantScreen(t, m, scrPreflight)
		if m.dest != 1 || !strings.Contains(m.View(), other.Path) {
			t.Fatalf("%s: the checks do not run for %s", lang, other.Path)
		}
		m = press(t, m, "esc")
		if view := m.View(); !strings.Contains(view, other.Path) || !strings.Contains(view, m.tr.T("status.never")) {
			t.Errorf("%s: the main menu does not show the chosen folder", lang)
		}

		// The question comes back with the cursor on the folder in use. Its
		// switch is the last row; clearing it does not leave the screen.
		m = press(t, m, "2")
		wantScreen(t, m, scrWhere)
		if m.where != 1 {
			t.Errorf("cursor on row %d, want the folder in use", m.where)
		}
		m = press(t, m, strconv.Itoa(m.whereRows()))
		wantScreen(t, m, scrWhere)
		if m.askDest || !strings.Contains(m.View(), "[ ] "+m.tr.T("dest.ask")) {
			t.Fatal("the switch was not cleared")
		}
		m = press(t, m, "up", "up", "up", "enter")
		wantScreen(t, m, scrPreflight)
		if m.dest != 0 {
			t.Errorf("dest = %d, want the first folder", m.dest)
		}
		// With the switch off a sync goes straight to the checks.
		m = press(t, m, "esc", "2")
		wantScreen(t, m, scrPreflight)

		// Settings → destination folder turns the question back on.
		m = press(t, m, "esc", "5", "2")
		wantScreen(t, m, scrDest)
		if !strings.Contains(m.View(), "[ ] "+m.tr.T("dest.ask")) {
			t.Errorf("%s: the destination settings do not show the switch", lang)
		}
		m = press(t, m, "2")
		if !m.askDest || !strings.Contains(m.View(), "[x] "+m.tr.T("dest.ask")) {
			t.Fatal("the settings did not switch the question on")
		}
		m = press(t, m, "down", " ", "enter") // Space and Enter on the row work too
		if !m.askDest {
			t.Fatal("Space then Enter should leave the question on")
		}
		m = press(t, m, "esc", "esc", "2")
		wantScreen(t, m, scrWhere)
		// Another folder cannot be typed on demo data, and says so.
		m = press(t, m, strconv.Itoa(len(demo.Destinations)+1))
		wantScreen(t, m, scrWhere)
		if m.flash != m.tr.T("demo.na") {
			t.Errorf("flash = %q", m.flash)
		}
		m = press(t, m, "esc")
		wantScreen(t, m, scrMenu)
		if missing := m.tr.Missing(); len(missing) > 0 {
			t.Errorf("%s: texts missing from the catalog: %v", lang, missing)
		}
	}
}

// The settings and their sub-screens answer to the digits their rows show.
func TestSettingsDigits(t *testing.T) {
	m := start(t, Options{})
	m = press(t, m, "5")
	for i, want := range settingsScreens {
		if !strings.Contains(m.View(), strconv.Itoa(i+1)+"  ") {
			t.Errorf("settings row %d shows no number", i+1)
		}
		m = press(t, m, strconv.Itoa(i+1))
		wantScreen(t, m, want)
		m = press(t, m, "esc")
		wantScreen(t, m, scrSettings)
	}
	// A digit past the last row does nothing.
	m = press(t, m, "9")
	wantScreen(t, m, scrSettings)

	m = press(t, m, "1")
	for i, key := range accountActions {
		if !strings.Contains(m.View(), strconv.Itoa(i+1)+"  "+m.tr.T(key)) {
			t.Errorf("Service Account action %d shows no number", i+1)
		}
		m = press(t, m, strconv.Itoa(i+1))
		if m.set.sub != i || m.flash != m.tr.T("demo.na") {
			t.Errorf("digit %d: row %d, flash %q", i+1, m.set.sub, m.flash)
		}
	}
	m = press(t, m, "esc", "3", "2")
	wantScreen(t, m, scrLang)
	if m.tr.Code != "en" {
		t.Errorf("digit 2 on the language screen left the language at %q", m.tr.Code)
	}
	m = press(t, m, "1", "esc", "esc")
	wantScreen(t, m, scrMenu)
	if missing := m.tr.Missing(); len(missing) > 0 {
		t.Errorf("texts missing from the catalog: %v", missing)
	}
}

func TestPaletteChoice(t *testing.T) {
	for _, lang := range langs() {
		m := start(t, Options{Lang: lang, Theme: "dark"})
		m = press(t, m, "5", "down", "down", "down", "enter")
		wantScreen(t, m, scrTheme)
		// Every scheme is on the list by name, so the choice is visible.
		for _, s := range theme.Schemes {
			if !strings.Contains(m.View(), s.Name) {
				t.Errorf("%s: scheme %q is not on screen", lang, s.Name)
			}
		}
		goTo := func(row int) {
			for m.set.sub > row {
				m = press(t, m, "up")
			}
			for m.set.sub < row {
				m = press(t, m, "down")
			}
		}
		for i := len(theme.Schemes) - 1; i >= 0; i-- {
			goTo(i)
			m = press(t, m, "enter")
			if m.scheme != i {
				t.Fatalf("scheme = %d after choosing row %d", m.scheme, i)
			}
			if want := lipgloss.Color(theme.Schemes[i].Dark.Accent); m.th.Accent != want {
				t.Errorf("%s: accent = %s, want %s", theme.Schemes[i].ID, m.th.Accent, want)
			}
		}
		// The light variant belongs to the chosen scheme, not to the default
		// one, and a scheme without it stays dark.
		light := len(theme.Schemes) + 2
		goTo(light)
		m = press(t, m, "enter")
		last := 0
		for i, s := range theme.Schemes {
			goTo(i)
			m = press(t, m, "enter")
			want, dark := lipgloss.Color(s.Dark.Accent), true
			if s.HasLight() {
				want, dark, last = lipgloss.Color(s.Light.Accent), false, i
			}
			if m.th.IsDark != dark || m.th.Accent != want {
				t.Errorf("%s with the light variant: dark = %v, accent = %s, want %v and %s",
					s.ID, m.th.IsDark, m.th.Accent, dark, want)
			}
			if said := strings.Contains(m.View(), m.tr.T("theme.darkonly")); said == s.HasLight() {
				t.Errorf("%s: the dark-only note is shown = %v", s.ID, said)
			}
		}
		goTo(last)
		m = press(t, m, "enter")
		m = press(t, m, "esc")
		wantScreen(t, m, scrSettings)
		if name := theme.Schemes[last].Name; !strings.Contains(m.View(), name) {
			t.Errorf("the settings do not show the chosen scheme %q", name)
		}
		// The theme screen opens with the cursor on the scheme in effect.
		m = press(t, m, "enter")
		if m.set.sub != last {
			t.Errorf("cursor on row %d, want the chosen scheme on row %d", m.set.sub, last)
		}
		// The palettes answer to the keys they show: 1 to 9, 0, then letters.
		for i, s := range theme.Schemes {
			m = press(t, m, s.Key)
			if m.scheme != i || m.set.sub != i {
				t.Errorf("key %s chose scheme %d, want %d (%s)", s.Key, m.scheme, i, s.Name)
			}
			if !strings.Contains(m.View(), s.Key+"  (•) "+s.Name) {
				t.Errorf("%s is not shown with its key %s", s.Name, s.Key)
			}
		}
		if i, ok := theme.Index("github"); !ok || theme.Schemes[i].Key != "g" || theme.Schemes[0].ID != "terminal" {
			t.Error("GitHub should answer to g, and Terminal should be the default")
		}
		if missing := m.tr.Missing(); len(missing) > 0 {
			t.Errorf("%s: texts missing from the catalog: %v", lang, missing)
		}
	}

	for i, s := range theme.Schemes {
		m, err := New(Options{Palette: s.ID})
		if err != nil || m.scheme != i {
			t.Errorf("New with palette %q: scheme = %d, err = %v", s.ID, m.scheme, err)
		}
	}
	if _, err := New(Options{Palette: "no-such-palette"}); err == nil {
		t.Error("New accepted a palette that does not exist")
	}
}

func TestNamesFollowTheAlphabet(t *testing.T) {
	inOrder := [][2]string{
		{"інтерв'ю-сире.mov", "чернетка.mov"}, // і is not after я
		{"Документи", "Ігри"},
		{"Ігри", "Музика"},
		{"гора", "ґанок"},
		{"ґанок", "дім"},
		{"їжак", "йод"},
		{"Архів", "архів-2"}, // case does not decide
		{"IMG_2.jpg", "IMG_10.jpg"},
		{"", "Архів 2019"}, // files outside any folder come first
	}
	for _, pair := range inOrder {
		if alphabet.CompareString(pair[0], pair[1]) >= 0 {
			t.Errorf("%q should come before %q", pair[0], pair[1])
		}
	}
	if alphabet.CompareString("Звіт.docx", "звіт.docx") != 0 {
		t.Error("names that differ only in case should be equal")
	}
}

func TestSpaceTogglesThePreview(t *testing.T) {
	previews = nil
	m := start(t, Options{})
	m = press(t, m, "4", " ")
	if row, _ := m.trash.current(); !row.folder() || len(previews) != 0 {
		t.Fatalf("the trash should open on a folder, and Space on it should preview nothing: %v", previews)
	}
	anyFile := func(r listRow) bool { return r.item >= 0 }
	m = goToRow(t, m, trashOf, anyFile)
	row, _ := m.trash.current()
	first := m.trash.items[row.item]
	m = press(t, m, " ")
	if len(previews) != 1 || previews[0] != first.Name() || !m.pv.open() {
		t.Fatalf("Space opened %v, want a preview of %s", previews, first.Name())
	}
	if !strings.Contains(m.flash, first.Path) {
		t.Errorf("flash %q does not name the previewed file", m.flash)
	}
	m = press(t, m, " ")
	if m.pv.open() || len(previews) != 1 {
		t.Fatal("Space on the same file did not close the preview")
	}
	// Space on another file replaces the open preview.
	m = press(t, m, " ", "down", " ")
	if len(previews) != 3 || !m.pv.open() {
		t.Fatalf("previews = %v, want the second file shown", previews)
	}
	m = press(t, m, "esc")
	wantScreen(t, m, scrMenu)
	if m.pv.open() {
		t.Error("leaving the trash left the preview open")
	}
	// The SKIP list previews its files the same way.
	m = press(t, m, "3")
	m = goToRow(t, m, skipOf, anyFile)
	m = press(t, m, " ")
	if len(previews) != 4 || !m.pv.open() {
		t.Fatalf("previews = %v, want a SKIP file shown", previews)
	}
	m = press(t, m, "esc")
	if m.pv.open() {
		t.Error("leaving the SKIP list left the preview open")
	}
	if missing := m.tr.Missing(); len(missing) > 0 {
		t.Errorf("texts missing from the catalog: %v", missing)
	}
}

// goToNode moves the cursor of the selection tree to a row.
func goToNode(t *testing.T, m Model, match func(treeRow) bool) Model {
	t.Helper()
	for range 200 {
		want := -1
		for i, r := range m.tree.rows() {
			if match(r) {
				want = i
				break
			}
		}
		if want < 0 {
			t.Fatal("the row is not in the tree")
		}
		switch at := m.tree.cursor; {
		case at < want:
			m = press(t, m, "down")
		case at > want:
			m = press(t, m, "up")
		default:
			return m
		}
	}
	t.Fatal("the cursor never reached the row")
	return m
}

func named(name string) func(treeRow) bool {
	return func(r treeRow) bool { return !r.isSwitch() && r.node.Name == name }
}

func switchOf(name string) func(treeRow) bool {
	return func(r treeRow) bool { return r.owner != nil && r.owner.Name == name }
}

func hasRow(m Model, match func(treeRow) bool) bool {
	for _, r := range m.tree.rows() {
		if match(r) {
			return true
		}
	}
	return false
}

// The demo shares one folder with the account. It is a row of its own, and
// everything on Drive sits inside it.
func TestOneSharedFolder(t *testing.T) {
	m := start(t, Options{})
	if len(m.tree.root) != 1 || m.tree.root[0].Name != demo.Root || !m.tree.root[0].Dir {
		t.Fatalf("the tree should have the one shared folder %q at the top", demo.Root)
	}
	if got := m.sharedFolders(); len(got) != 1 || got[0] != demo.Root {
		t.Errorf("shared folders = %v", got)
	}
	m = press(t, m, "1")
	if row := m.tree.rows()[m.tree.cursor]; row.isSwitch() || row.node.Name != demo.Root {
		t.Error("the tree does not open on the shared folder")
	}
	if !strings.Contains(m.View(), "▾ "+demo.Root) {
		t.Error("the shared folder is not drawn as an open folder")
	}
	// Its mark tells what is selected inside: one unselected file makes it partial.
	if m.tree.root[0].State() != demo.Partial {
		t.Error("the shared folder should be selected in part")
	}
	// Every path the demo knows starts with the shared folder.
	prefix := demo.Root + "/"
	for _, it := range append(demo.Trash(), demo.SkipList()...) {
		if !strings.HasPrefix(it.Path, prefix) {
			t.Errorf("%s is outside the shared folder", it.Path)
		}
	}
	for _, c := range demo.Changes() {
		if !strings.HasPrefix(c.Path, prefix) {
			t.Errorf("%s is outside the shared folder", c.Path)
		}
	}
	for _, d := range demo.DeletedOnDrive() {
		if !strings.HasPrefix(d.Path, prefix) {
			t.Errorf("%s is outside the shared folder", d.Path)
		}
	}
}

// Two selected files with one name in a folder cannot both keep it on the
// drive. The choice screen marks them, and the checks before a sync say so
// again for those who skipped the choice.
func TestSameNamesAreFlagged(t *testing.T) {
	for _, lang := range langs() {
		m := start(t, Options{Lang: lang})
		clash := m.tree.clashes()
		var kept, renamed int
		for n, saved := range clash {
			switch saved {
			case n.Name:
				kept++
			case "довідка (2).pdf":
				renamed++
				if n.Modified != "2026-11-14" {
					t.Errorf("the older file should keep the name; the renamed one is from %s", n.Modified)
				}
			default:
				t.Errorf("unexpected name on the drive: %q", saved)
			}
		}
		if len(clash) != 2 || kept != 1 || renamed != 1 {
			t.Fatalf("clashes = %d (kept %d, renamed %d), want one pair", len(clash), kept, renamed)
		}

		m = press(t, m, "1")
		m = goToNode(t, m, named("довідка.pdf"))
		view := m.View()
		if !strings.Contains(view, "≈ "+m.tr.T("tree.clash")) || !strings.Contains(view, "довідка (2).pdf") {
			t.Errorf("%s: the files are not marked on the choice screen", lang)
		}
		if !strings.Contains(view, m.tr.T("tree.clash.note", m.tr.N("n.files", 2))) {
			t.Errorf("%s: the choice screen has no warning line", lang)
		}
		// The checks repeat it before the sync starts.
		m = press(t, m, "enter", "2", "enter")
		wantScreen(t, m, scrPreflight)
		if !strings.Contains(m.View(), m.tr.T("pre.clash")) {
			t.Errorf("%s: the checks do not mention the same names", lang)
		}

		// Clearing one of the two removes the clash everywhere.
		m = press(t, m, "esc", "1")
		m = goToNode(t, m, named("довідка.pdf"))
		m = press(t, m, " ")
		if len(m.tree.clashes()) != 0 || strings.Contains(m.View(), m.tr.T("tree.clash")) {
			t.Errorf("%s: one file is cleared and the clash is still shown", lang)
		}
		m = press(t, m, "enter", "2", "enter")
		if strings.Contains(m.View(), m.tr.T("pre.clash")) {
			t.Errorf("%s: the checks still mention the same names", lang)
		}
		if missing := m.tr.Missing(); len(missing) > 0 {
			t.Errorf("%s: texts missing from the catalog: %v", lang, missing)
		}
	}

}

// The main menu starts with choosing: it is what a sync acts on.
func TestMenuStartsWithChoosing(t *testing.T) {
	m := start(t, Options{})
	items := m.menuItems()
	if items[0].target != scrTree || items[1].target != scrPreflight {
		t.Fatalf("menu starts with screens %d and %d", items[0].target, items[1].target)
	}
	m = press(t, m, "1")
	wantScreen(t, m, scrTree)
	if row := m.tree.rows()[m.tree.cursor]; row.isSwitch() {
		t.Error("the tree opens with the cursor on a switch")
	}
}

// A folder selected in part shows the switch that decides what happens to
// files and folders appearing in it later; a folder selected as a whole
// takes them and shows none.
func TestNewItemsSwitch(t *testing.T) {
	for _, lang := range langs() {
		m := start(t, Options{Lang: lang})
		m = press(t, m, "1")
		top := func(r treeRow) bool { return r.top }
		if !hasRow(m, top) || !strings.Contains(m.View(), m.tr.T("tree.new.top")) {
			t.Fatalf("%s: the top level is selected in part and shows no switch", lang)
		}
		if !hasRow(m, switchOf(demo.Root)) || !hasRow(m, switchOf("Відео")) || !hasRow(m, switchOf("2026 Лісабон")) {
			t.Fatal("a folder selected in part shows no switch")
		}
		if hasRow(m, switchOf("2025 Карпати")) || hasRow(m, switchOf("Документи")) {
			t.Fatal("a folder selected as a whole shows a switch")
		}
		if !strings.Contains(m.View(), "[x] + "+m.tr.T("tree.new.top")) {
			t.Errorf("%s: the switch is not drawn as a checked box", lang)
		}

		// With every switch on, nothing new is left out of a sync.
		if left := m.newRun().leftN; left != 0 {
			t.Fatalf("%d new files are left out by default", left)
		}
		all := len(m.newRun().queue)

		// Space clears a switch, Esc brings it back.
		m = goToNode(t, m, switchOf("2026 Лісабон"))
		m = press(t, m, " ")
		if m.tree.takesNew("Syncer/Відео/2026 Лісабон/тизер.mov") {
			t.Fatal("Space did not clear the switch")
		}
		m = press(t, m, "esc", "1")
		if !m.tree.takesNew("Syncer/Відео/2026 Лісабон/тизер.mov") {
			t.Fatal("Esc did not bring the switch back")
		}

		// Cleared and saved, it keeps the folder's new files out of the sync,
		// and the checks say how many.
		m = goToNode(t, m, switchOf("2026 Лісабон"))
		m = press(t, m, " ", "enter", "2", "enter")
		wantScreen(t, m, scrPreflight)
		if m.run.leftN != 2 || len(m.run.queue) != all-2 {
			t.Errorf("left out %d files, queued %d of %d", m.run.leftN, len(m.run.queue), all)
		}
		if !strings.Contains(m.View(), m.tr.T("pre.left")) {
			t.Errorf("%s: the checks do not mention the files left out", lang)
		}

		// A folder that is new itself follows the nearest known folder above
		// it: Музика appears inside the shared folder, whose switch decides.
		m = press(t, m, "esc", "1")
		m = goToNode(t, m, switchOf(demo.Root))
		m = press(t, m, " ")
		if m.tree.takesNew("Syncer/Музика/демо-запис.wav") || m.newRun().leftN != 3 {
			t.Errorf("the switch of the shared folder is off and %d files are left out, want 3", m.newRun().leftN)
		}
		// Another folder shared with the account later follows the top-level switch.
		if !m.tree.takesNew("Друга спільна папка/файл.txt") {
			t.Error("a newly shared folder is not taken while the top-level switch is on")
		}
		m = goToNode(t, m, top)
		m = press(t, m, " ")
		if m.tree.takesNew("Друга спільна папка/файл.txt") {
			t.Error("a newly shared folder is taken with the top-level switch off")
		}
		// New files in a folder selected as a whole are taken regardless.
		if !m.tree.takesNew("Syncer/Фото/IMG_2201.jpg") {
			t.Error("a folder selected as a whole does not take a new file")
		}
		// A folder not selected at all takes nothing.
		if m.tree.takesNew("Syncer/Архів 2019/новий.zip") {
			t.Error("a folder that is not selected takes a new file")
		}

		// Selecting the folder as a whole removes its switch; the cursor stays on the folder.
		m = goToNode(t, m, named("2026 Лісабон"))
		m = press(t, m, " ")
		if hasRow(m, switchOf("2026 Лісабон")) || !m.tree.takesNew("Syncer/Відео/2026 Лісабон/тизер.mov") {
			t.Error("the folder is selected as a whole and still has a switch")
		}
		if row := m.tree.rows()[m.tree.cursor]; row.isSwitch() || row.node.Name != "2026 Лісабон" {
			t.Error("the cursor left the folder")
		}

		// Clearing one file makes its folder partial: the switch appears above
		// the file and the cursor stays on the file.
		m = goToNode(t, m, named("Фото"))
		m = press(t, m, "right")
		m = goToNode(t, m, named("IMG_0042.jpg"))
		m = press(t, m, " ")
		if !hasRow(m, switchOf("Фото")) {
			t.Fatal("clearing a file gave its folder no switch")
		}
		if row := m.tree.rows()[m.tree.cursor]; row.isSwitch() || row.node.Name != "IMG_0042.jpg" {
			t.Error("the cursor left the file when the switch appeared")
		}
		if !m.tree.takesNew("Syncer/Фото/IMG_2201.jpg") {
			t.Error("a fresh switch should be on")
		}
		m = press(t, m, "esc")
		wantScreen(t, m, scrMenu)
		if missing := m.tr.Missing(); len(missing) > 0 {
			t.Errorf("%s: texts missing from the catalog: %v", lang, missing)
		}
	}
}

func TestEscRestoresTheSelection(t *testing.T) {
	m := start(t, Options{})
	_, before := m.tree.selected()
	m = press(t, m, "1", " ", "esc")
	if _, after := m.tree.selected(); after != before {
		t.Errorf("selection after Esc = %d bytes, want %d", after, before)
	}
	m = press(t, m, "1", " ", "enter")
	if _, after := m.tree.selected(); after == before {
		t.Error("selection did not change after Space and Enter")
	}
}

func TestNarrowAndWideTerminals(t *testing.T) {
	keys := []string{"1", "down", " ", "up", " ", "esc", "3", "esc", "4", "s", "down", "g", "f", "down", "down", "down", "down", "enter", "esc", "5", "enter", "esc", "down", "enter", "esc", "esc",
		"2", "down", "down", "down", "up", "up", "up", "enter", "enter", "enter"}
	for _, size := range [][2]int{{minW, minH}, {60, 16}, {80, 24}, {200, 60}} {
		for _, o := range []Options{{}, {Lang: "en"}, {FirstRun: true}, {Demo: "lost-access"}, {Demo: "no-drive"}} {
			m, err := New(o)
			if err != nil {
				t.Fatal(err)
			}
			next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			m = next.(Model)
			for _, k := range keys {
				next, _ := m.Update(keyMsg(k))
				m = next.(Model)
				lines := strings.Split(m.View(), "\n")
				if len(lines) != size[1] {
					t.Fatalf("%dx%d %+v after %q: %d lines", size[0], size[1], o, k, len(lines))
				}
				for i, l := range lines {
					if w := lipgloss.Width(l); w != size[0] {
						t.Fatalf("%dx%d %+v screen %d after %q: line %d is %d wide: %q",
							size[0], size[1], o, m.screen, k, i, w, l)
					}
				}
			}
		}
	}
}

func TestTooSmallWindowSaysSo(t *testing.T) {
	m, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 30, Height: 5})
	if view := next.(Model).View(); !strings.Contains(view, "40×10") {
		t.Errorf("view = %q, want the minimum size message", view)
	}
}

// firstHole finds a character drawn without a background colour.
func firstHole(view string) (line, col int, found bool) {
	for li, l := range strings.Split(view, "\n") {
		bg := false
		col := 0
		for i := 0; i < len(l); {
			if l[i] == 0x1b && i+1 < len(l) && l[i+1] == '[' {
				end := strings.IndexByte(l[i:], 'm')
				if end < 0 {
					break
				}
				params := strings.Split(l[i+2:i+end], ";")
				for p := 0; p < len(params); p++ {
					switch params[p] {
					case "", "0", "49":
						bg = false
					case "38", "48":
						if params[p] == "48" {
							bg = true
						}
						if p+1 < len(params) && params[p+1] == "2" {
							p += 4
						} else {
							p += 2
						}
					}
				}
				i += end + 1
				continue
			}
			if !bg {
				return li, col, true
			}
			_, size := utf8.DecodeRuneInString(l[i:])
			i += size
			col++
		}
	}
	return 0, 0, false
}

func TestPaintedBackgroundHasNoHoles(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)

	check := func(m Model, after string) {
		t.Helper()
		if line, col, found := firstHole(m.View()); found {
			t.Fatalf("screen %d after %q: no background at line %d, column %d", m.screen, after, line, col)
		}
	}
	for _, o := range []Options{{Theme: "dark"}, {Theme: "light"}, {FirstRun: true}, {Demo: "lost-access"}, {Demo: "no-drive"},
		{Palette: "ice", Theme: "dark"}, {Palette: "ice", Theme: "light"},
		{Palette: "violet", Theme: "dark"}, {Palette: "violet", Theme: "light"},
		{Palette: "dark-plus"}, {Palette: "matrix"}, {Palette: "word"}, {Palette: "ms-dos", Theme: "light"}, {Palette: "far"},
		{Palette: "terminal"}, {Palette: "nord", Theme: "light"},
		{Palette: "github", Theme: "dark"}, {Palette: "github", Theme: "light"}} {
		m := start(t, o)
		check(m, "start")
		for _, k := range []string{"enter", "enter", "enter", "enter", "esc", "esc", "1", "esc", "3", "esc", "4", "s", "g", "f", "down", "esc", "esc",
			"5", "enter", "esc", "down", "enter", "esc", "down", "enter", "esc", "down", "enter", "esc", "esc",
			"2", "enter", "enter", "enter"} {
			next, _ := m.Update(keyMsg(k))
			m = next.(Model)
			check(m, k)
		}
		for i := 0; i < 1000 && m.screen == scrProgress; i++ {
			next, _ := m.Update(tickMsg{})
			m = next.(Model)
			check(m, "tick")
		}
	}
}

// Under a Japanese, Chinese or Korean system locale go-runewidth counts
// the "ambiguous" characters, such as ─, … and ✓, as two columns, while
// terminals and Lip Gloss draw them in one. The frame must not follow the
// locale: with that measure every row came out too short.
func TestFrameDoesNotFollowTheLocale(t *testing.T) {
	was := runewidth.DefaultCondition.EastAsianWidth
	runewidth.DefaultCondition.EastAsianWidth = true
	t.Cleanup(func() { runewidth.DefaultCondition.EastAsianWidth = was })
	if runewidth.StringWidth("─") != 2 {
		t.Fatal("the test does not imitate an East Asian locale")
	}
	m := start(t, Options{})
	checkFrame(t, m, "start")
	// The tree, both lists, the settings with the themes, and a sync.
	press(t, m, "1", "esc", "3", "esc", "4", "esc", "5", "4", "esc", "1", "esc", "esc", "2", "enter")
}

// Labels that stand in a column of a fixed width must fit it in every
// language and leave a gap before the next column: a label that does not
// is cut with "…" or runs into the value beside it.
func TestLabelsFitTheirColumns(t *testing.T) {
	columns := []struct {
		width int
		keys  []string
	}{
		{11, []string{"status.account", "status.drive", "status.free", "status.last"}},
		{10, []string{"acc.email", "acc.key", "acc.access", "dest.path", "dest.volume", "dest.free", "dest.copy", "dest.trash"}},
		{23, []string{"set.account", "set.dest", "set.lang", "set.theme"}},
		{31, []string{"pre.nodrive", "pre.drive.ok", "pre.lost", "pre.access.ok", "pre.space.ok", "pre.space.no", "pre.clash"}},
		{21, []string{"pre.new", "pre.left", "pre.updated", "pre.moves", "pre.deleted", "pre.local",
			"pre.not.shortcut", "pre.not.unsupported", "pre.not.service", "pre.not.forbidden", "pre.not.too-large",
			"sum.downloaded", "sum.updated", "sum.trash", "sum.skip", "sum.removed", "sum.skipped", "sum.failed", "sum.moved"}},
		{25, []string{"del.act.delete", "del.act.skip", "del.act.trash"}},
		{11, []string{"del.act.delete.short", "del.act.skip.short", "del.act.trash.short"}},
		{7, []string{"run.total", "run.file"}},
		{8, []string{"col.size"}},
		{11, []string{"col.modified"}},
		{12, []string{"col.created", "col.added.trash", "col.added.skip"}},
	}
	for _, lang := range langs() {
		tr := start(t, Options{Lang: lang}).tr
		for _, c := range columns {
			for _, key := range c.keys {
				if w := textWidth(tr.T(key)); w > c.width {
					t.Errorf("%s: %q is %d columns wide, its column holds %d: %q", lang, key, w, c.width, tr.T(key))
				}
			}
		}
		if missing := tr.Missing(); len(missing) > 0 {
			t.Errorf("%s: texts missing from the catalog: %v", lang, missing)
		}
	}
}

func TestWrap(t *testing.T) {
	cases := []struct {
		text  string
		width int
		want  []string
	}{
		// Words stay whole, and the space at a break is dropped.
		{"one two three", 7, []string{"one two", "three"}},
		{"one two three", 8, []string{"one two", "three"}},
		{"один два  три", 9, []string{"один два", "три"}},
		// A word longer than a line is cut where the line ends.
		{"/Volumes/USB/backup", 8, []string{"/Volumes", "/USB/bac", "kup"}},
		{"", 10, []string{""}},
		{"a\n\nb", 10, []string{"a", "", "b"}},
		// Japanese breaks between any two characters…
		{"ファイルをコピー", 8, []string{"ファイル", "をコピー"}},
		// …but not before a closing mark or a small kana,
		{"コピーします。次へ", 12, []string{"コピーしま", "す。次へ"}},
		{"フォルダ自体", 4, []string{"フォ", "ルダ", "自体"}},
		// and not after an opening bracket.
		{"これは「閲覧者」です", 6, []string{"これは", "「閲覧", "者」で", "す"}},
		// A Latin word inside stays whole; a mark stays with its word.
		{"キーを Drive に保存:", 10, []string{"キーを", "Drive に保", "存:"}},
		{"共享给邮箱：abc", 10, []string{"共享给邮", "箱：abc"}},
	}
	for _, c := range cases {
		got := wrap(c.text, c.width)
		if !slices.Equal(got, c.want) {
			t.Errorf("wrap(%q, %d) = %q, want %q", c.text, c.width, got, c.want)
		}
		for _, line := range got {
			if textWidth(line) > c.width {
				t.Errorf("wrap(%q, %d): line %q is too wide", c.text, c.width, line)
			}
		}
	}
}
