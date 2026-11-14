package volume

import (
	"os"
	"path/filepath"
	"testing"

	"syncer-gdrive/internal/state"
)

func TestStat(t *testing.T) {
	dir := t.TempDir()
	info, err := Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Free <= 0 || info.Total < info.Free {
		t.Errorf("free %d of %d", info.Free, info.Total)
	}
	if info.FileSystem == "" || info.Mount == "" {
		t.Errorf("file system %q mounted at %q", info.FileSystem, info.Mount)
	}
	// A destination that has not been created yet is on the volume of the
	// nearest folder that exists.
	missing, err := Stat(filepath.Join(dir, "not", "yet", "there"))
	if err != nil || missing.Total != info.Total || missing.FileSystem != info.FileSystem {
		t.Errorf("a folder that does not exist yet: %+v, %v", missing, err)
	}
}

func TestPresent(t *testing.T) {
	dir := t.TempDir()
	if !Present(dir) {
		t.Error("an existing folder is reported as not there")
	}
	// A folder several levels below an existing one can be created.
	if !Present(filepath.Join(dir, "new", "deeper", "backup")) {
		t.Error("a folder that can be created is reported as not there")
	}
	// A file is not a folder.
	file := filepath.Join(dir, "file")
	os.WriteFile(file, nil, 0o644)
	if Present(file) {
		t.Error("a file is taken for a destination")
	}
	// A folder on a drive that is not connected must not be created.
	for _, parent := range mountParents() {
		if _, err := os.Stat(parent); err != nil {
			continue
		}
		if Present(filepath.Join(parent, "no-such-drive-for-the-test", "backup")) {
			t.Errorf("a folder on a drive missing from %s is reported as usable", parent)
		}
	}
}

func TestMaxFileSizeAndLabel(t *testing.T) {
	for fs, limited := range map[string]bool{"msdos": true, "vfat": true, "fat32": true, "exfat": false, "apfs": false, "ntfs": false, "ext4": false, "": false} {
		if got := (Info{FileSystem: fs}).MaxFileSize(); (got == 4<<30-1) != limited || (!limited && got != 0) {
			t.Errorf("%q: the largest file is %d", fs, got)
		}
	}
	labels := map[Info]string{
		{Name: "DOOMSDAY", FileSystem: "exfat"}: "DOOMSDAY · exFAT",
		{Name: "USB", FileSystem: "msdos"}:      "USB · FAT32",
		{FileSystem: "apfs"}:                    "APFS",
		{Name: "Data", FileSystem: "zfs"}:       "Data · zfs",
		{Name: "Data"}:                          "Data",
	}
	for info, want := range labels {
		if got := info.Label(); got != want {
			t.Errorf("Label(%+v) = %q, want %q", info, got, want)
		}
	}
}

func TestDestinationsAreFoundOnVolumes(t *testing.T) {
	a, b, c := t.TempDir(), t.TempDir(), t.TempDir()
	mk := func(parts ...string) { os.MkdirAll(filepath.Join(parts...), 0o755) }
	mk(a, state.Dir)                                      // a copy in the root of a volume
	mk(b, "drive-backup", state.Dir)                      // a copy in a folder of a volume
	mk(b, "other")                                        // a folder without a copy
	mk(b, ".hidden", state.Dir)                           // hidden folders are not looked into
	mk(c, "deep", "deeper", state.Dir)                    // too deep to be looked for
	os.WriteFile(filepath.Join(c, state.Dir), nil, 0o644) // a file with that name is not a copy

	got := destinationsIn([]string{a, b, c, filepath.Join(a, "gone")})
	want := []string{a, filepath.Join(b, "drive-backup")}
	if len(got) != 2 || !contains(got, want[0]) || !contains(got, want[1]) {
		t.Errorf("found %v, want %v", got, want)
	}
	// The real volumes can at least be listed without failing.
	_ = Roots()
	_ = Destinations()
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
