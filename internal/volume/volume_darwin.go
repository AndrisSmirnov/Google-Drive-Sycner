package volume

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// Stat describes the volume a folder is on. The folder need not exist yet.
func Stat(path string) (Info, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(existing(path), &st); err != nil {
		return Info{}, err
	}
	info := Info{
		Free:       int64(st.Bavail) * int64(st.Bsize),
		Total:      int64(st.Blocks) * int64(st.Bsize),
		FileSystem: unix.ByteSliceToString(st.Fstypename[:]),
		Mount:      unix.ByteSliceToString(st.Mntonname[:]),
	}
	info.Name = filepath.Base(info.Mount)
	if info.Mount == "/" || info.Mount == "/System/Volumes/Data" {
		info.Name = "" // the system disk has no name worth showing
	}
	return info, nil
}

// mountParents are the folders in which connected drives appear.
func mountParents() []string { return []string{"/Volumes"} }

// Roots lists the connected volumes.
func Roots() []string {
	entries, _ := os.ReadDir("/Volumes")
	var out []string
	for _, e := range entries {
		out = append(out, filepath.Join("/Volumes", e.Name()))
	}
	return out
}
