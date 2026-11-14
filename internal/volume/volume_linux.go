package volume

import (
	"bufio"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// Linux identifies a file system by a number.
var linuxTypes = map[int64]string{
	0x4d44:     "msdos",
	0x2011bab0: "exfat",
	0x5346544e: "ntfs",
	0x65735546: "fuse", // ntfs-3g and exfat-fuse report this; /proc/mounts tells which
	0xef53:     "ext4",
	0x9123683e: "btrfs",
	0x58465342: "xfs",
	0x01021994: "tmpfs",
	0x794c7630: "overlay",
	0x2fc12fc1: "zfs",
	0xf2f52010: "f2fs",
}

// Stat describes the volume a folder is on. The folder need not exist yet.
func Stat(path string) (Info, error) {
	path = existing(path)
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return Info{}, err
	}
	info := Info{
		Free:       int64(st.Bavail) * int64(st.Bsize),
		Total:      int64(st.Blocks) * int64(st.Bsize),
		FileSystem: linuxTypes[int64(st.Type)&0xffffffff],
	}
	// The mount table knows where the volume is attached and, for drivers
	// that all report "fuse", which file system it really is.
	if mount, fs := mountOf(path); mount != "" {
		info.Mount = mount
		if info.FileSystem == "" || info.FileSystem == "fuse" {
			info.FileSystem = strings.TrimPrefix(strings.TrimPrefix(fs, "fuse."), "fuseblk.")
			if fs == "fuseblk" {
				info.FileSystem = "ntfs"
			}
		}
		if mount != "/" {
			info.Name = filepath.Base(mount)
		}
	}
	return info, nil
}

// mountOf finds the mount point a path is under and its file system.
func mountOf(path string) (mount, fs string) {
	file, err := os.Open("/proc/self/mounts")
	if err != nil {
		return "", ""
	}
	defer file.Close()
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		// Spaces in a mount point are written as \040.
		point := strings.ReplaceAll(fields[1], `\040`, " ")
		if (path == point || strings.HasPrefix(path, strings.TrimSuffix(point, "/")+"/")) && len(point) >= len(mount) {
			mount, fs = point, fields[2]
		}
	}
	return mount, fs
}

// Roots lists the places where connected drives appear.
func Roots() []string {
	var parents []string
	if u, err := user.Current(); err == nil && u.Username != "" {
		parents = append(parents, filepath.Join("/media", u.Username), filepath.Join("/run/media", u.Username))
	}
	parents = append(parents, "/mnt")
	var out []string
	for _, parent := range parents {
		entries, _ := os.ReadDir(parent)
		for _, e := range entries {
			if e.IsDir() {
				out = append(out, filepath.Join(parent, e.Name()))
			}
		}
	}
	return out
}
