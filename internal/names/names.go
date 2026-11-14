// Package names decides what a Drive item is called on the drive. Drive
// accepts almost any name, and several items in one folder may share it; a
// file system accepts neither. The rules are the same on every operating
// system, so a copy made on one can be read on another.
package names

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// MaxBytes is the longest name the common file systems take.
const MaxBytes = 255

// reserved are the device names Windows does not allow as a file name, with
// or without an extension.
var reserved = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true, "com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true, "lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

// IsService tells the files macOS leaves in folders for its own use: the
// folder view settings and the custom folder icon, whose name ends with a
// carriage return. They are not part of the user's data and are not copied.
func IsService(name string) bool {
	return name == ".DS_Store" || name == "Icon\r"
}

// Clean turns a Drive name into one every file system accepts:
// characters that are forbidden somewhere become "_", spaces and dots at
// the end go, a Windows device name gets "_" added, and a name that is too
// long is shortened with its extension kept.
func Clean(name string) string {
	// One form for letters with marks: macOS, Linux and Windows otherwise
	// disagree on whether "й" typed two ways is one name or two.
	name = norm.NFC.String(name)
	name = strings.Map(func(r rune) rune {
		switch {
		case r < 0x20, r == 0x7f, strings.ContainsRune(`<>:"/\|?*`, r):
			return '_'
		}
		return r
	}, name)
	name = strings.TrimRight(name, " .")
	if name == "" {
		return "_"
	}
	base, ext := split(name)
	if reserved[strings.ToLower(base)] {
		name = base + "_" + ext
	}
	return fit(name, MaxBytes)
}

// split separates a name into what comes before the extension and the
// extension with its dot. A name like ".env" has no extension.
func split(name string) (base, ext string) {
	ext = filepath.Ext(name)
	if ext == name {
		return name, ""
	}
	return strings.TrimSuffix(name, ext), ext
}

// fit shortens a name to at most max bytes, cutting the part before the
// extension and never in the middle of a character.
func fit(name string, max int) string {
	if len(name) <= max {
		return name
	}
	base, ext := split(name)
	if len(ext) >= max { // an absurdly long "extension" is just part of the name
		base, ext = name, ""
	}
	room := max - len(ext)
	for len(base) > room {
		_, size := utf8.DecodeLastRuneInString(base)
		base = base[:len(base)-size]
	}
	return strings.TrimRight(base, " .") + ext
}

// Numbered gives the n-th file of a shared name its own: the first keeps
// the name, the second "a.pdf" becomes "a (2).pdf".
func Numbered(name string, n int) string {
	if n <= 1 {
		return name
	}
	base, ext := split(name)
	suffix := " (" + strconv.Itoa(n) + ")"
	return fit(base, MaxBytes-len(suffix)-len(ext)) + suffix + ext
}

// Key is what two names are compared by to tell whether they would collide
// on a drive: most file systems ignore the case of letters.
func Key(name string) string {
	return cases.Fold().String(norm.NFC.String(name))
}

// Item is one entry of a folder on Drive.
type Item struct {
	ID      string
	Name    string
	Created time.Time
}

// Resolve gives every item of one folder the name it has on the drive. Each
// name is cleaned; where several items end up with the same one, the oldest
// keeps it and the others are numbered in the order they were created.
// Because the order depends only on the creation date and the identifier,
// the result is the same on every run.
func Resolve(items []Item) map[string]string {
	sorted := append([]Item(nil), items...)
	sort.SliceStable(sorted, func(a, b int) bool {
		if !sorted[a].Created.Equal(sorted[b].Created) {
			return sorted[a].Created.Before(sorted[b].Created)
		}
		return sorted[a].ID < sorted[b].ID
	})
	out := make(map[string]string, len(items))
	taken := map[string]bool{}
	for _, it := range sorted {
		clean := Clean(it.Name)
		name := clean
		// A numbered name may itself be taken ("a (2).pdf" exists on Drive).
		for n := 2; taken[Key(name)]; n++ {
			name = Numbered(clean, n)
		}
		taken[Key(name)] = true
		out[it.ID] = name
	}
	return out
}

// Clashes lists the items of a folder whose names collide on the drive,
// with the name each of them gets. It is what the interface warns about.
func Clashes(items []Item) map[string]string {
	resolved := Resolve(items)
	groups := map[string][]string{}
	for _, it := range items {
		key := Key(Clean(it.Name))
		groups[key] = append(groups[key], it.ID)
	}
	out := map[string]string{}
	for _, ids := range groups {
		if len(ids) > 1 {
			for _, id := range ids {
				out[id] = resolved[id]
			}
		}
	}
	return out
}
