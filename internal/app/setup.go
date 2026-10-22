package app

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/provider"
)

const setupTimeout = 30 * time.Second

// SetupOptions controls Setup.
type SetupOptions struct {
	Print bool // print the configuration instead of writing it
}

var invalidIDChars = regexp.MustCompile(`[^a-z0-9-]+`)

// Setup writes a first configuration with one profile per GitHub account gh
// is signed in to, the public email of each account (or its noreply address
// when it shows none), and gh's active account as the default. It asks gh
// for account status (no tokens) and for each account's public profile.
func Setup(ctx context.Context, e Env, opts SetupOptions) error {
	if _, err := os.Stat(e.configFile()); err == nil && !opts.Print {
		return fmt.Errorf("%s already exists; change it with `gitpersona use` and `gitpersona pin`, or edit it", e.display(e.configFile()))
	}
	ctx, cancel := context.WithTimeout(ctx, setupTimeout)
	defer cancel()

	source, accounts, err := e.signedInAccounts(ctx)
	if err != nil {
		return err
	}

	text := renderSetup(ctx, source, accounts)
	cfg, err := config.Parse([]byte(text))
	if err == nil {
		err = cfg.Validate(e.Home, e.CaseInsensitive)
	}
	if err != nil {
		return fmt.Errorf("generated configuration is not valid (please report this): %w", err)
	}
	if opts.Print {
		fmt.Fprint(e.Stdout, text)
		return nil
	}
	if err := e.applyChanges([]change{{path: e.configFile(), content: text, create: true}}, ApplyOptions{Command: "setup"}); err != nil {
		return err
	}
	fmt.Fprintf(e.Stdout, "\nNext steps:\n"+
		"  1. Check names and emails in %s.\n"+
		"  2. In each directory that belongs to an account: gitpersona use <profile> (this also applies it)\n"+
		"     or, after editing the file yourself: gitpersona apply\n"+
		"  3. gitpersona install-shell, then gitpersona doctor\n", e.display(e.configFile()))
	return nil
}

func renderSetup(ctx context.Context, source provider.GitHub, accounts []provider.Account) string {
	var b strings.Builder
	b.WriteString("# GitPersona configuration, created by `gitpersona setup`.\n")
	b.WriteString("# Assign directories with `gitpersona use <profile>`; see the README for every option.\n")
	b.WriteString("version = 1\n")
	for _, a := range accounts {
		if a.Active {
			fmt.Fprintf(&b, "default = %q # gh's active account\n", profileID(a.Login))
		}
	}
	for _, a := range accounts {
		if a.State != "success" {
			continue
		}
		id := ghIdentity(ctx, source, a.Login)
		note := "public email on GitHub"
		if id.emailSource == "noreply" {
			note = "GitHub noreply address; the account shows no public email"
		}
		fmt.Fprintf(&b, "\n[[profile]]\nid      = %q\naccount = %q\nname    = %q\nemail   = %q # %s\n",
			profileID(a.Login), a.Login, id.name, id.email, note)
	}
	return b.String()
}

// profileID derives a valid profile id from a login.
func profileID(login string) string {
	id := strings.Trim(invalidIDChars.ReplaceAllString(strings.ToLower(login), "-"), "-")
	if len(id) > 32 {
		id = id[:32]
	}
	return id
}
