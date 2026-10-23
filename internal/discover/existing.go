// Package discover finds what a user already has, so GitPersona can suggest
// a configuration instead of asking for one. It only reads Git configuration
// and repository metadata; it never reads private keys or tokens.
package discover

import (
	"context"
	"path/filepath"
	"slices"
	"strings"

	"github.com/weikxs/gitpersona/internal/provider"
)

// Identity is one identity found in an existing setup.
type Identity struct {
	Name    string   `json:"name"`
	Email   string   `json:"email"`
	Account string   `json:"account,omitempty"` // credential username for github.com, if set
	Dirs    []string `json:"dirs"`              // directories, written with ~/ where possible
	Source  string   `json:"source"`            // the included file the identity came from
	SSHKey  string   `json:"ssh_key,omitempty"` // the -i key of core.sshCommand
	Signing Signing  `json:"signing"`           // commit signing, when the file turns it on
}

// Signing is an identity's commit signing: Method is "ssh" or "gpg", or ""
// when the identity does not sign.
type Signing struct {
	Method string `json:"method,omitempty"`
	Key    string `json:"key,omitempty"`
}

const includeIfPrefix = "includeif."

// Existing reads the includeIf rules of a global Git configuration and the
// files they include, and returns one identity per distinct email, in the
// order the rules appear. Rules that are not plain directory rules (such as
// hasconfig:) are skipped. GitPersona's own include is ignored.
func Existing(ctx context.Context, home, gitconfig string, run provider.Runner) ([]Identity, error) {
	var identities []Identity
	for _, r := range identityRules(ctx, home, gitconfig, run) {
		identities = merge(identities, r.id, r.dir)
	}
	return identities, nil
}

// Rule is a hand-made includeIf identity rule: its condition (such as
// "gitdir:~/old/") and the directory it covers. Keep says why GitPersona
// cannot replace it, or is "" when a profile does everything it does.
type Rule struct {
	Condition string
	Dir       string
	Email     string // the email the included file sets
	Keep      string
}

// IdentityRules returns the includeIf rules Existing reads identities from,
// in order, so they can be removed once GitPersona has taken them over.
func IdentityRules(ctx context.Context, home, gitconfig string, run provider.Runner) []Rule {
	var rules []Rule
	for _, r := range identityRules(ctx, home, gitconfig, run) {
		rules = append(rules, Rule{Condition: r.condition, Dir: r.dir, Email: r.id.Email, Keep: r.keep})
	}
	return rules
}

type identityRule struct {
	condition, dir, keep string
	id                   Identity
}

// identityRules returns the plain directory includeIf rules whose included
// file sets an email.
func identityRules(ctx context.Context, home, gitconfig string, run provider.Runner) []identityRule {
	out, err := run(ctx, "git", "config", "--file", gitconfig, "--get-regexp", `^includeif\.`)
	if err != nil {
		return nil // no includeIf rules (or no file): nothing to import
	}
	entries := map[string]int{} // condition -> number of settings in its sections
	type pathKey struct{ condition, path string }
	var paths []pathKey
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		key, value, _ := strings.Cut(line, " ")
		dot := strings.LastIndex(key, ".")
		if dot <= len(includeIfPrefix) {
			continue
		}
		condition := key[len(includeIfPrefix):dot]
		entries[condition]++
		if key[dot+1:] == "path" {
			paths = append(paths, pathKey{condition, strings.TrimSpace(value)})
		}
	}
	var rules []identityRule
	for _, p := range paths {
		dir := withTilde(gitdirToDir(p.condition), home, strings.HasPrefix(p.condition, "gitdir/i:"))
		if dir == "" || dir == "~" {
			continue // a rule for the whole home is a default, not a directory rule
		}
		id, keep := readIdentity(ctx, expandHome(p.path, home), run)
		if id.Email == "" {
			continue
		}
		if entries[p.condition] > 1 {
			keep = "its includeIf section includes more than one file"
		}
		rules = append(rules, identityRule{condition: p.condition, dir: dir, keep: keep, id: id})
	}
	return rules
}

// replaceable are the settings a profile takes over; a file with anything
// else keeps its includeIf rule.
var replaceable = map[string]bool{
	"user.name": true, "user.email": true, "user.signingkey": true, "gpg.format": true,
	"commit.gpgsign": true, "tag.gpgsign": true, "core.sshcommand": true,
	"credential.https://github.com.username": true,
}

// readIdentity reads the identity an included file sets. keep says why a
// profile cannot replace the file, or is "".
func readIdentity(ctx context.Context, file string, run provider.Runner) (Identity, string) {
	out, err := run(ctx, "git", "config", "--file", file, "--list")
	if err != nil {
		return Identity{}, ""
	}
	values, keep := map[string]string{}, ""
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		key, value, _ := strings.Cut(line, "=")
		values[key] = strings.TrimSpace(value)
		if !replaceable[key] && keep == "" {
			keep = "it also sets " + key
		}
	}
	id := Identity{
		Name:    values["user.name"],
		Email:   values["user.email"],
		Account: values["credential.https://github.com.username"],
		Source:  file,
		SSHKey:  sshCommandKey(values["core.sshcommand"]),
	}
	if cmd := values["core.sshcommand"]; cmd != "" && !plainSSHCommand(cmd) && keep == "" {
		keep = "its core.sshCommand has other options"
	}
	if key := values["user.signingkey"]; key != "" && gitTrue(values["commit.gpgsign"]) {
		switch values["gpg.format"] {
		case "", "openpgp":
			id.Signing = Signing{Method: "gpg", Key: key}
		case "ssh":
			id.Signing = Signing{Method: "ssh", Key: key}
		default:
			keep = "it signs with " + values["gpg.format"] // not a method GitPersona sets
		}
	}
	return id, keep
}

// gitTrue reports whether a Git boolean is true.
func gitTrue(v string) bool {
	switch strings.ToLower(v) {
	case "true", "yes", "on", "1":
		return true
	}
	return false
}

// plainSSHCommand reports whether an ssh command does no more than a
// profile's ssh_key: ssh with -i <key> and IdentitiesOnly.
func plainSSHCommand(command string) bool {
	args := shellWords(command)
	if len(args) == 0 || args[0] != "ssh" {
		return false
	}
	for i := 1; i < len(args); i++ {
		switch a := args[i]; {
		case a == "-i" || a == "-o":
			if i+1 >= len(args) || (a == "-o" && !identityOption(args[i+1])) {
				return false
			}
			i++
		case strings.HasPrefix(a, "-i"):
		case strings.HasPrefix(a, "-o") && identityOption(a[2:]):
		default:
			return false
		}
	}
	return true
}

func identityOption(o string) bool {
	return strings.HasPrefix(o, "IdentitiesOnly=") || strings.HasPrefix(o, "IdentityFile=")
}

// sshCommandKey returns the key file an ssh command line passes with -i (or
// -o IdentityFile=), or "".
func sshCommandKey(command string) string {
	args := shellWords(command)
	for i, a := range args {
		switch {
		case a == "-i" && i+1 < len(args):
			return args[i+1]
		case strings.HasPrefix(a, "-i"):
			return a[2:]
		case a == "-o" && i+1 < len(args):
			if v, ok := strings.CutPrefix(args[i+1], "IdentityFile="); ok {
				return v
			}
		case strings.HasPrefix(a, "-oIdentityFile="):
			return strings.TrimPrefix(a, "-oIdentityFile=")
		}
	}
	return ""
}

// shellWords splits a command line on spaces outside single or double quotes.
func shellWords(s string) []string {
	var words []string
	var word strings.Builder
	quote, inWord := rune(0), false
	for _, r := range s {
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote != 0:
			word.WriteRune(r)
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == ' ' || r == '\t':
			if inWord {
				words, inWord = append(words, word.String()), false
				word.Reset()
			}
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	if inWord {
		words = append(words, word.String())
	}
	return words
}

func merge(identities []Identity, id Identity, dir string) []Identity {
	i := slices.IndexFunc(identities, func(x Identity) bool { return x.Email == id.Email })
	if i < 0 {
		id.Dirs = []string{dir}
		return append(identities, id)
	}
	out := slices.Clone(identities)
	if !slices.Contains(out[i].Dirs, dir) {
		out[i].Dirs = append(slices.Clone(out[i].Dirs), dir)
	}
	return out
}

// gitdirToDir turns an includeIf condition into the directory it covers, or
// "" when the condition is not a plain directory rule.
func gitdirToDir(condition string) string {
	pattern, ok := strings.CutPrefix(condition, "gitdir:")
	if !ok {
		pattern, ok = strings.CutPrefix(condition, "gitdir/i:")
	}
	if !ok {
		return ""
	}
	pattern = strings.TrimSuffix(strings.TrimSuffix(pattern, "**"), "/")
	if !strings.HasSuffix(condition, "/") && !strings.HasSuffix(condition, "/**") {
		return "" // an exact .git directory: a single repository, not a directory rule
	}
	if !strings.HasPrefix(pattern, "~/") && !filepath.IsAbs(pattern) {
		return ""
	}
	return pattern
}

// withTilde writes a directory under home as ~/..., the way GitPersona
// shows and stores directories; the home itself is "~". With ignoreCase
// (gitdir/i: rules) the home may be written in another case.
func withTilde(dir, home string, ignoreCase bool) string {
	if dir == "" || home == "" {
		return dir
	}
	for _, h := range []string{home, realPath(home)} {
		h = strings.TrimSuffix(h, "/")
		switch {
		case dir == h || (ignoreCase && strings.EqualFold(dir, h)):
			return "~"
		case len(dir) > len(h) && dir[len(h)] == '/' && (dir[:len(h)] == h || (ignoreCase && strings.EqualFold(dir[:len(h)], h))):
			return "~" + dir[len(h):]
		}
	}
	return dir
}

// realPath resolves symbolic links in a path, or returns it unchanged.
func realPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

func expandHome(path, home string) string {
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		return filepath.Join(home, rest)
	}
	return path
}
