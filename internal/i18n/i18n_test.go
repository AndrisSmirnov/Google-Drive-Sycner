package i18n

import (
	"reflect"
	"regexp"
	"slices"
	"testing"
)

func load(t *testing.T, code string) *Catalog {
	t.Helper()
	c, err := Load(code)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCatalogsHaveTheSameKeys(t *testing.T) {
	var base *Catalog
	for _, l := range Langs {
		c, err := Load(l.Code)
		if err != nil {
			t.Fatalf("Load(%q): %v", l.Code, err)
		}
		if base == nil {
			base = c
			continue
		}
		if !reflect.DeepEqual(base.Keys(), c.Keys()) {
			t.Errorf("%s.json and %s.json have different keys", base.Code, c.Code)
		}
	}
}

func TestPluralForms(t *testing.T) {
	uk, err := Load("uk")
	if err != nil {
		t.Fatal(err)
	}
	en, err := Load("en")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		c    *Catalog
		n    int
		want string
	}{
		{uk, 1, "1 файл"},
		{uk, 4, "4 файли"},
		{uk, 5, "5 файлів"},
		{uk, 11, "11 файлів"},
		{uk, 21, "21 файл"},
		{uk, 112, "112 файлів"},
		{uk, 18207, "18 207 файлів"},
		{en, 1, "1 file"},
		{en, 18207, "18,207 files"},
		{load(t, "pl"), 1, "1 plik"},
		{load(t, "pl"), 2, "2 pliki"},
		{load(t, "pl"), 5, "5 plików"},
		{load(t, "pl"), 12, "12 plików"},
		{load(t, "pl"), 21, "21 plików"},
		{load(t, "pl"), 22, "22 pliki"},
		{load(t, "pl"), 18207, "18 207 plików"},
		{load(t, "ja"), 1, "1 ファイル"},
		{load(t, "ja"), 18207, "18,207 ファイル"},
		{load(t, "zh"), 1, "1 个文件"},
		{load(t, "zh"), 18207, "18,207 个文件"},
	}
	for _, tc := range cases {
		if got := tc.c.N("n.files", tc.n); got != tc.want {
			t.Errorf("%s N(n.files, %d) = %q, want %q", tc.c.Code, tc.n, got, tc.want)
		}
	}
}

func TestMissingKeyIsReported(t *testing.T) {
	c, err := Load("uk")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.T("no.such.key"); got != "no.such.key" {
		t.Errorf("T on a missing key = %q, want the key back", got)
	}
	if got := c.Missing(); len(got) != 1 || got[0] != "no.such.key" {
		t.Errorf("Missing() = %v", got)
	}
}

// A translation fills in the same values in the same order as the English
// text: the code passes them by position.
func TestTranslationsKeepThePlaceholders(t *testing.T) {
	verb := regexp.MustCompile(`%[a-z%]`)
	en := load(t, "en")
	for _, l := range Langs {
		c := load(t, l.Code)
		for key, want := range en.msgs {
			got, ok := c.msgs[key]
			if !ok {
				continue // the keys are compared by another test
			}
			if (want.forms == nil) != (got.forms == nil) {
				t.Errorf("%s: %q is a plural in one catalog and a plain text in the other", l.Code, key)
				continue
			}
			if want.forms == nil {
				if a, b := verb.FindAllString(got.text, -1), verb.FindAllString(want.text, -1); !slices.Equal(a, b) {
					t.Errorf("%s: %q fills in %v, the English text %v", l.Code, key, a, b)
				}
				continue
			}
			for form, text := range got.forms {
				if a := verb.FindAllString(text, -1); !slices.Equal(a, []string{"%s"}) {
					t.Errorf("%s: %q, form %q fills in %v, want one %%s", l.Code, key, form, a)
				}
			}
		}
	}
}

// Every plural text has a form for each category its language can ask for.
func TestPluralTextsHaveEveryForm(t *testing.T) {
	for _, l := range Langs {
		c := load(t, l.Code)
		needed := map[string]bool{}
		for n := 0; n < 200; n++ {
			needed[plural(l.Code, n)] = true
		}
		for key, e := range c.msgs {
			for form := range needed {
				if e.forms != nil && e.forms[form] == "" {
					t.Errorf("%s: %q has no form %q", l.Code, key, form)
				}
			}
		}
	}
}
