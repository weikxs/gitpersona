package generate

import "strings"

// quoteValue renders s as a Git config value, quoting it when Git would
// otherwise trim it or treat part of it as a comment.
func quoteValue(s string) string {
	needsQuotes := s == "" ||
		strings.TrimSpace(s) != s ||
		strings.ContainsAny(s, "#;\"\\\n\t")
	if !needsQuotes {
		return s
	}
	return `"` + escapeQuoted(s) + `"`
}

// quoteSubsection renders s as a quoted Git config subsection name.
func quoteSubsection(s string) string {
	return `"` + escapeQuoted(s) + `"`
}

func escapeQuoted(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`).Replace(s)
}

// escapeGlob makes Git's includeIf "gitdir:" pattern match path literally;
// otherwise characters such as [ ] * ? in a directory name act as wildcards.
func escapeGlob(path string) string {
	return strings.NewReplacer(`\`, `\\`, "*", `\*`, "?", `\?`, "[", `\[`).Replace(path)
}

// shellQuote quotes s for POSIX sh, which runs "!"-prefixed Git helpers.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
