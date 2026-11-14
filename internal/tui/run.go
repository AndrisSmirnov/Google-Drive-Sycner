package tui

import (
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"syncer-gdrive/internal/demo"
	"syncer-gdrive/internal/format"
)

// What to do with a file deleted on Drive.
const (
	actDelete = iota
	actSkip
	actTrash
)

// State of a queued file during the run.
const (
	stPending = iota
	stOK
	stSkipped
	stFailed
)

// The demo run moves this much per 100 ms tick.
const bytesPerTick = 14 * format.Mb

// runState is one pass of "Sync now": check, deleted files, progress, summary.
type runState struct {
	btn int // focused button on the check and summary screens

	deleted []demo.Deleted
	actions []int
	delIdx  int

	queue   []demo.Change
	state   []int
	cur     int   // file being downloaded
	curDone int64 // bytes of that file
	done    int64
	total   int64

	// New files on Drive that the selection does not take.
	leftN     int
	leftBytes int64

	stopped bool
	started time.Time
	took    time.Duration
}

// newRun queues what the sync would download. A file that is new on Drive
// is taken only if the selection says so (treeState.takesNew).
func (m Model) newRun() runState {
	r := runState{deleted: demo.DeletedOnDrive()}
	for _, c := range demo.Changes() {
		if !c.Updated && !m.tree.takesNew(c.Path) {
			r.leftN++
			r.leftBytes += c.Size
			continue
		}
		r.queue = append(r.queue, c)
	}
	r.actions = make([]int, len(r.deleted))
	for i := range r.actions {
		r.actions[i] = actTrash
	}
	r.state = make([]int, len(r.queue))
	for _, c := range r.queue {
		if c.Outcome == demo.OutcomeOK {
			r.total += c.Size
		}
	}
	return r
}

func (r runState) deletedBytes() int64 {
	var sum int64
	for _, d := range r.deleted {
		sum += d.Size
	}
	return sum
}

// --- check before the run ---

func (m Model) viewPreflight() ([]string, string) {
	th, tr, r := m.th, m.tr, m.run
	dest := m.destination()
	out := m.head(tr.T("pre.title"), "Drive → "+dest.Path)
	check := func(mark string, fg lipgloss.Color, label, detail string, failed bool) string {
		text := t(label, th.Fg).w(32)
		if failed {
			text = t(label, fg).b().w(32)
		}
		return m.line(t(" "+mark+" ", fg), text, t(detail, th.Dim))
	}
	hints := m.hints("← →", tr.T("hint.button"), "Enter", tr.T("hint.confirm"), "Esc", tr.T("hint.back"))

	if m.opts.Demo == "no-drive" {
		out = append(out, check("×", th.Err, tr.T("pre.nodrive"), dest.Path, true), m.blank())
		out = append(out, m.note(th.Err, tr.T("pre.nodrive.text"), tr.T("pre.nodrive.note"))...)
		out = append(out, m.blank(), m.buttons(0, tr.T("pre.back")))
		return out, hints
	}

	out = append(out, check("✓", th.Ok, tr.T("pre.drive.ok"), dest.Path, false))
	if m.opts.Demo == "lost-access" {
		detail := tr.T("pre.lost.detail", demo.Lost.Name, format.Size(demo.Lost.Size))
		out = append(out, check("!", th.Warn, tr.T("pre.lost"), detail, true), m.blank())
		out = append(out, m.note(th.Warn, tr.T("pre.lost.text"), tr.T("pre.lost.note"))...)
		out = append(out, m.blank(), m.buttons(r.btn, tr.T("pre.rest"), tr.T("pre.cancel")))
		return out, hints
	}

	folders := 0
	for _, n := range m.tree.root {
		if n.Dir {
			folders++
		}
	}
	out = append(out,
		check("✓", th.Ok, tr.T("pre.access.ok"), tr.N("n.folders", folders), false),
		check("✓", th.Ok, tr.T("pre.space.ok"), tr.T("pre.space.detail", format.Size(r.total), format.Size(dest.Free)), false),
	)
	// Selected files that share a name in one folder: the sync goes on, but
	// says so before it starts, in case the choice was made long ago.
	if n := len(m.tree.clashes()); n > 0 {
		out = append(out, check("!", th.Warn, tr.T("pre.clash"), tr.T("pre.clash.detail", tr.N("n.files", n)), true))
	}
	out = append(out,
		m.blank(),
		m.line(t(" "+tr.T("pre.changes"), th.Dim)),
	)

	var newN, updN, localN int
	var newB, updB int64
	for _, c := range r.queue {
		if c.Local {
			localN++
		}
		if c.Updated {
			updN++
			updB += c.Size
		} else {
			newN++
			newB += c.Size
		}
	}
	change := func(mark string, fg lipgloss.Color, label, amount, note string) string {
		return m.line(t("   "+mark+" ", fg), t(label, th.Fg).w(22), t(amount, th.Fg).w(22), t(note, th.Dim))
	}
	out = append(out,
		change("+", th.Ok, tr.T("pre.new"), tr.N("n.files", newN)+" · "+format.Size(newB), ""),
	)
	if r.leftN > 0 {
		out = append(out, change("·", th.Dim, tr.T("pre.left"), tr.N("n.files", r.leftN)+" · "+format.Size(r.leftBytes), tr.T("pre.left.note")))
	}
	out = append(out,
		change("~", th.Second, tr.T("pre.updated"), tr.N("n.files", updN)+" · "+format.Size(updB), ""),
		change("−", th.Warn, tr.T("pre.deleted"), tr.N("n.files", len(r.deleted))+" · "+format.Size(r.deletedBytes()), tr.T("pre.deleted.note")),
		change("≠", th.Warn, tr.T("pre.local"), tr.N("n.files", localN), tr.T("pre.local.note")),
		m.blank(),
		m.buttons(r.btn, tr.T("pre.start"), tr.T("pre.cancel")),
	)
	return out, hints
}

func (m Model) keyPreflight(key string) (tea.Model, tea.Cmd) {
	blocked := m.opts.Demo == "no-drive"
	switch key {
	case "left", "h", "right", "l", "tab":
		if !blocked {
			m.run.btn = 1 - m.run.btn
		}
	case "esc":
		m.screen = scrMenu
	case "enter":
		if blocked || m.run.btn == 1 {
			m.screen = scrMenu
			return m, nil
		}
		if len(m.run.deleted) > 0 {
			m.screen = scrDeleted
			return m, nil
		}
		return m.startRun()
	}
	return m, nil
}

// --- files deleted on Drive ---

func (m Model) actionCell(action int) cell {
	switch action {
	case actDelete:
		return t(m.tr.T("del.act.delete.short"), m.th.Err)
	case actSkip:
		return t(m.tr.T("del.act.skip.short"), m.th.Warn)
	default:
		return t(m.tr.T("del.act.trash.short"), m.th.Second)
	}
}

func (m Model) viewDeleted() ([]string, string) {
	th, tr, r := m.th, m.tr, m.run
	width := m.cw() - 4
	row := func(bg lipgloss.TerminalColor, cells ...cell) string { return renderRow(bg, width, cells) }

	inner := []string{
		row(th.Bg, t(tr.T("del.title"), th.Warn).b(),
			t(" · "+tr.N("n.files", len(r.deleted))+" · "+format.Size(r.deletedBytes()), th.Dim)),
		row(th.Bg, t(tr.T("del.text"), th.Fg)),
		row(th.Bg),
	}
	for i, d := range r.deleted {
		gap := t("", th.Dim).flex()
		size := t(format.Size(d.Size), th.Fg).w(9).r()
		action := m.actionCell(r.actions[i]).w(12).r()
		if i == r.delIdx {
			inner = append(inner, row(th.Panel, t("› ", th.Accent).b(), t(d.Path, th.Accent).b(), gap, size, action, t(" ", th.Dim)))
		} else {
			inner = append(inner, row(th.Bg, t("  ", th.Fg), t(d.Path, th.Fg), gap, size, action, t(" ", th.Dim)))
		}
	}
	inner = append(inner, row(th.Bg), row(th.Bg, t(tr.T("del.legend"), th.Dim)))
	legend := []struct {
		key string
		fg  lipgloss.Color
	}{{"del.act.delete", th.Err}, {"del.act.skip", th.Warn}, {"del.act.trash", th.Second}}
	for i, l := range legend {
		inner = append(inner, row(th.Bg,
			t(strconv.Itoa(i+1)+"  ", th.Second), t(tr.T(l.key), l.fg).w(26), t(tr.T(l.key+".note"), th.Dim)))
	}

	out := m.box(th.Warn, inner)
	return out, m.hints("↑↓", tr.T("hint.file"), "1 2 3", tr.T("hint.action"), "a", tr.T("hint.all"),
		"Enter", tr.T("hint.apply"), "Esc", tr.T("hint.back"))
}

func (m Model) keyDeleted(key string) (tea.Model, tea.Cmd) {
	r := &m.run
	switch key {
	case "up", "k", "down", "j":
		r.delIdx = move(r.delIdx, len(r.deleted), key)
	case "1", "2", "3":
		r.actions[r.delIdx] = int(key[0] - '1')
	case "a":
		for i := range r.actions {
			r.actions[i] = r.actions[r.delIdx]
		}
		m.flash = m.tr.T("del.all")
	case "esc":
		m.screen = scrPreflight
	case "enter":
		m.applyDeleted()
		return m.startRun()
	}
	return m, nil
}

// applyDeleted moves the deleted files into SKIP or the trash as chosen.
func (m *Model) applyDeleted() {
	for i, d := range m.run.deleted {
		switch m.run.actions[i] {
		case actSkip:
			m.skip.items = append(m.skip.items, demo.Entry{
				Path: d.Path, Size: d.Size, Created: d.Created, Modified: d.Modified, Added: demo.Today,
			})
		case actTrash:
			m.trash.items = append(m.trash.items, demo.Entry{
				Path: d.Path, Size: d.Size, Created: d.Created, Modified: d.Modified,
				Added: demo.Today, Reason: "drive",
			})
		}
	}
}

// --- progress ---

func (m Model) startRun() (tea.Model, tea.Cmd) {
	m.screen = scrProgress
	m.run.started = time.Now()
	return m, tick()
}

// onTick advances the simulated download by one step.
func (m Model) onTick() (tea.Model, tea.Cmd) {
	if m.screen != scrProgress {
		return m, nil
	}
	r := &m.run
	budget := bytesPerTick
	for budget > 0 && r.cur < len(r.queue) {
		c := r.queue[r.cur]
		if c.Outcome != demo.OutcomeOK {
			r.state[r.cur] = stSkipped
			if c.Outcome == demo.OutcomeNetwork {
				r.state[r.cur] = stFailed
			}
			r.cur++
			continue
		}
		left := c.Size - r.curDone
		if left > budget {
			r.curDone += budget
			r.done += budget
			break
		}
		r.done += left
		budget -= left
		r.state[r.cur] = stOK
		r.cur++
		r.curDone = 0
	}
	if r.cur >= len(r.queue) {
		return m.finishRun(false)
	}
	return m, tick()
}

func (m Model) finishRun(stopped bool) (tea.Model, tea.Cmd) {
	r := &m.run
	r.stopped = stopped
	r.took = time.Since(r.started)
	r.btn = 0
	// A file edited by hand on the drive: its old version goes to the trash.
	for i, c := range r.queue {
		if c.Local && r.state[i] == stOK {
			m.trash.items = append(m.trash.items, demo.Entry{
				Path: c.Path, Size: c.Size, Created: "2026-07-15", Modified: "2026-11-12",
				Added: demo.Today, Reason: "local",
			})
		}
	}
	m.screen = scrSummary
	return m, nil
}

func (m Model) viewProgress() ([]string, string) {
	th, tr, r := m.th, m.tr, m.run
	out := m.head(tr.T("run.title"), "Drive → "+m.destination().Path)

	barW := max(m.cw()-8-6-2-36, 10)
	bar := func(label string, fg lipgloss.Color, done, total int64, detail ...cell) string {
		pct := 100
		if total > 0 {
			pct = int(done * 100 / total)
		}
		fill := barW * pct / 100
		cells := []cell{
			t(label, th.Dim).w(8),
			t(strings.Repeat("━", fill), fg), t(strings.Repeat("━", barW-fill), th.Line),
			t(strconv.Itoa(pct)+"%", th.Fg).b().w(6).r(), t("  ", th.Dim),
		}
		return m.line(append(cells, detail...)...)
	}
	out = append(out, bar(tr.T("run.total"), th.Accent, r.done, r.total,
		t(tr.T("run.of", format.Size(r.done), format.Size(r.total)), th.Fg)))
	if r.cur < len(r.queue) {
		c := r.queue[r.cur]
		out = append(out, bar(tr.T("run.file"), th.Second, r.curDone, c.Size,
			t(tr.T("run.of", format.Size(r.curDone), format.Size(c.Size)), th.Fg),
			t(" · "+baseName(c.Path), th.Dim)))
	}

	speed := bytesPerTick * 10 // per second
	eta := tr.T("eta.sec", int((r.total-r.done)/speed)+1)
	if secs := (r.total - r.done) / speed; secs >= 60 {
		eta = tr.T("eta.min", int(secs/60))
	}
	out = append(out, m.blank(), m.line(
		t(tr.T("run.files")+" ", th.Dim), t(tr.T("run.files.val", r.cur, len(r.queue)), th.Fg).b(),
		t("     "+tr.T("run.speed")+" ", th.Dim), t(format.Size(speed)+"/s", th.Fg).b(),
		t("     "+tr.T("run.left")+" ", th.Dim), t(eta, th.Fg).b(),
	), m.rule())

	for i, c := range r.queue {
		size := format.Size(c.Size)
		switch {
		case r.state[i] == stOK:
			out = append(out, m.line(t(" ✓ ", th.Ok), t(c.Path, th.Fg), m.gap(), t(size+" ", th.Dim)))
		case r.state[i] == stSkipped:
			out = append(out, m.line(t(" ! ", th.Warn), t(c.Path, th.Fg), m.gap(), t(tr.T("run.toobig")+" ", th.Warn)))
		case r.state[i] == stFailed:
			out = append(out, m.line(t(" × ", th.Err), t(c.Path, th.Fg), m.gap(), t(tr.T("run.network")+" ", th.Err)))
		case i == r.cur:
			out = append(out, m.line(t(" ↓ ", th.Accent), t(c.Path, th.Accent).b(), m.gap(), t(size+" ", th.Fg)))
		default:
			out = append(out, m.line(t(" · ", th.Dim), t(c.Path, th.Dim), m.gap(), t(size+" · "+tr.T("run.queued")+" ", th.Dim)))
		}
	}
	return out, m.hints("Esc", tr.T("hint.stop"))
}

func (m Model) keyProgress(key string) (tea.Model, tea.Cmd) {
	if key == "esc" {
		return m.finishRun(true)
	}
	return m, nil
}

// --- summary ---

func (m Model) viewSummary() ([]string, string) {
	th, tr, r := m.th, m.tr, m.run

	took := tr.T("dur.sec", int(r.took.Seconds()))
	if r.took >= time.Minute {
		took = tr.T("dur.min", int(r.took.Minutes()), int(r.took.Seconds())%60)
	}
	title := tr.T("sum.done")
	if r.stopped {
		title = tr.T("sum.stopped")
	}
	out := m.head(title, took)

	var newN, updN, skippedN, failedN, pendingN, localN int
	var newB, updB int64
	var skippedPath, failedPath string
	for i, c := range r.queue {
		switch r.state[i] {
		case stOK:
			if c.Local {
				localN++
			}
			if c.Updated {
				updN++
				updB += c.Size
			} else {
				newN++
				newB += c.Size
			}
		case stSkipped:
			skippedN++
			skippedPath = c.Path
		case stFailed:
			failedN++
			failedPath = c.Path
		default:
			pendingN++
		}
	}
	var byAction [3]int
	for _, a := range r.actions {
		byAction[a]++
	}

	row := func(mark string, fg lipgloss.Color, label, amount, note string, noteFg lipgloss.Color) string {
		return m.line(t(" "+mark+" ", fg), t(label, th.Fg).w(22), t(amount, th.Fg).w(22), t(note, noteFg))
	}
	out = append(out,
		row("✓", th.Ok, tr.T("sum.downloaded"), tr.N("n.files", newN)+" · "+format.Size(newB), "", th.Dim),
		row("✓", th.Ok, tr.T("sum.updated"), tr.N("n.files", updN)+" · "+format.Size(updB), "", th.Dim),
		row("→", th.Second, tr.T("sum.trash"), tr.N("n.files", byAction[actTrash]+localN), "", th.Dim),
		row("→", th.Warn, tr.T("sum.skip"), tr.N("n.files", byAction[actSkip]), "", th.Dim),
		row("−", th.Err, tr.T("sum.removed"), tr.N("n.files", byAction[actDelete]), "", th.Dim),
		row("!", th.Warn, tr.T("sum.skipped"), tr.N("n.files", skippedN), skippedPath, th.Warn),
		row("×", th.Err, tr.T("sum.failed"), tr.N("n.files", failedN), failedPath, th.Err),
	)
	if r.stopped {
		out = append(out, m.blank(), m.line(t(" "+tr.T("sum.pending", tr.N("n.files", pendingN)), th.Warn)))
	}
	out = append(out, m.blank(), m.buttons(0, tr.T("sum.ok")))
	return out, m.hints("Enter", tr.T("hint.confirm"))
}

func (m Model) keySummary(key string) (tea.Model, tea.Cmd) {
	if key == "enter" || key == "esc" {
		m.screen = scrMenu
	}
	return m, nil
}
