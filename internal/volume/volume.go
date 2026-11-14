// Package volume tells what the program needs to know about the place a
// copy goes to: how much room is left, which file system it is, and which
// drives are connected. The answers come from the operating system, so
// each one has its own file here.
package volume

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"syncer-gdrive/internal/state"
)

// Info describes the volume a folder is on.
type Info struct {
	Free  int64 // bytes available to the user
	Total int64
	// FileSystem is the name the system gives it, in lower case: "apfs",
	// "exfat", "msdos", "ntfs", "ext4"… Empty if it could not be told.
	FileSystem string
	// Mount is where the volume is attached, and Name what it is called.
	Mount string
	Name  string
}

// fat32Limit is the largest file FAT32 can hold: 4 GiB less one byte.
const fat32Limit = 4<<30 - 1

// MaxFileSize is the largest file the volume can hold, or zero where the
// limit is beyond anything a backup meets.
func (i Info) MaxFileSize() int64 {
	switch i.FileSystem {
	case "msdos", "vfat", "fat32", "fat":
		return fat32Limit
	}
	return 0
}

// Label is how the volume is shown: its name and file system.
func (i Info) Label() string {
	fs := map[string]string{
		"apfs": "APFS", "hfs": "HFS+", "exfat": "exFAT", "msdos": "FAT32", "vfat": "FAT32", "fat32": "FAT32", "fat": "FAT",
		"ntfs": "NTFS", "ext4": "ext4", "ext2/3/4": "ext4", "btrfs": "Btrfs", "xfs": "XFS",
	}[i.FileSystem]
	if fs == "" {
		fs = i.FileSystem
	}
	switch {
	case i.Name == "":
		return fs
	case fs == "":
		return i.Name
	}
	return i.Name + " · " + fs
}

// Destinations looks on the connected volumes for folders that already
// hold a copy: the root of each volume and the folders right inside it.
// That is how a drive plugged into a new computer is recognised.
func Destinations() []string { return destinationsIn(Roots()) }

func destinationsIn(roots []string) []string {
	var out []string
	holds := func(dir string) bool {
		info, err := os.Stat(filepath.Join(dir, state.Dir))
		return err == nil && info.IsDir()
	}
	for _, root := range roots {
		if holds(root) {
			out = append(out, root)
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && holds(filepath.Join(root, e.Name())) {
				out = append(out, filepath.Join(root, e.Name()))
			}
		}
	}
	sort.Strings(out)
	return out
}

// Present tells whether a destination can be used: the folder is there, or
// it can be created. A folder on a drive that is not connected cannot: its
// nearest existing ancestor is the place where drives appear (/Volumes on
// macOS), or nothing exists at all (a drive letter not in use on Windows).
// Creating the folder there would put the copy on the system disk under
// the name of the missing drive.
func Present(path string) bool {
	nearest := existing(path)
	if info, err := os.Stat(nearest); err != nil || !info.IsDir() {
		return false
	}
	if nearest == filepath.Clean(path) {
		return true
	}
	for _, parent := range mountParents() {
		if nearest == filepath.Clean(parent) {
			return false
		}
	}
	return true
}

// existing walks up from a path to the nearest folder that exists: a
// destination may not have been created yet.
func existing(path string) string {
	path = filepath.Clean(path)
	for {
		if _, err := os.Stat(path); err == nil {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			return path
		}
		path = parent
	}
}
