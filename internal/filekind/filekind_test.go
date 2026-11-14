package filekind

import "testing"

func TestOf(t *testing.T) {
	cases := map[string]string{
		"IMG_0042.jpg":      Image,
		"панорама.TIF":      Image,
		"IMG_1180.heic":     Image,
		"концерт.FLAC":      Music,
		"подкаст.mp3":       Music,
		"нотатка.m4a":       Music,
		"запис.wav":         Music,
		"дзвінок.ogg":       Music,
		"голос.opus":        Music,
		"семпл.aiff":        Music,
		"підйом.mov":        Video,
		"нарізка.mp4":       Video,
		"договір.pdf":       Docs,
		"звіт.v2.docx":      Docs,
		"Податки 2025.xlsx": Docs,
		"архів.zip":         Other,
		"бекап.tar.gz":      Other,
		"без-розширення":    Other,
		".DS_Store":         Other,
	}
	for name, want := range cases {
		if got := Of(name); got != want {
			t.Errorf("Of(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestEveryExtensionHasOneKind(t *testing.T) {
	seen := map[string]string{}
	for kind, exts := range extensions {
		for _, e := range exts {
			if other, dup := seen[e]; dup {
				t.Errorf("%q is listed under both %s and %s", e, other, kind)
			}
			seen[e] = kind
		}
	}
	for _, kind := range All {
		if kind != Other && len(extensions[kind]) == 0 {
			t.Errorf("kind %s has no extensions", kind)
		}
	}
}
