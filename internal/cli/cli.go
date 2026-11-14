// Package cli is the program without the menu: the commands that can be
// run from a script. The menu itself is started from here too.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"syncer-gdrive/internal/config"
	"syncer-gdrive/internal/drive"
	"syncer-gdrive/internal/engine"
	"syncer-gdrive/internal/format"
	"syncer-gdrive/internal/i18n"
	"syncer-gdrive/internal/theme"
	"syncer-gdrive/internal/tui"
	"syncer-gdrive/internal/volume"
)

// Version of the program.
const Version = "0.1.0-dev"

// Exit codes of a sync.
const (
	ExitOK         = 0 // everything planned was copied
	ExitFailures   = 1 // the sync ran, some files failed or it was stopped
	ExitNotStarted = 2 // the sync could not start
)

// connect makes the link to Google Drive. Tests replace it.
var connect = func(key *drive.Key) engine.Remote { return drive.New(key) }

// connected lists the drives that already hold a copy. Tests replace it.
var connected = volume.Destinations

// tick is how often a running sync reports its progress.
var tick = 2 * time.Second

// Main runs the program with the given arguments and returns its exit code.
func Main(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "sync":
			return syncCommand(args[1:], stdout, stderr)
		case "key":
			return keyCommand(args[1:], stdout, stderr)
		case "version":
			fmt.Fprintln(stdout, "syncer", Version)
			return ExitOK
		}
	}
	return menu(args, stderr)
}

// menu starts the interface.
func menu(args []string, stderr io.Writer) int {
	var opts tui.Options
	flags := flag.NewFlagSet("syncer", flag.ContinueOnError)
	flags.SetOutput(stderr)
	demoMode := flags.String("demo", "", "run on made-up data instead of Google Drive: data, no-drive or lost-access")
	flags.BoolVar(&opts.FirstRun, "first-run", false, "with -demo: start with the first-run steps")
	flags.StringVar(&opts.Lang, "lang", "", "interface language: uk, en, pl, ja or zh")
	flags.StringVar(&opts.Theme, "theme", "", "light or dark variant: auto, dark or light")
	var ids string
	for i, s := range theme.Schemes {
		if i > 0 {
			ids += ", "
		}
		ids += s.ID
	}
	flags.StringVar(&opts.Palette, "palette", "", "colour scheme: "+ids)
	flags.Usage = func() {
		fmt.Fprintln(stderr, "syncer                    the menu\nsyncer sync [-dest DIR]   sync without the menu\nsyncer key FILE           import a Service Account key\nsyncer version\n\nOptions of the menu:")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return ExitNotStarted
	}
	opts.Version = Version
	var model tui.Model
	var err error
	if *demoMode != "" || opts.FirstRun {
		// Made-up data: nothing reads Drive or writes to a drive.
		if *demoMode != "data" {
			opts.Demo = *demoMode
		}
		model, err = tui.New(opts)
	} else {
		var home string
		if home, err = config.Home(); err == nil {
			model, err = tui.NewLive(opts, tui.LiveOptions{Home: home, Connect: connect, Volumes: connected})
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, "syncer:", err)
		return ExitNotStarted
	}
	final, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	if m, ok := final.(tui.Model); ok {
		m.Close()
	}
	if err != nil {
		fmt.Fprintln(stderr, "syncer:", err)
		return ExitFailures
	}
	return ExitOK
}

// texts returns the catalog of the language the user has chosen.
func texts(home string) *i18n.Catalog {
	settings, _, _ := config.Load(home, "")
	if tr, err := i18n.Load(settings.Language); err == nil {
		return tr
	}
	tr, _ := i18n.Load("uk")
	return tr
}

// keyCommand imports a Service Account key: syncer key FILE.
func keyCommand(args []string, stdout, stderr io.Writer) int {
	home, err := config.Home()
	if err != nil {
		fmt.Fprintln(stderr, "syncer:", err)
		return ExitNotStarted
	}
	tr := texts(home)
	if len(args) != 1 {
		fmt.Fprintln(stderr, tr.T("cli.key.usage"))
		return ExitNotStarted
	}
	key, err := config.ImportKey(home, args[0])
	if err != nil {
		fmt.Fprintln(stderr, "syncer:", err)
		return ExitNotStarted
	}
	fmt.Fprintln(stdout, tr.T("cli.key.done", key.Email))
	return ExitOK
}

// syncCommand syncs without the menu: syncer sync [-dest DIR].
func syncCommand(args []string, stdout, stderr io.Writer) int {
	home, err := config.Home()
	if err != nil {
		fmt.Fprintln(stderr, "syncer:", err)
		return ExitNotStarted
	}
	tr := texts(home)
	flags := flag.NewFlagSet("syncer sync", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dest := flags.String("dest", "", tr.T("cli.flag.dest"))
	if err := flags.Parse(args); err != nil {
		return ExitNotStarted
	}
	fail := func(text string) int {
		fmt.Fprintln(stderr, text)
		return ExitNotStarted
	}

	key, _, err := config.LoadKey(home)
	if errors.Is(err, config.ErrNoKey) {
		return fail(tr.T("cli.nokey"))
	}
	if err != nil {
		return fail("syncer: " + err.Error())
	}
	destination, err := chooseDestination(home, *dest)
	if err != nil {
		return fail(tr.T("cli.nodest"))
	}
	settings, _, err := config.Load(home, destination)
	if err != nil {
		return fail("syncer: " + err.Error())
	}
	if loaded, err := i18n.Load(settings.Language); err == nil {
		tr = loaded // the drive may bring its own language
	}

	e, err := engine.Open(destination, connect(key))
	if err != nil {
		return fail("syncer: " + err.Error())
	}
	defer e.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	fmt.Fprintln(stdout, tr.T("cli.looking", key.Email))
	plan, err := e.Plan(ctx)
	if err != nil {
		return fail("syncer: " + err.Error())
	}
	printPlan(stdout, tr, destination, plan)
	if !plan.Writable {
		return fail(tr.T("cli.readonly", destination))
	}
	if !plan.Fits() {
		return fail(tr.T("cli.nospace", format.Size(plan.Bytes), format.Size(plan.Volume.Free)))
	}

	// Without the menu nobody can be asked: leftovers go into the trash,
	// from where they can be restored.
	progress := newProgress(stdout, tr, plan)
	done := progress.start()
	result, err := e.Run(ctx, plan, nil, progress.event)
	done()
	if err != nil {
		fmt.Fprintln(stderr, "syncer:", err)
		return ExitFailures
	}
	printResult(stdout, tr, result)
	if err := config.Save(home, destination, settings); err != nil {
		fmt.Fprintln(stderr, "syncer:", err)
	}
	if result.Stopped || len(result.Failures) > 0 {
		return ExitFailures
	}
	return ExitOK
}

// chooseDestination finds where to sync: the folder named on the command
// line, else a connected drive that already holds a copy, else the folder
// this computer synced to last.
func chooseDestination(home, named string) (string, error) {
	if named != "" {
		return named, nil
	}
	if found := connected(); len(found) == 1 {
		return found[0], nil
	}
	settings, _, _ := config.Load(home, "")
	for _, known := range settings.Destinations {
		if info, err := os.Stat(known); err == nil && info.IsDir() {
			return known, nil
		}
	}
	return "", errors.New("no destination")
}

func printPlan(w io.Writer, tr *i18n.Catalog, destination string, p *engine.Plan) {
	fmt.Fprintln(w, tr.T("cli.dest", destination, p.Volume.Label(), format.Size(p.Volume.Free)))
	var fresh, again engine.Tally
	for _, d := range p.Downloads {
		t := &again
		if d.Why == engine.ForNew {
			t = &fresh
		}
		t.Files++
		t.Bytes += d.Size
	}
	line := func(key string, files int, bytes int64) {
		if files > 0 {
			fmt.Fprintln(w, "  "+tr.T(key, tr.N("n.files", files), format.Size(bytes)))
		}
	}
	line("cli.plan.new", fresh.Files, fresh.Bytes)
	line("cli.plan.again", again.Files, again.Bytes)
	if n := len(p.Moves); n > 0 {
		fmt.Fprintln(w, "  "+tr.T("cli.plan.moves", tr.N("n.files", n)))
	}
	if n := len(p.Leftovers); n > 0 {
		fmt.Fprintln(w, "  "+tr.T("cli.plan.leftovers", tr.N("n.files", n)))
	}
	for _, lost := range p.Lost {
		fmt.Fprintln(w, "  "+tr.T("cli.plan.lost", lost.Name))
	}
	if p.Clashes > 0 {
		fmt.Fprintln(w, "  "+tr.T("cli.plan.clashes", tr.N("n.files", p.Clashes)))
	}
	line("cli.plan.unselected", p.NewUnselected.Files, p.NewUnselected.Bytes)
	printNotCopied(w, tr, p.NotCopied)
	if len(p.Downloads)+len(p.Moves)+len(p.Leftovers) == 0 {
		fmt.Fprintln(w, "  "+tr.T("cli.plan.nothing"))
	}
}

func printNotCopied(w io.Writer, tr *i18n.Catalog, notCopied map[string]engine.Tally) {
	reasons := make([]string, 0, len(notCopied))
	for reason := range notCopied {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	for _, reason := range reasons {
		if t := notCopied[reason]; t.Files > 0 {
			fmt.Fprintln(w, "  "+tr.T("cli.not."+reason, tr.N("n.files", t.Files)))
		}
	}
}

func printResult(w io.Writer, tr *i18n.Catalog, r *engine.Result) {
	head := "cli.done"
	if r.Stopped {
		head = "cli.stopped"
	}
	fmt.Fprintln(w, tr.T(head, r.Took.Round(time.Second).String()))
	line := func(key string, files int, bytes int64) {
		if files > 0 {
			fmt.Fprintln(w, "  "+tr.T(key, tr.N("n.files", files), format.Size(bytes)))
		}
	}
	count := func(key string, n int) {
		if n > 0 {
			fmt.Fprintln(w, "  "+tr.T(key, tr.N("n.files", n)))
		}
	}
	line("cli.result.downloaded", r.Downloaded.Files, r.Downloaded.Bytes)
	line("cli.result.updated", r.Updated.Files, r.Updated.Bytes)
	count("cli.result.moved", r.Moved)
	count("cli.result.trashed", r.Trashed)
	count("cli.result.skipped", r.Skipped)
	count("cli.result.deleted", r.Deleted)
	printNotCopied(w, tr, r.NotCopied)
	line("cli.result.left", r.Left.Files, r.Left.Bytes)
	for _, f := range r.Failures {
		fmt.Fprintln(w, "  "+tr.T("cli.result.failed", f.Target, f.Err.Error()))
	}
}

// progress adds up the events of a run and prints a line now and then.
type progress struct {
	w     io.Writer
	tr    *i18n.Catalog
	total int64
	files int

	mu       sync.Mutex
	bytes    int64
	finished int
	began    time.Time
}

func newProgress(w io.Writer, tr *i18n.Catalog, p *engine.Plan) *progress {
	return &progress{w: w, tr: tr, total: p.Bytes, files: len(p.Downloads)}
}

func (p *progress) event(ev engine.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch ev.Kind {
	case engine.Advanced:
		p.bytes += ev.Bytes
	case engine.Finished, engine.Failed, engine.LeftOut:
		p.finished++
	}
}

// start prints the progress until the returned function is called.
func (p *progress) start() (stop func()) {
	if p.files == 0 {
		return func() {}
	}
	p.began = time.Now()
	quit := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(tick)
		defer ticker.Stop()
		for {
			select {
			case <-quit:
				return
			case <-ticker.C:
				p.print()
			}
		}
	}()
	return func() {
		close(quit)
		wg.Wait()
	}
}

func (p *progress) print() {
	p.mu.Lock()
	bytes, finished := p.bytes, p.finished
	p.mu.Unlock()
	percent := 100
	if p.total > 0 {
		percent = int(min(bytes*100/p.total, 100))
	}
	speed := int64(0)
	if secs := time.Since(p.began).Seconds(); secs > 0 {
		speed = int64(float64(bytes) / secs)
	}
	fmt.Fprintln(p.w, "  "+p.tr.T("cli.progress", percent, format.Size(bytes), format.Size(p.total), finished, p.files, format.Size(speed)))
}
