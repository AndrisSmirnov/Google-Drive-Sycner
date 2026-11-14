package config

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"syncer-gdrive/internal/state"
)

func TestSettingsPreferTheDrive(t *testing.T) {
	home, dest := t.TempDir(), t.TempDir()

	// Nothing saved anywhere: the defaults.
	s, fromDrive, err := Load(home, dest)
	if err != nil || fromDrive || s.Palette != "terminal" || !s.AskDestination || s.Language != "uk" {
		t.Fatalf("first run: %+v, fromDrive %v, %v", s, fromDrive, err)
	}

	// Saved with a destination: both places get them.
	s.Language, s.Palette, s.AskDestination = "en", "far", false
	if err := Save(home, dest, s); err != nil {
		t.Fatal(err)
	}
	got, fromDrive, _ := Load(home, dest)
	if !fromDrive || got.Language != "en" || got.Palette != "far" || got.AskDestination {
		t.Errorf("after saving: %+v, fromDrive %v", got, fromDrive)
	}
	if len(got.Destinations) != 1 || got.Destinations[0] != dest {
		t.Errorf("known destinations: %v", got.Destinations)
	}
	// The drive does not carry the paths of this computer.
	onTheDrive, _ := os.ReadFile(filepath.Join(dest, state.Dir, "settings.json"))
	if bytes.Contains(onTheDrive, []byte("destinations")) {
		t.Errorf("the settings on the drive list this computer's folders: %s", onTheDrive)
	}

	// Another computer, same drive: the drive's settings are used.
	other := t.TempDir()
	got, fromDrive, _ = Load(other, dest)
	if !fromDrive || got.Language != "en" || got.Palette != "far" || len(got.Destinations) != 0 {
		t.Errorf("on another computer: %+v, fromDrive %v", got, fromDrive)
	}

	// A new drive on the first computer: the computer's settings are used.
	fresh := t.TempDir()
	got, fromDrive, _ = Load(home, fresh)
	if fromDrive || got.Language != "en" || got.Palette != "far" {
		t.Errorf("a drive without settings: %+v, fromDrive %v", got, fromDrive)
	}

	// The latest destination comes first and is listed once.
	Save(home, fresh, got)
	Save(home, dest, got)
	got, _, _ = Load(home, "")
	if len(got.Destinations) != 2 || got.Destinations[0] != dest || got.Destinations[1] != fresh {
		t.Errorf("known destinations: %v", got.Destinations)
	}

	// Broken settings are reported, not silently replaced.
	os.WriteFile(filepath.Join(home, "settings.json"), []byte("{"), 0o600)
	if _, _, err := Load(home, ""); err == nil {
		t.Error("broken settings were accepted")
	}
}

func TestHome(t *testing.T) {
	t.Setenv("SYNCER_HOME", "/somewhere/else")
	if dir, _ := Home(); dir != "/somewhere/else" {
		t.Errorf("SYNCER_HOME is ignored: %q", dir)
	}
	t.Setenv("SYNCER_HOME", "")
	if runtime.GOOS != "windows" {
		t.Setenv("XDG_CONFIG_HOME", "/xdg")
		if dir, _ := Home(); dir != filepath.Join("/xdg", "syncer") {
			t.Errorf("XDG_CONFIG_HOME is ignored: %q", dir)
		}
		t.Setenv("XDG_CONFIG_HOME", "")
		dir, err := Home()
		if err != nil || filepath.Base(dir) != "syncer" || filepath.Base(filepath.Dir(dir)) != ".config" {
			t.Errorf("default home: %q, %v", dir, err)
		}
	}
}

// keyJSON makes a Service Account key the way Google Cloud hands it out.
func keyJSON(t *testing.T) []byte {
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
	return data
}

func TestKeyOnTheComputer(t *testing.T) {
	home := t.TempDir()
	if _, _, err := LoadKey(home); !errors.Is(err, ErrNoKey) {
		t.Fatalf("no key yet: %v", err)
	}

	source := filepath.Join(t.TempDir(), "downloaded.json")
	os.WriteFile(source, keyJSON(t), 0o644)
	key, err := ImportKey(home, source)
	if err != nil || key.Email != "robot@example.iam.gserviceaccount.com" {
		t.Fatalf("import: %v, %v", key, err)
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(filepath.Join(home, "account.json")); info.Mode().Perm() != 0o600 {
			t.Errorf("the key is readable by others: %v", info.Mode().Perm())
		}
	}
	if key, _, err := LoadKey(home); err != nil || key.Email == "" {
		t.Errorf("the imported key does not load: %v", err)
	}

	// Something that is not a key is refused and does not replace the key.
	os.WriteFile(source, []byte(`{"type":"authorized_user"}`), 0o644)
	if _, err := ImportKey(home, source); err == nil {
		t.Error("a file that is not a Service Account key was imported")
	}
	if _, _, err := LoadKey(home); err != nil {
		t.Errorf("a refused import damaged the key: %v", err)
	}

	if err := RemoveKey(home); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadKey(home); !errors.Is(err, ErrNoKey) {
		t.Errorf("after removing: %v", err)
	}
	if err := RemoveKey(home); err != nil {
		t.Errorf("removing a key that is not there: %v", err)
	}
}

func TestKeyOnTheDriveIsEncrypted(t *testing.T) {
	dest := t.TempDir()
	plain := keyJSON(t)
	if HasSealedKey(dest) {
		t.Fatal("a new destination claims to carry a key")
	}
	if err := SealKey(dest, plain, ""); err == nil {
		t.Error("the key was put on the drive without a password")
	}
	if err := SealKey(dest, []byte("not a key"), "secret"); err == nil {
		t.Error("something that is not a key was put on the drive")
	}
	if err := SealKey(dest, plain, "пароль з кирилицею"); err != nil {
		t.Fatal(err)
	}
	if !HasSealedKey(dest) {
		t.Fatal("the key is not on the drive")
	}

	// Nothing of the key is readable in the file.
	stored, _ := os.ReadFile(filepath.Join(dest, state.Dir, "account.enc"))
	for _, secret := range []string{"PRIVATE KEY", "robot@example", "service_account"} {
		if bytes.Contains(stored, []byte(secret)) {
			t.Errorf("the file on the drive shows %q", secret)
		}
	}

	key, opened, err := OpenSealedKey(dest, "пароль з кирилицею")
	if err != nil || key.Email != "robot@example.iam.gserviceaccount.com" || !bytes.Equal(opened, plain) {
		t.Fatalf("the right password: %v", err)
	}
	if _, _, err := OpenSealedKey(dest, "wrong"); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("a wrong password: %v", err)
	}

	// A changed byte is noticed.
	var s sealed
	json.Unmarshal(stored, &s)
	s.Data[len(s.Data)/2] ^= 1
	tampered, _ := json.Marshal(s)
	os.WriteFile(filepath.Join(dest, state.Dir, "account.enc"), tampered, 0o600)
	if _, _, err := OpenSealedKey(dest, "пароль з кирилицею"); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("a tampered file opened: %v", err)
	}

	// Two seals of one key differ: salt and nonce are fresh each time.
	SealKey(dest, plain, "p")
	first, _ := os.ReadFile(filepath.Join(dest, state.Dir, "account.enc"))
	SealKey(dest, plain, "p")
	second, _ := os.ReadFile(filepath.Join(dest, state.Dir, "account.enc"))
	if bytes.Equal(first, second) {
		t.Error("sealing twice gave the same bytes")
	}

	if err := RemoveSealedKey(dest); err != nil || HasSealedKey(dest) {
		t.Errorf("removing the key from the drive: %v", err)
	}
	if _, _, err := OpenSealedKey(dest, "p"); !errors.Is(err, ErrNoKey) {
		t.Errorf("opening a key that is not there: %v", err)
	}
}
