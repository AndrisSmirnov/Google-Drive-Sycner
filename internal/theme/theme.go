// Package theme holds the colour schemes of the interface, all approved on
// the mockup. Terminal is the default.
package theme

import "github.com/charmbracelet/lipgloss"

// Palette is one variant of a colour scheme, as hex strings.
type Palette struct {
	Bg, Panel, Line, Fg, Dim, Accent, On, Second, Ok, Warn, Err string
}

// Scheme is a colour scheme with a dark variant and, for most, a light one.
type Scheme struct {
	ID   string // stable identifier, used on the command line
	Key  string // the key that picks the scheme in the settings
	Name string // shown as is in every language
	Dark Palette
	// Light is the variant for light terminal backgrounds. Schemes that
	// copy a dark-only look leave it empty.
	Light Palette
}

// Schemes lists the colour schemes in the order the settings show them.
// The first ten are picked by 1 to 9 and 0, later ones by a letter. The
// first one is the default.
var Schemes = []Scheme{
	{
		// The colours of the terminal this tool was designed in: pale green
		// text on black, bright green headings, blue links. The dim colour
		// is lightened to stay readable on a highlighted row.
		ID: "terminal", Key: "1", Name: "Terminal",
		Dark: Palette{
			Bg: "#000000", Panel: "#0d2a14", Line: "#2f4f33",
			Fg: "#bffdc1", Dim: "#799379",
			Accent: "#00fa51", On: "#000000", Second: "#00a9ff",
			Ok: "#00ff00", Warn: "#dfad4f", Err: "#ff6142",
		},
	},
	{
		// FAR Manager panels: aqua on navy, yellow highlights.
		ID: "far", Key: "2", Name: "FAR Manager",
		Dark: Palette{
			Bg: "#000080", Panel: "#00545c", Line: "#008080",
			Fg: "#00ffff", Dim: "#c0c0c0",
			Accent: "#ffff00", On: "#000000", Second: "#ffffff",
			Ok: "#00ff00", Warn: "#ffaa55", Err: "#ff6b6b",
		},
	},
	{
		// The 16 colours of a DOS screen: grey on black, a blue bar.
		ID: "ms-dos", Key: "3", Name: "MS-DOS",
		Dark: Palette{
			Bg: "#000000", Panel: "#0000aa", Line: "#555555",
			Fg: "#aaaaaa", Dim: "#00aaaa",
			Accent: "#ffffff", On: "#000000", Second: "#55ffff",
			Ok: "#55ff55", Warn: "#ffff55", Err: "#ff5555",
		},
	},
	{
		// After the VS Code theme Word: Word for DOS, white on blue.
		ID: "word", Key: "4", Name: "Word",
		Dark: Palette{
			Bg: "#0000aa", Panel: "#000055", Line: "#aaaaaa",
			Fg: "#ffffff", Dim: "#aaaaaa",
			Accent: "#55ffff", On: "#000000", Second: "#ffff55",
			Ok: "#55ff55", Warn: "#ffaa55", Err: "#ff6666",
		},
	},
	{
		// After the VS Code theme Matrix: green on black. Its red is
		// brightened, the original does not read on black.
		ID: "matrix", Key: "5", Name: "Matrix",
		Dark: Palette{
			Bg: "#0c0c0c", Panel: "#003300", Line: "#006600",
			Fg: "#00cc00", Dim: "#00aa00",
			Accent: "#88ff88", On: "#000000", Second: "#ddffdd",
			Ok: "#bbffbb", Warn: "#cccc00", Err: "#ff4040",
		},
	},
	{
		// The built-in VS Code theme: dark grey, blue selection.
		ID: "dark-plus", Key: "6", Name: "Dark+",
		Dark: Palette{
			Bg: "#1e1e1e", Panel: "#04395e", Line: "#3c3c3c",
			Fg: "#d4d4d4", Dim: "#a6a6a6",
			Accent: "#75beff", On: "#1e1e1e", Second: "#dcdcaa",
			Ok: "#89d185", Warn: "#cca700", Err: "#f14c4c",
		},
	},
	{
		ID: "gruvbox", Key: "7", Name: "Gruvbox",
		// Gruvbox Material Dark.
		Dark: Palette{
			Bg: "#292828", Panel: "#3a3635", Line: "#45403d",
			Fg: "#d4be98", Dim: "#ad9f8b",
			Accent: "#e78a4e", On: "#292828", Second: "#89b482",
			Ok: "#a9b665", Warn: "#d8a657", Err: "#ea6962",
		},
		// Gruvbox Material Light, darkened so text stays readable.
		Light: Palette{
			Bg: "#fbf1c7", Panel: "#ebddb5", Line: "#e0cfa9",
			Fg: "#654735", Dim: "#685d54",
			Accent: "#984908", On: "#fbf1c7", Second: "#416950",
			Ok: "#66712b", Warn: "#975f08", Err: "#b54646",
		},
	},
	{
		// Nord: cold blue-greys. Its dim text, folder colour and red are
		// lightened; the original is too low in contrast for small text.
		ID: "nord", Key: "8", Name: "Nord",
		Dark: Palette{
			Bg: "#2e3440", Panel: "#3b4252", Line: "#4c566a",
			Fg: "#d8dee9", Dim: "#a5aebf",
			Accent: "#88c0d0", On: "#2e3440", Second: "#98b2cc",
			Ok: "#a3be8c", Warn: "#ebcb8b", Err: "#d08a91",
		},
	},
	{
		// Cold blue-grey with a turquoise accent.
		ID: "ice", Key: "9", Name: "Ice",
		Dark: Palette{
			Bg: "#0e1922", Panel: "#182a39", Line: "#2f485d",
			Fg: "#d9e7f0", Dim: "#8fabbd",
			Accent: "#4fd6c9", On: "#04211e", Second: "#86bdff",
			Ok: "#86dc96", Warn: "#f2c45a", Err: "#ff8590",
		},
		Light: Palette{
			Bg: "#f3f8fb", Panel: "#e0edf4", Line: "#b9d0dc",
			Fg: "#0f212d", Dim: "#4b6676",
			Accent: "#08706a", On: "#ffffff", Second: "#1958a8",
			Ok: "#196f36", Warn: "#7d5600", Err: "#b3243a",
		},
	},
	{
		// Deep violet with a lilac accent.
		ID: "violet", Key: "0", Name: "Violet",
		Dark: Palette{
			Bg: "#17131f", Panel: "#261e35", Line: "#403457",
			Fg: "#e8e2f3", Dim: "#a79bbf",
			Accent: "#b89aff", On: "#1a0f33", Second: "#f58ad9",
			Ok: "#74e3aa", Warn: "#f7c063", Err: "#ff7a6b",
		},
		Light: Palette{
			Bg: "#fbf8ff", Panel: "#ede5fb", Line: "#cfc1ea",
			Fg: "#211933", Dim: "#62557d",
			Accent: "#6237c8", On: "#ffffff", Second: "#a01f80",
			Ok: "#1a7248", Warn: "#7c5200", Err: "#b82020",
		},
	},
	{
		// GitHub's own colours, from its design system (Primer 11.10) rather
		// than from the editor theme. The text colours are the originals.
		// The dark highlight is GitHub's selected list row; the light one is
		// a shade lighter than GitHub's so the blue accent stays readable.
		ID: "github", Key: "g", Name: "GitHub",
		Dark: Palette{
			Bg: "#0d1117", Panel: "#1f232a", Line: "#3d444d",
			Fg: "#f0f6fc", Dim: "#9198a1",
			Accent: "#4493f8", On: "#0d1117", Second: "#ab7df8",
			Ok: "#3fb950", Warn: "#d29922", Err: "#f85149",
		},
		Light: Palette{
			Bg: "#ffffff", Panel: "#f1f3f5", Line: "#d1d9e0",
			Fg: "#1f2328", Dim: "#59636e",
			Accent: "#0969da", On: "#ffffff", Second: "#8250df",
			Ok: "#1a7f37", Warn: "#9a6700", Err: "#d1242f",
		},
	},
}

// Index finds a scheme by its identifier.
func Index(id string) (int, bool) {
	for i, s := range Schemes {
		if s.ID == id {
			return i, true
		}
	}
	return 0, false
}

// HasLight tells whether the scheme has a variant for light backgrounds.
func (s Scheme) HasLight() bool { return s.Light != Palette{} }

// Variant picks the dark or the light palette of a scheme. A scheme without
// a light variant is always dark.
func (s Scheme) Variant(dark bool) Palette {
	if dark || !s.HasLight() {
		return s.Dark
	}
	return s.Light
}

// Theme is a palette resolved to terminal colours.
type Theme struct {
	IsDark bool
	// Bg is NoColor when the terminal's own background is kept.
	Bg, Panel                                        lipgloss.TerminalColor
	Line, Fg, Dim, Accent, On, Second, Ok, Warn, Err lipgloss.Color
}

// New resolves the dark or light variant of a scheme. With paintBg off, the
// terminal's own background shows through everywhere except highlighted rows.
func New(s Scheme, dark, paintBg bool) Theme {
	dark = dark || !s.HasLight()
	p := s.Variant(dark)
	t := Theme{
		IsDark: dark,
		Bg:     lipgloss.NoColor{},
		Panel:  lipgloss.Color(p.Panel),
		Line:   lipgloss.Color(p.Line),
		Fg:     lipgloss.Color(p.Fg),
		Dim:    lipgloss.Color(p.Dim),
		Accent: lipgloss.Color(p.Accent),
		On:     lipgloss.Color(p.On),
		Second: lipgloss.Color(p.Second),
		Ok:     lipgloss.Color(p.Ok),
		Warn:   lipgloss.Color(p.Warn),
		Err:    lipgloss.Color(p.Err),
	}
	if paintBg {
		t.Bg = lipgloss.Color(p.Bg)
	}
	return t
}
