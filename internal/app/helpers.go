package app

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/discover"
)

// secureHelpers are credential helpers that keep one token per account and
// host in a protected store, so several accounts on one host work: GitPersona
// only names the account and the helper answers with its token.
var secureHelpers = []string{"osxkeychain", "manager", "manager-core", "libsecret", "wincred", "gnome-keyring"}

// helperKind says what a credential.helper value is: "secure", "plain"
// (the store helper, which writes tokens to a file in clear text), "memory"
// (the cache helper) or "other".
func helperKind(helper string) string {
	switch name := helperName(shellFields(helper)); {
	case slices.Contains(secureHelpers, name):
		return "secure"
	case name == "store":
		return "plain"
	case name == "cache":
		return "memory"
	}
	return "other"
}

// urlMatches reports whether a credential.<url> pattern applies to every
// repository on https://host for a request with user, as Git's URL
// matching does: the https scheme, the same user if the pattern names one,
// no path, and the same host and port. A pattern with no host applies to
// every host; host labels may be "*" globs; letter case, a trailing dot,
// leading zeros in the port and the default port 443 do not count. A
// pattern without a scheme is [user@]host, which Git compares exactly.
func urlMatches(pattern, host, user string) bool { return urlMatch(pattern, host, user, false) }

// urlMatch is urlMatches; with somePaths, a pattern for some repositories
// of the host (one with a path) matches too.
func urlMatch(pattern, host, user string, somePaths bool) bool {
	scheme, rest, hasScheme := strings.Cut(pattern, "://")
	if !hasScheme {
		rest = pattern
	} else if !strings.EqualFold(scheme, "https") {
		return false
	}
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		named, _, _ := strings.Cut(rest[:at], ":")
		if decoded, err := url.PathUnescape(named); err == nil {
			named = decoded // Git decodes %-escapes
		}
		if named != user { // Git compares user names exactly
			return false
		}
		rest = rest[at+1:]
	}
	if h, p, ok := strings.Cut(rest, "/"); ok {
		if p != "" && !somePaths {
			return false // only some repositories of the host
		}
		rest = h
	}
	switch {
	case rest == "":
		return true // no host: every host
	case !hasScheme:
		return rest == host
	}
	wantHost, wantPort, ok1 := normalHostPort(rest)
	gotHost, gotPort, ok2 := normalHostPort(host)
	if !ok1 || !ok2 || wantPort != gotPort {
		return false
	}
	want, got := strings.Split(wantHost, "."), strings.Split(gotHost, ".")
	if len(want) != len(got) {
		return false
	}
	for i := range want {
		if !labelMatches(want[i], got[i]) {
			return false
		}
	}
	return true
}

// labelMatches matches one host label, where Git knows only "*", for any
// run of characters.
func labelMatches(pattern, label string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == label
	}
	if !strings.HasPrefix(label, parts[0]) {
		return false
	}
	label = label[len(parts[0]):]
	for _, part := range parts[1 : len(parts)-1] {
		i := strings.Index(label, part)
		if i < 0 {
			return false
		}
		label = label[i+len(part):]
	}
	return strings.HasSuffix(label, parts[len(parts)-1])
}

// normalHostPort returns host:port the way Git compares it: lowercase,
// without a trailing dot, the port as a number and "" for 443. Brackets
// are for IPv6 addresses only; ok is false for anything else in them.
func normalHostPort(hp string) (host, port string, ok bool) {
	hp = strings.ToLower(hp)
	if strings.HasPrefix(hp, "[") {
		end := strings.Index(hp, "]")
		if end < 0 || !strings.Contains(hp[1:end], ":") {
			return "", "", false
		}
		host, port = hp[1:end], strings.TrimPrefix(hp[end+1:], ":")
	} else {
		host, port, _ = strings.Cut(hp, ":")
	}
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil {
			return "", "", false
		}
		port = strconv.Itoa(n)
	}
	if port == "443" {
		port = ""
	}
	return strings.TrimSuffix(host, "."), port, true
}

// splitHostPort splits host:port, or [v6]:port; the default HTTPS port is
// no port.
func splitHostPort(hp string) (host, port string) {
	host, port, ok := normalHostPort(hp)
	if !ok {
		return hp, ""
	}
	return host, port
}

// helperProblem says what is wrong with the helpers that would keep a
// token, or "" when one keeps it safely, one per account.
func (e Env) helperProblem(host string, helpers []string) string {
	kinds := make([]string, len(helpers))
	for i, h := range helpers {
		kinds[i] = helperKind(h)
	}
	switch {
	case len(helpers) == 0:
		return e.tr("no credential helper would keep the token for %s, so Git asks for one on every push over HTTPS", host)
	case slices.Contains(kinds, "plain"):
		return e.tr("the credential helper for %s (store) keeps tokens in a file in clear text", host)
	case !slices.ContainsFunc(kinds, func(k string) bool { return k != "memory" }):
		return e.tr("the credential helper for %s (cache) forgets tokens after a while, so Git asks again", host)
	}
	return ""
}

// helperAdvice is how to set up a helper that keeps tokens safely, naming
// one that is installed here.
func (e Env) helperAdvice(ctx context.Context) string {
	execPath, _ := e.git(ctx, e.Home, "--exec-path")
	if h := installedHelper(runtime.GOOS, execPath, e.LookPath); h != "" {
		return e.tr("use the system keychain, for example `git config --global credential.helper %s`", h)
	}
	return e.tr("install a helper that keeps tokens in the system keychain, such as Git Credential Manager " +
		"(https://github.com/git-ecosystem/git-credential-manager) or your distribution's git-credential-libsecret, " +
		"then set it with `git config --global credential.helper <name>`")
}

// installedHelper returns the first secure helper of this system that is
// installed (on PATH or in Git's exec path), or "". Many Linux systems ship
// Git without libsecret's helper, so it is never suggested blindly; macOS's
// Git always has osxkeychain.
func installedHelper(goos, execPath string, lookPath func(string) (string, error)) string {
	candidates := []string{"manager", "libsecret"}
	if goos == "darwin" {
		candidates = []string{"osxkeychain", "manager"}
	}
	for _, name := range candidates {
		program := "git-credential-" + name
		if _, err := lookPath(program); err == nil {
			return name
		}
		if execPath == "" {
			continue
		}
		if info, err := os.Stat(filepath.Join(execPath, program)); err == nil && !info.IsDir() {
			return name
		}
	}
	if goos == "darwin" {
		return "osxkeychain"
	}
	return ""
}

// shellFields splits a command line on blanks (helpers rarely quote).
func shellFields(s string) []string { return strings.Fields(strings.TrimPrefix(s, "!")) }

// hostSettings returns the values of credential.<name> (name in lowercase)
// that Git takes for https://host and a request with user, in the order Git
// reads them: credential.<name> and every credential.<url>.<name> whose URL
// matches (see urlMatches), the system settings first.
func (e Env) hostSettings(ctx context.Context, host, user, name string, somePaths bool) ([]string, error) {
	entries, err := e.gitConfigRegexp(ctx, `^credential\..*`+name+`$`)
	if err != nil {
		return nil, err
	}
	var values []string
	for _, kv := range entries {
		rest, ok := strings.CutPrefix(kv[0], "credential.")
		if !ok {
			continue
		}
		pattern, key := "", rest
		if dot := strings.LastIndex(rest, "."); dot >= 0 {
			pattern, key = rest[:dot], rest[dot+1:]
		}
		if key == name && (pattern == "" || urlMatch(pattern, host, user, somePaths)) {
			values = append(values, kv[1])
		}
	}
	return values, nil
}

// helperName finds the helper a credential.helper command runs: the first
// git-credential-<name> program, or `git credential-<name>` (after git's
// own options), wherever it is in a shell command; "" when there is none.
// It is a best effort: a helper hidden in a script is "other".
func helperName(words []string) string {
	base := func(w string) string {
		return strings.TrimSuffix(filepath.Base(strings.Trim(w, `"'`)), ".exe")
	}
	for i := 0; i < len(words); i++ {
		b := base(strings.TrimPrefix(words[i], "!"))
		if name, ok := strings.CutPrefix(b, "git-credential-"); ok {
			return name
		}
		if b != "git" {
			continue
		}
		for j := i + 1; j < len(words); j++ {
			w := strings.Trim(words[j], `"'`)
			if w == "-c" || w == "-C" {
				j++ // the option's value
				continue
			}
			if strings.HasPrefix(w, "-") {
				continue
			}
			if name, ok := strings.CutPrefix(w, "credential-"); ok {
				return name
			}
			break
		}
	}
	if len(words) > 0 {
		return base(words[0]) // a short name such as "store"
	}
	return ""
}

// hostHelpers returns the credential helpers Git runs for https://host
// outside every rule: the user's own, in order, after the last reset ("").
// user is the user name in the request: Git matches https://user@host
// settings only with the user written in the remote's URL, never with
// credential.username, so for a push it is "" while `gitpersona auth
// login`, which names the account, also reaches them.
func (e Env) hostHelpers(ctx context.Context, host, user string) ([]string, error) {
	values, err := e.hostSettings(ctx, host, user, "helper", false)
	if err != nil {
		return nil, err
	}
	var helpers []string
	for _, v := range values {
		if v == "" {
			helpers = nil // an empty value resets the list
			continue
		}
		helpers = append(helpers, v)
	}
	return helpers, nil
}

// perRepositoryTokens says where credential.useHttpPath is on for host:
// the host itself, some of its repositories, or "" for nowhere. The last
// value Git reads wins.
func (e Env) perRepositoryTokens(ctx context.Context, host string) (string, error) {
	whole, err := e.hostSettings(ctx, host, "", "usehttppath", false)
	if err != nil {
		return "", err
	}
	if len(whole) > 0 && discover.GitTrue(whole[len(whole)-1]) {
		return host, nil
	}
	some, err := e.hostSettings(ctx, host, "", "usehttppath", true)
	if err != nil {
		return "", err
	}
	if slices.ContainsFunc(some, discover.GitTrue) {
		return e.tr("some repositories of %s", host), nil
	}
	return "", nil
}

// helperIssue is what is wrong with the credential helpers of a profile,
// and how to fix it.
type helperIssue struct{ problem, advice string }

// helpersProblem checks the helpers of a profile: those a push asks must
// keep the token, one per account and not per repository; those `auth
// login` saves it with must not write it in clear text, and must include
// one a push asks.
func (e Env) helpersProblem(ctx context.Context, p config.Profile) ([]string, helperIssue, error) {
	push, err := e.hostHelpers(ctx, p.Host, "")
	if err != nil {
		return nil, helperIssue{}, err
	}
	if problem := e.helperProblem(p.Host, push); problem != "" {
		return push, helperIssue{problem, e.helperAdvice(ctx)}, nil
	}
	if where, err := e.perRepositoryTokens(ctx, p.Host); err != nil || where != "" {
		return push, helperIssue{
			e.tr("credential.useHttpPath is on for %s, so your helper keeps a token per repository while GitPersona saves one per account", where),
			e.tr("turn it off where you set it (`git config --show-origin --get-regexp 'credential.*usehttppath'` shows where)")}, err
	}
	save, err := e.hostHelpers(ctx, p.Host, p.Account)
	if err != nil {
		return nil, helperIssue{}, err
	}
	accountSettings := e.tr("remove the credential helper settings for https://%s@%s (or %s@%s)", p.Account, p.Host, p.Account, p.Host)
	if slices.ContainsFunc(save, func(h string) bool { return helperKind(h) == "plain" }) {
		return push, helperIssue{e.tr("a credential helper for %s on %s (store) would keep the token in a file in clear text", p.Account, p.Host),
			e.tr("%s, and %s", e.helperAdvice(ctx), accountSettings)}, nil
	}
	if !slices.ContainsFunc(save, func(h string) bool { return helperKind(h) != "memory" && slices.Contains(push, h) }) {
		return push, helperIssue{e.tr("your settings for %s on %s save the token with %s, which a push to %s does not ask",
			p.Account, p.Host, strings.Join(save, ", "), p.Host), accountSettings}, nil
	}
	return push, helperIssue{}, nil
}

// checkHelpers checks the credential helpers of every profile on a host
// other than GitHub (see helpersProblem).
func (e Env) checkHelpers(ctx context.Context, cfg config.Config) []check {
	var checks []check
	for _, p := range cfg.Profiles {
		if p.Provider == config.ProviderGitHub {
			continue
		}
		helpers, issue, err := e.helpersProblem(ctx, p)
		switch {
		case err != nil:
			checks = append(checks, check{label: e.tr("%v", err)})
		case issue.problem != "":
			checks = append(checks, check{warn: true, label: e.tr("profile %s: %s", p.ID, issue.problem),
				advice: e.tr("%s, then `gitpersona auth login --profile %s`", issue.advice, p.ID)})
		default:
			checks = append(checks, check{ok: true,
				label: e.tr("profile %s: credentials for %s on %s come from %s (`gitpersona auth status` checks the token)",
					p.ID, p.Account, p.Host, strings.Join(helpers, ", "))})
		}
	}
	return checks
}
