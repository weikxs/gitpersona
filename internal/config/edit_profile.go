package config

import (
	"fmt"
	"slices"
	"strings"

	"github.com/weikxs/gitpersona/internal/i18n"
)

// editableFields are the profile keys SetProfileField may change. dirs are
// changed with AddDir and RemoveDir, signing with SetProfileSigning.
var editableFields = []string{"account", "name", "email", "provider", "host", "ssh_key"}

// AddProfile appends a new profile after the last existing one.
func AddProfile(text string, p Profile) (string, error) {
	lines, nl := splitText(text)
	if _, exists := findProfile(lines, p.ID); exists {
		return "", i18n.Errorf("profile %q already exists", p.ID)
	}
	cr := eol(lines)
	block := []string{profileHeader + cr}
	add := func(key, value string) {
		if value != "" {
			block = append(block, fmt.Sprintf("%-7s = %s%s", key, value, cr))
		}
	}
	add("id", tomlQuote(p.ID))
	if p.Provider != "" && p.Provider != ProviderGitHub {
		add("provider", tomlQuote(p.Provider))
	}
	if p.Host != "" && p.Host != DefaultGitHubHost {
		add("host", tomlQuote(p.Host))
	}
	add("account", tomlQuote(p.Account))
	add("name", tomlQuote(p.Name))
	add("email", tomlQuote(p.Email))
	if p.SSHKey != "" {
		add("ssh_key", tomlQuote(p.SSHKey))
	}
	if p.Signing.Method != "" {
		add("signing", signingValue(p.Signing))
	}
	if len(p.Dirs) > 0 {
		quoted := make([]string, len(p.Dirs))
		for i, d := range p.Dirs {
			quoted[i] = tomlQuote(d)
		}
		add("dirs", "["+strings.Join(quoted, ", ")+"]")
	}

	at := len(lines)
	if ts := tables(lines, profileHeader); len(ts) > 0 {
		at = ts[len(ts)-1].end
		for at > 0 && strings.TrimSpace(lines[at-1]) == "" {
			at--
		}
	}
	insertion := append([]string{cr}, block...)
	if at < len(lines) {
		insertion = append(insertion, cr) // keep a blank line before what follows
	}
	out := append(append(append([]string{}, lines[:at]...), insertion...), lines[at:]...)
	return joinText(out, nl || at == len(lines)), nil
}

// SetProfileField sets one key of a profile; an empty value removes the key.
func SetProfileField(text, profileID, key, value string) (string, error) {
	if !slices.Contains(editableFields, key) {
		return "", i18n.Errorf("%s cannot be set this way", key)
	}
	lines, nl := splitText(text)
	b, ok := findProfile(lines, profileID)
	if !ok {
		return "", i18n.Errorf("profile %q does not exist", profileID)
	}
	return joinText(setKey(lines, b, key, value), nl), nil
}

// SetProfileSigning sets a profile's signing; method "" removes it.
func SetProfileSigning(text, profileID string, s Signing) (string, error) {
	lines, nl := splitText(text)
	b, ok := findProfile(lines, profileID)
	if !ok {
		return "", i18n.Errorf("profile %q does not exist", profileID)
	}
	if k := keyLine(lines, b, "signing"); k >= 0 && !strings.Contains(lines[k], "{") {
		return "", i18n.Errorf("signing is written as a table; edit it by hand")
	}
	if s.Method == "" {
		return joinText(setKey(lines, b, "signing", ""), nl), nil
	}
	return joinText(setRawKey(lines, b, "signing", signingValue(s)), nl), nil
}

// RemoveProfile deletes a profile's block, including comments inside it.
func RemoveProfile(text, profileID string) (string, error) {
	lines, nl := splitText(text)
	b, ok := findProfile(lines, profileID)
	if !ok {
		return "", i18n.Errorf("profile %q does not exist", profileID)
	}
	out := append(append([]string{}, lines[:b.start]...), lines[b.end:]...)
	return joinText(out, nl), nil
}

// SetDefault sets the default profile; "" removes the default.
func SetDefault(text, profileID string) (string, error) {
	lines, nl := splitText(text)
	top := table{start: -1, end: len(lines)}
	for i, l := range lines {
		if strings.HasPrefix(stripComment(l), "[") {
			top.end = i
			break
		}
	}
	if k := keyLine(lines, top, "default"); k >= 0 {
		if profileID == "" {
			return joinText(append(append([]string{}, lines[:k]...), lines[k+1:]...), nl), nil
		}
		lines[k] = replaceValue(lines[k], tomlQuote(profileID))
		return joinText(lines, nl), nil
	}
	if profileID == "" {
		return text, nil
	}
	at := keyLine(lines, top, "version") + 1
	return joinText(insert(lines, at, "default = "+tomlQuote(profileID)+eol(lines)), nl), nil
}

// setKey replaces a string key's value, inserts it after the id line, or
// removes it when value is empty.
func setKey(lines []string, b table, key, value string) []string {
	if value == "" {
		if k := keyLine(lines, b, key); k >= 0 {
			return append(append([]string{}, lines[:k]...), lines[k+1:]...)
		}
		return lines
	}
	return setRawKey(lines, b, key, tomlQuote(value))
}

func setRawKey(lines []string, b table, key, raw string) []string {
	out := append([]string{}, lines...)
	if k := keyLine(out, b, key); k >= 0 {
		if strings.Contains(raw, "{") {
			// An inline table replaces the rest of the line.
			loc := keyPattern.FindStringIndex(out[k])
			out[k] = out[k][:loc[1]] + raw + eol(out)
			return out
		}
		out[k] = replaceValue(out[k], raw)
		return out
	}
	return insertKey(out, b, key, raw)
}

func signingValue(s Signing) string {
	if s.Key == "" {
		return "{ method = " + tomlQuote(s.Method) + " }"
	}
	return "{ method = " + tomlQuote(s.Method) + ", key = " + tomlQuote(s.Key) + " }"
}
