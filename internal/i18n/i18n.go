// Package i18n loads interface texts from embedded JSON catalogs.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"

	"syncer-gdrive/internal/format"
)

//go:embed locales/*.json
var locales embed.FS

// Lang is one interface language. Each has a catalog in locales/.
type Lang struct {
	Code string
	Name string // in the language itself
}

// Langs lists the interface languages in menu order.
var Langs = []Lang{
	{"uk", "Українська"},
	{"en", "English"},
	{"pl", "Polski"},
	{"ja", "日本語"},
	{"zh", "中文"}, // simplified
}

type entry struct {
	text  string
	forms map[string]string
}

// Catalog holds the texts of one language.
type Catalog struct {
	Code    string
	msgs    map[string]entry
	missing map[string]bool // keys asked for but absent
}

func (c *Catalog) miss(key string) string {
	if c.missing == nil {
		c.missing = map[string]bool{}
	}
	c.missing[key] = true
	return key
}

// Missing lists the keys that were requested but are not in the catalog.
func (c *Catalog) Missing() []string {
	keys := make([]string, 0, len(c.missing))
	for k := range c.missing {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Load reads the catalog for a language code.
func Load(code string) (*Catalog, error) {
	raw, err := locales.ReadFile("locales/" + code + ".json")
	if err != nil {
		return nil, fmt.Errorf("i18n: no catalog for %q: %w", code, err)
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("i18n: %s.json: %w", code, err)
	}
	c := &Catalog{Code: code, msgs: make(map[string]entry, len(data))}
	for key, value := range data {
		var text string
		if json.Unmarshal(value, &text) == nil {
			c.msgs[key] = entry{text: text}
			continue
		}
		var forms map[string]string
		if err := json.Unmarshal(value, &forms); err != nil {
			return nil, fmt.Errorf("i18n: %s.json: key %q: %w", code, key, err)
		}
		c.msgs[key] = entry{forms: forms}
	}
	return c, nil
}

// T returns the text for key, formatted with args when given.
// A missing key comes back as the key itself so gaps are visible.
func (c *Catalog) T(key string, args ...any) string {
	e, ok := c.msgs[key]
	if !ok || e.forms != nil {
		return c.miss(key)
	}
	if len(args) == 0 {
		return e.text
	}
	return fmt.Sprintf(e.text, args...)
}

// N returns the plural form of key for n with the number filled in:
// N("n.files", 4) -> "4 файли".
func (c *Catalog) N(key string, n int) string {
	e, ok := c.msgs[key]
	if !ok || e.forms == nil {
		return c.miss(key)
	}
	form := e.forms[plural(c.Code, n)]
	if form == "" {
		form = e.forms["other"]
	}
	return fmt.Sprintf(form, c.Num(n))
}

// Num groups the digits of n the way the language writes them.
func (c *Catalog) Num(n int) string {
	return format.Group(n, c.T("num.sep"))
}

// Keys returns every key of the catalog, sorted.
func (c *Catalog) Keys() []string {
	keys := make([]string, 0, len(c.msgs))
	for k := range c.msgs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// plural picks the CLDR plural category for whole numbers.
func plural(code string, n int) string {
	if n < 0 {
		n = -n
	}
	m10, m100 := n%10, n%100
	switch code {
	case "uk":
		switch {
		case m10 == 1 && m100 != 11:
			return "one"
		case m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14):
			return "few"
		default:
			return "many"
		}
	case "pl":
		switch {
		case n == 1:
			return "one"
		case m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14):
			return "few"
		default:
			return "many"
		}
	case "en":
		if n == 1 {
			return "one"
		}
		return "other"
	default:
		return "other"
	}
}
