package cli

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"syncer-gdrive/internal/config"
	"syncer-gdrive/internal/drive"
	"syncer-gdrive/internal/engine"
	"syncer-gdrive/internal/i18n"
)

// fakeDrive is a made-up Google Drive with one shared folder.
type fakeDrive struct {
	files   []drive.File
	content map[string]string
}

func (d *fakeDrive) add(id, parent, name, content string) {
	sum := md5.Sum([]byte(content))
	d.files = append(d.files, drive.File{ID: id, Parent: parent, Name: name, MimeType: "text/plain", Size: int64(len(content)),
		MD5: hex.EncodeToString(sum[:]), Created: time.Unix(1700000000, 0), Modified: time.Unix(1700000000, 0), CanDownload: true})
	d.content[id] = content
}

func newFakeDrive() *fakeDrive {
	d := &fakeDrive{content: map[string]string{}}
	d.files = append(d.files, drive.File{ID: "root", Name: "Syncer", MimeType: "application/vnd.google-apps.folder", CanDownload: true})
	d.add("a", "root", "нотатки.md", "нотатки")
	d.add("b", "root", "звіт.pdf", "звіт")
	return d
}

func (d *fakeDrive) List(context.Context) ([]drive.File, error) { return d.files, nil }

func (d *fakeDrive) Download(_ context.Context, id string, offset int64, w io.Writer) (int64, error) {
	n, err := io.WriteString(w, d.content[id][offset:])
	return int64(n), err
}

func (d *fakeDrive) Export(context.Context, string, string, io.Writer) (int64, error) { return 0, nil }

// setup gives the program a home of its own, a made-up Drive and no
// connected drives, and returns a way to run a command.
func setup(t *testing.T, d *fakeDrive) (home string, run func(args ...string) (code int, out, errs string)) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("SYNCER_HOME", home)
	oldConnect, oldConnected, oldTick := connect, connected, tick
	connect = func(*drive.Key) engine.Remote { return d }
	connected = func() []string { return nil }
	tick = time.Millisecond
	t.Cleanup(func() { connect, connected, tick = oldConnect, oldConnected, oldTick })
	return home, func(args ...string) (int, string, string) {
		var out, errs bytes.Buffer
		code := Main(args, &out, &errs)
		return code, out.String(), errs.String()
	}
}

// keyFile writes a Service Account key the way Google Cloud hands it out.
func keyFile(t *testing.T) string {
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
	path := filepath.Join(t.TempDir(), "key.json")
	os.WriteFile(path, data, 0o600)
	return path
}

func TestVersion(t *testing.T) {
	_, run := setup(t, newFakeDrive())
	if code, out, _ := run("version"); code != ExitOK || !strings.Contains(out, Version) {
		t.Errorf("version: %d, %q", code, out)
	}
}

func TestSyncNeedsAKeyAndADestination(t *testing.T) {
	_, run := setup(t, newFakeDrive())
	code, _, errs := run("sync")
	if code != ExitNotStarted || !strings.Contains(errs, "syncer key") {
		t.Errorf("without a key: %d, %q", code, errs)
	}

	if code, _, errs := run("key"); code != ExitNotStarted || errs == "" {
		t.Errorf("key without a file: %d, %q", code, errs)
	}
	notAKey := filepath.Join(t.TempDir(), "x.json")
	os.WriteFile(notAKey, []byte(`{"type":"authorized_user"}`), 0o600)
	if code, _, _ := run("key", notAKey); code != ExitNotStarted {
		t.Errorf("a file that is not a key was imported: %d", code)
	}
	code, out, _ := run("key", keyFile(t))
	if code != ExitOK || !strings.Contains(out, "robot@example.iam.gserviceaccount.com") {
		t.Errorf("key import: %d, %q", code, out)
	}

	code, _, errs = run("sync")
	if code != ExitNotStarted || !strings.Contains(errs, "-dest") {
		t.Errorf("without a destination: %d, %q", code, errs)
	}
}

func TestSyncCopiesAndRemembersTheDestination(t *testing.T) {
	d := newFakeDrive()
	home, run := setup(t, d)
	run("key", keyFile(t))
	dest := t.TempDir()

	code, out, errs := run("sync", "-dest", dest)
	if code != ExitOK {
		t.Fatalf("sync: %d\n%s\n%s", code, out, errs)
	}
	for _, want := range []string{"robot@example", dest, "нових: 2 файли", "завантажено: 2 файли"} {
		if !strings.Contains(out, want) {
			t.Errorf("the output lacks %q:\n%s", want, out)
		}
	}
	for name, content := range map[string]string{"нотатки.md": "нотатки", "звіт.pdf": "звіт"} {
		if got, _ := os.ReadFile(filepath.Join(dest, "Syncer", name)); string(got) != content {
			t.Errorf("%s holds %q", name, got)
		}
	}

	// The destination is remembered: the next sync needs no -dest, and
	// finds the copy up to date.
	code, out, _ = run("sync")
	if code != ExitOK || !strings.Contains(out, "Копія актуальна") {
		t.Errorf("second sync: %d\n%s", code, out)
	}
	settings, fromDrive, _ := config.Load(home, dest)
	if !fromDrive || len(settings.Destinations) != 1 || settings.Destinations[0] != dest {
		t.Errorf("settings after a sync: %+v, from the drive %v", settings, fromDrive)
	}

	// A file deleted on Drive goes into the trash: nobody can be asked.
	d.files = d.files[:2]
	code, out, _ = run("sync")
	if code != ExitOK || !strings.Contains(out, "у кошик: 1 файл") {
		t.Errorf("after a deletion on Drive: %d\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(dest, engine.TrashDir, "Syncer", "звіт.pdf")); err != nil {
		t.Errorf("the deleted file is not in the trash: %v", err)
	}

	// The language follows the settings.
	settings.Language = "en"
	config.Save(home, dest, settings)
	if _, out, _ := run("sync"); !strings.Contains(out, "The copy is up to date") {
		t.Errorf("English output:\n%s", out)
	}
}

func TestFailedFilesGiveExitCodeOne(t *testing.T) {
	d := newFakeDrive()
	d.files[1].MD5 = "00000000000000000000000000000000" // Drive describes other content
	_, run := setup(t, d)
	run("key", keyFile(t))
	code, out, _ := run("sync", "-dest", t.TempDir())
	if code != ExitFailures || !strings.Contains(out, "помилка:") || !strings.Contains(out, "завантажено: 1 файл") {
		t.Errorf("a sync with a failed file: %d\n%s", code, out)
	}
}

// Every text the commands use is in both catalogs.
func TestEveryTextIsTranslated(t *testing.T) {
	source, err := os.ReadFile("cli.go")
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	for _, m := range regexp.MustCompile(`"(cli\.[a-z.\-]+)"`).FindAllSubmatch(source, -1) {
		if key := string(m[1]); !strings.HasSuffix(key, ".") {
			keys[key] = true
		}
	}
	for _, reason := range []string{engine.NotShortcut, engine.NotUnsupported, engine.NotService, engine.NotForbidden, engine.NotTooLarge, engine.NotExportLimit} {
		keys["cli.not."+reason] = true
	}
	if len(keys) < 30 {
		t.Fatalf("only %d keys found in the source", len(keys))
	}
	for _, lang := range []string{"uk", "en"} {
		tr, err := i18n.Load(lang)
		if err != nil {
			t.Fatal(err)
		}
		for key := range keys {
			tr.T(key)
		}
		if missing := tr.Missing(); len(missing) > 0 {
			t.Errorf("%s lacks %v", lang, missing)
		}
	}
}
