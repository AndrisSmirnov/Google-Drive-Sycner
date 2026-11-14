package tui

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"syncer-gdrive/internal/config"
	"syncer-gdrive/internal/demo"
	"syncer-gdrive/internal/drive"
	"syncer-gdrive/internal/engine"
	"syncer-gdrive/internal/state"
	"syncer-gdrive/internal/theme"
	"syncer-gdrive/internal/volume"
)

// LiveOptions connect the interface to the real world: the program's
// folder, Google Drive and the connected drives. Without them the
// interface runs on demo data.
type LiveOptions struct {
	Home    string                         // the program's folder on this computer
	Connect func(*drive.Key) engine.Remote // the link to Google Drive
	Volumes func() []string                // connected drives that already hold a copy
}

// live is the program at work on real data. The model holds it by pointer:
// the model is copied on every update, the work in progress is not.
type live struct {
	home    string
	connect func(*drive.Key) engine.Remote
	volumes func() []string

	settings config.Settings
	key      *drive.Key
	keyData  []byte
	dest     string
	eng      *engine.Engine
	info     demo.Destination   // the destination as the screens show it
	dests    []demo.Destination // what the question "where to sync" offers
	plan     *engine.Plan
	mon      *monitor
	stopRun  context.CancelFunc
	access   []string // shared folders the account saw when last checked
	checked  bool     // the account has been checked in the first-run steps
}

// What the destination question is asked for.
const (
	whereSync = iota
	whereSettings
	whereFirstRun
)

// Messages that bring the results of work done off the screen.
type (
	listedMsg struct {
		snap *engine.Snapshot
		err  error
	}
	plannedMsg struct {
		plan *engine.Plan
		err  error
	}
	ranMsg struct {
		res *engine.Result
		err error
	}
	accessMsg struct {
		folders []string
		files   int
		err     error
	}
)

// copyToClipboard hands text to the terminal for the clipboard. Tests
// replace it.
var copyToClipboard = func(text string) { osc52(text) }

// NewLive builds the interface over real data. Options given on the
// command line win over the saved settings.
func NewLive(o Options, lo LiveOptions) (Model, error) {
	lv := &live{home: lo.Home, connect: lo.Connect, volumes: lo.Volumes}
	if lv.volumes == nil {
		lv.volumes = func() []string { return nil }
	}
	settings, _, err := config.Load(lv.home, "")
	if err != nil {
		return Model{}, err
	}
	lv.settings = settings

	// Where the copy is: a connected drive that holds one, else the folder
	// this computer synced to last, if it is there.
	if found := lv.volumes(); len(found) == 1 {
		lv.dest = found[0]
	} else {
		for _, known := range settings.Destinations {
			if dirExists(known) {
				lv.dest = known
				break
			}
		}
	}
	if lv.dest != "" {
		if s, fromDrive, err := config.Load(lv.home, lv.dest); err == nil && fromDrive {
			lv.settings = s // the drive brings its own settings
		}
	}
	key, data, err := config.LoadKey(lv.home)
	switch {
	case err == nil:
		lv.key, lv.keyData = key, data
	case !errors.Is(err, config.ErrNoKey):
		return Model{}, err
	}

	if o.Lang == "" {
		o.Lang = lv.settings.Language
	}
	if o.Theme == "" {
		o.Theme = lv.settings.Variant
	}
	if o.Palette == "" {
		o.Palette = lv.settings.Palette
	}
	if _, ok := theme.Index(o.Palette); !ok {
		o.Palette = "" // a palette this version does not have: the default
	}
	o.Demo, o.FirstRun = "", false
	m, err := New(o)
	if err != nil {
		return Model{}, err
	}
	m.lv = lv
	m.paintBg, m.askDest = lv.settings.PaintBackground, lv.settings.AskDestination
	m.applyTheme()
	m.tree = treeState{newTop: true}
	m.skip, m.trash = newBrowser("skip", nil), newBrowser("trash", nil)
	lv.info = describe(lv.dest)

	switch {
	case lv.key == nil && lv.dest != "" && config.HasSealedKey(lv.dest):
		m = m.askSealedPassword()
	case lv.key == nil:
		m.screen = scrWizard
	case lv.dest == "":
		m = m.askWhere(whereFirstRun)
	default:
		if err := m.openEngine(); err != nil {
			m.failure = err.Error()
		}
	}
	return m, nil
}

func dirExists(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

// day shows a moment as a date; an unknown moment as nothing.
func day(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02")
}

// describe tells what the screens show about a destination folder.
func describe(dest string) demo.Destination {
	d := demo.Destination{Path: dest}
	if dest == "" {
		d.Missing = true
		return d
	}
	// A folder that is not there yet is fine as long as it can be created.
	// On a drive that is not connected it cannot.
	d.Missing = !volume.Present(dest)
	if d.Missing {
		return d
	}
	if info, err := volume.Stat(dest); err == nil {
		d.Volume, d.Free, d.Capacity = info.Label(), info.Free, info.Total
	}
	if last := engine.LastSync(dest); !last.IsZero() {
		d.LastSync = last.Local().Format("2006-01-02 15:04")
	}
	return d
}

// closeEngine releases the state of the destination in use.
func (lv *live) closeEngine() {
	if lv.eng != nil {
		lv.eng.Close()
		lv.eng = nil
	}
}

// openEngine starts work on the current destination with the current key.
func (m *Model) openEngine() error {
	lv := m.lv
	lv.closeEngine()
	lv.info = describe(lv.dest)
	if lv.key == nil || lv.dest == "" {
		return nil
	}
	eng, err := engine.Open(lv.dest, lv.connect(lv.key))
	if err != nil {
		return err
	}
	lv.eng = eng
	lv.info = describe(lv.dest)
	m.reload()
	return nil
}

// reload reads from the state what the screens show without asking Drive.
func (m *Model) reload() {
	lv := m.lv
	if lv.eng == nil {
		return
	}
	if snap, err := lv.eng.Known(); err == nil {
		m.setTree(snap)
	}
	m.loadKept()
}

// exportLabel says what a Google document is saved as.
func exportLabel(google string) string {
	return map[string]string{
		"document": "Docs → .docx", "spreadsheet": "Sheets → .xlsx", "presentation": "Slides → .pptx",
	}[google]
}

// setTree shows Drive as a look found it. Folders that were open stay open;
// on a first look the shared folders are.
func (m *Model) setTree(s *engine.Snapshot) {
	open := map[string]bool{}
	first := len(m.tree.root) == 0
	eachNode(m.tree.root, func(n *demo.Node) {
		if n.Dir && n.Open {
			open[n.ID] = true
		}
	})
	current := ""
	if rows := m.tree.rows(); m.tree.cursor < len(rows) && rows[m.tree.cursor].node != nil {
		current = rows[m.tree.cursor].node.ID
	}

	var build func(id string, depth int) *demo.Node
	build = func(id string, depth int) *demo.Node {
		it := s.Items[id]
		n := &demo.Node{ID: id, Name: it.Name, Dir: it.Folder, Size: it.Size, Modified: day(it.Modified), Export: exportLabel(it.Google)}
		if !it.Folder {
			n.Checked = s.Checked[id]
			n.Local = path.Base(s.Wanted[id])
			return n
		}
		on, set := s.TakeNew[id]
		n.New = !set || on
		n.Open = open[id] || (first && depth == 0)
		for _, child := range s.Children[id] {
			n.Children = append(n.Children, build(child, depth+1))
		}
		return n
	}
	var root []*demo.Node
	for _, id := range s.Top {
		root = append(root, build(id, 0))
	}
	on, set := s.TakeNew[""]
	m.tree.root, m.tree.newTop = root, !set || on
	m.tree.cursor = 0
	for i, r := range m.tree.rows() {
		if r.node != nil && (current == "" || r.node.ID == current) {
			m.tree.cursor = i
			break
		}
	}
}

// saveSelection records what the tree screen shows as the user's choice.
func (m *Model) saveSelection() error {
	checked, takeNew := map[string]bool{}, map[string]bool{"": m.tree.newTop}
	eachNode(m.tree.root, func(n *demo.Node) {
		if n.Dir {
			takeNew[n.ID] = n.New
		} else {
			checked[n.ID] = n.Checked
		}
	})
	return m.lv.eng.SetSelection(checked, takeNew)
}

// loadKept reads the trash and the SKIP list from the state.
func (m *Model) loadKept() {
	entries := func(list []state.Kept, err error) []demo.Entry {
		var out []demo.Entry
		for _, k := range list {
			out = append(out, demo.Entry{ID: k.ID, Stored: k.Stored, Path: k.Path, Size: k.Size,
				Created: day(k.Created), Modified: day(k.Modified), Added: day(k.Added), Reason: k.Reason})
		}
		return out
	}
	store := m.lv.eng.Store()
	m.trash.items = entries(store.Trashed())
	m.skip.items = entries(store.Skipped())
	m.trash.clamp()
	m.skip.clamp()
}

// keptAct does to real files what the trash or the SKIP list was asked,
// then reads the lists again.
func (m *Model) keptAct(action string, entries []demo.Entry) error {
	var failed error
	for _, e := range entries {
		k := state.Kept{ID: e.ID, Path: e.Path, Stored: e.Stored}
		var err error
		switch action {
		case "restore":
			_, err = m.lv.eng.Restore(k)
		case "delete":
			err = m.lv.eng.DeleteForever(k)
		case "unskip":
			err = m.lv.eng.Unskip(k)
		}
		if err != nil && failed == nil {
			failed = err
		}
	}
	m.loadKept()
	m.lv.info = describe(m.lv.dest)
	return failed
}

// applySettings makes the interface look and behave as the settings say.
func (m *Model) applySettings(s config.Settings) {
	for i, l := range langs() {
		if l == s.Language {
			m.setLang(i)
		}
	}
	if i, ok := theme.Index(s.Palette); ok {
		m.scheme = i
	}
	if slices.Contains(themeModes, s.Variant) {
		m.themeMode = s.Variant
	}
	m.paintBg, m.askDest = s.PaintBackground, s.AskDestination
	m.applyTheme()
	m.lv.settings = s
}

// persist saves what the user set, to the computer and to the drive.
func (m *Model) persist() {
	lv := m.lv
	if lv == nil {
		return
	}
	lv.settings.Language = m.tr.Code
	lv.settings.Palette = theme.Schemes[m.scheme].ID
	lv.settings.Variant = m.themeMode
	lv.settings.PaintBackground, lv.settings.AskDestination = m.paintBg, m.askDest
	dest := lv.dest
	if lv.eng == nil {
		dest = "" // nothing has been set up in that folder yet
	}
	if err := config.Save(lv.home, dest, lv.settings); err != nil {
		m.flash = err.Error()
		return
	}
	if s, _, err := config.Load(lv.home, dest); err == nil {
		lv.settings.Destinations = s.Destinations
	}
}

// listDests collects the folders to offer: the one in use, those this
// computer has synced to, and connected drives that hold a copy.
func (lv *live) listDests() []demo.Destination {
	var paths []string
	add := func(p string) {
		if p != "" && !slices.Contains(paths, p) {
			paths = append(paths, p)
		}
	}
	add(lv.dest)
	for _, p := range lv.settings.Destinations {
		add(p)
	}
	for _, p := range lv.volumes() {
		add(p)
	}
	var out []demo.Destination
	for _, p := range paths {
		out = append(out, describe(p))
	}
	return out
}

// askWhere opens the question where the copy goes.
func (m Model) askWhere(purpose int) Model {
	m.whereFor = purpose
	m.where = 0
	if m.lv != nil {
		m.lv.dests = m.lv.listDests()
		for i, d := range m.lv.dests {
			if d.Path == m.lv.dest {
				m.where = i
			}
		}
	} else {
		m.where = m.dest
	}
	m.screen = scrWhere
	return m
}

// useDest makes a folder the destination and goes on with what the
// question was asked for.
func (m Model) useDest(dest string) (Model, tea.Cmd) {
	lv := m.lv
	if describe(dest).Missing {
		m.flash = m.tr.T("where.missing")
		return m, nil
	}
	previous := lv.dest
	lv.dest = dest
	if s, fromDrive, err := config.Load(lv.home, dest); err == nil && fromDrive {
		m.applySettings(s)
	}
	if err := m.openEngine(); err != nil {
		lv.dest = previous
		m.openEngine()
		m.flash = m.tr.T("where.failed", err.Error())
		return m, nil
	}
	m.persist()
	switch m.whereFor {
	case whereSettings:
		m.screen = scrDest
		return m, nil
	case whereFirstRun:
		return m.listThenTree()
	}
	return m.planSync()
}

// askOtherFolder asks for the path of a folder that is not on the list.
func (m Model) askOtherFolder() Model {
	tr := m.tr
	return m.ask(prompt{title: tr.T("where.title"), text: tr.T("where.other.text"), label: tr.T("dest.path"),
		done: func(m Model, value string) (Model, tea.Cmd) {
			dest := cleanPath(value)
			if !filepath.IsAbs(dest) {
				return m.refuse(m.tr.T("where.abs")), nil
			}
			m.prompt = nil
			return m.useDest(dest)
		}})
}

// wait shows that the program is busy until a message arrives.
func (m Model) wait(text string, work func(ctx context.Context) tea.Msg) (Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	m.busy, m.cancel = text, cancel
	return m, func() tea.Msg { return work(ctx) }
}

// listThenTree asks Drive what is there and opens the choice of files.
func (m Model) listThenTree() (Model, tea.Cmd) {
	eng := m.lv.eng
	if eng == nil {
		m.screen = scrMenu
		return m, nil
	}
	return m.wait(m.tr.T("busy.listing"), func(ctx context.Context) tea.Msg {
		snap, err := eng.Refresh(ctx)
		return listedMsg{snap, err}
	})
}

func (m Model) onListed(msg listedMsg) (Model, tea.Cmd) {
	m.busy, m.cancel = "", nil
	switch {
	case msg.err == nil:
		m.setTree(msg.snap)
	case errors.Is(msg.err, context.Canceled):
		m.screen = scrMenu
		return m, nil
	default:
		// No connection: the last look is still worth choosing from.
		m.reload()
		if len(m.tree.root) == 0 {
			m.failure = m.tr.T("fail.drive", msg.err.Error())
			return m, nil
		}
		m.flash = m.tr.T("offline")
	}
	if len(m.tree.root) == 0 {
		m.failure = m.tr.T("acc.check.none")
		return m, nil
	}
	m.tree.remember()
	m.screen = scrTree
	if m.whereFor == whereFirstRun {
		m.whereFor = whereSync
		if m.flash == "" {
			m.flash = m.tr.T("wiz.done")
		}
	}
	return m, nil
}

// planSync looks at Drive and at the drive, and shows the checks.
func (m Model) planSync() (Model, tea.Cmd) {
	eng := m.lv.eng
	if eng == nil {
		m.run = runState{pre: preflight{noDrive: true}, active: -1}
		m.screen = scrPreflight
		return m, nil
	}
	return m.wait(m.tr.T("busy.looking"), func(ctx context.Context) tea.Msg {
		plan, err := eng.Plan(ctx)
		return plannedMsg{plan, err}
	})
}

func (m Model) onPlanned(msg plannedMsg) (Model, tea.Cmd) {
	m.busy, m.cancel = "", nil
	if msg.err != nil {
		if errors.Is(msg.err, context.Canceled) {
			m.screen = scrMenu
			return m, nil
		}
		m.failure = m.tr.T("fail.drive", msg.err.Error())
		return m, nil
	}
	lv := m.lv
	lv.plan = msg.plan
	lv.info = describe(lv.dest)
	if snap := lv.eng.Latest(); snap != nil {
		m.setTree(snap)
	}
	m.run = m.planRun(msg.plan)
	m.screen = scrPreflight
	return m, nil
}

// planRun turns a plan into what the sync screens show.
func (m Model) planRun(p *engine.Plan) runState {
	r := runState{active: -1, total: p.Bytes, leftN: p.NewUnselected.Files, leftBytes: p.NewUnselected.Bytes}
	for _, l := range p.Leftovers {
		size := l.Item.Size
		if l.Item.Copy != nil {
			size = l.Item.Copy.Size
		}
		r.deleted = append(r.deleted, demo.Deleted{ID: l.Item.ID, Path: l.Item.Path, Size: size,
			Created: day(l.Item.Created), Modified: day(l.Item.Modified)})
	}
	r.actions = make([]int, len(r.deleted))
	for i := range r.actions {
		r.actions[i] = actTrash
	}
	for _, d := range p.Downloads {
		r.queue = append(r.queue, demo.Change{Path: d.Target, Size: d.Size, Updated: d.Why != engine.ForNew, Local: d.Why == engine.ForEdited})
	}
	r.state = make([]int, len(r.queue))

	folders := 0
	for _, n := range m.tree.root {
		if n.Dir {
			folders++
		}
	}
	r.pre = preflight{
		noDrive: !p.Writable, folders: folders, need: p.Bytes, free: p.Volume.Free, fits: p.Fits(),
		clashes: p.Clashes, moves: len(p.Moves), lostFiles: p.LostFiles.Files, lostBytes: p.LostFiles.Bytes,
	}
	for _, lost := range p.Lost {
		if r.pre.lostName != "" {
			r.pre.lostName += ", "
		}
		r.pre.lostName += lost.Name
	}
	for _, reason := range []string{engine.NotShortcut, engine.NotUnsupported, engine.NotService, engine.NotForbidden, engine.NotTooLarge} {
		if t := p.NotCopied[reason]; t.Files > 0 {
			r.pre.notCopied = append(r.pre.notCopied, notCopiedRow{reason, t.Files})
		}
	}
	return r
}

// monitor adds up the events of a run. The run reports from several
// goroutines; the screen reads the sums on every tick.
type monitor struct {
	mu       sync.Mutex
	index    map[string]int // target → place in the queue
	state    []int
	got      []int64
	bytes    int64
	finished int
	began    time.Time
}

func newMonitor(queue []demo.Change) *monitor {
	mon := &monitor{index: map[string]int{}, state: make([]int, len(queue)), got: make([]int64, len(queue)), began: time.Now()}
	for i, c := range queue {
		mon.index[c.Path] = i
	}
	return mon
}

func (mon *monitor) event(ev engine.Event) {
	mon.mu.Lock()
	defer mon.mu.Unlock()
	i, ok := mon.index[ev.Target]
	if !ok {
		return
	}
	switch ev.Kind {
	case engine.Started:
		mon.state[i] = stRunning
	case engine.Advanced:
		mon.got[i] += ev.Bytes
		mon.bytes += ev.Bytes
	case engine.Finished:
		mon.state[i] = stOK
		mon.finished++
	case engine.Failed:
		mon.state[i] = stFailed
		mon.finished++
	case engine.LeftOut:
		mon.state[i] = stSkipped
		mon.finished++
	}
}

// fill copies how far the run is into what the progress screen draws.
func (mon *monitor) fill(r *runState) {
	mon.mu.Lock()
	defer mon.mu.Unlock()
	copy(r.state, mon.state)
	r.done = min(mon.bytes, r.total)
	r.finished = mon.finished
	r.active = -1
	for i, st := range mon.state {
		if st == stRunning {
			r.active, r.activeDone = i, min(mon.got[i], r.queue[i].Size)
			break
		}
	}
	if secs := time.Since(mon.began).Seconds(); secs > 0.2 {
		r.speed = int64(float64(mon.bytes) / secs)
	}
}

// startLiveRun carries out the plan in the background.
func (m Model) startLiveRun() (Model, tea.Cmd) {
	lv := m.lv
	actions := map[string]int{}
	for i, d := range m.run.deleted {
		actions[d.ID] = m.run.actions[i]
	}
	decide := func(l engine.Leftover) int {
		switch actions[l.Item.ID] {
		case actDelete:
			return engine.ToDelete
		case actSkip:
			return engine.ToSkip
		}
		return engine.ToTrash
	}
	lv.mon = newMonitor(m.run.queue)
	ctx, cancel := context.WithCancel(context.Background())
	lv.stopRun = cancel
	eng, plan, mon := lv.eng, lv.plan, lv.mon
	m.screen = scrProgress
	m.run.started = time.Now()
	return m, tea.Batch(tick(), func() tea.Msg {
		res, err := eng.Run(ctx, plan, decide, mon.event)
		return ranMsg{res, err}
	})
}

func (m Model) onRan(msg ranMsg) (Model, tea.Cmd) {
	lv := m.lv
	if lv.mon != nil {
		lv.mon.fill(&m.run)
	}
	lv.mon, lv.stopRun, lv.plan = nil, nil, nil
	m.run.result = msg.res
	m.run.took = time.Since(m.run.started)
	m.run.stopping = false
	if msg.res != nil {
		m.run.stopped = msg.res.Stopped
	}
	if msg.err != nil {
		m.flash = msg.err.Error()
	}
	m.reload()
	lv.info = describe(lv.dest)
	m.persist()
	m.run.btn = 0
	m.screen = scrSummary
	return m, nil
}

// checkAccess asks Drive what the account can see, without touching any
// destination: it works before one is chosen.
func (m Model) checkAccess() (Model, tea.Cmd) {
	lv := m.lv
	if lv.key == nil {
		m.flash = m.tr.T("acc.none")
		return m, nil
	}
	remote := lv.connect(lv.key)
	return m.wait(m.tr.T("busy.access"), func(ctx context.Context) tea.Msg {
		files, err := remote.List(ctx)
		if err != nil {
			return accessMsg{err: err}
		}
		ids := map[string]bool{}
		for _, f := range files {
			if f.IsFolder() {
				ids[f.ID] = true
			}
		}
		var msg accessMsg
		for _, f := range files {
			switch {
			case !f.IsFolder():
				msg.files++
			case !ids[f.Parent]:
				msg.folders = append(msg.folders, f.Name) // shared with the account directly
			}
		}
		slices.Sort(msg.folders)
		return msg
	})
}

func (m Model) onAccess(msg accessMsg) (Model, tea.Cmd) {
	m.busy, m.cancel = "", nil
	tr := m.tr
	switch {
	case errors.Is(msg.err, context.Canceled):
	case msg.err != nil:
		m.flash = tr.T("fail.drive", msg.err.Error())
	case len(msg.folders) == 0 && msg.files == 0:
		m.lv.access, m.lv.checked = nil, true
		m.flash = tr.T("acc.check.none")
	default:
		m.lv.access, m.lv.checked = msg.folders, true
		m.flash = tr.T("acc.check.ok", tr.N("n.folders", len(msg.folders)), tr.N("n.files", msg.files))
	}
	return m, nil
}

// useKey takes a key into use and reopens the destination with it.
func (m *Model) useKey(key *drive.Key, data []byte) error {
	m.lv.key, m.lv.keyData = key, data
	m.lv.access, m.lv.checked = nil, false
	return m.openEngine()
}

// askKeyFile asks for the key file and imports it. then says where to go
// once the key is in.
func (m Model) askKeyFile(then func(Model) (Model, tea.Cmd)) Model {
	tr := m.tr
	return m.ask(prompt{title: tr.T("acc.key.title"), text: tr.T("acc.key.text"), label: tr.T("wiz.key.label"),
		done: func(m Model, value string) (Model, tea.Cmd) {
			if _, err := config.ImportKey(m.lv.home, cleanPath(value)); err != nil {
				return m.refuse(err.Error()), nil
			}
			key, data, err := config.LoadKey(m.lv.home)
			if err != nil {
				return m.refuse(err.Error()), nil
			}
			if err := m.useKey(key, data); err != nil {
				return m.refuse(err.Error()), nil
			}
			m.prompt = nil
			return then(m)
		}})
}

// askSealedPassword asks for the password of the key kept on the drive.
// It is how a drive plugged into another computer gets to work at once.
func (m Model) askSealedPassword() Model {
	tr := m.tr
	return m.ask(prompt{title: tr.T("acc.pass.open.title"), text: tr.T("acc.pass.open.text"), label: tr.T("acc.pass.label"), secret: true,
		done: func(m Model, value string) (Model, tea.Cmd) {
			key, data, err := config.OpenSealedKey(m.lv.dest, value)
			if errors.Is(err, config.ErrWrongPassword) {
				return m.refuse(m.tr.T("acc.pass.wrong")), nil
			}
			if err != nil {
				return m.refuse(err.Error()), nil
			}
			if err := m.useKey(key, data); err != nil {
				return m.refuse(err.Error()), nil
			}
			m.prompt = nil
			m.screen = scrMenu
			return m, nil
		}})
}

// askSealPassword asks twice for a password and puts the encrypted key on
// the drive.
func (m Model) askSealPassword() Model {
	tr := m.tr
	return m.ask(prompt{title: tr.T("acc.pass.title"), text: tr.T("acc.pass.text"), label: tr.T("acc.pass.label"), secret: true,
		done: func(m Model, first string) (Model, tea.Cmd) {
			tr := m.tr
			return m.ask(prompt{title: tr.T("acc.pass.title"), text: tr.T("acc.pass.text"), label: tr.T("acc.pass.again"), secret: true,
				done: func(m Model, second string) (Model, tea.Cmd) {
					if first != second {
						return m.refuse(m.tr.T("acc.pass.differ")), nil
					}
					m.prompt = nil
					if err := config.SealKey(m.lv.dest, m.lv.keyData, first); err != nil {
						m.flash = err.Error()
					} else {
						m.flash = m.tr.T("acc.sealed")
					}
					return m, nil
				}}), nil
		}})
}
