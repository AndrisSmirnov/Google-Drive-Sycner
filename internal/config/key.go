package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"syncer-gdrive/internal/drive"
	"syncer-gdrive/internal/state"
)

// ErrNoKey means no Service Account key has been imported.
var ErrNoKey = errors.New("no Service Account key has been imported")

// ErrWrongPassword means the encrypted key on the drive did not open.
var ErrWrongPassword = errors.New("the password does not open the key on this drive")

// ImportKey checks that a file is a Service Account key and copies it into
// the program's folder, readable by the owner only.
func ImportKey(home, path string) (*drive.Key, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	key, err := drive.ParseKey(data)
	if err != nil {
		return nil, err
	}
	if err := replace(filepath.Join(home, keyFile), data, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

// LoadKey returns the key kept on this computer, both parsed and as the
// bytes of the file.
func LoadKey(home string) (*drive.Key, []byte, error) {
	data, err := os.ReadFile(filepath.Join(home, keyFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, ErrNoKey
	}
	if err != nil {
		return nil, nil, err
	}
	key, err := drive.ParseKey(data)
	return key, data, err
}

// RemoveKey deletes the key from this computer.
func RemoveKey(home string) error {
	err := os.Remove(filepath.Join(home, keyFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// The copy of the key on a drive is never stored in the clear: whoever
// finds the drive must not get access to Google Drive with it.
const (
	sealedFile = "account.enc"
	iterations = 600_000 // PBKDF2-HMAC-SHA256
)

type sealed struct {
	Version    int    `json:"version"`
	KDF        string `json:"kdf"`
	Iterations int    `json:"iterations"`
	Salt       []byte `json:"salt"`
	Nonce      []byte `json:"nonce"`
	Data       []byte `json:"data"`
}

func cipherFor(password string, salt []byte, rounds int) (cipher.AEAD, error) {
	derived, err := pbkdf2.Key(sha256.New, password, salt, rounds, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(derived)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// SealKey puts an encrypted copy of the key into the destination. The
// password is turned into an encryption key slowly on purpose, so guessing
// it takes long; the content is sealed with AES-256-GCM.
func SealKey(destination string, key []byte, password string) error {
	if password == "" {
		return errors.New("a password is needed to keep the key on the drive")
	}
	if _, err := drive.ParseKey(key); err != nil {
		return err
	}
	s := sealed{Version: 1, KDF: "pbkdf2-sha256", Iterations: iterations, Salt: make([]byte, 16), Nonce: make([]byte, 12)}
	if _, err := rand.Read(s.Salt); err != nil {
		return err
	}
	if _, err := rand.Read(s.Nonce); err != nil {
		return err
	}
	aead, err := cipherFor(password, s.Salt, s.Iterations)
	if err != nil {
		return err
	}
	s.Data = aead.Seal(nil, s.Nonce, key, nil)
	return write(filepath.Join(destination, state.Dir, sealedFile), s, 0o600)
}

// HasSealedKey tells whether the destination carries a copy of the key.
func HasSealedKey(destination string) bool {
	_, err := os.Stat(filepath.Join(destination, state.Dir, sealedFile))
	return err == nil
}

// OpenSealedKey decrypts the copy of the key kept in the destination.
func OpenSealedKey(destination, password string) (*drive.Key, []byte, error) {
	data, err := os.ReadFile(filepath.Join(destination, state.Dir, sealedFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, ErrNoKey
	}
	if err != nil {
		return nil, nil, err
	}
	var s sealed
	if err := json.Unmarshal(data, &s); err != nil || s.Version != 1 || s.KDF != "pbkdf2-sha256" || s.Iterations < 1 {
		return nil, nil, errors.New("the key on this drive is in a format this version does not know")
	}
	aead, err := cipherFor(password, s.Salt, s.Iterations)
	if err != nil {
		return nil, nil, err
	}
	if len(s.Nonce) != aead.NonceSize() {
		return nil, nil, errors.New("the key on this drive is damaged")
	}
	plain, err := aead.Open(nil, s.Nonce, s.Data, nil)
	if err != nil {
		return nil, nil, ErrWrongPassword
	}
	key, err := drive.ParseKey(plain)
	return key, plain, err
}

// RemoveSealedKey deletes the copy of the key from the destination.
func RemoveSealedKey(destination string) error {
	err := os.Remove(filepath.Join(destination, state.Dir, sealedFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
