package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/provider"
)

// noPrompt keeps Git and Git Credential Manager from asking on the terminal
// or in a window: GitPersona asks for the token itself, and a check must
// never wait for an answer.
var noPrompt = []string{"GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", "GIT_ASKPASS=", "SSH_ASKPASS="}

// AuthOptions selects the profile of an auth command; empty means the
// profile of the current directory.
type AuthOptions struct {
	Profile string
}

// AuthLogin saves the token of a profile on a host other than GitHub in the
// user's own credential store (the keychain, Git Credential Manager), next
// to the tokens of the other accounts on that host. GitPersona keeps no
// copy: Git's helper stores it, and Git and glab ask for it by account.
func AuthLogin(ctx context.Context, e Env, opts AuthOptions) error {
	p, err := e.authProfile(opts)
	if err != nil {
		return err
	}
	if err := e.requireSafeHelper(ctx, p); err != nil {
		return err
	}
	token, err := e.readToken(e.tr("Token for %s on %s (input is hidden): ", p.Account, p.Host))
	if err != nil {
		return err
	}
	if _, err := e.runInput()(ctx, credentialRequest(p, token), noPrompt, "git", "-C", e.Home, "credential", "approve"); err != nil {
		return e.trErr("the credential helper did not save the token: %w", err)
	}
	saved := e.tr("Saved the token for %s on %s in your credential store; Git uses it in profile %s's directories.",
		p.Account, p.Host, p.ID)
	if p.Provider == config.ProviderGitLab {
		saved = e.tr("Saved the token for %s on %s in your credential store; Git and glab use it in profile %s's directories.",
			p.Account, p.Host, p.ID)
	}
	fmt.Fprintln(e.Stdout, saved)
	return nil
}

// AuthLogout removes a profile's token from the user's credential store.
func AuthLogout(ctx context.Context, e Env, opts AuthOptions) error {
	p, err := e.authProfile(opts)
	if err != nil {
		return err
	}
	if _, err := e.runInput()(ctx, credentialRequest(p, ""), noPrompt, "git", "-C", e.Home, "credential", "reject"); err != nil {
		return e.trErr("the credential helper did not remove the token: %w", err)
	}
	fmt.Fprintln(e.Stdout, e.tr("Removed the token for %s on %s from your credential store.", p.Account, p.Host))
	return nil
}

// AuthStatus says, for each profile on a host other than GitHub, whether
// the user's credential store has its token. It asks the store the way Git
// does and throws the token away at once.
// With a profile it checks that one only. It fails when a token is missing,
// so scripts can tell.
func AuthStatus(ctx context.Context, e Env, opts AuthOptions) error {
	cfg, err := e.loadConfig()
	if err != nil {
		return err
	}
	if opts.Profile != "" {
		if err := knownProfile(cfg, opts.Profile, "auth status --profile"); err != nil {
			return err
		}
	}
	shown, missing, invalid := 0, 0, 0
	for _, p := range cfg.Profiles {
		if p.Provider == config.ProviderGitHub || (opts.Profile != "" && p.ID != opts.Profile) {
			continue
		}
		shown++
		if err := e.checkAuthProfile(p); err != nil {
			invalid++
			fmt.Fprintln(e.Stdout, e.tr("✗ %v", err))
			continue
		}
		if _, err := e.profileToken(ctx, p); err != nil {
			missing++
			fmt.Fprintln(e.Stdout, e.tr("✗ %s: no token for %s on %s (or your credential helper failed); run `gitpersona auth login --profile %s`", p.ID, p.Account, p.Host, p.ID))
			continue
		}
		fmt.Fprintln(e.Stdout, e.tr("✓ %s: the credential store has a token for %s on %s", p.ID, p.Account, p.Host))
	}
	switch {
	case shown == 0 && opts.Profile != "":
		return e.trErr("profile %s is a GitHub profile; check its account with `gh auth status`", opts.Profile)
	case shown == 0:
		fmt.Fprintln(e.Stdout, e.tr("No profile is on a host other than GitHub; GitHub accounts are signed in with `gh auth login`."))
	case missing > 0 && invalid > 0:
		return e.trErr("no token for %d of %d profiles, and %d need a valid host and account", missing, shown, invalid)
	case missing > 0:
		return e.trErr("no token for %d of %d profiles", missing, shown)
	case invalid > 0:
		return e.trErr("%d of %d profiles need a valid host and account", invalid, shown)
	}
	return nil
}

// checkAuthProfile refuses a profile whose host or account is not valid: a
// token must never be saved for a host the configuration does not name.
// Mistakes in other profiles do not stop auth for this one.
func (e Env) checkAuthProfile(p config.Profile) error {
	if !config.ValidHost(p.Host) || !config.ValidAccount(p.Account) {
		return e.trErr("profile %s needs a valid host and account in %s (host %q, account %q)",
			p.ID, e.display(e.configFile()), p.Host, p.Account)
	}
	return nil
}

// profileToken asks the user's credential store for a profile's token, the
// way Git does, without any prompt. The token is for one process only.
func (e Env) profileToken(ctx context.Context, p config.Profile) (string, error) {
	// A credential helper that waits for something (a locked keychain, a
	// prompt it should not show) must not hang glab forever.
	ctx, cancel := context.WithTimeout(ctx, credentialTimeout)
	defer cancel()
	out, err := e.runInput()(ctx, credentialRequest(p, ""), noPrompt, "git", "-C", e.Home, "credential", "fill")
	if err != nil {
		return "", errors.New("no token") // never repeat the helper's output: it may hold a secret
	}
	for _, line := range strings.Split(string(out), "\n") {
		if token, ok := strings.CutPrefix(line, "password="); ok && token != "" {
			return token, nil
		}
	}
	return "", errors.New("no token")
}

// credentialRequest is a Git credential request for a profile's account.
func credentialRequest(p config.Profile, token string) string {
	req := "protocol=https\nhost=" + p.Host + "\nusername=" + p.Account + "\n"
	if token != "" {
		req += "password=" + token + "\n"
	}
	return req + "\n"
}

// authProfile returns the profile an auth command is for.
func (e Env) authProfile(opts AuthOptions) (config.Profile, error) {
	cfg, err := e.loadConfig()
	if err != nil {
		return config.Profile{}, err
	}
	var p config.Profile
	if opts.Profile != "" {
		found, ok := cfg.ProfileByID(opts.Profile)
		if !ok {
			return p, e.trErr("no profile %q; the profiles are %s", opts.Profile, profileIDs(cfg))
		}
		p = found
	} else {
		r := e.resolveDir(cfg)
		if !r.Found {
			return p, e.trErr("no profile applies here; name one with --profile")
		}
		p = r.Profile
	}
	if p.Provider == config.ProviderGitHub {
		return p, e.trErr("profile %s is on GitHub, whose accounts gh keeps; sign in with `gh auth login --hostname %s`", p.ID, p.Host)
	}
	return p, e.checkAuthProfile(p)
}

// requireSafeHelper refuses to save a token where no helper would keep it,
// or where the helper keeps it in a file in clear text.
func (e Env) requireSafeHelper(ctx context.Context, p config.Profile) error {
	_, issue, err := e.helpersProblem(ctx, p)
	if err != nil {
		return err
	}
	if issue.problem != "" {
		return e.trErr("%s; %s first", issue.problem, issue.advice)
	}
	return nil
}

// readToken reads a token: hidden from a terminal, or the first line of
// standard input otherwise (for scripts).
func (e Env) readToken(prompt string) (string, error) {
	var token string
	if f, ok := e.Stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprint(e.Stderr, prompt)
		raw, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(e.Stderr)
		if err != nil {
			return "", e.trErr("read the token: %w", err)
		}
		token = string(raw)
	} else {
		line, err := bufio.NewReader(e.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", e.trErr("no token given; paste it when asked, or pipe it on standard input")
		}
		token = line
	}
	token = strings.TrimSpace(token)
	if token == "" || strings.ContainsAny(token, "\r\n\x00") {
		return "", e.trErr("the token is empty or not on one line")
	}
	return token, nil
}

// runInput returns how programs with input are run.
func (e Env) runInput() provider.InputRunner {
	if e.RunInput != nil {
		return e.RunInput
	}
	return provider.ExecInputRunner
}
