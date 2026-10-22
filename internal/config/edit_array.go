package config

import (
	"errors"
	"strings"
)

// array is a TOML array of strings that may span several lines.
type array struct {
	keyLine   int
	closeLine int
	closeCol  int
	elems     []element
}

// element is one string in the array, located by line and columns.
type element struct {
	line, start, end int // end is just after the closing quote
	value            string
}

// parseArray reads the array value of the key on line k.
func parseArray(lines []string, k int) (array, error) {
	loc := keyPattern.FindStringIndex(lines[k])
	if loc == nil || loc[1] >= len(lines[k]) || lines[k][loc[1]] != '[' {
		return array{}, errors.New("dirs is not an array; edit it by hand")
	}
	arr := array{keyLine: k}
	for ln, col := k, loc[1]+1; ln < len(lines); ln, col = ln+1, 0 {
		s := lines[ln]
		for col < len(s) {
			switch c := s[col]; {
			case c == ']':
				arr.closeLine, arr.closeCol = ln, col
				return arr, nil
			case c == '#':
				col = len(s)
			case c == '"' || c == '\'':
				v, end, err := scanString(s, col)
				if err != nil {
					return array{}, err
				}
				arr.elems = append(arr.elems, element{line: ln, start: col, end: end, value: v})
				col = end
			default:
				col++
			}
		}
	}
	return array{}, errors.New("dirs array is not closed")
}

func (a array) singleLine() bool { return a.closeLine == a.keyLine }

// add appends a quoted value, matching the array's layout.
func (a array) add(lines []string, quoted string) []string {
	out := append([]string{}, lines...)
	if a.singleLine() || len(a.elems) == 0 {
		line := out[a.closeLine]
		before := strings.TrimRight(line[:a.closeCol], " ")
		switch {
		case len(a.elems) == 0:
			before += quoted
		case strings.HasSuffix(before, ","):
			before += " " + quoted
		default:
			before += ", " + quoted
		}
		out[a.closeLine] = before + line[a.closeCol:]
		return out
	}

	last := a.elems[len(a.elems)-1]
	if !strings.HasPrefix(strings.TrimSpace(out[last.line][last.end:]), ",") {
		out[last.line] = out[last.line][:last.end] + "," + out[last.line][last.end:]
	}
	if strings.TrimSpace(out[a.closeLine][:a.closeCol]) != "" {
		// The closing bracket follows the last element on the same line.
		line := out[a.closeLine]
		col := a.closeCol
		if a.closeLine == last.line {
			col++ // account for the comma just inserted
		}
		out[a.closeLine] = line[:col] + " " + quoted + line[col:]
		return out
	}
	indent := out[last.line][:len(out[last.line])-len(strings.TrimLeft(out[last.line], " \t"))]
	return insert(out, a.closeLine, indent+quoted+","+eol(out))
}

// remove deletes element i; an array left empty takes its key with it.
func (a array) remove(lines []string, i int) []string {
	if len(a.elems) == 1 {
		return append(append([]string{}, lines[:a.keyLine]...), lines[a.closeLine+1:]...)
	}
	el := a.elems[i]
	if a.singleLine() {
		rest := make([]string, 0, len(a.elems)-1)
		for j, e := range a.elems {
			if j != i {
				rest = append(rest, lines[e.line][e.start:e.end])
			}
		}
		line := lines[a.keyLine]
		open := strings.IndexByte(line, '[')
		out := append([]string{}, lines...)
		out[a.keyLine] = line[:open+1] + strings.Join(rest, ", ") + line[a.closeCol:]
		return out
	}
	if onlyElementOnLine(lines[el.line], el) {
		return append(append([]string{}, lines[:el.line]...), lines[el.line+1:]...)
	}
	out := append([]string{}, lines...)
	line := out[el.line]
	end := el.end
	if trimmed := strings.TrimLeft(line[end:], " "); strings.HasPrefix(trimmed, ",") {
		end = len(line) - len(trimmed) + 1
	}
	out[el.line] = line[:el.start] + strings.TrimLeft(line[end:], " ")
	return out
}

func onlyElementOnLine(line string, el element) bool {
	before := strings.TrimSpace(line[:el.start])
	after := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line[el.end:]), ","))
	return before == "" && (after == "" || strings.HasPrefix(after, "#"))
}
