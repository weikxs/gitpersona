package app

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/discover"
)

// Profiles for hosts other than GitHub (GitLab, Bitbucket, a company
// server) come from the user's existing setup: gh knows only GitHub
// accounts, so an identity whose rule names the account for another host
// (credential.https://<host>.username), or whose repositories all push to
// one such host as one user, becomes a profile for that host.

// hostAccount is the one host other than GitHub an identity is for, and its
// account there.
type hostAccount struct{ host, account string }

// hostAccountOf works out the host and account of an existing identity: the
// one its settings name, or else the one user its repositories' remotes on
// a single host other than GitHub are written with. ok is false when there
// is none, or more than one.
func (e Env) hostAccountOf(id discover.Identity, repos []discover.Repo, githubAlias map[string]bool) (hostAccount, bool) {
	ha, ok := e.namedHostAccount(id, repos, githubAlias)
	// An account or host the configuration cannot hold, such as an email
	// address as the user name, stays the user's own rule.
	if !ok || !config.ValidAccount(ha.account) || !config.ValidHost(ha.host) {
		return hostAccount{}, false
	}
	return ha, true
}

func (e Env) namedHostAccount(id discover.Identity, repos []discover.Repo, githubAlias map[string]bool) (hostAccount, bool) {
	if len(id.HostAccounts) == 1 {
		host := slices.Collect(maps.Keys(id.HostAccounts))[0]
		return hostAccount{host: strings.ToLower(host), account: id.HostAccounts[host]}, true
	}
	if len(id.HostAccounts) > 1 {
		return hostAccount{}, false
	}
	seen := map[hostAccount]bool{}
	for _, r := range repos {
		host := strings.ToLower(r.Host)
		if host == "" || !coversWith(e.comparisonKey, id.Dirs, r.Dir) {
			continue
		}
		if host == config.DefaultGitHubHost || githubAlias[host] || r.User == "" || r.User == "git" {
			return hostAccount{}, false // GitHub, or no account in the remote
		}
		seen[hostAccount{host: host, account: r.User}] = true
	}
	if len(seen) != 1 {
		return hostAccount{}, false
	}
	return slices.Collect(maps.Keys(seen))[0], true
}

// forOtherHost reports whether an identity's settings name its account for
// another host and none for GitHub: it is not a GitHub account's, even with
// the same email.
func forOtherHost(id discover.Identity) bool { return len(id.HostAccounts) > 0 && id.Account == "" }

// providerOf returns the provider of a host: GitLab for gitlab.com and hosts
// named gitlab, Bitbucket for bitbucket.org, generic otherwise.
func providerOf(host string) string {
	h := strings.ToLower(host)
	h, _, _ = strings.Cut(h, ":")
	switch {
	case h == "gitlab.com" || strings.HasPrefix(h, "gitlab.") || strings.Contains(h, ".gitlab."):
		return config.ProviderGitLab
	case h == "bitbucket.org":
		return config.ProviderBitbucket
	}
	return config.ProviderGeneric
}

// hostProfiles makes a profile for each identity no GitHub account took
// that is for one other host, and marks it used. Profile names stay unique:
// one that a GitHub profile already has gets the host's first label. from
// maps each new profile to the identity it comes from.
func (e Env) hostProfiles(taken []SetupProfile, existing []discover.Identity, used []bool, repos []discover.Repo,
	aliases []discover.SSHAlias, globalName string) (profiles []SetupProfile, from map[string]int) {
	from = map[string]int{}
	githubAlias := map[string]bool{}
	for _, a := range aliases {
		githubAlias[strings.ToLower(a.Alias)] = true
	}
	ids := map[string]bool{}
	for _, p := range taken {
		ids[p.ID] = true
	}
	for i, id := range existing {
		if used[i] {
			continue
		}
		ha, ok := e.hostAccountOf(id, repos, githubAlias)
		if !ok {
			continue
		}
		p := SetupProfile{
			ID: uniqueID(profileID(ha.account), ha.host, ids), Provider: providerOf(ha.host), Host: ha.host,
			Account: ha.account, Dirs: []string{},
		}
		ids[p.ID] = true
		used[i] = true
		from[p.ID] = i
		profiles = append(profiles, e.adopt(p, id, globalName))
	}
	return profiles, from
}

// withoutEmptyHostProfiles drops the profiles of other hosts that kept no
// folder (each was a rule that does more than a profile can): they would
// do nothing, and their identity stays the user's own.
func withoutEmptyHostProfiles(profiles []SetupProfile, from map[string]int, used []bool) []SetupProfile {
	return slices.DeleteFunc(profiles, func(p SetupProfile) bool {
		i, ok := from[p.ID]
		if !ok || len(p.Dirs) > 0 {
			return false
		}
		used[i] = false
		return true
	})
}

// uniqueID returns id, or id with the host's first label (and a number)
// when a profile already has it.
func uniqueID(id, host string, taken map[string]bool) string {
	if !taken[id] {
		return id
	}
	label, _, _ := strings.Cut(host, ".")
	base := profileID(id + "-" + label)
	if len(base) > maxIDLen-4 { // room for a number that keeps it unique
		base = base[:maxIDLen-4]
	}
	candidate := base
	for n := 2; taken[candidate]; n++ {
		candidate = profileID(base + "-" + strconv.Itoa(n))
	}
	return candidate
}

// onGitHub reports whether a proposed profile is for GitHub, where the
// repositories on disk are matched to accounts.
func (p SetupProfile) onGitHub() bool {
	return p.Provider == "" || p.Provider == config.ProviderGitHub
}
