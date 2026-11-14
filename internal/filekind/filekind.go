// Package filekind sorts files into broad kinds by their extension.
package filekind

import (
	"path/filepath"
	"strings"
)

// The kinds, in the order the interface lists them.
const (
	Image = "image"
	Music = "music"
	Video = "video"
	Docs  = "docs"
	Other = "other" // archives and everything that fits nowhere else
)

// All lists the kinds in display order.
var All = []string{Image, Music, Video, Docs, Other}

var extensions = map[string][]string{
	Image: {"jpg", "jpeg", "png", "gif", "webp", "avif", "heic", "heif", "tif", "tiff", "bmp", "svg", "ico",
		"psd", "raw", "dng", "cr2", "cr3", "nef", "arw", "orf", "rw2"},
	Music: {"mp3", "wav", "flac", "aac", "m4a", "ogg", "oga", "opus", "aiff", "aif", "wma", "ape", "amr",
		"mid", "midi"},
	Video: {"mov", "mp4", "m4v", "mkv", "avi", "webm", "wmv", "flv", "mpg", "mpeg", "3gp", "mts", "m2ts"},
	Docs: {"pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx", "odt", "ods", "odp", "rtf", "txt", "md", "csv",
		"epub", "pages", "numbers", "key"},
}

var byExt = func() map[string]string {
	m := map[string]string{}
	for kind, exts := range extensions {
		for _, e := range exts {
			m[e] = kind
		}
	}
	return m
}()

// Ext is the lower-case extension of a file name without the dot.
func Ext(name string) string {
	return strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
}

// Of tells the kind of a file from its name.
func Of(name string) string {
	if kind, ok := byExt[Ext(name)]; ok {
		return kind
	}
	return Other
}
