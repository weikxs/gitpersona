// Package textdiff renders line differences between two small texts, so the
// configuration page can show exactly what a change will write.
package textdiff

import "strings"

// contextLines is how many unchanged lines surround each change.
const contextLines = 3

type op struct {
	kind byte // ' ', '-' or '+'
	line string
}

// Unified returns a unified diff of before and after, or "" when they are
// equal. An empty before or after is shown as /dev/null.
func Unified(before, after, name string) string {
	if before == after {
		return ""
	}
	from, to := name, name
	if before == "" {
		from = "/dev/null"
	}
	if after == "" {
		to = "/dev/null"
	}
	ops := diff(splitLines(before), splitLines(after))

	var b strings.Builder
	b.WriteString("--- " + from + "\n+++ " + to + "\n")
	for _, hunk := range hunks(ops) {
		b.WriteString("@@\n")
		for _, o := range hunk {
			b.WriteByte(o.kind)
			b.WriteString(o.line + "\n")
		}
	}
	return b.String()
}

// diff computes an edit script with a longest-common-subsequence table,
// which is fine for configuration files of a few hundred lines.
func diff(a, b []string) []op {
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var ops []op
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			ops = append(ops, op{' ', a[i]})
			i, j = i+1, j+1
		case i < len(a) && (j == len(b) || lcs[i+1][j] >= lcs[i][j+1]):
			ops = append(ops, op{'-', a[i]}) // deletions first, as diff tools show them
			i++
		default:
			ops = append(ops, op{'+', b[j]})
			j++
		}
	}
	return ops
}

// hunks groups changes with up to contextLines unchanged lines around them.
func hunks(ops []op) [][]op {
	keep := make([]bool, len(ops))
	for i, o := range ops {
		if o.kind == ' ' {
			continue
		}
		for k := max(0, i-contextLines); k <= min(len(ops)-1, i+contextLines); k++ {
			keep[k] = true
		}
	}
	var out [][]op
	var current []op
	for i, o := range ops {
		if keep[i] {
			current = append(current, o)
			continue
		}
		if len(current) > 0 {
			out = append(out, current)
			current = nil
		}
	}
	if len(current) > 0 {
		out = append(out, current)
	}
	return out
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}
