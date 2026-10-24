package app

import (
	"bufio"
	"cmp"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/discover"
	"github.com/weikxs/gitpersona/internal/i18n"
)

const setupTimeout = 30 * time.Second

// SetupOptions controls Setup.
type SetupOptions struct {
	Print   bool // print the configuration instead of writing it
	Yes     bool // apply without asking
	NoShell bool // leave the shells' startup files alone
	// KeepOldRules leaves hand-made includeIf rules in ~/.gitconfig even
	// when the profiles take them over.
	KeepOldRules bool
	// NoDefault sets no default profile, so the global Git identity stays
	// outside the directories.
	NoDefault bool
	// Default sets this profile as the default, also when the user's own
	// includeIf rules would be overridden by it.
	Default string
}

// proposedIDs lists the ids of proposed profiles.
func proposedIDs(profiles []SetupProfile) string {
	ids := make([]string, len(profiles))
	for i, p := range profiles {
		ids[i] = p.ID
	}
	return strings.Join(ids, ", ")
}

var invalidIDChars = regexp.MustCompile(`[^a-z0-9-]+`)

// Setup finds everything GitPersona can use (see ProposeSetup), shows the
// configuration it would create, and after the user agrees writes it, the
// Git configuration and the shell integration in one backup.
func Setup(ctx context.Context, e Env, opts SetupOptions) error {
	if _, err := os.Stat(e.configFile()); err == nil && !opts.Print {
		return e.trErr("%s already exists; change it on the configuration page (`gitpersona ui`), with `gitpersona use` and `gitpersona pin`, "+
			"or by hand (after `gitpersona uninstall`, `gitpersona apply` sets GitPersona up again from it)", e.display(e.configFile()))
	}
	proposal, err := ProposeSetup(ctx, e)
	if err != nil {
		return err
	}
	if opts.NoShell {
		proposal.Shells, proposal.NoShell = []string{}, len(proposal.Shells) > 0 // record only a shell declined
	}
	if opts.KeepOldRules {
		proposal.OldRules = []string{}
	}
	switch {
	case opts.NoDefault && opts.Default != "":
		return e.trErr("choose either --default or --no-default")
	case opts.NoDefault:
		proposal = e.withDefault(proposal, "")
	case opts.Default != "":
		if !slices.ContainsFunc(proposal.Profiles, func(p SetupProfile) bool { return p.ID == opts.Default }) {
			return e.trErr("--default %s: no such profile; setup proposes %s", opts.Default, proposedIDs(proposal.Profiles))
		}
		proposal = e.withDefault(proposal, opts.Default)
	}
	plan := proposal.Plan()
	if opts.Print {
		fmt.Fprint(e.Stdout, renderPlan(plan))
		printLeftOutComments(e.Stdout, proposal, e.comparisonKey)
		return nil
	}
	e.printProposal(proposal)
	if !opts.Yes && !e.agree(e.tr("Apply this setup? Changed files are backed up first. [Y/n] ")) {
		fmt.Fprintln(e.Stdout, e.tr("Nothing was written. Run `gitpersona setup --yes` to apply without asking, "+
			"or `gitpersona setup --web` to adjust it in your browser."))
		return nil
	}
	if err := ApplySetup(ctx, e, plan); err != nil {
		return err
	}
	fmt.Fprintln(e.Stdout, "\n"+e.tr("GitPersona is set up. Check it with `gitpersona doctor`; change anything with `gitpersona ui`."))
	if len(plan.Shells) > 0 {
		fmt.Fprintln(e.Stdout, e.tr("Open a new terminal so gh and the prompt follow your directories."))
	}
	return nil
}

// agree asks a yes/no question; Enter means yes, and no answer (no
// terminal) means no.
func (e Env) agree(question string) bool {
	fmt.Fprint(e.Stdout, "\n"+question)
	answer, err := bufio.NewReader(e.Stdin).ReadString('\n')
	if err != nil && answer == "" {
		fmt.Fprintln(e.Stdout)
		return false
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "" || answer == "y" || answer == "yes"
}

// setupListIn joins items the way a list reads in l.
func setupListIn(l i18n.Lang, items []string) string {
	if l == i18n.ZH {
		return strings.Join(items, "\u3001")
	}
	return strings.Join(items, ", ")
}

// setupSemiIn joins items with the semicolon of l.
func setupSemiIn(l i18n.Lang, items []string) string {
	if l == i18n.ZH {
		return strings.Join(items, "\uff1b")
	}
	return strings.Join(items, "; ")
}

func (e Env) setupList(items []string) string { return setupListIn(e.lang(), items) }
func (e Env) setupSemi(items []string) string { return setupSemiIn(e.lang(), items) }

// foundHeading says how many accounts setup found and where it looked.
func (e Env) foundHeading(github, other int, roots []string) string {
	accounts := e.githubAccounts(github)
	switch {
	case other > 0 && len(roots) > 0:
		return e.tr("GitPersona found %s, %s for other hosts in your Git settings and looked for repositories in %s:",
			accounts, e.otherAccounts(other), e.setupList(roots))
	case other > 0:
		return e.tr("GitPersona found %s, %s for other hosts in your Git settings:", accounts, e.otherAccounts(other))
	case len(roots) > 0:
		return e.tr("GitPersona found %s and looked for repositories in %s:", accounts, e.setupList(roots))
	}
	return e.tr("GitPersona found %s:", accounts)
}

func (e Env) githubAccounts(n int) string {
	if n == 1 {
		return e.tr("1 GitHub account")
	}
	return e.tr("%d GitHub accounts", n)
}

func (e Env) otherAccounts(n int) string {
	if n == 1 {
		return e.tr("1 account")
	}
	return e.tr("%d accounts", n)
}

// printProposal shows what setup found and would configure.
func (e Env) printProposal(p SetupProposal) {
	w := e.Stdout
	github, accounts := 0, map[string]bool{}
	for _, prof := range p.Profiles {
		if prof.onGitHub() {
			github++
			accounts[strings.ToLower(prof.Account)] = true
		}
	}
	var roots []string
	if len(p.Roots) > 0 {
		roots = outermost(p.Roots, e.comparisonKey)
	}
	fmt.Fprintln(w, e.foundHeading(len(accounts), len(p.Profiles)-github, roots))
	if p.GitHubUnavailable != "" {
		fmt.Fprintf(w, "\n  %s\n", e.tr("GitHub accounts were not looked up: %s.", strings.TrimRight(p.GitHubUnavailable, ".\u3002")))
	}
	if p.ScanIncomplete {
		fmt.Fprintf(w, "\n  ! %s\n", e.tr("Looking for repositories stopped after %s, before it finished %s; repositories it did not reach "+
			"are not included. Add their folders with `gitpersona use`.",
			discover.DefaultScanLimits.Timeout, e.setupList(p.ScanUnfinished)))
	}
	if p.ScanTooDeep {
		fmt.Fprintf(w, "\n  %s\n", e.tr("Repositories nested more than %d folders deep were not looked at; add their folders with `gitpersona use`.",
			discover.DefaultScanLimits.MaxDepth))
	}
	for _, prof := range p.Profiles {
		e.printProposedProfile(prof, prof.ID == p.Default)
	}
	for _, pin := range p.Pins {
		if pin.needed(p.Default) {
			fmt.Fprintf(w, "\n  %s\n", e.tr("Pin %s to %s: %s", pin.Repo, pin.Profile, pin.Why))
		}
	}
	if p.Default == "" && len(p.OwnRules) > 0 {
		fmt.Fprintf(w, "\n  %s\n", e.tr("No default profile: one would override your own includeIf rules everywhere (%s). Outside these "+
			"directories Git keeps your own settings; `gitpersona setup --default %s` sets one anyway.",
			e.setupSemi(p.OwnRules), cmp.Or(p.SuggestedDefault, "<profile>")))
	}
	if p.Default != "" && len(p.OwnRules) > 0 {
		fmt.Fprintf(w, "\n  ! %s\n", e.tr("The default profile %s overrides your own includeIf rules everywhere: %s.", p.Default, e.setupSemi(p.OwnRules)))
	}
	if k := slices.IndexFunc(p.Profiles, func(x SetupProfile) bool { return x.ID == p.Default }); k >= 0 {
		e.printDefaultEffect(p, p.Profiles[k])
	}
	if len(p.OldRules) > 0 {
		fmt.Fprintf(w, "\n  %s\n", e.tr("The profiles take over your includeIf rules for %s; they are removed from your Git configuration (--keep-old-rules to keep them).",
			e.setupList(p.OldRules)))
	}
	for _, k := range p.KeptRules {
		if k.OverriddenBy != "" {
			fmt.Fprintf(w, "\n  ! %s\n", e.tr("Your includeIf rule for %s does more than a profile can (%s), but profile %s applies there, "+
				"so that is lost; take %s out of the profile to keep it.", k.Dir, k.Why, k.OverriddenBy, k.Dir))
			continue
		}
		fmt.Fprintf(w, "\n  %s\n", e.tr("Kept your includeIf rule for %s, which stays in charge there: %s.", k.Dir, k.Why))
		if len(k.ReplacedIn) > 0 {
			fmt.Fprintf(w, "    %s\n", e.tr("Except in %s, which go to profiles (what the rule does more is lost there).", e.setupList(k.ReplacedIn)))
		}
	}
	for _, s := range p.Shadowed {
		fmt.Fprintf(w, "\n  ! %s\n", e.tr("Changed: %s.", s))
	}
	if len(p.Shells) > 0 {
		fmt.Fprintf(w, "\n  %s\n", e.tr("Shell integration for %s, so gh and your prompt follow the directory (--no-shell to skip).",
			e.setupList(p.Shells)))
	}
	if len(p.GhSwitchers) > 0 && len(p.Shells) > 0 && slices.ContainsFunc(p.Profiles, SetupProfile.onGitHub) {
		fmt.Fprintf(w, "\n  %s\n", e.tr("Note: %s.", e.ghSwitcherNote(p.GhSwitchers)))
	}
	e.printUnassigned(p)
	for _, u := range p.Unmatched {
		e.printLeftOut(p, u)
	}
}

// printDefaultEffect says what the default profile changes outside the
// directories of the profiles.
func (e Env) printDefaultEffect(p SetupProposal, def SetupProfile) {
	w := e.Stdout
	if p.Global.Email != "" {
		fmt.Fprintf(w, "\n  %s\n", e.tr("Outside these directories, commits will use %s (%s <%s>) instead of %s <%s> (--no-default to keep your global Git identity).",
			def.ID, def.Name, def.Email, p.Global.Name, p.Global.Email))
	} else {
		fmt.Fprintf(w, "\n  %s\n", e.tr("Outside these directories, commits will use %s (%s <%s>) (--no-default to keep your global Git identity).",
			def.ID, def.Name, def.Email))
	}
	if def.Signing.Method == config.SigningSSH || def.Signing.Method == config.SigningGPG {
		fmt.Fprintf(w, "  %s\n", e.tr("There, commits will also be signed (%s), as %s signs every commit.", def.Signing.Method, def.ID))
	}
	if def.SSHKey != "" {
		fmt.Fprintf(w, "  %s\n", e.tr("There, Git also connects with %s's SSH key %s (instead of your own SSH settings).", def.ID, def.SSHKey))
	}
}

// printLeftOut says what happens in the folder of an identity setup does not
// take over: its rule keeps applying, or a profile replaces it.
func (e Env) printLeftOut(p SetupProposal, u LeftOut) {
	w := e.Stdout
	where := e.setupList(u.Dirs)
	if u.NeverApplied != "" {
		fmt.Fprintf(w, "\n  %s\n", e.tr("Not taken over: %s for %s (from %s). It never applies today, because %s; setup leaves it as it is.",
			author(u.Name, u.Email), where, u.Source, u.NeverApplied))
		return
	}
	if k := slices.IndexFunc(p.Profiles, func(x SetupProfile) bool { return x.ID == u.OverriddenBy }); k >= 0 {
		fmt.Fprintf(w, "\n  ! %s\n", e.tr("Replaced: %s for %s (from %s). Profile %s (%s) applies there instead, because GitPersona's rules "+
			"come after yours. To keep it, choose its account with `gitpersona setup --web`.",
			author(u.Name, u.Email), where, u.Source, u.OverriddenBy, author(p.Profiles[k].Name, p.Profiles[k].Email)))
		return
	}
	if allKept(p, u, e.comparisonKey) {
		return // listed with the kept rules, and why
	}
	fmt.Fprintf(w, "\n  %s\n", e.tr("Not taken over: %s for %s (from %s); GitPersona could not tie it to one account, so your rule keeps "+
		"applying there. To make it a profile, choose its account with `gitpersona setup --web`.", author(u.Name, u.Email), where, u.Source))
	if len(u.ReplacedIn) > 0 {
		fmt.Fprintf(w, "    %s\n", e.tr("Except in %s, which go to profiles.", e.setupList(u.ReplacedIn)))
	}
}

// outermost returns the folders not inside another one of them, in order.
func outermost(dirs []string, key func(string) string) []string {
	var out []string
	for i, d := range dirs {
		inside := slices.ContainsFunc(dirs, func(o string) bool {
			return key(o) != key(d) && coversWith(key, []string{o}, d)
		})
		if !inside && !slices.ContainsFunc(dirs[:i], func(o string) bool { return key(o) == key(d) }) {
			out = append(out, d)
		}
	}
	return out
}

// allKept reports whether every folder of a left-out identity is a kept
// rule, which the proposal lists with its reason.
// key compares paths as the disk does.
func allKept(p SetupProposal, u LeftOut, key func(string) string) bool {
	return len(u.Dirs) > 0 && !slices.ContainsFunc(u.Dirs, func(d string) bool {
		return !slices.ContainsFunc(p.KeptRules, func(k KeptRule) bool { return key(k.Dir) == key(d) })
	})
}

// printUnassigned lists, together, the folders whose repositories belong to
// someone else, such as an organization.
func (e Env) printUnassigned(p SetupProposal) {
	if len(p.Unassigned) == 0 {
		return
	}
	w := e.Stdout
	if p.Default != "" {
		fmt.Fprintf(w, "\n  %s\n", e.tr("Owned by someone else, so the default profile %s applies there unless you choose (run `gitpersona use <profile>` in the folder):", p.Default))
	} else {
		fmt.Fprintf(w, "\n  %s\n", e.tr("Owned by someone else, so your own Git settings apply there unless you choose (run `gitpersona use <profile>` in the folder):"))
	}
	for _, u := range p.Unassigned {
		fmt.Fprintf(w, "    %s\n", e.tr("%s  (%s, owned by %s)", u.Dir, u.Why, u.Owner))
	}
}

// printLeftOutComments adds to a printed configuration, as comments, what
// it leaves out of the existing setup.
func printLeftOutComments(w io.Writer, p SetupProposal, key func(string) string) {
	for _, u := range p.Unmatched {
		if !allKept(p, u, key) {
			fmt.Fprintf(w, "\n# Left out: %s for %s; GitPersona could not tie it to one account.\n", author(u.Name, u.Email), strings.Join(u.Dirs, ", "))
		}
	}
	for _, k := range p.KeptRules {
		fmt.Fprintf(w, "# Kept: your includeIf rule for %s (%s).\n", k.Dir, k.Why)
	}
}

func (e Env) printProposedProfile(p SetupProfile, isDefault bool) {
	w := e.Stdout
	label := p.ID
	if isDefault {
		label = e.tr("%s (default)", p.ID)
	}
	fmt.Fprintf(w, "\n  %s: %s <%s>\n", label, p.Name, p.Email)
	if !p.onGitHub() {
		fmt.Fprintf(w, "    %s\n", e.tr("account %s on %s (%s)", p.Account, p.Host, p.Provider))
	}
	switch p.NameSource {
	case "login":
		fmt.Fprintf(w, "    %s\n", e.tr("(the account shows no public name, so the name is its login)"))
	case "global":
		fmt.Fprintf(w, "    %s\n", e.tr("(your includeIf setup sets no name, so the name is your global Git name)"))
	}
	if p.SSHKey != "" {
		fmt.Fprintf(w, "    %s\n", e.tr("SSH key %s", p.SSHKey))
	}
	if p.Signing.Method != "" {
		fmt.Fprintf(w, "    %s\n", e.tr("signs commits (%s)", p.Signing.Method))
	}
	for _, d := range p.Dirs {
		fmt.Fprintf(w, "    %s\n", e.tr("%s  (your existing setup)", d))
	}
	for _, f := range p.Found {
		fmt.Fprintf(w, "    %s\n", e.tr("%s  (%s)", f.Dir, f.Why))
	}
	if len(p.Dirs)+len(p.Found) == 0 {
		fmt.Fprintf(w, "    %s\n", e.tr("no directories yet"))
	}
	for _, n := range p.Notes {
		fmt.Fprintf(w, "    ! %s\n", n)
	}
}

// maxIDLen is the longest profile name the configuration accepts.
const maxIDLen = 32

// profileID derives a valid profile id from a login.
func profileID(login string) string {
	id := strings.Trim(invalidIDChars.ReplaceAllString(strings.ToLower(login), "-"), "-")
	if len(id) > maxIDLen {
		id = id[:maxIDLen]
	}
	return id
}

// withDefault makes id (or none, for "") the default profile of a proposal,
// and works out again which profile replaces the identities left out.
func (e Env) withDefault(p SetupProposal, id string) SetupProposal {
	owners := setupOwners(p.Profiles)
	unmatched := make([]LeftOut, len(p.Unmatched))
	for i, u := range p.Unmatched {
		u.OverriddenBy = ""
		for _, d := range u.Dirs {
			if by, ok := e.overriddenBy(d, owners, id); ok && !by.sameIdentity(u.Email, u.Name) && u.OverriddenBy == "" {
				u.OverriddenBy = by.id
			}
		}
		unmatched[i] = u
	}
	p.Default, p.Unmatched, p.KeptRules = id, unmatched, e.keptOverrides(p.KeptRules, owners, id)
	return p
}
