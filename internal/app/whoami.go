package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/paths"
	"github.com/weikxs/gitpersona/internal/resolve"
)

// WhoamiReport is the stable --json output of `gitpersona whoami`.
type WhoamiReport struct {
	Found          bool   `json:"found"`
	Profile        string `json:"profile,omitempty"`
	Account        string `json:"account,omitempty"`
	Provider       string `json:"provider,omitempty"`
	Host           string `json:"host,omitempty"`
	Name           string `json:"name,omitempty"`
	Email          string `json:"email,omitempty"`
	Reason         string `json:"reason"`
	Match          string `json:"match,omitempty"`
	Path           string `json:"path"`
	InRepository   bool   `json:"in_repository"`
	EffectiveEmail string `json:"effective_email,omitempty"`
	EffectiveName  string `json:"effective_name,omitempty"`
	// EmailSource is the file Git reads user.email from here, with ~.
	EmailSource string `json:"email_source,omitempty"`
	Consistent  bool   `json:"consistent"`
	// EnvOverride names an environment variable that sets the author email
	// for commits made here, overriding every configuration file.
	EnvOverride string `json:"env_override,omitempty"`
	// LocalEmail is a repository-local user.email, which also overrides profiles.
	LocalEmail string `json:"local_email,omitempty"`
	// NewRepositories, outside a repository, is the profile that repositories
	// created or cloned here will use.
	NewRepositories *NewRepositories `json:"new_repositories,omitempty"`
}

// NewRepositories describes the profile future repositories in a directory get.
type NewRepositories struct {
	Profile string `json:"profile"`
	Account string `json:"account"`
	Reason  string `json:"reason"`
	Match   string `json:"match,omitempty"`
}

// WhoamiOptions controls Whoami.
type WhoamiOptions struct {
	JSON  bool // print a WhoamiReport
	Check bool // fail when Git does not use the profile
}

// Whoami reports which profile applies to the current directory and whether
// Git currently agrees.
func Whoami(ctx context.Context, e Env, asJSON bool) error {
	return WhoamiWith(ctx, e, WhoamiOptions{JSON: asJSON})
}

// WhoamiWith runs Whoami with options.
func WhoamiWith(ctx context.Context, e Env, opts WhoamiOptions) error {
	cfg, err := e.loadConfig()
	if err != nil {
		return err
	}
	report := e.whoami(ctx, cfg)
	if opts.JSON {
		enc := json.NewEncoder(e.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return err
		}
	} else {
		printWhoami(e, report)
	}
	if opts.Check && !report.Consistent {
		return e.trErr("this repository does not use profile %s: %s", report.Profile, e.mismatchAdvice(report))
	}
	return nil
}

func (e Env) whoami(ctx context.Context, cfg config.Config) WhoamiReport {
	report, _ := e.whoamiRepo(ctx, cfg)
	return report
}

// repoState is what one rev-parse and one config dump tell about the
// directory whoami ran in; verify reuses it for the remotes.
type repoState struct {
	root, gitDir string
	config       repoConfig
}

// whoamiRepo builds the whoami report with two git processes: rev-parse for
// the Git directory and the work tree, and one dump of every setting.
func (e Env) whoamiRepo(ctx context.Context, cfg config.Config) (WhoamiReport, repoState) {
	opts := resolve.Options{Home: e.Home, CaseInsensitive: e.CaseInsensitive, Canonicalize: paths.ResolveSymlinks}
	// Bare repositories and the inside of .git have a Git directory but no
	// work tree, so the Git directory decides whether we are in a repository.
	gitDir, root, inRepo := e.gitDirAndRoot(ctx, e.WorkDir)
	state := repoState{root: root, gitDir: gitDir}

	var r resolve.Result
	report := WhoamiReport{InRepository: inRepo}
	if inRepo {
		state.config, _ = e.readRepoConfig(ctx, root)
		report.Path = paths.ResolveSymlinks(root)
		r = resolve.GitDir(cfg, paths.ResolveSymlinks(gitDir), opts)
		report.EffectiveEmail = state.config.get("user.email")
		report.EffectiveName = state.config.get("user.name")
		if file := state.config.originFile("user.email", root); file != "" {
			report.EmailSource = e.display(file)
		}
	} else {
		report.Path = paths.ResolveSymlinks(e.WorkDir)
		r = resolve.OutsideRepository(cfg)
		if future := resolve.Resolve(cfg, report.Path, opts); future.Found {
			report.NewRepositories = &NewRepositories{Profile: future.Profile.ID, Account: future.Profile.Account,
				Reason: string(future.Reason), Match: future.Match}
		}
	}

	report.Found, report.Reason, report.Match = r.Found, string(r.Reason), r.Match
	if r.Found {
		p := r.Profile
		report.Profile, report.Account, report.Provider, report.Host = p.ID, p.Account, p.Provider, p.Host
		report.Name, report.Email = p.Name, p.Email
	}
	if inRepo {
		report.LocalEmail = state.config.getLocal("user.email", root, gitDir)
	}
	for _, name := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		if v := os.Getenv(name); v != "" && v != report.Email {
			report.EnvOverride = name
			break
		}
	}
	report.Consistent = report.EnvOverride == "" && (!inRepo || !r.Found || report.EffectiveEmail == report.Email)
	return report, state
}

// gitDirAndRoot asks Git once for dir's Git directory and work tree. root is
// the top of the work tree, or the Git directory itself in a bare repository
// or inside .git; inRepo is false outside any repository.
func (e Env) gitDirAndRoot(ctx context.Context, dir string) (gitDir, root string, inRepo bool) {
	out, err := e.Run(ctx, "git", "-C", dir, "rev-parse", "--absolute-git-dir", "--is-inside-work-tree", "--show-cdup")
	if err != nil {
		return "", "", false
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	gitDir = strings.TrimSpace(lines[0])
	if gitDir == "" {
		return "", "", false
	}
	if len(lines) < 2 || strings.TrimSpace(lines[1]) != "true" {
		return gitDir, gitDir, true
	}
	cdup := ""
	if len(lines) > 2 {
		cdup = strings.TrimSpace(lines[2])
	}
	// Git counts the way up from the real directory, so the symbolic links
	// in dir are resolved before walking up.
	return gitDir, filepath.Clean(filepath.Join(paths.ResolveSymlinks(dir), cdup)), true
}

// mismatchAdvice explains why Git does not use the profile and how to fix it.
func (e Env) mismatchAdvice(r WhoamiReport) string {
	switch {
	case r.EnvOverride != "":
		return e.tr("the environment variable %s overrides the profile; unset it (for example in your direnv or shell setup)", r.EnvOverride)
	case r.LocalEmail != "":
		return e.tr("this repository sets user.email %q itself; remove it with `git config --local --unset user.email`", r.LocalEmail)
	case strings.HasSuffix(r.EmailSource, "config.worktree"):
		return e.tr("this worktree sets user.email %q itself; remove it with `git config --worktree --unset user.email`", r.EffectiveEmail)
	case r.EmailSource != "":
		return e.tr("Git currently uses %q from %s", r.EffectiveEmail, r.EmailSource)
	default:
		return e.tr("Git currently uses %q; run `gitpersona doctor`", r.EffectiveEmail)
	}
}

func printWhoami(e Env, r WhoamiReport) {
	if !r.InRepository {
		fmt.Fprintln(e.Stdout, e.tr("Not inside a Git repository (%s).", e.display(r.Path)))
	}
	if !r.Found {
		fmt.Fprintln(e.Stdout, e.tr("No GitPersona profile applies here; Git uses its own settings."))
		if r.InRepository && r.EffectiveEmail != "" {
			fmt.Fprintln(e.Stdout, e.tr("  author  %s <%s>%s", r.EffectiveName, r.EffectiveEmail, e.fromFile(r.EmailSource)))
		}
	} else {
		printProfile(e, r)
	}
	if n := r.NewRepositories; n != nil {
		fmt.Fprintln(e.Stdout, e.tr("Repositories created or cloned here will use %s (profile %s, %s).",
			n.Account, n.Profile, e.reasonWithMatch(n.Reason, n.Match)))
	}
}

func printProfile(e Env, r WhoamiReport) {
	fmt.Fprintln(e.Stdout, e.tr("%s  (profile %s · %s on %s)", r.Account, r.Profile, r.Provider, r.Host))
	name, email := r.Name, r.Email
	if r.InRepository && r.EffectiveEmail != "" {
		name, email = r.EffectiveName, r.EffectiveEmail // what commits really use
	}
	source := e.fromFile(r.EmailSource)
	if env := os.Getenv("GIT_AUTHOR_EMAIL"); env != "" {
		email, source = env, e.tr(" (from GIT_AUTHOR_EMAIL)") // set for every commit, whatever the configuration says
	}
	fmt.Fprintln(e.Stdout, e.tr("  author  %s <%s>%s", name, email, source))
	fmt.Fprintln(e.Stdout, e.tr("  reason  %s", e.reasonWithMatch(r.Reason, r.Match)))
	fmt.Fprintln(e.Stdout, e.tr("  path    %s", e.display(r.Path)))
	if !r.Consistent {
		fmt.Fprintln(e.Stdout, e.tr("  warning %s", e.mismatchAdvice(r)))
	}
}

// WhoamiReportFor returns the whoami report for e.WorkDir.
func WhoamiReportFor(ctx context.Context, e Env) (WhoamiReport, error) {
	cfg, err := e.loadConfig()
	if err != nil {
		return WhoamiReport{}, err
	}
	return e.whoami(ctx, cfg), nil
}

// fromFile says which file the author's email comes from, or "".
func (e Env) fromFile(source string) string {
	if source == "" {
		return ""
	}
	return e.tr(" (email from %s)", source)
}
