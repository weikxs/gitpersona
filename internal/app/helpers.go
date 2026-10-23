package app

import (
	"context"
	"fmt"
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

// hostHelpers returns the credential helpers Git asks for https://host
// outside every rule: the user's own, after the last reset ("").
func (e Env) hostHelpers(ctx context.Context, host string) ([]string, error) {
	var helpers []string
	for _, key := range []string{"credential.helper", "credential.https://" + host + ".helper"} {
		values, err := e.gitConfigAll(ctx, key)
		if err != nil {
			return nil, err
		}
		for _, v := range values {
			if v == "" {
				helpers = nil // an empty value resets the list
				continue
			}
			helpers = append(helpers, v)
		}
	}
	return helpers, nil
}

// checkHelpers checks, for profiles on hosts other than GitHub, that a
// helper can keep a token per account: without one Git asks for a password
// on every push, and the store helper keeps tokens in a file in clear text.
func (e Env) checkHelpers(ctx context.Context, cfg config.Config) []check {
	var checks []check
	seen := map[string]bool{}
	for _, p := range cfg.Profiles {
		if p.Provider == config.ProviderGitHub || seen[p.Host] {
			continue
		}
		seen[p.Host] = true
		helpers, err := e.hostHelpers(ctx, p.Host)
		if err != nil {
			checks = append(checks, check{label: err.Error()})
			continue
		}
		kinds := make([]string, len(helpers))
		for i, h := range helpers {
			kinds[i] = helperKind(h)
		}
		switch {
		case slices.Contains(kinds, "plain"):
			checks = append(checks, check{warn: true,
				label:  fmt.Sprintf("the credential helper for %s keeps tokens in a file in clear text (store)", p.Host),
				advice: "use the system keychain instead, for example `git config --global credential.helper " + suggestedHelper() + "`"})
		case len(helpers) == 0:
			checks = append(checks, check{warn: true,
				label:  fmt.Sprintf("no credential helper keeps the tokens for %s, so Git asks for one on every push over HTTPS", p.Host),
				advice: "set one up, for example `git config --global credential.helper " + suggestedHelper() + "`, then `gitpersona auth login`"})
		default:
			checks = append(checks, check{ok: true,
				label: fmt.Sprintf("credentials for %s come from %s, one per account", p.Host, strings.Join(helpers, ", "))})
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
