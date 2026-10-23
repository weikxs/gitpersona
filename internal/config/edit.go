package config

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/weikxs/gitpersona/internal/paths"
)

// Text edits of the configuration file. They change only the lines that
// matter and keep the user's comments and layout. Callers must re-parse the
// result and compare it with the intended configuration before writing it
// (see app.saveEdited), so an unusual layout can never be silently corrupted.

// EditOptions controls how configured paths are compared.
type EditOptions struct {
	Home            string
	CaseInsensitive bool
}

const (
	profileHeader = "[[profile]]"
	pinHeader     = "[[pin]]"
)

// keyPattern matches `key =`, `"key" =` and `'key' =` (all valid TOML).
var keyPattern = regexp.MustCompile(`^(\s*)(?:"([A-Za-z_]+)"|'([A-Za-z_]+)'|([A-Za-z_]+))\s*=\s*`)

// keyName returns the key of a keyPattern match.
func keyName(m []string) string {
	return m[2] + m[3] + m[4]
}

// eol is "\r" when the file uses CRLF line endings, so inserted lines match.
func eol(lines []string) string {
	for _, l := range lines {
		if strings.HasSuffix(l, "\r") {
			return "\r"
		}
	}
	return ""
}

// AddDir appends dir to the dirs of the profile with the given id.
func AddDir(text, profileID, dir string, _ EditOptions) (string, error) {
	lines, nl := splitText(text)
	b, ok := findProfile(lines, profileID)
	if !ok {
		return "", fmt.Errorf("profile %q does not exist", profileID)
	}
	value := tomlQuote(dir)
	k := keyLine(lines, b, "dirs")
	if k < 0 {
		return joinText(insertKey(lines, b, "dirs", "["+value+"]"), nl), nil
	}
	arr, err := parseArray(lines, k)
	if err != nil {
		return "", err
	}
	return joinText(arr.add(lines, value), nl), nil
}

// RemoveDir removes dir from whichever profile lists it and reports that
// profile's id. A dirs key left empty is removed.
func RemoveDir(text, dir string, o EditOptions) (string, string, error) {
	target, err := paths.Normalize(dir, o.Home)
	if err != nil {
		return "", "", err
	}
	lines, nl := splitText(text)
	for _, b := range tables(lines, profileHeader) {
		k := keyLine(lines, b, "dirs")
		if k < 0 {
			continue
		}
		arr, err := parseArray(lines, k)
		if err != nil {
			return "", "", err
		}
		for i, el := range arr.elems {
			if norm, err := paths.Normalize(el.value, o.Home); err == nil && paths.Same(norm, target, o.CaseInsensitive) {
				id, _ := stringValue(lines[keyLine(lines, b, "id")])
				return joinText(arr.remove(lines, i), nl), id, nil
			}
		}
	}
	return "", "", fmt.Errorf("%s is not assigned to any profile", dir)
}

// AddPin pins repo to the profile, updating an existing pin for the same repository.
func AddPin(text, repo, profileID string, o EditOptions) (string, error) {
	lines, nl := splitText(text)
	if b, ok := findPin(lines, repo, o); ok {
		k := keyLine(lines, b, "profile")
		if k < 0 {
			return "", errors.New("existing pin has no profile key; edit it by hand")
		}
		lines[k] = replaceValue(lines[k], tomlQuote(profileID))
		return joinText(lines, nl), nil
	}
	cr := eol(lines)
	if len(lines) > 0 {
		lines = append(lines, cr)
	}
	lines = append(lines, pinHeader+cr, "repo    = "+tomlQuote(repo)+cr, "profile = "+tomlQuote(profileID)+cr)
	return joinText(lines, true), nil
}

// RemovePin removes the pin for repo, together with the blank line before it.
func RemovePin(text, repo string, o EditOptions) (string, error) {
	lines, nl := splitText(text)
	b, ok := findPin(lines, repo, o)
	if !ok {
		return "", fmt.Errorf("%s is not pinned", repo)
	}
	start := b.start
	if start > 0 && strings.TrimSpace(lines[start-1]) == "" {
		start--
	}
	return joinText(append(append([]string{}, lines[:start]...), lines[b.end:]...), nl), nil
}

// table is a [[header]] section: lines [start, end).
type table struct{ start, end int }

func tables(lines []string, header string) []table {
	var ts []table
	for i, l := range lines {
		if stripComment(l) != header {
			continue
		}
		end := i + 1
		for end < len(lines) && !strings.HasPrefix(stripComment(lines[end]), "[") {
			end++
		}
		ts = append(ts, table{start: i, end: end})
	}
	return ts
}

func findProfile(lines []string, id string) (table, bool) {
	for _, b := range tables(lines, profileHeader) {
		if k := keyLine(lines, b, "id"); k >= 0 {
			if v, ok := stringValue(lines[k]); ok && v == id {
				return b, true
			}
		}
	}
	return table{}, false
}

func findPin(lines []string, repo string, o EditOptions) (table, bool) {
	target, err := paths.Normalize(repo, o.Home)
	if err != nil {
		return table{}, false
	}
	for _, b := range tables(lines, pinHeader) {
		k := keyLine(lines, b, "repo")
		if k < 0 {
			continue
		}
		v, _ := stringValue(lines[k])
		if norm, err := paths.Normalize(v, o.Home); err == nil && paths.Same(norm, target, o.CaseInsensitive) {
			return b, true
		}
	}
	return table{}, false
}

func keyLine(lines []string, b table, key string) int {
	for i := b.start + 1; i < b.end; i++ {
		if m := keyPattern.FindStringSubmatch(lines[i]); m != nil && keyName(m) == key {
			return i
		}
	}
	return -1
}

// stringValue returns the string value of a `key = "value"` line.
func stringValue(line string) (string, bool) {
	loc := keyPattern.FindStringIndex(line)
	if loc == nil {
		return "", false
	}
	v, end, err := scanString(line, loc[1])
	return v, err == nil && end > loc[1]
}

func replaceValue(line, quoted string) string {
	loc := keyPattern.FindStringIndex(line)
	_, end, err := scanString(line, loc[1])
	if err != nil {
		return line[:loc[1]] + quoted
	}
	return line[:loc[1]] + quoted + line[end:]
}

// scanString reads the TOML string literal starting at s[i] and returns its
// value and the index just after it.
func scanString(s string, i int) (string, int, error) {
	if i >= len(s) {
		return "", i, errors.New("missing string")
	}
	switch s[i] {
	case '\'':
		j := strings.IndexByte(s[i+1:], '\'')
		if j < 0 {
			return "", i, errors.New("unterminated string")
		}
		return s[i+1 : i+1+j], i + j + 2, nil
	case '"':
		for j := i + 1; j < len(s); j++ {
			switch s[j] {
			case '\\':
				j++
			case '"':
				v, err := strconv.Unquote(s[i : j+1])
				return v, j + 1, err
			}
		}
		return "", i, errors.New("unterminated string")
	default:
		return "", i, errors.New("expected a string")
	}
}

func stripComment(line string) string {
	if i := strings.IndexByte(line, '#'); i >= 0 && !strings.ContainsAny(line[:i], `"'`) {
		line = line[:i]
	}
	return strings.TrimSpace(line)
}

func tomlQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`).Replace(s) + `"`
}

func splitText(text string) ([]string, bool) {
	nl := strings.HasSuffix(text, "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil, nl
	}
	return strings.Split(text, "\n"), nl
}

func joinText(lines []string, nl bool) string {
	s := strings.Join(lines, "\n")
	if nl && s != "" {
		s += "\n"
	}
	return s
}

func insert(lines []string, at int, line string) []string {
	out := append([]string{}, lines[:at]...)
	out = append(out, line)
	return append(out, lines[at:]...)
}

// insertKey adds key = raw at the end of table b's keys, aligned like its id
// line ("id      = ..." gives "dirs    = ..."), so the result reads as if
// written by hand.
func insertKey(lines []string, b table, key, raw string) []string {
	at := b.start + 1
	for i := b.end - 1; i > b.start; i-- {
		if t := strings.TrimSpace(lines[i]); t != "" && !strings.HasPrefix(t, "#") {
			at = i + 1
			break
		}
	}
	indent, width := "", len(key)+1
	if id := keyLine(lines, b, "id"); id >= 0 {
		m := keyPattern.FindStringSubmatch(lines[id])
		indent = m[1]
		// The key column: everything before "=", such as "id      ".
		width = max(width, len(strings.TrimRight(m[0], " \t"))-len(indent)-1)
	}
	return insert(lines, at, fmt.Sprintf("%s%-*s= %s%s", indent, width, key, raw, eol(lines)))
}
