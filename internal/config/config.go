// Package config keeps what the program needs before it can do anything:
// the settings and the Service Account key.
//
// Settings live in two places — in the destination folder on the drive and
// on the computer. The drive wins: a drive plugged into another computer
// brings its language, palette and habits with it. The key lives on the
// computer; a copy may be put on the drive, always encrypted with a password.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	"syncer-gdrive/internal/state"
)

const (
	settingsFile = "settings.json"
	keyFile      = "account.json"
)

// Settings is everything the user can change that is not about files.
type Settings struct {
	Language        string `json:"language"`
	Palette         string `json:"palette"`
	Variant         string `json:"variant"` // auto, dark or light
	PaintBackground bool   `json:"paint_background"`
	// AskDestination makes every sync start with the question where it goes.
	AskDestination bool `json:"ask_destination"`
	// Destinations are the folders this computer has synced to, the latest
	// first. They are kept on the computer only: paths of one machine mean
	// nothing on another.
	Destinations []string `json:"destinations,omitempty"`
}

// Default is what a first run starts with.
func Default() Settings {
	return Settings{Language: "uk", Palette: "terminal", Variant: "auto", PaintBackground: true, AskDestination: true}
}

// Home returns the program's folder on this computer: SYNCER_HOME if set,
// %AppData%\syncer on Windows, otherwise $XDG_CONFIG_HOME/syncer or
// ~/.config/syncer.
func Home() (string, error) {
	if dir := os.Getenv("SYNCER_HOME"); dir != "" {
		return dir, nil
	}
	if runtime.GOOS == "windows" {
		if dir := os.Getenv("AppData"); dir != "" {
			return filepath.Join(dir, "syncer"), nil
		}
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "syncer"), nil
	}
	user, err := os.UserHomeDir()
	if err != nil {
		return "", errors.New("the home folder of the user cannot be found")
	}
	return filepath.Join(user, ".config", "syncer"), nil
}

// onDrive is where a destination keeps its settings.
func onDrive(destination string) string {
	return filepath.Join(destination, state.Dir, settingsFile)
}

// Load returns the settings to work with. Those on the drive are used if
// the destination has them; otherwise those on the computer; otherwise the
// defaults. The list of known destinations always comes from the computer.
// fromDrive tells which of the two answered.
func Load(home, destination string) (s Settings, fromDrive bool, err error) {
	local := Default()
	if err := read(filepath.Join(home, settingsFile), &local); err != nil {
		return Default(), false, err
	}
	if destination == "" {
		return local, false, nil
	}
	s = local
	switch err := read(onDrive(destination), &s); {
	case errors.Is(err, os.ErrNotExist):
		return local, false, nil
	case err != nil:
		return local, false, err
	}
	s.Destinations = local.Destinations
	return s, true, nil
}

// read fills s from a file. A file that does not exist leaves s as it is
// and, for the drive, is reported; for the computer it just means defaults.
func read(path string, s *Settings) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if filepath.Base(filepath.Dir(path)) == state.Dir {
			return err
		}
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return fmt.Errorf("the settings in %s cannot be read: %w", path, err)
	}
	return nil
}

// Save writes the settings to the computer and, when a destination is
// given, to the drive as well. The destination moves to the front of the
// list of known ones. That list is taken from the computer, not from the
// caller: settings loaded from a drive do not carry it.
func Save(home, destination string, s Settings) error {
	var stored Settings
	if err := read(filepath.Join(home, settingsFile), &stored); err == nil {
		s.Destinations = stored.Destinations
	}
	if destination != "" {
		s.Destinations = slices.DeleteFunc(s.Destinations, func(d string) bool { return d == destination })
		s.Destinations = append([]string{destination}, s.Destinations...)
	}
	if err := write(filepath.Join(home, settingsFile), s, 0o600); err != nil {
		return err
	}
	if destination == "" {
		return nil
	}
	portable := s
	portable.Destinations = nil
	return write(onDrive(destination), portable, 0o644)
}

// write replaces a file in one step: a crash never leaves half of it.
func write(path string, value any, mode os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return replace(path, append(data, '\n'), mode)
}

func replace(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
