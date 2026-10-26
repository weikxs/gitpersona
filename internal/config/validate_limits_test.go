package config

import (
	"strings"
	"testing"
)

func TestLengthLimitsMatchTheOldBoundedPatterns(t *testing.T) {
	cases := []struct {
		name  string
		valid func(string) bool
		s     string
		want  bool
	}{
		{"id at 32", validID, "a" + strings.Repeat("b", 31), true},
		{"id at 33", validID, "a" + strings.Repeat("b", 32), false},
		{"account at 100", ValidAccount, "a" + strings.Repeat("b", 99), true},
		{"account at 101", ValidAccount, "a" + strings.Repeat("b", 100), false},
		{"host at 253", ValidHost, "a" + strings.Repeat("b", 251) + "c", true},
		{"host at 254", ValidHost, "a" + strings.Repeat("b", 252) + "c", false},
		{"host at 253 with a port", ValidHost, "a" + strings.Repeat("b", 251) + "c:8443", true},
		{"host ending in a dot", ValidHost, "example.", false},
		{"one-letter host", ValidHost, "h", true},
	}
	for _, c := range cases {
		if got := c.valid(c.s); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
