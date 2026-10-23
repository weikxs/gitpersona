package app

import (
	"context"
	"fmt"
	"net"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/weikxs/gitpersona/internal/config"
)

// secureHelpers are credential helpers that keep one token per account and
// host in a protected store, so several accounts on one host work: GitPersona
// only names the account and the helper answers with its token.
var secureHelpers = []string{"osxkeychain", "manager", "manager-core", "libsecret", "wincred", "gnome-keyring"}

// helperKind says what a credential.helper value is: "secure", "plain"
// (the store helper, which writes tokens to a file in clear text), "memory"
// (the cache helper) or "other".
func helperKind(helper string) string {
	words := shellFields(helper)
	if len(words) == 0 {
		return "other"
	}
	name := strings.TrimPrefix(filepath.Base(words[0]), "git-credential-")
	switch {
	case slices.Contains(secureHelpers, name):
		return "secure"
	case name == "store":
		return "plain"
	case name == "cache":
		return "memory"
	}
	return "other"
}

// hostHelpers returns the credential helpers Git runs for account on
// https://host outside every rule: the user's own, in the order Git runs
// them, after the last reset (""). Like Git, it reads the system settings
// first, and takes credential.helper and every credential.<url>.helper
// whose URL matches (see urlMatches).
func (e Env) hostHelpers(ctx context.Context, host, account string) ([]string, error) {
	entries, err := e.gitConfigRegexp(ctx, `^credential\..*helper$`)
	if err != nil {
		return nil, err
	}
	var helpers []string
	for _, kv := range entries {
		rest, ok := strings.CutPrefix(kv[0], "credential.")
		if !ok {
			continue
		}
		pattern, name := "", rest
		if dot := strings.LastIndex(rest, "."); dot >= 0 {
			pattern, name = rest[:dot], rest[dot+1:]
		}
		if name != "helper" || (pattern != "" && !urlMatches(pattern, host, account)) {
			continue
		}
		if kv[1] == "" {
			helpers = nil // an empty value resets the list
			continue
		}
		helpers = append(helpers, kv[1])
	}
	return helpers, nil
}

// urlMatches reports whether a credential.<url> pattern applies to every
// repository of account on https://host, as Git's URL matching does: the
// https scheme (or none, a partial URL), the same user if the pattern names
// one, the same port (443 is the default), a host whose labels may be "*"
// globs, and no path.
func urlMatches(pattern, host, account string) bool {
	rest := pattern
	if scheme, after, ok := strings.Cut(pattern, "://"); ok {
		if !strings.EqualFold(scheme, "https") {
			return false
		}
		rest = after
	}
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		user, _, _ := strings.Cut(rest[:at], ":")
		if user != account { // Git compares user names exactly
			return false
		}
		rest = rest[at+1:]
	}
	if h, p, ok := strings.Cut(rest, "/"); ok {
		if p != "" {
			return false // only some repositories of the host
		}
		rest = h
	}
	wantHost, wantPort := splitHostPort(strings.ToLower(rest))
	gotHost, gotPort := splitHostPort(strings.ToLower(host))
	if wantPort != gotPort {
		return false
	}
	want, got := strings.Split(wantHost, "."), strings.Split(gotHost, ".")
	if len(want) != len(got) {
		return false
	}
	for i := range want {
		if ok, err := path.Match(want[i], got[i]); err != nil || !ok {
			return false
		}
	}
	return true
}

// splitHostPort splits host:port, or [v6]:port; the default HTTPS port is
// no port.
func splitHostPort(hp string) (host, port string) {
	if h, p, err := net.SplitHostPort(hp); err == nil {
		host, port = h, p
	} else {
		host = strings.Trim(hp, "[]")
	}
	if port == "443" {
		port = ""
	}
	return host, port
}

// helperProblem says what is wrong with the helpers that would keep a
// token, or "" when one keeps it safely, one per account.
func helperProblem(host string, helpers []string) string {
	kinds := make([]string, len(helpers))
	for i, h := range helpers {
		kinds[i] = helperKind(h)
	}
	switch {
	case len(helpers) == 0:
		return fmt.Sprintf("no credential helper would keep the token for %s, so Git asks for one on every push over HTTPS", host)
	case slices.Contains(kinds, "plain"):
		return fmt.Sprintf("the credential helper for %s (store) keeps tokens in a file in clear text", host)
	case !slices.ContainsFunc(kinds, func(k string) bool { return k != "memory" }):
		return fmt.Sprintf("the credential helper for %s (cache) forgets tokens after a while, so Git asks again", host)
	}
	return ""
}

// helperAdvice is how to set up a helper that keeps tokens safely.
func helperAdvice() string {
	return "use the system keychain, for example `git config --global credential.helper " + suggestedHelper() + "`"
}

// checkHelpers checks, for profiles on hosts other than GitHub, that a
// helper can keep each account's token: without one Git asks for a
// password on every push, and the store helper keeps tokens in a file in
// clear text.
func (e Env) checkHelpers(ctx context.Context, cfg config.Config) []check {
	var checks []check
	for _, p := range cfg.Profiles {
		if p.Provider == config.ProviderGitHub {
			continue
		}
		helpers, err := e.hostHelpers(ctx, p.Host, p.Account)
		switch problem := helperProblem(p.Host, helpers); {
		case err != nil:
			checks = append(checks, check{label: err.Error()})
		case problem != "":
			checks = append(checks, check{warn: true, label: "profile " + p.ID + ": " + problem,
				advice: helperAdvice() + ", then `gitpersona auth login --profile " + p.ID + "`"})
		default:
			checks = append(checks, check{ok: true,
				label: fmt.Sprintf("profile %s: credentials for %s on %s come from %s (`gitpersona auth status` checks the token)",
					p.ID, p.Account, p.Host, strings.Join(helpers, ", "))})
		}
	}
	return checks
}

// suggestedHelper is the usual secure helper of this system.
func suggestedHelper() string {
	switch runtime.GOOS {
	case "darwin":
		return "osxkeychain"
	case "windows":
		return "manager"
	}
	return "libsecret"
}

// shellFields splits a command line on blanks (helpers rarely quote).
func shellFields(s string) []string { return strings.Fields(strings.TrimPrefix(s, "!")) }
