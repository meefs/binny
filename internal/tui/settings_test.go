package tui

import "testing"

func TestParseBudget(t *testing.T) {
	ok := map[string]int{
		"120000": 120000,
		"150k":   150000,
		"1m":     1000000,
		"1.5m":   1500000,
		" 200K ": 200000,
		"2000":   2000,
	}
	for in, want := range ok {
		got, valid := parseBudget(in)
		if !valid || got != want {
			t.Errorf("parseBudget(%q) = %d,%v; want %d,true", in, got, valid, want)
		}
	}
	for _, bad := range []string{"", "abc", "0", "-5", "999", "12x", "k"} {
		if got, valid := parseBudget(bad); valid {
			t.Errorf("parseBudget(%q) = %d,true; want invalid", bad, got)
		}
	}
}
