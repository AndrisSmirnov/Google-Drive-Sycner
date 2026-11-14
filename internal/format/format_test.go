package format

import "testing"

func TestSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{512, "512b"},
		{3 * Kb, "3Kb"},
		{199 * Mb, "199Mb"},
		{2 * Mb, "2Mb"},
		{Gb + Gb/10, "1.1Gb"},
		{12 * Gb, "12Gb"},
		{Tb, "1Tb"},
	}
	for _, c := range cases {
		if got := Size(c.in); got != c.want {
			t.Errorf("Size(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestGroup(t *testing.T) {
	cases := []struct {
		n    int
		sep  string
		want string
	}{
		{0, " ", "0"},
		{999, " ", "999"},
		{18207, " ", "18 207"},
		{1000000, ",", "1,000,000"},
		{-1234, " ", "-1 234"},
	}
	for _, c := range cases {
		if got := Group(c.n, c.sep); got != c.want {
			t.Errorf("Group(%d, %q) = %q, want %q", c.n, c.sep, got, c.want)
		}
	}
}
