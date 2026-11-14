package theme

import (
	"math"
	"strconv"
	"testing"
)

// luminance is the WCAG relative luminance of a #rrggbb colour.
func luminance(t *testing.T, hex string) float64 {
	t.Helper()
	if len(hex) != 7 || hex[0] != '#' {
		t.Fatalf("%q is not a #rrggbb colour", hex)
	}
	var sum float64
	for i, weight := range []float64{0.2126, 0.7152, 0.0722} {
		v, err := strconv.ParseUint(hex[1+2*i:3+2*i], 16, 8)
		if err != nil {
			t.Fatalf("%q is not a #rrggbb colour", hex)
		}
		c := float64(v) / 255
		if c <= 0.03928 {
			c /= 12.92
		} else {
			c = math.Pow((c+0.055)/1.055, 2.4)
		}
		sum += weight * c
	}
	return sum
}

func contrast(t *testing.T, a, b string) float64 {
	t.Helper()
	x, y := luminance(t, a), luminance(t, b)
	if x < y {
		x, y = y, x
	}
	return (x + 0.05) / (y + 0.05)
}

// Every text colour must be readable on the background, and the text of a
// filled button on the accent: at least 4.5:1. The same holds on a
// highlighted row for the colours that carry text there. The status colours
// only mark a short word on such a row, where 3:1 is the floor.
func TestEverySchemeIsReadable(t *testing.T) {
	const min, statusOnPanel = 4.5, 3.0
	seen := map[string]bool{}
	for _, s := range Schemes {
		if s.ID == "" || seen[s.ID] {
			t.Errorf("scheme %q: the identifier is empty or repeated", s.ID)
		}
		seen[s.ID] = true
		if len(s.Key) != 1 || seen["key "+s.Key] {
			t.Errorf("scheme %s: the key %q is not one character or is taken", s.ID, s.Key)
		}
		seen["key "+s.Key] = true
		if s.Name == "" {
			t.Errorf("scheme %s has no name", s.ID)
		}
		if (s.Dark == Palette{}) {
			t.Errorf("scheme %s has no dark variant", s.ID)
			continue
		}
		variants := map[string]Palette{"dark": s.Dark}
		if s.HasLight() {
			variants["light"] = s.Light
		}
		for name, p := range variants {
			text := map[string]string{"Fg": p.Fg, "Dim": p.Dim, "Accent": p.Accent, "Second": p.Second,
				"Ok": p.Ok, "Warn": p.Warn, "Err": p.Err}
			for role, colour := range text {
				if c := contrast(t, colour, p.Bg); c < min {
					t.Errorf("%s %s: %s %s on Bg %s is %.2f:1", s.ID, name, role, colour, p.Bg, c)
				}
				want := min
				if role == "Ok" || role == "Warn" || role == "Err" {
					want = statusOnPanel
				}
				if c := contrast(t, colour, p.Panel); c < want {
					t.Errorf("%s %s: %s %s on Panel %s is %.2f:1, want %.1f:1", s.ID, name, role, colour, p.Panel, c, want)
				}
			}
			if c := contrast(t, p.On, p.Accent); c < min {
				t.Errorf("%s %s: On %s on Accent %s is %.2f:1", s.ID, name, p.On, p.Accent, c)
			}
			if dark := luminance(t, p.Bg) < 0.2; dark != (name == "dark") {
				t.Errorf("%s %s: the background %s belongs to the other variant", s.ID, name, p.Bg)
			}
		}
	}
}

// A scheme without a light variant stays dark whatever is asked for.
func TestDarkOnlySchemesStayDark(t *testing.T) {
	found := false
	for _, s := range Schemes {
		if s.HasLight() {
			if th := New(s, false, true); th.IsDark {
				t.Errorf("%s: the light variant was not used", s.ID)
			}
			continue
		}
		found = true
		th := New(s, false, true)
		if !th.IsDark || string(th.Accent) != s.Dark.Accent {
			t.Errorf("%s: asked for light, got dark = %v, accent = %s", s.ID, th.IsDark, th.Accent)
		}
	}
	if !found {
		t.Error("no dark-only scheme to test")
	}
}

func TestIndex(t *testing.T) {
	for want, s := range Schemes {
		if got, ok := Index(s.ID); !ok || got != want {
			t.Errorf("Index(%q) = %d, %v; want %d", s.ID, got, ok, want)
		}
	}
	if _, ok := Index("no-such-scheme"); ok {
		t.Error("Index found a scheme that does not exist")
	}
}
