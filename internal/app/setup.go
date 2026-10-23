package app

import (
	"bufio"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/discover"
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
		return fmt.Errorf("%s already exists; change it on the configuration page (`gitpersona ui`), with `gitpersona use` and `gitpersona pin`, "+
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
		return errors.New("choose either --default or --no-default")
	case opts.NoDefault:
		proposal = e.withDefault(proposal, "")
	case opts.Default != "":
		if !slices.ContainsFunc(proposal.Profiles, func(p SetupProfile) bool { return p.ID == opts.Default }) {
			return fmt.Errorf("--default %s: no such profile; setup proposes %s", opts.Default, proposedIDs(proposal.Profiles))
		}
		proposal = e.withDefault(proposal, opts.Default)
	}
	plan := proposal.Plan()
	if opts.Print {
		fmt.Fprint(e.Stdout, renderPlan(plan))
		printLeftOutComments(e.Stdout, proposal)
		return nil
	}
	e.printProposal(proposal)
	if !opts.Yes && !e.agree("Apply this setup? Changed files are backed up first. [Y/n] ") {
		fmt.Fprintln(e.Stdout, "Nothing was written. Run `gitpersona setup --yes` to apply without asking, "+
			"or `gitpersona setup --web` to adjust it in your browser.")
		return nil
	}
	if err := ApplySetup(ctx, e, plan); err != nil {
		return err
	}
	fmt.Fprintln(e.Stdout, "\nGitPersona is set up. Check it with `gitpersona doctor`; change anything with `gitpersona ui`.")
	if len(plan.Shells) > 0 {
		fmt.Fprintln(e.Stdout, "Open a new terminal so gh and the prompt follow your directories.")
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

// printProposal shows what setup found and would configure.
func (e Env) printProposal(p SetupProposal) {
	w := e.Stdout
	github := 0
	for _, prof := range p.Profiles {
		if prof.onGitHub() {
			github++
		}
	}
	fmt.Fprintf(w, "GitPersona found %s", plural(github, "GitHub account"))
	if other := len(p.Profiles) - github; other > 0 {
		fmt.Fprintf(w, ", %s for other hosts in your Git settings", plural(other, "account"))
	}
	if len(p.Roots) > 0 {
		fmt.Fprintf(w, " and looked for repositories in %s", strings.Join(p.Roots, ", "))
	}
	fmt.Fprintln(w, ":")
	if p.GitHubUnavailable != "" {
		fmt.Fprintf(w, "\n  ! No GitHub profiles: %s.\n", strings.TrimSuffix(p.GitHubUnavailable, "."))
	}
	if p.ScanIncomplete {
		fmt.Fprintf(w, "\n  ! Looking for repositories stopped after %s, before it finished %s; repositories it did not reach "+
			"are not included. Add their folders with `gitpersona use`.\n",
			discover.DefaultScanLimits.Timeout, strings.Join(p.ScanUnfinished, ", "))
	}
	if p.ScanTooDeep {
		fmt.Fprintf(w, "\n  Repositories nested more than %d folders deep were not looked at; add their folders with `gitpersona use`.\n",
			discover.DefaultScanLimits.MaxDepth)
	}
	for _, prof := range p.Profiles {
		printProposedProfile(w, prof, prof.ID == p.Default)
	}
	for _, pin := range p.Pins {
		if pin.needed(p.Default) {
			fmt.Fprintf(w, "\n  Pin %s to %s: %s\n", pin.Repo, pin.Profile, pin.Why)
		}
	}
	if p.Default == "" && len(p.OwnRules) > 0 {
		fmt.Fprintf(w, "\n  No default profile: one would override your own includeIf rules everywhere (%s). Outside these "+
			"directories Git keeps your own settings; `gitpersona setup --default %s` sets one anyway.\n",
			strings.Join(p.OwnRules, "; "), cmp.Or(p.SuggestedDefault, "<profile>"))
	}
	if p.Default != "" && len(p.OwnRules) > 0 {
		fmt.Fprintf(w, "\n  ! The default profile %s overrides your own includeIf rules everywhere: %s.\n", p.Default, strings.Join(p.OwnRules, "; "))
	}
	if k := slices.IndexFunc(p.Profiles, func(x SetupProfile) bool { return x.ID == p.Default }); k >= 0 {
		def := p.Profiles[k]
		fmt.Fprintf(w, "\n  Outside these directories, commits will use %s (%s <%s>)", def.ID, def.Name, def.Email)
		if p.Global.Email != "" {
			fmt.Fprintf(w, " instead of %s <%s>", p.Global.Name, p.Global.Email)
		}
		fmt.Fprintln(w, " (--no-default to keep your global Git identity).")
		if def.Signing.Method == config.SigningSSH || def.Signing.Method == config.SigningGPG {
			fmt.Fprintf(w, "  There, commits will also be signed (%s), as %s signs every commit.\n", def.Signing.Method, def.ID)
		}
		if def.SSHKey != "" {
			fmt.Fprintf(w, "  There, Git also connects with %s's SSH key %s (instead of your own SSH settings).\n", def.ID, def.SSHKey)
		}
	}
	if len(p.OldRules) > 0 {
		fmt.Fprintf(w, "\n  The profiles take over your includeIf rules for %s; they are removed from your Git configuration (--keep-old-rules to keep them).\n",
			strings.Join(p.OldRules, ", "))
	}
	for _, k := range p.KeptRules {
		if k.OverriddenBy != "" {
			fmt.Fprintf(w, "\n  ! Your includeIf rule for %s does more than a profile can (%s), but profile %s applies there, "+
				"so that is lost; take %s out of the profile to keep it.\n", k.Dir, k.Why, k.OverriddenBy, k.Dir)
			continue
		}
		fmt.Fprintf(w, "\n  Kept your includeIf rule for %s, which stays in charge there: %s.\n", k.Dir, k.Why)
		if len(k.ReplacedIn) > 0 {
			fmt.Fprintf(w, "    Except in %s, which go to profiles (what the rule does more is lost there).\n", strings.Join(k.ReplacedIn, ", "))
		}
	}
	for _, s := range p.Shadowed {
		fmt.Fprintf(w, "\n  ! Changed: %s.\n", s)
	}
	if len(p.Shells) > 0 {
		fmt.Fprintf(w, "\n  Shell integration for %s, so gh and your prompt follow the directory (--no-shell to skip).\n",
			strings.Join(p.Shells, ", "))
	}
	printUnassigned(w, p)
	for _, u := range p.Unmatched {
		printLeftOut(w, p, u)
	}
}

// printLeftOut says what happens in the folder of an identity setup does not
// take over: its rule keeps applying, or a profile replaces it.
func printLeftOut(w io.Writer, p SetupProposal, u LeftOut) {
	where := strings.Join(u.Dirs, ", ")
	if u.NeverApplied != "" {
		fmt.Fprintf(w, "\n  Not taken over: %s for %s (from %s). It never applies today, because %s; setup leaves it as it is.\n",
			author(u.Name, u.Email), where, u.Source, u.NeverApplied)
		return
	}
	if k := slices.IndexFunc(p.Profiles, func(x SetupProfile) bool { return x.ID == u.OverriddenBy }); k >= 0 {
		fmt.Fprintf(w, "\n  ! Replaced: %s for %s (from %s). Profile %s (%s) applies there instead, because GitPersona's rules "+
			"come after yours. To keep it, choose its account with `gitpersona setup --web`.\n",
			author(u.Name, u.Email), where, u.Source, u.OverriddenBy, author(p.Profiles[k].Name, p.Profiles[k].Email))
		return
	}
	fmt.Fprintf(w, "\n  Not taken over: %s for %s (from %s); no signed-in account could be tied to it, so your rule keeps "+
		"applying there. To make it a profile, choose its account with `gitpersona setup --web`.\n", author(u.Name, u.Email), where, u.Source)
	if len(u.ReplacedIn) > 0 {
		fmt.Fprintf(w, "    Except in %s, which go to profiles.\n", strings.Join(u.ReplacedIn, ", "))
	}
}

// printUnassigned lists, together, the folders whose repositories belong to
// someone else, such as an organization.
func printUnassigned(w io.Writer, p SetupProposal) {
	if len(p.Unassigned) == 0 {
		return
	}
	applies := "your own Git settings apply"
	if p.Default != "" {
		applies = "the default profile " + p.Default + " applies"
	}
	fmt.Fprintf(w, "\n  Owned by someone else, so %s there unless you choose (run `gitpersona use <profile>` in the folder):\n", applies)
	for _, u := range p.Unassigned {
		fmt.Fprintf(w, "    %s  (%s, owned by %s)\n", u.Dir, u.Why, u.Owner)
	}
}

// printLeftOutComments adds to a printed configuration, as comments, what
// it leaves out of the existing setup.
func printLeftOutComments(w io.Writer, p SetupProposal) {
	for _, u := range p.Unmatched {
		fmt.Fprintf(w, "\n# Left out: %s for %s; no signed-in account could be tied to it.\n", author(u.Name, u.Email), strings.Join(u.Dirs, ", "))
	}
	for _, k := range p.KeptRules {
		fmt.Fprintf(w, "# Kept: your includeIf rule for %s (%s).\n", k.Dir, k.Why)
	}
}

func printProposedProfile(w io.Writer, p SetupProfile, isDefault bool) {
	label := p.ID
	if isDefault {
		label += " (default)"
	}
	fmt.Fprintf(w, "\n  %s: %s <%s>\n", label, p.Name, p.Email)
	if !p.onGitHub() {
		fmt.Fprintf(w, "    account %s on %s (%s)\n", p.Account, p.Host, p.Provider)
	}
	switch p.NameSource {
	case "login":
		fmt.Fprintln(w, "    (the account shows no public name, so the name is its login)")
	case "global":
		fmt.Fprintln(w, "    (your includeIf setup sets no name, so the name is your global Git name)")
	}
	if p.SSHKey != "" {
		fmt.Fprintf(w, "    SSH key %s\n", p.SSHKey)
	}
	if p.Signing.Method != "" {
		fmt.Fprintf(w, "    signs commits (%s)\n", p.Signing.Method)
	}
	for _, d := range p.Dirs {
		fmt.Fprintf(w, "    %s  (your existing setup)\n", d)
	}
	for _, f := range p.Found {
		fmt.Fprintf(w, "    %s  (%s)\n", f.Dir, f.Why)
	}
	if len(p.Dirs)+len(p.Found) == 0 {
		fmt.Fprintln(w, "    no directories yet")
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
