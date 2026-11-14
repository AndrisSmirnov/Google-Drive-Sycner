package i18n

import (
	"reflect"
	"testing"
)

func TestReadyCatalogsHaveTheSameKeys(t *testing.T) {
	var base *Catalog
	for _, l := range Langs {
		if !l.Ready {
			continue
		}
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
