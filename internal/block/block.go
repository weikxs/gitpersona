// Package block edits the one GitPersona-managed region inside a user file
// such as ~/.gitconfig or a shell startup file. Text outside the markers is
// never changed, and Remove undoes Upsert exactly.
package block

import (
	"errors"
	"fmt"
	"strings"
)

// Markers that delimit the managed region. They are comments in Git config,
// sh, zsh, bash and fish.
const (
	Begin = "# >>> gitpersona >>>"
	End   = "# <<< gitpersona <<<"
)

const newline = "\n"

// Upsert returns content with the managed region set to body. An existing
// region is replaced in place; otherwise the region is appended after a
// blank line.
func Upsert(content, body string) (string, error) {
	lines := splitLines(content)
	begin, end, err := locate(lines)
	if err != nil {
		return "", err
	}
	region := render(body)

	if begin >= 0 {
		return join(lines[:begin]) + region + join(lines[end+1:]), nil
	}
	switch {
	case content == "":
		return region, nil
	case strings.HasSuffix(content, newline):
		return content + newline + region, nil
	default:
		// No blank line: Remove can then tell that the newline was added.
		return content + newline + region, nil
	}
}

// Remove returns content without the managed region. When the region is at
// the end of the file, the blank line Upsert added before it is removed too.
func Remove(content string) (string, error) {
	lines := splitLines(content)
	begin, end, err := locate(lines)
	if err != nil || begin < 0 {
		return content, err
	}

	before, after := lines[:begin], lines[end+1:]
	if len(after) == 0 && len(before) > 0 {
		if before[len(before)-1] == newline {
			// The blank line Upsert added before the region.
			before = before[:len(before)-1]
		} else {
			// The file had no final newline; Upsert added one.
			return strings.TrimSuffix(join(before), newline), nil
		}
	}
	return join(before) + join(after), nil
}

// Contains reports whether content has a well-formed managed region.
func Contains(content string) bool {
	begin, _, err := locate(splitLines(content))
	return err == nil && begin >= 0
}

func render(body string) string {
	if body != "" && !strings.HasSuffix(body, newline) {
		body += newline
	}
	return Begin + newline + body + End + newline
}

// locate returns the line indexes of the markers, or -1, -1 if absent.
func locate(lines []string) (begin, end int, err error) {
	begins, ends := markerIndexes(lines, Begin), markerIndexes(lines, End)
	switch {
	case len(begins) == 0 && len(ends) == 0:
		return -1, -1, nil
	case len(begins) != 1 || len(ends) != 1:
		return 0, 0, fmt.Errorf("found %d %q and %d %q markers; expected exactly one pair", len(begins), Begin, len(ends), End)
	case begins[0] > ends[0]:
		return 0, 0, errors.New("the gitpersona end marker comes before the begin marker")
	default:
		return begins[0], ends[0], nil
	}
}

func markerIndexes(lines []string, marker string) []int {
	var idx []int
	for i, l := range lines {
		if strings.TrimRight(l, "\r\n") == marker {
			idx = append(idx, i)
		}
	}
	return idx
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.SplitAfter(s, newline)
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func join(lines []string) string {
	return strings.Join(lines, "")
}

// UpsertLast returns content with the managed region set to body and placed
// at the end of the file. Git lets later settings override earlier ones, so
// the region must come last to take effect over the user's own settings.
func UpsertLast(content, body string) (string, error) {
	without, err := Remove(content)
	if err != nil {
		return "", err
	}
	return Upsert(without, body)
}

// ContentAfter reports whether any setting (not a blank line or comment)
// follows the managed region; such settings override it.
func ContentAfter(content string) bool {
	lines := splitLines(content)
	_, end, err := locate(lines)
	if err != nil || end < 0 {
		return false
	}
	for _, l := range lines[end+1:] {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, ";") {
			return true
		}
	}
	return false
}

// Render returns the managed region for body, markers included, as Upsert
// would write it.
func Render(body string) string {
	return render(body)
}
