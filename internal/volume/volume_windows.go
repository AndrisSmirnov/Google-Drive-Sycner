package volume

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// Stat describes the volume a folder is on. The folder need not exist yet.
func Stat(path string) (Info, error) {
	path = existing(path)
	abs, err := filepath.Abs(path)
	if err != nil {
		return Info{}, err
	}
	dir, err := windows.UTF16PtrFromString(abs)
	if err != nil {
		return Info{}, err
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(dir, &free, &total, &totalFree); err != nil {
		return Info{}, err
	}
	info := Info{Free: int64(free), Total: int64(total), Mount: filepath.VolumeName(abs) + `\`}

	root, err := windows.UTF16PtrFromString(info.Mount)
	if err != nil {
		return info, nil
	}
	var name, fs [windows.MAX_PATH + 1]uint16
	if windows.GetVolumeInformation(root, &name[0], uint32(len(name)), nil, nil, nil, &fs[0], uint32(len(fs))) == nil {
		info.Name = windows.UTF16ToString(name[:])
		info.FileSystem = strings.ToLower(windows.UTF16ToString(fs[:]))
	}
	if info.Name == "" {
		info.Name = strings.TrimSuffix(info.Mount, `\`)
	}
	return info, nil
}

// Roots lists the drive letters in use.
func Roots() []string {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil
	}
	var out []string
	for i := range 26 {
		if mask&(1<<i) != 0 {
			out = append(out, string(rune('A'+i))+`:\`)
		}
	}
	return out
}
