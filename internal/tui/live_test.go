package tui

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"syncer-gdrive/internal/config"
	"syncer-gdrive/internal/drive"
	"syncer-gdrive/internal/engine"
	"syncer-gdrive/internal/state"
	"syncer-gdrive/internal/theme"
)

// madeUpDrive is a Google Drive in memory for the interface to work on.
type madeUpDrive struct {
	files   []drive.File
	content map[string]string
	down    bool // no connection
}

func newMadeUpDrive() *madeUpDrive {
	d := &madeUpDrive{content: map[string]string{}}
	d.files = append(d.files,
		drive.File{ID: "root", Name: "Syncer", MimeType: "application/vnd.google-apps.folder", CanDownload: true},
		drive.File{ID: "docs", Parent: "root", Name: "Документи", MimeType: "application/vnd.google-apps.folder", CanDownload: true})
	d.put("a", "root", "нотатки.md", "нотатки")
	d.put("b", "docs", "звіт.pdf", "звіт")
	d.put("c", "docs", "договір.pdf", "договір")
	return d
}

func (d *madeUpDrive) put(id, parent, name, content string) {
	sum := md5.Sum([]byte(content))
	f := drive.File{ID: id, Parent: parent, Name: name, MimeType: "text/plain", Size: int64(len(content)),
		MD5: hex.EncodeToString(sum[:]), Created: time.Unix(1700000000, 0), Modified: time.Unix(1700000000, 0), CanDownload: true}
	for i := range d.files {
		if d.files[i].ID == id {
			d.files[i] = f
			d.content[id] = content
			return
		}
	}
	d.files = append(d.files, f)
	d.content[id] = content
}

func (d *madeUpDrive) drop(id string) {
	for i := range d.files {
		if d.files[i].ID == id {
			d.files = append(d.files[:i], d.files[i+1:]...)
			return
		}
	}
}

func (d *madeUpDrive) List(context.Context) ([]drive.File, error) {
	if d.down {
		return nil, errors.New("no route to host")
	}
	return append([]drive.File(nil), d.files...), nil
}

func (d *madeUpDrive) Download(_ context.Context, id string, offset int64, w io.Writer) (int64, error) {
	n, err := io.WriteString(w, d.content[id][offset:])
	return int64(n), err
}

func (d *madeUpDrive) Export(context.Context, string, string, io.Writer) (int64, error) {
	return 0, nil
}

// keyOnDisk writes a Service Account key the way Google Cloud hands it out.
func keyOnDisk(t *testing.T) string {
	t.Helper()
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(private)
	data, _ := json.Marshal(map[string]string{
		"type": "service_account", "client_email": "robot@example.iam.gserviceaccount.com",
		"private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"token_uri":   "https://oauth2.googleapis.com/token",
	})
	file := filepath.Join(t.TempDir(), "downloaded key.json")
	os.WriteFile(file, data, 0o600)
	return file
}

// world is what a real run of the program has around it.
type world struct {
	home    string
	drive   *madeUpDrive
	volumes []string
}

func (w *world) open(t *testing.T) Model {
	t.Helper()
	m, err := NewLive(Options{}, LiveOptions{Home: w.home,
		Connect: func(*drive.Key) engine.Remote { return w.drive },
		Volumes: func() []string { return w.volumes }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	next, _ := m.Update(tea.WindowSizeMsg{Width: testW, Height: testH})
	return next.(Model)
}

// settle feeds the model the messages its commands produce until none is
// left: the way the program runs work off the screen and comes back.
func settle(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for queue := []tea.Cmd{cmd}; len(queue) > 0; {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case nil:
		case tea.BatchMsg:
			queue = append(queue, msg...)
		default:
			next, more := m.Update(msg)
			m = next.(Model)
			checkFrame(t, m, "a message")
			queue = append(queue, more)
		}
	}
	return m
}

// do presses keys, letting the work each one starts finish.
func do(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		next, cmd := m.Update(keyMsg(k))
		m = next.(Model)
		checkFrame(t, m, k)
		m = settle(t, m, cmd)
	}
	return m
}

func filesIn(t *testing.T, dest string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.WalkDir(dest, func(p string, e os.DirEntry, err error) error {
		rel, _ := filepath.Rel(dest, p)
		if e != nil && e.IsDir() {
			if rel == state.Dir {
				return filepath.SkipDir
			}
			return nil
		}
		data, _ := os.ReadFile(p)
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	return out
}

// ready gives a world where the key is imported and a copy has been made.
func ready(t *testing.T) (*world, string, Model) {
	t.Helper()
	w := &world{home: t.TempDir(), drive: newMadeUpDrive()}
	dest := filepath.Join(t.TempDir(), "backup")
	if _, err := config.ImportKey(w.home, keyOnDisk(t)); err != nil {
		t.Fatal(err)
	}
	settings := config.Default()
	settings.AskDestination = false
	os.MkdirAll(dest, 0o755)
	if err := config.Save(w.home, dest, settings); err != nil {
		t.Fatal(err)
	}
	m := w.open(t)
	wantScreen(t, m, scrMenu)
	m = do(t, m, "2")
	wantScreen(t, m, scrPreflight)
	m = do(t, m, "enter")
	wantScreen(t, m, scrSummary)
	m = do(t, m, "enter")
	wantScreen(t, m, scrMenu)
	return w, dest, m
}

func TestFirstRunOnRealData(t *testing.T) {
	w := &world{home: t.TempDir(), drive: newMadeUpDrive()}
	dest := filepath.Join(t.TempDir(), "my drive", "backup")
	m := w.open(t)
	wantScreen(t, m, scrWizard)

	// Language, then the key: a wrong path is refused and can be corrected.
	m = do(t, m, "enter", "enter")
	if m.prompt == nil {
		t.Fatal("the key file is not asked for")
	}
	m = do(t, m, "/no/such/key.json", "enter")
	if m.prompt == nil || m.prompt.err == "" || m.lv.key != nil {
		t.Fatal("a file that does not exist was taken as a key")
	}
	// A path dragged into the terminal comes with its spaces escaped.
	m = do(t, m, "ctrl+u", strings.ReplaceAll(keyOnDisk(t), " ", `\ `), "enter")
	if m.prompt != nil || m.wiz != 2 || m.account() != "robot@example.iam.gserviceaccount.com" {
		t.Fatalf("after the import: prompt %v, step %d, account %q", m.prompt != nil, m.wiz, m.account())
	}
	if !strings.Contains(m.View(), m.account()) {
		t.Error("the step does not show the email to share the folder with")
	}

	// The access is checked on Enter; a second Enter goes on.
	m = do(t, m, "enter")
	if !m.lv.checked || len(m.lv.access) != 1 || !strings.Contains(m.View(), "Syncer") {
		t.Fatalf("after the check: %v, view lacks the shared folder", m.lv.access)
	}
	m = do(t, m, "enter")
	wantScreen(t, m, scrWhere)

	// No folder is known yet: the path is typed. A relative one is refused.
	m = do(t, m, "1")
	if m.prompt == nil {
		t.Fatal("the folder is not asked for")
	}
	m = do(t, m, "backup", "enter")
	if m.prompt == nil || m.prompt.err == "" {
		t.Fatal("a relative path was accepted")
	}
	m = do(t, m, "ctrl+u", dest, "enter")
	wantScreen(t, m, scrTree)
	if !strings.Contains(m.View(), "▾ Syncer") || m.flash != m.tr.T("wiz.done") {
		t.Errorf("the choice of files does not show Drive; flash %q", m.flash)
	}
	// Everything is selected on a first run.
	if files, _ := m.tree.selected(); files != 3 {
		t.Errorf("%d files selected, want 3", files)
	}
	m = do(t, m, "enter")
	wantScreen(t, m, scrMenu)
	// The folder is compared as data: on screen a long path is shortened.
	view := m.View()
	if !strings.Contains(view, "robot@example") || m.destination().Path != dest || !strings.Contains(view, m.tr.T("status.never")) {
		t.Errorf("the menu does not show the account, the folder %q and that nothing was synced:\n%s", m.destination().Path, view)
	}
	if info, err := os.Stat(filepath.Join(dest, state.Dir)); err != nil || !info.IsDir() {
		t.Errorf("the folder was not set up: %v", err)
	}

	// What was set up is there for the next start.
	again := w.open(t)
	wantScreen(t, again, scrMenu)
	if again.destination().Path != dest || again.account() == "" {
		t.Errorf("after a restart: folder %q, account %q", again.destination().Path, again.account())
	}
	if missing := m.tr.Missing(); len(missing) > 0 {
		t.Errorf("texts missing from the catalog: %v", missing)
	}
}

func TestSyncFromTheMenu(t *testing.T) {
	w, dest, m := ready(t)
	want := map[string]string{"Syncer/нотатки.md": "нотатки", "Syncer/Документи/звіт.pdf": "звіт", "Syncer/Документи/договір.pdf": "договір"}
	got := filesIn(t, dest)
	for file, content := range want {
		if got[file] != content {
			t.Errorf("%s holds %q", file, got[file])
		}
	}
	if m.run.result == nil || m.run.result.Downloaded.Files != 3 {
		t.Fatalf("the run reports %+v", m.run.result)
	}
	if strings.Contains(m.View(), m.tr.T("status.never")) {
		t.Error("the menu still says nothing was synced")
	}

	// On Drive one file is deleted and one changed; on the drive one is
	// edited by hand; a new one appears.
	w.drive.drop("b")
	w.drive.put("c", "docs", "договір.pdf", "договір, нова редакція")
	w.drive.put("d", "root", "нове.txt", "нове")
	os.WriteFile(filepath.Join(dest, "Syncer", "нотатки.md"), []byte("мої правки"), 0o644)

	m = do(t, m, "2")
	wantScreen(t, m, scrPreflight)
	if len(m.run.queue) != 3 || len(m.run.deleted) != 1 {
		t.Fatalf("the checks plan %d downloads and %d deleted files", len(m.run.queue), len(m.run.deleted))
	}
	m = do(t, m, "enter")
	wantScreen(t, m, scrDeleted)
	if !strings.Contains(m.View(), "звіт.pdf") {
		t.Error("the deleted file is not named")
	}
	// It is kept on the SKIP list.
	m = do(t, m, "2", "enter")
	wantScreen(t, m, scrSummary)
	res := m.run.result
	if res.Downloaded.Files != 1 || res.Updated.Files != 2 || res.Skipped != 1 || res.Trashed != 1 || len(res.Failures) != 0 {
		t.Errorf("the run reports %+v", res)
	}
	got = filesIn(t, dest)
	if got["Syncer/нотатки.md"] != "нотатки" || got["Syncer/Документи/договір.pdf"] != "договір, нова редакція" ||
		got["Syncer/нове.txt"] != "нове" || got["Syncer/Документи/звіт.pdf"] != "звіт" {
		t.Errorf("the copy holds %v", got)
	}
	m = do(t, m, "enter")

	// The SKIP list and the trash show what really is on the drive.
	m = do(t, m, "3")
	if len(m.skip.items) != 1 || m.skip.items[0].Path != "Syncer/Документи/звіт.pdf" {
		t.Fatalf("SKIP list: %+v", m.skip.items)
	}
	m = do(t, m, "esc", "4")
	if len(m.trash.items) != 1 || m.trash.items[0].Reason != engine.TrashEdited {
		t.Fatalf("trash: %+v", m.trash.items)
	}
	// Space opens the real file; Enter on it restores it next to the copy.
	m = goToRow(t, m, trashOf, func(r listRow) bool { return r.item >= 0 })
	previews = nil
	m = do(t, m, " ")
	if len(previews) != 1 || !strings.HasPrefix(previews[0], dest) || !strings.HasSuffix(previews[0], "нотатки.md") {
		t.Errorf("the preview opened %v", previews)
	}
	m = do(t, m, "enter")
	if len(m.trash.items) != 0 {
		t.Errorf("after restoring the trash holds %+v", m.trash.items)
	}
	got = filesIn(t, dest)
	if got["Syncer/нотатки (2).md"] != "мої правки" || got["Syncer/нотатки.md"] != "нотатки" {
		t.Errorf("after restoring the copy holds %v", got)
	}
	m = do(t, m, "esc")
	wantScreen(t, m, scrMenu)
	if missing := m.tr.Missing(); len(missing) > 0 {
		t.Errorf("texts missing from the catalog: %v", missing)
	}
}

func TestChoiceIsSavedAndUsed(t *testing.T) {
	w, dest, m := ready(t)
	// One file is cleared on the choice screen.
	m = do(t, m, "1")
	wantScreen(t, m, scrTree)
	m = goToNode(t, m, named("Документи"))
	m = do(t, m, "right")
	m = goToNode(t, m, named("звіт.pdf"))
	m = do(t, m, " ", "enter")
	wantScreen(t, m, scrMenu)

	// The copy of it becomes a leftover, and the trash takes it by default.
	m = do(t, m, "2")
	if len(m.run.deleted) != 1 || len(m.run.queue) != 0 {
		t.Fatalf("the checks plan %d downloads and %d leftovers", len(m.run.queue), len(m.run.deleted))
	}
	m = do(t, m, "enter", "enter")
	wantScreen(t, m, scrSummary)
	if _, there := filesIn(t, dest)["Syncer/Документи/звіт.pdf"]; there {
		t.Error("the unselected file is still in the copy")
	}
	m = do(t, m, "enter")

	// The choice is remembered across restarts, and a new file in a folder
	// selected in part follows that folder's switch, which is on.
	w.drive.put("e", "docs", "новий.pdf", "новий")
	again := w.open(t)
	if files, _ := again.tree.selected(); files != 2 {
		t.Errorf("after a restart %d files are selected, want 2", files)
	}
	again = do(t, again, "2", "enter")
	wantScreen(t, again, scrSummary)
	if got := filesIn(t, dest)["Syncer/Документи/новий.pdf"]; got != "новий" {
		t.Errorf("the new file holds %q", got)
	}
}

func TestWithoutConnectionTheLastListingIsShown(t *testing.T) {
	w, _, m := ready(t)
	w.drive.down = true
	m = do(t, m, "1")
	wantScreen(t, m, scrTree)
	if m.flash != m.tr.T("offline") || !strings.Contains(m.View(), "▾ Syncer") {
		t.Errorf("without a connection: flash %q", m.flash)
	}
	// A sync cannot be planned, and says why instead of doing anything.
	m = do(t, m, "esc", "2")
	if m.failure == "" || !strings.Contains(m.View(), m.tr.T("fail.title")) {
		t.Fatal("a sync without a connection gave no message")
	}
	m = do(t, m, "enter")
	wantScreen(t, m, scrMenu)
}

func TestSettingsTravelWithTheDrive(t *testing.T) {
	w, dest, m := ready(t)
	far, _ := theme.Index("far")
	// Settings → theme → FAR Manager; settings → language → English.
	m = do(t, m, "5", "4", theme.Schemes[far].Key, "esc", "3", "2", "esc", "esc")
	wantScreen(t, m, scrMenu)
	onDrive, fromDrive, err := config.Load(t.TempDir(), dest)
	if err != nil || !fromDrive || onDrive.Palette != "far" || onDrive.Language != "en" {
		t.Fatalf("on the drive: %+v, %v, %v", onDrive, fromDrive, err)
	}

	// Another computer: its own home, the key imported, the drive plugged in.
	other := &world{home: t.TempDir(), drive: w.drive, volumes: []string{dest}}
	config.ImportKey(other.home, keyOnDisk(t))
	there := other.open(t)
	wantScreen(t, there, scrMenu)
	if there.scheme != far || there.tr.Code != "en" || there.destination().Path != dest || there.askDest {
		t.Errorf("on another computer: palette %d, language %s, folder %q, asks %v", there.scheme, there.tr.Code, there.destination().Path, there.askDest)
	}
	// It knows what is selected without asking Drive.
	if files, _ := there.tree.selected(); files != 3 {
		t.Errorf("%d files selected, want 3", files)
	}
}

func TestKeyOnTheDriveOpensWithItsPassword(t *testing.T) {
	w, dest, m := ready(t)
	// Settings → Service Account → keep the key on the drive.
	m = do(t, m, "5", "1", "5")
	if m.prompt == nil || !m.prompt.secret {
		t.Fatal("no password is asked for")
	}
	m = do(t, m, "таємниця", "enter", "інше", "enter")
	if m.prompt == nil || m.prompt.err == "" || config.HasSealedKey(dest) {
		t.Fatal("two different passwords were accepted")
	}
	// The password is never drawn.
	m = do(t, m, "таємниця")
	if strings.Contains(m.View(), "таємниця") {
		t.Error("the password is shown as typed")
	}
	m = do(t, m, "ctrl+u", "esc", "5", "таємниця", "enter", "таємниця", "enter")
	if m.prompt != nil || !config.HasSealedKey(dest) || m.flash != m.tr.T("acc.sealed") {
		t.Fatalf("after sealing: prompt %v, on the drive %v, flash %q", m.prompt != nil, config.HasSealedKey(dest), m.flash)
	}
	// The action turns into its opposite.
	if rows := m.accountRows(); rows[len(rows)-1] != "acc.unseal" {
		t.Errorf("account actions: %v", rows)
	}

	// Another computer has no key of its own: the drive brings one.
	other := &world{home: t.TempDir(), drive: w.drive, volumes: []string{dest}}
	there := other.open(t)
	if there.prompt == nil || !there.prompt.secret {
		t.Fatal("the password of the key on the drive is not asked for")
	}
	there = do(t, there, "не той", "enter")
	if there.prompt == nil || there.prompt.err != there.tr.T("acc.pass.wrong") || there.account() != "" {
		t.Fatal("a wrong password opened the key")
	}
	there = do(t, there, "таємниця", "enter")
	wantScreen(t, there, scrMenu)
	if there.prompt != nil || there.account() != "robot@example.iam.gserviceaccount.com" {
		t.Fatalf("after the right password: account %q", there.account())
	}
	// The key stays in memory: nothing is written to that computer.
	if _, _, err := config.LoadKey(other.home); !errors.Is(err, config.ErrNoKey) {
		t.Errorf("the key was copied onto the other computer: %v", err)
	}
	there = do(t, there, "2", "enter")
	wantScreen(t, there, scrSummary)

	// Taking the key off the drive again.
	m = do(t, m, "5")
	if config.HasSealedKey(dest) || m.flash != m.tr.T("acc.unsealed") {
		t.Error("the key is still on the drive")
	}
	if missing := m.tr.Missing(); len(missing) > 0 {
		t.Errorf("texts missing from the catalog: %v", missing)
	}
}

func TestEmptyingTheTrashAndCheckingTheCopy(t *testing.T) {
	w, dest, m := ready(t)
	// Two files are deleted on Drive and go into the trash.
	w.drive.drop("b")
	w.drive.drop("c")
	m = do(t, m, "2", "enter", "enter", "enter")
	wantScreen(t, m, scrMenu)
	m = do(t, m, "4")
	if len(m.trash.items) != 2 {
		t.Fatalf("trash: %+v", m.trash.items)
	}
	// "e" asks before emptying; the first answer is no.
	m = do(t, m, "e")
	if m.trash.menu != menuConfirm || !strings.Contains(m.View(), m.tr.N("n.files", 2)) {
		t.Fatal("emptying the trash does not ask, or does not say how much goes")
	}
	m = do(t, m, "enter")
	if len(m.trash.items) != 2 {
		t.Fatal("the first answer emptied the trash")
	}
	m = do(t, m, "e", "2")
	if len(m.trash.items) != 0 {
		t.Fatalf("after emptying: %+v", m.trash.items)
	}
	if entries, _ := os.ReadDir(filepath.Join(dest, engine.TrashDir)); len(entries) != 0 {
		t.Errorf("files are left in the trash folder: %v", entries)
	}
	m = do(t, m, "esc")

	// Settings → destination folder → check the copy.
	file := filepath.Join(dest, "Syncer", "нотатки.md")
	info, _ := os.Stat(file)
	os.WriteFile(file, []byte("нотатkи"), 0o644) // the same size, another content
	os.Chtimes(file, info.ModTime(), info.ModTime())
	m = do(t, m, "5", "2", "3")
	wantScreen(t, m, scrDest)
	if want := m.tr.T("verify.bad", m.tr.N("n.files", 1), m.tr.N("n.files", 1)); m.flash != want {
		t.Errorf("the check says %q, want %q", m.flash, want)
	}
	// The next sync repairs it.
	m = do(t, m, "esc", "esc", "2", "enter", "enter")
	if got, _ := os.ReadFile(file); string(got) != "нотатки" {
		t.Errorf("after the repair the file holds %q", got)
	}
	m = do(t, m, "5", "2", "3")
	if want := m.tr.T("verify.ok", m.tr.N("n.files", 1)); m.flash != want {
		t.Errorf("a clean copy: %q, want %q", m.flash, want)
	}
	if missing := m.tr.Missing(); len(missing) > 0 {
		t.Errorf("texts missing from the catalog: %v", missing)
	}
}

func TestHintsThatDoNotFitDropFromTheEnd(t *testing.T) {
	m := start(t, Options{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 44, Height: 20})
	m = next.(Model)
	line := m.hints("Enter", "відновити", "d", "видалити", "Esc", "назад", "Space", "перегляд")
	if !strings.Contains(line, "Esc") || strings.Contains(line, "Space") || strings.Contains(line, "…") {
		t.Errorf("hints on a narrow terminal: %q", line)
	}
	// One pair is always kept, even if it has to be cut.
	if line := m.hints("Enter", strings.Repeat("дуже довга підказка ", 5)); !strings.Contains(line, "Enter") {
		t.Errorf("the only hint is gone: %q", line)
	}
}

func TestServiceAccountActions(t *testing.T) {
	w, _, m := ready(t)
	m = do(t, m, "5", "1")
	wantScreen(t, m, scrAccount)
	if !strings.Contains(m.View(), "robot@example.iam.gserviceaccount.com") {
		t.Error("the screen does not show the account")
	}
	m = do(t, m, "1") // check the access
	if want := m.tr.T("acc.check.ok", m.tr.N("n.folders", 1), m.tr.N("n.files", 3)); m.flash != want {
		t.Errorf("after the check: %q, want %q", m.flash, want)
	}
	copied = ""
	m = do(t, m, "2") // copy the email
	if copied != "robot@example.iam.gserviceaccount.com" {
		t.Errorf("copied %q", copied)
	}
	w.drive.down = true
	m = do(t, m, "1")
	if !strings.Contains(m.flash, "no route to host") {
		t.Errorf("a failed check says %q", m.flash)
	}
	// Removing the key leads back to the step that asks for one.
	m = do(t, m, "4")
	wantScreen(t, m, scrWizard)
	if m.wiz != 1 || m.account() != "" {
		t.Errorf("after removing the key: step %d, account %q", m.wiz, m.account())
	}
	if _, _, err := config.LoadKey(w.home); !errors.Is(err, config.ErrNoKey) {
		t.Errorf("the key is still on the computer: %v", err)
	}
	if missing := m.tr.Missing(); len(missing) > 0 {
		t.Errorf("texts missing from the catalog: %v", missing)
	}
}
