package app

import (
	"bufio"
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
}

var invalidIDChars = regexp.MustCompile(`[^a-z0-9-]+`)

// Setup finds everything GitPersona can use (see ProposeSetup), shows the
// configuration it would create, and after the user agrees writes it, the
// Git configuration and the shell integration in one backup.
func Setup(ctx context.Context, e Env, opts SetupOptions) error {
	if _, err := os.Stat(e.configFile()); err == nil && !opts.Print {
		return fmt.Errorf("%s already exists; change it on the configuration page (`gitpersona ui`), with `gitpersona use` and `gitpersona pin`, or by hand", e.display(e.configFile()))
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
	if opts.NoDefault {
		proposal = e.withoutDefault(proposal)
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
	fmt.Fprintf(w, "GitPersona found %s", plural(len(p.Profiles), "GitHub account"))
	if len(p.Roots) > 0 {
		fmt.Fprintf(w, " and looked for repositories in %s", strings.Join(p.Roots, ", "))
	}
	fmt.Fprintln(w, ":")
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
	}
	if len(p.OldRules) > 0 {
		fmt.Fprintf(w, "\n  The profiles take over your includeIf rules for %s; they are removed from your Git configuration (--keep-old-rules to keep them).\n",
			strings.Join(p.OldRules, ", "))
	}
	for _, k := range p.KeptRules {
		fmt.Fprintf(w, "\n  Kept your includeIf rule for %s: %s.\n", k.Dir, k.Why)
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
// take over: its rule keeps working, or a profile replaces it.
func printLeftOut(w io.Writer, p SetupProposal, u LeftOut) {
	where := strings.Join(u.Dirs, ", ")
	if u.Overridden != "" {
		by := "Git's own settings"
		if k := slices.IndexFunc(p.Profiles, func(x SetupProfile) bool { return x.ID == u.AccountFrom }); k >= 0 {
			by = fmt.Sprintf("profile %s (%s)", u.AccountFrom, author(p.Profiles[k].Name, p.Profiles[k].Email))
		}
		fmt.Fprintf(w, "\n  ! Replaced: %s for %s (from %s). GitPersona cannot keep your rule working, because %s, so commits "+
			"there will use %s. To keep it, choose its account with `gitpersona setup --web`, or move those settings out of the rule's file.\n",
			author(u.Name, u.Email), where, u.Source, u.Overridden, by)
		return
	}
	fmt.Fprintf(w, "\n  Not taken over: %s for %s (from %s); no signed-in account could be tied to it, so your rule keeps "+
		"applying there. To make it a profile, choose its account with `gitpersona setup --web`.\n", author(u.Name, u.Email), where, u.Source)
	if u.AccountFrom != "" {
		fmt.Fprintf(w, "    What your rule does not set, such as the account for pushes, comes from profile %s there.\n", u.AccountFrom)
	}
}

// printUnassigned lists, together, the folders whose repositories belong to
// someone else, such as an organization.
func printUnassigned(w io.Writer, p SetupProposal) {
	if len(p.Unassigned) == 0 {
		return
	}
	applies := "Git's own settings apply"
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

// profileID derives a valid profile id from a login.
func profileID(login string) string {
	id := strings.Trim(invalidIDChars.ReplaceAllString(strings.ToLower(login), "-"), "-")
	if len(id) > 32 {
		id = id[:32]
	}
	return id
}

// withoutDefault drops the default profile from a proposal, and works out
// again which profile applies around the identities left out.
func (e Env) withoutDefault(p SetupProposal) SetupProposal {
	owners := setupOwners(p.Profiles)
	unmatched := make([]LeftOut, len(p.Unmatched))
	for i, u := range p.Unmatched {
		u.AccountFrom = ""
		for _, d := range u.Dirs {
			if by, ok := e.overriddenBy(d, owners, ""); ok && u.AccountFrom == "" {
				u.AccountFrom = by.id
			}
		}
		unmatched[i] = u
	}
	p.Default, p.Unmatched = "", unmatched
	return p
}
