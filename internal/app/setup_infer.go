package app

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/weikxs/gitpersona/internal/discover"
)

// layout is what the repositories on disk say about the user's directories.
type layout struct {
	found        map[string][]DirSuggestion // by account
	pins         []PinSuggestion
	unassigned   []UnassignedDir
	aliasAccount map[string]string // lowercase SSH alias -> account
}

// Evidence that a repository belongs to an account, strongest first.
const (
	byEmail = iota // the repository sets the account's email
	byAlias        // its remote uses an SSH alias of the account
	byOwner        // its remote's owner is the account
)

// attribution is who a repository belongs to and how that is known.
type attribution struct {
	repo    discover.Repo
	account string // "" when no account matches
	how     int
}

// inference works out a layout. key turns a path into the form paths are
// compared in (absolute, and lowercase on a case-insensitive disk).
type inference struct {
	profiles     []SetupProfile
	key          func(string) string
	githubAlias  map[string]bool   // lowercase SSH aliases that connect to github.com
	aliasAccount map[string]string // lowercase SSH alias -> account
}

// inferLayout works out directory rules and pins from the repositories on
// disk. Repository directories are written the way the configuration writes
// them (~/...); key compares them with the profiles' directories.
func inferLayout(profiles []SetupProfile, repos []discover.Repo, aliases []discover.SSHAlias, key func(string) string) layout {
	in := inference{profiles: profiles, key: key, githubAlias: map[string]bool{}}
	for _, a := range aliases {
		in.githubAlias[strings.ToLower(a.Alias)] = true
	}
	in.aliasAccount = in.aliasAccounts(repos, aliases)

	out := layout{found: map[string][]DirSuggestion{}, aliasAccount: in.aliasAccount}
	byParent := map[string][]attribution{}
	for _, r := range repos {
		if a, ok := in.attribute(r); ok {
			parent := filepath.Dir(r.Dir)
			byParent[parent] = append(byParent[parent], a)
		}
	}
	for _, parent := range slices.Sorted(maps.Keys(byParent)) {
		in.suggest(&out, parent, byParent[parent])
	}
	out.pins = in.pins(out.found, byParent)
	return out
}

// suggest adds a directory rule for parent when most of its repositories
// belong to one account, or asks the user when they mostly belong to an
// owner that is not an account. A folder that is already some profile's
// directory gets no new rule; repositories that differ from it become pins.
func (in inference) suggest(out *layout, parent string, repos []attribution) {
	counts, owners := map[string]int{}, map[string]int{}
	for _, a := range repos {
		if a.account != "" {
			counts[a.account]++
		} else {
			owners[strings.ToLower(a.repo.Owner)]++
		}
	}
	if in.ownedByAny(parent) {
		return
	}
	account, n := majority(counts)
	total := len(repos)
	if n*2 > total {
		if !in.covers(profileOf(in.profiles, account).Dirs, parent) {
			why := countPhrase(n, total) + " " + reason(repos, account, n == 1)
			out.found[account] = append(out.found[account], DirSuggestion{Dir: parent, Why: why})
		}
		return
	}
	if owner, m := majority(owners); m*2 > total && !in.coveredByAny(parent) {
		out.unassigned = append(out.unassigned, UnassignedDir{Dir: parent, Owner: owner, Why: countPhrase(m, total)})
	}
}

// pins pins repositories whose own email or SSH alias names another account
// than the rule that would cover them.
func (in inference) pins(found map[string][]DirSuggestion, byParent map[string][]attribution) []PinSuggestion {
	var pins []PinSuggestion
	for _, parent := range slices.Sorted(maps.Keys(byParent)) {
		for _, a := range byParent[parent] {
			if a.account == "" || a.how == byOwner {
				continue // an owner alone is not enough to override a rule
			}
			if ruled := in.ruleAccount(a.repo.Dir, found); ruled != "" && ruled != a.account {
				p := profileOf(in.profiles, a.account)
				pins = append(pins, PinSuggestion{Repo: a.repo.Dir, Profile: p.ID, Why: "the repository " + evidence(a)})
			}
		}
	}
	return pins
}

// ruleAccount returns the account of the most specific directory, existing
// or suggested, that covers dir.
func (in inference) ruleAccount(dir string, found map[string][]DirSuggestion) string {
	best, account := -1, ""
	consider := func(rule, acc string) {
		if in.covers([]string{rule}, dir) && len(in.key(rule)) > best {
			best, account = len(in.key(rule)), acc
		}
	}
	for _, p := range in.profiles {
		for _, d := range p.Dirs {
			consider(d, p.Account)
		}
		for _, s := range found[p.Account] {
			consider(s.Dir, p.Account)
		}
	}
	return account
}

// aliasAccounts maps each SSH alias for github.com to an account: the one
// its repositories show by owner or email, or else the one whose existing
// SSH key the alias uses.
func (in inference) aliasAccounts(repos []discover.Repo, aliases []discover.SSHAlias) map[string]string {
	out := map[string]string{}
	for _, alias := range aliases {
		name := strings.ToLower(alias.Alias)
		if name == "github.com" {
			continue // the plain host says nothing about the account
		}
		votes := map[string]int{}
		for _, r := range repos {
			if strings.ToLower(r.Host) != name {
				continue
			}
			for _, p := range in.profiles {
				if emailOf(r.Email, p) || strings.EqualFold(r.Owner, p.Account) {
					votes[p.Account]++
				}
			}
		}
		if account, n := majority(votes); n > 0 {
			out[name] = account
			continue
		}
		for _, p := range in.profiles {
			if p.SSHKey != "" && alias.IdentityFile != "" && in.key(p.SSHKey) == in.key(alias.IdentityFile) {
				out[name] = p.Account
				break
			}
		}
	}
	return out
}

// attribute says which account a GitHub repository belongs to; ok is false
// for repositories that are not on GitHub.
func (in inference) attribute(r discover.Repo) (attribution, bool) {
	host := strings.ToLower(r.Host)
	if host != "github.com" && !in.githubAlias[host] {
		return attribution{}, false
	}
	for _, p := range in.profiles {
		if emailOf(r.Email, p) {
			return attribution{repo: r, account: p.Account, how: byEmail}, true
		}
	}
	if account, ok := in.aliasAccount[host]; ok {
		return attribution{repo: r, account: account, how: byAlias}, true
	}
	for _, p := range in.profiles {
		if strings.EqualFold(r.Owner, p.Account) {
			return attribution{repo: r, account: p.Account, how: byOwner}, true
		}
	}
	return attribution{repo: r}, true
}

// covers reports whether dir is one of dirs or inside one of them.
func (in inference) covers(dirs []string, dir string) bool {
	k := in.key(dir)
	return slices.ContainsFunc(dirs, func(d string) bool {
		d = in.key(d)
		return k == d || strings.HasPrefix(k, d+"/")
	})
}

func (in inference) coveredByAny(dir string) bool {
	return slices.ContainsFunc(in.profiles, func(p SetupProfile) bool { return in.covers(p.Dirs, dir) })
}

// ownedByAny reports whether dir is exactly one of the profiles' directories.
func (in inference) ownedByAny(dir string) bool {
	k := in.key(dir)
	return slices.ContainsFunc(in.profiles, func(p SetupProfile) bool {
		return slices.ContainsFunc(p.Dirs, func(d string) bool { return in.key(d) == k })
	})
}

// emailOf reports whether email is p's email or the account's noreply address.
func emailOf(email string, p SetupProfile) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false
	}
	login := strings.ToLower(p.Account)
	return email == strings.ToLower(p.Email) || email == login+"@users.noreply.github.com" ||
		strings.HasSuffix(email, "+"+login+"@users.noreply.github.com")
}

// countPhrase says how many of a folder's repositories something holds for.
func countPhrase(n, total int) string {
	switch {
	case n == total && n == 1:
		return "its one repository"
	case n == total:
		return fmt.Sprintf("all %d repositories", n)
	}
	return fmt.Sprintf("%d of %d repositories", n, total)
}

// reason describes the strongest evidence shared by account's repositories;
// one selects the verb for a single repository.
func reason(repos []attribution, account string, one bool) string {
	verb := func(plural, singular string) string {
		if one {
			return singular
		}
		return plural
	}
	strongest := byOwner
	var example attribution
	for _, a := range repos {
		if a.account == account && a.how <= strongest {
			strongest, example = a.how, a
		}
	}
	switch strongest {
	case byEmail:
		return verb("set ", "sets ") + strings.ToLower(example.repo.Email)
	case byAlias:
		return verb("use", "uses") + " the SSH alias " + example.repo.Host
	default:
		return verb("belong", "belongs") + " to " + account
	}
}

// evidence says in words why a single repository belongs to its account.
func evidence(a attribution) string {
	if a.how == byAlias {
		return "uses the SSH alias " + a.repo.Host
	}
	return "sets " + strings.ToLower(a.repo.Email)
}

// majority returns the key with the highest count (the first by name on a tie).
func majority(counts map[string]int) (string, int) {
	best, n := "", 0
	for _, k := range slices.Sorted(maps.Keys(counts)) {
		if counts[k] > n {
			best, n = k, counts[k]
		}
	}
	return best, n
}

func profileOf(profiles []SetupProfile, account string) SetupProfile {
	i := slices.IndexFunc(profiles, func(p SetupProfile) bool { return p.Account == account })
	if i < 0 {
		return SetupProfile{ID: account, Account: account}
	}
	return profiles[i]
}
