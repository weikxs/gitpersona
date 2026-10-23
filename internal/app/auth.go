package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
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
	if err := e.requireSafeHelper(ctx, p.Host); err != nil {
		return err
	}
	token, err := e.readToken(fmt.Sprintf("Token for %s on %s (input is hidden): ", p.Account, p.Host))
	if err != nil {
		return err
	}
	if _, err := e.runInput()(ctx, credentialRequest(p, token), noPrompt, "git", "-C", e.Home, "credential", "approve"); err != nil {
		return fmt.Errorf("the credential helper did not save the token: %w", err)
	}
	fmt.Fprintf(e.Stdout, "Saved the token for %s on %s in your credential store; Git and glab use it in profile %s's directories.\n",
		p.Account, p.Host, p.ID)
	return nil
}

// AuthLogout removes a profile's token from the user's credential store.
func AuthLogout(ctx context.Context, e Env, opts AuthOptions) error {
	p, err := e.authProfile(opts)
	if err != nil {
		return err
	}
	if _, err := e.runInput()(ctx, credentialRequest(p, ""), noPrompt, "git", "-C", e.Home, "credential", "reject"); err != nil {
		return fmt.Errorf("the credential helper did not remove the token: %w", err)
	}
	fmt.Fprintf(e.Stdout, "Removed the token for %s on %s from your credential store.\n", p.Account, p.Host)
	return nil
}

// AuthStatus says, for each profile on a host other than GitHub, whether
// the user's credential store has its token. It asks the store the way Git
// does and throws the token away at once.
func AuthStatus(ctx context.Context, e Env) error {
	cfg, err := e.loadConfig()
	if err != nil {
		return err
	}
	shown := false
	for _, p := range cfg.Profiles {
		if p.Provider == config.ProviderGitHub {
			continue
		}
		shown = true
		if _, err := e.profileToken(ctx, p); err != nil {
			fmt.Fprintf(e.Stdout, "✗ %s: no token for %s on %s; run `gitpersona auth login --profile %s`\n", p.ID, p.Account, p.Host, p.ID)
			continue
		}
		fmt.Fprintf(e.Stdout, "✓ %s: the credential store has a token for %s on %s\n", p.ID, p.Account, p.Host)
	}
	if !shown {
		fmt.Fprintln(e.Stdout, "No profile is on a host other than GitHub; GitHub accounts are signed in with `gh auth login`.")
	}
	return nil
}

// profileToken asks the user's credential store for a profile's token, the
// way Git does, without any prompt. The token is for one process only.
func (e Env) profileToken(ctx context.Context, p config.Profile) (string, error) {
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
			return p, fmt.Errorf("no profile %q; the profiles are %s", opts.Profile, profileIDs(cfg))
		}
		p = found
	} else {
		r := e.resolveDir(cfg)
		if !r.Found {
			return p, errors.New("no profile applies here; name one with --profile")
		}
		p = r.Profile
	}
	if p.Provider == config.ProviderGitHub {
		return p, fmt.Errorf("profile %s is on GitHub, whose accounts gh keeps; sign in with `gh auth login --hostname %s`", p.ID, p.Host)
	}
	return p, nil
}

// requireSafeHelper refuses to save a token where no helper would keep it,
// or where the helper keeps it in a file in clear text.
func (e Env) requireSafeHelper(ctx context.Context, host string) error {
	helpers, err := e.hostHelpers(ctx, host)
	if err != nil {
		return err
	}
	kinds := make([]string, len(helpers))
	for i, h := range helpers {
		kinds[i] = helperKind(h)
	}
	switch {
	case len(helpers) == 0:
		return fmt.Errorf("no credential helper would keep the token for %s; set one up first, for example "+
			"`git config --global credential.helper %s`", host, suggestedHelper())
	case slices.Contains(kinds, "plain"):
		return fmt.Errorf("your credential helper for %s (store) keeps tokens in a file in clear text; "+
			"use the system keychain instead, for example `git config --global credential.helper %s`", host, suggestedHelper())
	case !slices.ContainsFunc(kinds, func(k string) bool { return k != "memory" }):
		return fmt.Errorf("your credential helper for %s (cache) forgets tokens after a while; "+
			"use the system keychain instead, for example `git config --global credential.helper %s`", host, suggestedHelper())
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
			return "", fmt.Errorf("read the token: %w", err)
		}
		token = string(raw)
	} else {
		line, err := bufio.NewReader(e.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", errors.New("no token given; paste it when asked, or pipe it on standard input")
		}
		token = line
	}
	token = strings.TrimSpace(token)
	if token == "" || strings.ContainsAny(token, "\r\n\x00") {
		return "", errors.New("the token is empty or not on one line")
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
