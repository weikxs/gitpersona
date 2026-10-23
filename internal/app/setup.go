package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
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
		proposal.Shells = []string{}
	}
	if opts.KeepOldRules {
		proposal.OldRules = []string{}
	}
	plan := proposal.Plan()
	if opts.Print {
		fmt.Fprint(e.Stdout, renderPlan(plan))
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
	for _, prof := range p.Profiles {
		printProposedProfile(w, prof, prof.ID == p.Default)
	}
	for _, pin := range p.Pins {
		fmt.Fprintf(w, "\n  Pin %s to %s: %s\n", pin.Repo, pin.Profile, pin.Why)
	}
	if p.Default != "" && p.Global.Email != "" {
		fmt.Fprintf(w, "\n  Outside these directories, commits will use %s instead of %s <%s>.\n",
			p.Default, p.Global.Name, p.Global.Email)
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
	for _, u := range p.Unassigned {
		fmt.Fprintf(w, "\n  Left out: %s (%s, owned by %s). Run `gitpersona use <profile>` there to add it.\n", u.Dir, u.Why, u.Owner)
	}
	for _, u := range p.Unmatched {
		fmt.Fprintf(w, "\n  Left out: %s <%s> from %s; no account signed in to gh uses it.\n", u.Name, u.Email, u.Source)
	}
}

func printProposedProfile(w io.Writer, p SetupProfile, isDefault bool) {
	label := p.ID
	if isDefault {
		label += " (default)"
	}
	fmt.Fprintf(w, "\n  %s: %s <%s>\n", label, p.Name, p.Email)
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
}

// profileID derives a valid profile id from a login.
func profileID(login string) string {
	id := strings.Trim(invalidIDChars.ReplaceAllString(strings.ToLower(login), "-"), "-")
	if len(id) > 32 {
		id = id[:32]
	}
	return id
}
