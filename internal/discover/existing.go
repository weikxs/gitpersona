// Package discover finds what a user already has, so GitPersona can suggest
// a configuration instead of asking for one. It only reads Git configuration
// and repository metadata; it never reads private keys or tokens.
package discover

import (
	"cmp"
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
	globalName := readGlobals(ctx, home, gitconfig, run).name
	for _, r := range identityRules(ctx, home, gitconfig, run) {
		identities = merge(identities, r.id, r.dir, globalName)
	}
	return identities, nil
}

// globals are the settings an included file inherits from the global
// configuration that includes it.
type globals struct {
	name, format, gpgSign string
}

// readGlobals reads the global configuration with the files it includes,
// such as dotfiles, from the home (outside any repository, so no includeIf
// gitdir rule applies).
func readGlobals(ctx context.Context, home, gitconfig string, run provider.Runner) globals {
	get := func(key string) string {
		out, _ := run(ctx, "git", "-C", home, "config", "--file", gitconfig, "--includes", "--get", key)
		return strings.TrimSpace(string(out))
	}
	return globals{name: get("user.name"), format: get("gpg.format"), gpgSign: get("commit.gpgsign")}
}

// Rule is a hand-made includeIf identity rule: its condition (such as
// "gitdir:~/old/") and the directory it covers. Keep says why GitPersona
// cannot replace it, or is "" when a profile does everything it does.
type Rule struct {
	Condition string
	Dir       string
	Email     string // the email the included file sets
	Name      string // the user.name it sets, or ""
	Keep      string
}

// RuleSettings is an includeIf rule of a configuration file and the
// settings (lowercase keys) its file sets. Dir is the directory a directory
// rule covers ("~" for the whole home), or "" for other rules.
type RuleSettings struct {
	Condition string
	Dir       string
	Keys      map[string]bool
}

// RulesSettings returns the includeIf rules of a configuration file, in
// order, with the settings their files set.
func RulesSettings(ctx context.Context, home, gitconfig string, run provider.Runner) []RuleSettings {
	var out []RuleSettings
	for _, p := range includeIfPaths(ctx, gitconfig, run) {
		file := expandHome(p.path, home)
		if !filepath.IsAbs(file) {
			file = filepath.Join(filepath.Dir(gitconfig), file)
		}
		if realPath(file) == realPath(gitconfig) {
			continue
		}
		r := RuleSettings{Condition: p.condition, Dir: withTilde(gitdirToDir(p.condition), home, strings.HasPrefix(p.condition, "gitdir/i:")), Keys: map[string]bool{}}
		if out, err := run(ctx, "git", "config", "--file", file, "--name-only", "--list"); err == nil {
			for _, key := range strings.Fields(string(out)) {
				r.Keys[key] = true
			}
		}
		out = append(out, r)
	}
	return out
}

// OtherIdentityRules returns the includeIf rules that set an identity but
// are not plain directory rules (onbranch, hasconfig, a single repository,
// one for the whole home), with Dir "".
func OtherIdentityRules(ctx context.Context, home, gitconfig string, run provider.Runner) []Rule {
	var rules []Rule
	global := readGlobals(ctx, home, gitconfig, run)
	for _, p := range includeIfPaths(ctx, gitconfig, run) {
		// Directory rules are IdentityRules, except one for the whole home.
		if dir := withTilde(gitdirToDir(p.condition), home, strings.HasPrefix(p.condition, "gitdir/i:")); dir != "" && dir != "~" {
			continue
		}
		file := expandHome(p.path, home)
		if realPath(file) == realPath(gitconfig) {
			continue
		}
		if id, _ := readIdentity(ctx, file, global, run); id.Email != "" || id.Name != "" {
			rules = append(rules, Rule{Condition: p.condition, Email: id.Email, Name: id.Name})
		}
	}
	return rules
}

// IdentityRules returns the includeIf rules Existing reads identities from,
// in order, so they can be removed once GitPersona has taken them over.
func IdentityRules(ctx context.Context, home, gitconfig string, run provider.Runner) []Rule {
	var rules []Rule
	for _, r := range identityRules(ctx, home, gitconfig, run) {
		rules = append(rules, Rule{Condition: r.condition, Dir: r.dir, Email: r.id.Email, Name: r.id.Name, Keep: r.keep})
	}
	return rules
}

// includeIfPath is the path setting of an includeIf section; settings is
// the number of settings its condition has in all, counted on its first path.
type includeIfPath struct {
	condition, path string
	settings        int
}

// includeIfPaths returns the path settings of the includeIf sections of a
// configuration file, in order.
func includeIfPaths(ctx context.Context, gitconfig string, run provider.Runner) []includeIfPath {
	out, err := run(ctx, "git", "config", "--file", gitconfig, "--get-regexp", `^includeif\.`)
	if err != nil {
		return nil // no includeIf rules (or no file)
	}
	counts := map[string]int{}
	var paths []includeIfPath
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		key, value, _ := strings.Cut(line, " ")
		dot := strings.LastIndex(key, ".")
		if dot <= len(includeIfPrefix) {
			continue
		}
		condition := key[len(includeIfPrefix):dot]
		counts[condition]++
		if key[dot+1:] == "path" {
			paths = append(paths, includeIfPath{condition: condition, path: strings.TrimSpace(value)})
		}
	}
	seen := map[string]bool{}
	for i, p := range paths {
		if !seen[p.condition] {
			seen[p.condition] = true
			paths[i].settings = counts[p.condition]
		}
	}
	return paths
}

type identityRule struct {
	condition, dir, keep string
	id                   Identity
}

// identityRules returns the plain directory includeIf rules whose included
// file sets an email.
func identityRules(ctx context.Context, home, gitconfig string, run provider.Runner) []identityRule {
	paths := includeIfPaths(ctx, gitconfig, run)
	entries := map[string]int{} // condition -> number of settings in its sections
	for _, p := range paths {
		entries[p.condition] += p.settings
	}
	global := readGlobals(ctx, home, gitconfig, run)
	var rules []identityRule
	for _, p := range paths {
		dir := withTilde(gitdirToDir(p.condition), home, strings.HasPrefix(p.condition, "gitdir/i:"))
		if dir == "" || dir == "~" {
			continue // a rule for the whole home is a default, not a directory rule
		}
		file := expandHome(p.path, home)
		if realPath(file) == realPath(gitconfig) {
			continue // a rule that includes the global configuration itself sets no identity of its own
		}
		id, keep := readIdentity(ctx, file, global, run)
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
// Signing settings the file leaves out come from global, as in Git.
func readIdentity(ctx context.Context, file string, global globals, run provider.Runner) (Identity, string) {
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
	inherit := func(key, global string) string {
		if v, ok := values[key]; ok {
			return v
		}
		return global
	}
	key := values["user.signingkey"]
	if key != "" && !gitTrue(inherit("commit.gpgsign", global.gpgSign)) && keep == "" {
		// A profile that signs signs every commit; this key serves tags and
		// `git commit -S` only, so the rule stays.
		keep = "it sets a signing key without signing every commit"
	}
	if key != "" && gitTrue(inherit("commit.gpgsign", global.gpgSign)) {
		format := inherit("gpg.format", global.format)
		switch format {
		case "", "openpgp":
			id.Signing = Signing{Method: "gpg", Key: key}
		case "ssh":
			id.Signing = Signing{Method: "ssh", Key: key}
		default:
			keep = "it signs with " + format // not a method GitPersona sets
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

// merge adds dir to the identity with the same email and author name, or
// adds the identity: the same email with another name is another identity.
// A file without a name commits with the global name.
func merge(identities []Identity, id Identity, dir, globalName string) []Identity {
	name := func(x Identity) string { return cmp.Or(x.Name, globalName) }
	i := slices.IndexFunc(identities, func(x Identity) bool { return x.Email == id.Email && name(x) == name(id) })
	if i < 0 {
		id.Dirs = []string{dir}
		return append(identities, id)
	}
	out := slices.Clone(identities)
	out[i].Name = cmp.Or(out[i].Name, id.Name)
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
	// "dir/", "dir/**" and "dir/**/" all cover dir.
	pattern = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(pattern, "/"), "**"), "/")
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
