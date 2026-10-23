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
	byUser         // its https remote names the account as the user
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
	layoutHints
}

// layoutHints are what the inference must respect besides the repositories.
type layoutHints struct {
	// reserved are the folders of existing identities no account took:
	// their own rule stays in force there unless a more specific rule
	// clearly belongs to an account.
	reserved []string
	// roots are the scanned folders: a rule for one of them covers much the
	// scan did not see, so a single repository there is pinned instead.
	roots []string
}

// newInference prepares the comparison of profiles with the repositories on
// disk: which SSH aliases connect to GitHub and whose they are.
func newInference(profiles []SetupProfile, repos []discover.Repo, aliases []discover.SSHAlias, key func(string) string) inference {
	in := inference{profiles: profiles, key: key, githubAlias: map[string]bool{}}
	for _, a := range aliases {
		in.githubAlias[strings.ToLower(a.Alias)] = true
	}
	in.aliasAccount = in.aliasAccounts(repos, aliases)
	return in
}

// inferLayout works out directory rules and pins from the repositories on
// disk. Repository directories are written the way the configuration writes
// them (~/...); key compares them with the profiles' directories.
func inferLayout(profiles []SetupProfile, repos []discover.Repo, aliases []discover.SSHAlias, key func(string) string, hints layoutHints) layout {
	in := newInference(profiles, repos, aliases, key)
	in.layoutHints = hints
	out := layout{found: map[string][]DirSuggestion{}, aliasAccount: in.aliasAccount}
	byParent := map[string][]attribution{}
	var all []attribution
	for _, r := range repos {
		a, ok := in.attribute(r)
		if !ok {
			continue
		}
		all = append(all, a)
		if !r.Lone { // a lone repository's folder says nothing about others
			parent := filepath.Dir(r.Dir)
			byParent[parent] = append(byParent[parent], a)
		}
	}
	folderAccount := map[string]string{} // folder -> the account most of its repositories belong to
	for _, parent := range slices.Sorted(maps.Keys(byParent)) {
		in.suggest(&out, parent, byParent[parent])
		if account, n := majority(accountCounts(byParent[parent])); n*2 > len(byParent[parent]) {
			folderAccount[in.key(parent)] = account
		}
	}
	out.found = in.withoutNestedRules(out.found)
	out.pins = in.pins(out.found, all, folderAccount)
	return out
}

// withoutNestedRules drops suggested rules inside a rule for the same
// account: they add nothing, and would keep the old profile if the outer
// folder later moves to another one.
func (in inference) withoutNestedRules(found map[string][]DirSuggestion) map[string][]DirSuggestion {
	out := map[string][]DirSuggestion{}
	for account, dirs := range found {
		out[account] = slices.Clone(dirs)
	}
	for _, account := range slices.Sorted(maps.Keys(out)) {
		for i := 0; i < len(out[account]); {
			rest := maps.Clone(out)
			rest[account] = slices.Delete(slices.Clone(out[account]), i, i+1)
			if ruled, dir := in.ruleAccount(out[account][i].Dir, rest); dir != "" && ruled == account {
				out[account] = rest[account]
				continue
			}
			i++
		}
		if len(out[account]) == 0 {
			delete(out, account)
		}
	}
	return out
}

// sum adds up counts.
func sum(counts map[string]int) int {
	total := 0
	for _, n := range counts {
		total += n
	}
	return total
}

// accountCounts counts repositories per account.
func accountCounts(repos []attribution) map[string]int {
	counts := map[string]int{}
	for _, a := range repos {
		if a.account != "" {
			counts[a.account]++
		}
	}
	return counts
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
	if in.ownedByAny(parent) || slices.ContainsFunc(in.reserved, func(r string) bool { return in.covers([]string{parent}, r) }) {
		return // a rule here would override an identity rule the user keeps
	}
	account, n := majority(counts)
	total := len(repos)
	if total == 1 && slices.ContainsFunc(in.roots, func(r string) bool { return in.key(r) == in.key(parent) }) {
		return // one repository does not speak for a whole scanned folder; it is pinned if needed
	}
	if n*2 > total {
		if !in.covers(profileOf(in.profiles, account).Dirs, parent) {
			why := reason(repos, account, n, total)
			if slices.ContainsFunc(repos, func(a attribution) bool { return a.repo.Partial }) {
				why += " (the scan stopped in this folder; it may hold more)"
			}
			out.found[account] = append(out.found[account], DirSuggestion{Dir: parent, Why: why})
		}
		return
	}
	if owner, m := majority(owners); m*2 > total && !in.coveredByAny(parent) {
		out.unassigned = append(out.unassigned, UnassignedDir{Dir: parent, Owner: owner, Why: countPhrase(m, total)})
	}
}

// pins pins repositories that belong to another account than the one that
// would apply to them: the most specific directory rule, or else the
// default. Where no rule applies, the pin is marked Outside, since it is
// needed only while the default is another profile. An owner alone does not
// pin a repository against a rule for its own folder, or one whose folder
// mostly belongs to the rule's account; its own email, SSH alias or remote
// user still do.
func (in inference) pins(found map[string][]DirSuggestion, repos []attribution, folderAccount map[string]string) []PinSuggestion {
	var pins []PinSuggestion
	for _, a := range repos {
		if a.account == "" {
			continue
		}
		ruled, ruleDir := in.ruleAccount(a.repo.Dir, found)
		if ruleDir != "" && ruled == a.account || in.keptByReservedRule(a.repo.Dir, ruleDir) {
			continue
		}
		parent := in.key(filepath.Dir(a.repo.Dir))
		outvoted := ruleDir != "" && !a.repo.Lone && (parent == in.key(ruleDir) || folderAccount[parent] == ruled)
		if a.how == byOwner && outvoted {
			continue
		}
		p := profileOf(in.profiles, a.account)
		pins = append(pins, PinSuggestion{Repo: a.repo.Dir, Profile: p.ID, Why: "the repository " + evidence(a), Outside: ruleDir == ""})
	}
	slices.SortFunc(pins, func(x, y PinSuggestion) int { return strings.Compare(x.Repo, y.Repo) })
	return pins
}

// keptByReservedRule reports whether the user's own rule for a left-out
// identity is the most specific one for dir, so a pin would override it.
func (in inference) keptByReservedRule(dir, ruleDir string) bool {
	for _, r := range in.reserved {
		if in.covers([]string{r}, dir) && (ruleDir == "" || len(in.key(r)) > len(in.key(ruleDir))) {
			return true
		}
	}
	return false
}

// ruleAccount returns the account and directory of the most specific
// directory rule, existing or suggested, that covers dir.
func (in inference) ruleAccount(dir string, found map[string][]DirSuggestion) (string, string) {
	best, account, ruleDir := -1, "", ""
	consider := func(rule, acc string) {
		if in.covers([]string{rule}, dir) && len(in.key(rule)) > best {
			best, account, ruleDir = len(in.key(rule)), acc, rule
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
	return account, ruleDir
}

// aliasAccounts maps each SSH alias for github.com to an account: the one
// whose existing SSH key the alias uses (the key decides who signs in), or
// else the one most of its repositories show by email or owner. An alias
// whose repositories belong to several accounts is nobody's.
func (in inference) aliasAccounts(repos []discover.Repo, aliases []discover.SSHAlias) map[string]string {
	out := map[string]string{}
	for _, alias := range aliases {
		name := strings.ToLower(alias.Alias)
		if name == "github.com" {
			continue // the plain host says nothing about the account
		}
		if account := in.keyAccount(alias.IdentityFile); account != "" {
			out[name] = account
			continue
		}
		votes := map[string]int{}
		for _, r := range repos {
			if strings.ToLower(r.Host) == name {
				if account := in.repoAccount(r); account != "" {
					votes[account]++ // one vote per repository
				}
			}
		}
		if account, n := majority(votes); n > 0 && n*2 > sum(votes) {
			out[name] = account
		}
	}
	return out
}

// keyAccount returns the account whose existing SSH key is file, or "".
func (in inference) keyAccount(file string) string {
	for _, p := range in.profiles {
		if p.SSHKey != "" && file != "" && in.key(p.SSHKey) == in.key(file) {
			return p.Account
		}
	}
	return ""
}

// repoAccount returns the account a repository's own email names, or else
// its owner, without looking at aliases.
func (in inference) repoAccount(r discover.Repo) string {
	for _, p := range in.profiles {
		if emailOf(r.Email, p) {
			return p.Account
		}
	}
	for _, p := range in.profiles {
		if strings.EqualFold(r.Owner, p.Account) {
			return p.Account
		}
	}
	return ""
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
		if r.User != "" && strings.EqualFold(r.User, p.Account) {
			return attribution{repo: r, account: p.Account, how: byUser}, true
		}
	}
	for _, p := range in.profiles {
		if strings.EqualFold(r.Owner, p.Account) {
			return attribution{repo: r, account: p.Account, how: byOwner}, true
		}
	}
	return attribution{repo: r}, true
}

// covers reports whether dir is one of dirs or inside one of them.
func (in inference) covers(dirs []string, dir string) bool { return coversWith(in.key, dirs, dir) }

// coversWith reports whether dir is one of dirs or inside one of them, with
// paths compared in the form key gives.
func coversWith(key func(string) string, dirs []string, dir string) bool {
	k := key(dir)
	return slices.ContainsFunc(dirs, func(d string) bool {
		d = key(d)
		return k == d || strings.HasPrefix(k, strings.TrimSuffix(d, "/")+"/")
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
	case total == 1:
		return "its one repository"
	case n == total:
		return fmt.Sprintf("all %d repositories", n)
	}
	return fmt.Sprintf("%d of %d repositories", n, total)
}

// reason says why n of a folder's total repositories belong to account,
// with what their own settings say, such as the SSH aliases they use.
func reason(repos []attribution, account string, n, total int) string {
	one := n == 1
	text := countPhrase(n, total) + " " + verb(one, "belong", "belongs") + " to " + account
	type group struct {
		example attribution
		count   int
	}
	groups := map[string]group{}
	for _, a := range repos {
		if a.account == account && a.how != byOwner {
			k := evidenceOf(a, false)
			groups[k] = group{example: a, count: groups[k].count + 1}
		}
	}
	var notes []string
	for _, k := range slices.Sorted(maps.Keys(groups)) {
		g := groups[k]
		switch {
		case one:
			notes = append(notes, "it "+evidenceOf(g.example, true))
		case g.count == n:
			notes = append(notes, "all "+k)
		case g.count == 1:
			notes = append(notes, "1 "+evidenceOf(g.example, true))
		default:
			notes = append(notes, fmt.Sprintf("%d %s", g.count, k))
		}
	}
	if len(notes) == 0 {
		return text
	}
	return text + "; " + strings.Join(notes, ", ")
}

func verb(one bool, plural, singular string) string {
	if one {
		return singular
	}
	return plural
}

// evidenceOf says what a repository's own settings say about its account,
// for one repository ("sets ...") or several ("set ...").
func evidenceOf(a attribution, one bool) string {
	switch a.how {
	case byEmail:
		return verb(one, "set ", "sets ") + strings.ToLower(a.repo.Email)
	case byAlias:
		// The alias, not the owner, decides: say whose alias it is.
		return verb(one, "use", "uses") + " " + a.account + "'s SSH alias " + a.repo.Host
	case byUser:
		return verb(one, "sign", "signs") + " in as " + a.repo.User
	}
	return verb(one, "belong", "belongs") + " to " + a.account
}

// evidence says in words why a single repository belongs to its account.
func evidence(a attribution) string { return evidenceOf(a, true) }

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
