package names

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestClean(t *testing.T) {
	cases := map[string]string{
		"звіт.pdf":            "звіт.pdf",
		"26071782a?.webm":     "26071782a_.webm",
		"Billie Marten:.avif": "Billie Marten_.avif",
		"tumblr_500*.gif":     "tumblr_500_.gif",
		`a<b>c"d/e\f|g`:       "a_b_c_d_e_f_g",
		"Icon\r":              "Icon_",
		"tab\there":           "tab_here",
		"кінець з крапкою.":   "кінець з крапкою",
		"пробіл у кінці   ":   "пробіл у кінці",
		"...":                 "_",
		"":                    "_",
		"CON":                 "CON_",
		"con.txt":             "con_.txt",
		"LPT1.log":            "LPT1_.log",
		"console.txt":         "console.txt",
		".env":                ".env",
		"й":                   "й", // two code points become one
		"архів.tar.gz":        "архів.tar.gz",
	}
	for name, want := range cases {
		if got := Clean(name); got != want {
			t.Errorf("Clean(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestCleanShortensLongNames(t *testing.T) {
	long := strings.Repeat("ї", 200) + ".docx" // 400 bytes before the extension
	got := Clean(long)
	if len(got) > MaxBytes || !strings.HasSuffix(got, ".docx") || !utf8.ValidString(got) {
		t.Errorf("Clean gave %d bytes, valid %v, %q…", len(got), utf8.ValidString(got), got[:20])
	}
	if got := Clean(strings.Repeat("a", 300)); len(got) != MaxBytes {
		t.Errorf("a name without an extension is %d bytes", len(got))
	}
}

func TestNumbered(t *testing.T) {
	cases := map[string]string{"a.pdf": "a (2).pdf", "README": "README (2)", ".env": ".env (2)", "архів.tar.gz": "архів.tar (2).gz"}
	for name, want := range cases {
		if got := Numbered(name, 2); got != want {
			t.Errorf("Numbered(%q, 2) = %q, want %q", name, got, want)
		}
	}
	if got := Numbered("a.pdf", 1); got != "a.pdf" {
		t.Errorf("the first copy was renamed to %q", got)
	}
	long := strings.Repeat("я", 126) + ".pdf" // 252 + 4 = 256 bytes once cleaned to 255
	if got := Numbered(Clean(long), 12); len(got) > MaxBytes || !strings.HasSuffix(got, " (12).pdf") {
		t.Errorf("a numbered long name is %d bytes: …%q", len(got), got[len(got)-12:])
	}
}

func day(n int) time.Time { return time.Date(2026, 11, n, 0, 0, 0, 0, time.UTC) }

func TestResolve(t *testing.T) {
	items := []Item{
		{"new", "5. DevOps.pdf", day(14)},
		{"old", "5. DevOps.pdf", day(1)},
		{"mid", "5. devops.PDF", day(7)}, // differs only in case
		{"q", "what?.txt", day(2)},
		{"u", "what_.txt", day(3)}, // collides with the cleaned name above
		{"solo", "нотатки.md", day(5)},
		{"taken", "5. DevOps (2).pdf", day(10)}, // its own name is taken by then
	}
	got := Resolve(items)
	want := map[string]string{
		"old":   "5. DevOps.pdf",
		"mid":   "5. devops (2).PDF",
		"taken": "5. DevOps (2) (2).pdf", // the older "5. devops (2).PDF" holds that name
		"new":   "5. DevOps (3).pdf",
		"q":     "what_.txt",
		"u":     "what_ (2).txt",
		"solo":  "нотатки.md",
	}
	for id, name := range want {
		if got[id] != name {
			t.Errorf("%s is saved as %q, want %q", id, got[id], name)
		}
	}
	// No two items share a name on the drive.
	seen := map[string]string{}
	for id, name := range got {
		if other, dup := seen[Key(name)]; dup {
			t.Errorf("%s and %s both become %q", id, other, name)
		}
		seen[Key(name)] = id
	}
	// The order of the input does not matter.
	reversed := append([]Item(nil), items...)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	for id, name := range Resolve(reversed) {
		if got[id] != name {
			t.Errorf("%s: %q in one order, %q in another", id, got[id], name)
		}
	}
}

func TestClashes(t *testing.T) {
	clashes := Clashes([]Item{
		{"a", "довідка.pdf", day(1)},
		{"b", "довідка.pdf", day(9)},
		{"c", "інше.pdf", day(2)},
	})
	if len(clashes) != 2 || clashes["a"] != "довідка.pdf" || clashes["b"] != "довідка (2).pdf" {
		t.Errorf("clashes = %v", clashes)
	}
}

func TestIsService(t *testing.T) {
	for name, want := range map[string]bool{".DS_Store": true, "Icon\r": true, "Icon": false, "icon.png": false, ".env": false} {
		if IsService(name) != want {
			t.Errorf("IsService(%q) = %v", name, !want)
		}
	}
}
