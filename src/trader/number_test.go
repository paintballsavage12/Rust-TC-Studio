package trader

import "testing"

func TestParseNumber(t *testing.T) {
	for _, tc := range []struct {
		raw       string
		max, want int
		ok        bool
	}{
		{"0", 30, 0, true}, {" 29 ", 30, 29, true}, {"30", 30, 30, true},
		{"999", 500000, 999, true}, {"500000", 500000, 500000, true},
		{"", 30, 0, false}, {"31", 30, 0, false}, {"-1", 30, 0, false},
		{"1.2", 30, 0, false}, {"10x", 30, 0, false}, {"999999999999999999999", 500000, 0, false},
	} {
		n, err := ParseNumber(tc.raw, tc.max)
		if (err == nil) != tc.ok || (tc.ok && n != tc.want) {
			t.Errorf("ParseNumber(%q,%d)=(%d,%v), want (%d,%v)", tc.raw, tc.max, n, err, tc.want, tc.ok)
		}
	}
}
