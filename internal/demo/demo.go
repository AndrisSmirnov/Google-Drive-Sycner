// Package demo provides the made-up data the menu runs on in demo mode.
// Nothing here touches Google Drive or the file system.
package demo

import (
	"strings"

	"syncer-gdrive/internal/format"
)

const (
	Account = "backup@syncer-demo.iam.gserviceaccount.com"
	KeyPath = "~/Downloads/syncer-demo-4f2a91.json"
	KeyDate = "2026-11-13"
	Today   = "2026-11-14"
)

// Destination is a folder the copy can go to.
type Destination struct {
	Path     string
	Volume   string
	Free     int64
	Capacity int64
	LastSync string // empty when nothing has been synced there yet
	Missing  bool   // the folder is not there: the drive is not connected
}

// Destinations lists the folders the demo knows: the one set up in the
// first run, and a second drive that has never been synced to.
var Destinations = []Destination{
	{Path: "/Volumes/DOOMSDAY/drive-backup", Volume: "DOOMSDAY · exFAT", Free: 412 * format.Gb, Capacity: format.Tb, LastSync: "2026-11-12 21:40"},
	{Path: "/Volumes/ARCHIVE-2/drive-backup", Volume: "ARCHIVE-2 · APFS", Free: 1600 * format.Gb, Capacity: 2 * format.Tb},
}

// Selection state of a node.
const (
	None = iota
	Partial
	All
)

// Node is a file or folder on Drive.
type Node struct {
	ID       string // on Drive; empty in the demo
	Name     string
	Local    string // the name on the drive, where it differs from Name
	Dir      bool
	Size     int64  // files only
	Modified string // files only, YYYY-MM-DD
	Export   string // Google documents: what they are exported to
	Checked  bool   // files only
	Open     bool   // folders only
	// New tells, for a folder selected only in part, whether files and
	// folders that appear in it later are synced too. A folder selected as
	// a whole always takes them.
	New      bool
	Children []*Node
}

// Total is the size of a file, or of everything inside a folder.
func (n *Node) Total() int64 {
	if !n.Dir {
		return n.Size
	}
	var sum int64
	for _, c := range n.Children {
		sum += c.Total()
	}
	return sum
}

// Selected counts the checked files under the node and their size.
func (n *Node) Selected() (files int, bytes int64) {
	if !n.Dir {
		if n.Checked {
			return 1, n.Size
		}
		return 0, 0
	}
	for _, c := range n.Children {
		f, b := c.Selected()
		files += f
		bytes += b
	}
	return files, bytes
}

func (n *Node) count() int {
	if !n.Dir {
		return 1
	}
	total := 0
	for _, c := range n.Children {
		total += c.count()
	}
	return total
}

// State tells whether all, some or none of the files under the node are checked.
func (n *Node) State() int {
	sel, _ := n.Selected()
	switch {
	case sel == 0:
		return None
	case sel == n.count():
		return All
	default:
		return Partial
	}
}

// SetChecked checks or unchecks the node and everything inside it.
func (n *Node) SetChecked(v bool) {
	if !n.Dir {
		n.Checked = v
		return
	}
	for _, c := range n.Children {
		c.SetChecked(v)
	}
}

// LastModified is the file's date, or the newest date inside a folder.
func (n *Node) LastModified() string {
	if !n.Dir {
		return n.Modified
	}
	latest := ""
	for _, c := range n.Children {
		if d := c.LastModified(); d > latest {
			latest = d
		}
	}
	return latest
}

// Root is the one folder shared with the Service Account. Everything the
// demo knows about Drive lives in it, so every path starts with its name:
// the account sees the shared folder itself, not its contents as a root.
const Root = "Syncer"

func rooted(path string) string { return Root + "/" + path }

// Lost is a second shared folder that the account can no longer see. It is
// what the lost-access state warns about.
var Lost = struct {
	Name string
	Size int64
}{"Спільні звіти", 18 * format.Gb}

// Tree returns what is shared with the Service Account: the one folder.
func Tree() []*Node {
	file := func(name string, size int64, mod string, checked bool) *Node {
		return &Node{Name: name, Size: size, Modified: mod, Checked: checked}
	}
	gdoc := func(name, export string, size int64, mod string) *Node {
		return &Node{Name: name, Export: export, Size: size, Modified: mod, Checked: true}
	}
	dir := func(name string, open bool, kids ...*Node) *Node {
		return &Node{Name: name, Dir: true, Open: open, New: true, Children: kids}
	}
	return []*Node{dir(Root, true,
		dir("Відео", true,
			dir("2025 Карпати", false,
				file("підйом.mov", 12*format.Gb, "2025-08-19", true),
				file("вершина.mov", 38*format.Gb, "2025-08-20", true),
				file("спуск.mov", 34*format.Gb, "2025-08-21", true),
			),
			dir("2026 Лісабон", true,
				file("день-1.mov", 31*format.Gb, "2026-05-01", true),
				file("день-2.mov", 38*format.Gb, "2026-05-02", false),
				file("нарізка.mp4", 199*format.Mb, "2026-05-09", true),
			),
		),
		dir("Документи", false,
			file("архів-сканів.zip", 11*format.Gb+900*format.Mb, "2026-11-12", true),
			file("договір-оренди.pdf", 2*format.Mb, "2026-10-03", true),
			gdoc("Податки 2025", "Sheets → .xlsx", 3*format.Kb, "2026-03-14"),
			gdoc("Річний звіт", "Docs → .docx", 14*format.Mb, "2026-11-10"),
		),
		dir("Фото", false,
			file("IMG_0042.jpg", 4*format.Mb, "2026-08-19", true),
			file("IMG_1180.heic", 3*format.Mb, "2026-09-02", true),
			file("панорама.tif", 38*format.Mb, "2026-08-19", true),
		),
		dir("Архів 2019", false,
			file("бекап-ноутбука.dmg", 44*format.Gb, "2019-12-30", false),
			file("фото-2019.zip", 4*format.Gb, "2019-11-17", false),
		),
		gdoc("Бюджет 2026", "Sheets → .xlsx", 5*format.Kb, "2026-11-01"),
		file("нотатки.md", 3*format.Kb, "2026-11-13", true),
		// Drive allows two files with one name in a folder; a drive does not.
		file("довідка.pdf", 1300*format.Kb, "2026-10-21", true),
		file("довідка.pdf", 1300*format.Kb, "2026-11-14", true),
	)}
}

// Entry is a file kept on the drive outside the copy: in the .bin trash or
// on the SKIP list. Dates are YYYY-MM-DD.
type Entry struct {
	ID       int64  // of the record; zero in the demo
	Stored   string // trash only: where the file lies now, relative to the destination
	Path     string // where the file was on Drive, with its folders
	Size     int64
	Created  string // on Drive
	Modified string // on Drive
	Added    string // when it went into the trash or onto the SKIP list
	Reason   string // trash only. drive: deleted on Drive; local: replaced by the Drive version
}

// Name is the file name without its folders.
func (e Entry) Name() string {
	return e.Path[strings.LastIndex(e.Path, "/")+1:]
}

// Dir is the folder the file was in, empty for a file at the top level.
func (e Entry) Dir() string {
	if i := strings.LastIndex(e.Path, "/"); i >= 0 {
		return e.Path[:i]
	}
	return ""
}

// Trash returns the contents of the trash. Two files share the name 1.doc
// and differ only in where they were.
func Trash() []Entry {
	out := []Entry{
		{0, "", "Відео/Інтерв'ю/інтерв'ю-сире.mov", 1900 * format.Mb, "2026-03-02", "2026-03-02", "2026-10-14", "drive"},
		{0, "", "Відео/Інтерв'ю/чернетка.mov", 1100 * format.Mb, "2026-09-21", "2026-10-30", "2026-11-01", "drive"},
		{0, "", "Відео/тест-камери.mp4", 612 * format.Mb, "2025-12-11", "2025-12-11", "2026-09-30", "drive"},
		{0, "", "Документи/Договори/архів-договорів.zip", 212 * format.Mb, "2024-06-18", "2026-01-09", "2026-10-02", "drive"},
		{0, "", "Документи/Договори/кошторис.xlsx", 840 * format.Kb, "2025-04-08", "2026-09-29", "2026-10-02", "drive"},
		{0, "", "Документи/звіт-v2.docx", 41 * format.Mb, "2026-07-15", "2026-10-27", "2026-10-28", "local"},
		{0, "", "Важливе/Документи/Мої/1.doc", 2 * format.Mb, "2024-02-11", "2025-03-19", "2026-11-09", "drive"},
		{0, "", "Ігри/NFSU2/stats/1.doc", 12 * format.Kb, "2019-07-04", "2019-08-30", "2026-11-09", "drive"},
		{0, "", "Архів 2019/старий-образ.dmg", 96 * format.Mb, "2023-02-27", "2023-02-27", "2026-08-03", "drive"},
		{0, "", "Фото/Карпати/панорама-стара.tif", 38 * format.Mb, "2025-08-17", "2025-08-20", "2026-08-19", "drive"},
		{0, "", "Фото/Карпати/IMG_0031.jpg", 5 * format.Mb, "2026-08-16", "2026-08-16", "2026-08-19", "drive"},
		{0, "", "Фото/Карпати/IMG_0032.jpg", 4 * format.Mb, "2026-08-16", "2026-08-16", "2026-08-19", "drive"},
		{0, "", "Музика/Концерти/концерт-запис.flac", 410 * format.Mb, "2025-05-24", "2025-06-02", "2026-10-20", "drive"},
		{0, "", "Музика/подкаст-чернетка.mp3", 58 * format.Mb, "2026-08-30", "2026-09-10", "2026-09-12", "drive"},
		{0, "", "Музика/голосова-нотатка.m4a", 3 * format.Mb, "2026-11-04", "2026-11-04", "2026-11-05", "drive"},
		{0, "", "нотатки-старі.md", 3 * format.Kb, "2025-01-15", "2026-06-01", "2026-07-22", "drive"},
	}
	for i := range out {
		out[i].Path = rooted(out[i].Path)
	}
	return out
}

// SkipList returns the SKIP list: files kept on the drive but no longer
// tracked.
func SkipList() []Entry {
	out := []Entry{
		{0, "", "нотатки-2024.md", 3 * format.Kb, "2024-01-08", "2024-12-20", "2026-11-13", ""},
		{0, "", "Архів/старий-проєкт.zip", 1200 * format.Mb, "2022-05-30", "2022-06-02", "2026-10-02", ""},
		{0, "", "Документи/чернетка-листа.docx", 48 * format.Kb, "2026-09-01", "2026-09-15", "2026-09-18", ""},
		{0, "", "Відео/Чернетки/невдалий-дубль.mov", 2300 * format.Mb, "2026-08-28", "2026-08-28", "2026-08-30", ""},
		{0, "", "Фото/Скріншоти/скріншот-1.png", format.Mb, "2026-08-10", "2026-08-10", "2026-08-12", ""},
		{0, "", "Фото/Скріншоти/скріншот-2.png", format.Mb, "2026-08-11", "2026-08-11", "2026-08-12", ""},
	}
	for i := range out {
		out[i].Path = rooted(out[i].Path)
	}
	return out
}

// Deleted is a file that vanished from Drive but still exists on the drive.
type Deleted struct {
	ID       string // on Drive; empty in the demo
	Path     string
	Size     int64
	Created  string
	Modified string
}

// DeletedOnDrive returns the files found deleted on Drive.
func DeletedOnDrive() []Deleted {
	out := []Deleted{
		{"", "Відео/чернетка-2.mov", 1100 * format.Mb, "2026-09-22", "2026-10-30"},
		{"", "Фото/IMG_0042-копія.jpg", 4 * format.Mb, "2026-08-19", "2026-08-19"},
		{"", "Документи/старий-договір.pdf", 2 * format.Mb, "2024-10-03", "2024-10-03"},
		{"", "нотатки-2025.md", 3 * format.Kb, "2025-01-02", "2025-12-29"},
	}
	for i := range out {
		out[i].Path = rooted(out[i].Path)
	}
	return out
}

// How a queued file ends up in the demo run.
const (
	OutcomeOK      = iota
	OutcomeTooBig  // Google document export over the 10Mb API limit
	OutcomeNetwork // network error
)

// Change is a file waiting to be downloaded.
type Change struct {
	Path    string
	Size    int64
	Updated bool // an existing file changed on Drive
	Local   bool // the copy on the drive was edited by hand
	Outcome int
}

// Changes returns the download queue of the demo run.
func Changes() []Change {
	out := []Change{
		{Path: "Документи/договір-оренди.pdf", Size: 2 * format.Mb, Updated: true},
		{Path: "Документи/Податки 2025.xlsx", Size: 3 * format.Kb, Updated: true},
		{Path: "Фото/IMG_2201.jpg", Size: 6 * format.Mb},
		{Path: "Фото/IMG_2202.jpg", Size: 5 * format.Mb},
		{Path: "Відео/2026 Лісабон/нарізка-v2.mp4", Size: 640 * format.Mb},
		{Path: "Документи/Річний звіт · Google Docs", Size: 14 * format.Mb, Outcome: OutcomeTooBig},
		{Path: "Фото/IMG_1180.heic", Size: 3 * format.Mb, Outcome: OutcomeNetwork},
		{Path: "Відео/2026 Лісабон/тизер.mov", Size: 410 * format.Mb},
		{Path: "Документи/звіт.docx", Size: 41 * format.Mb, Updated: true, Local: true},
		{Path: "Музика/демо-запис.wav", Size: 240 * format.Mb},
		{Path: "Документи/презентація.pptx", Size: 86 * format.Mb},
	}
	for i := range out {
		out[i].Path = rooted(out[i].Path)
	}
	return out
}
