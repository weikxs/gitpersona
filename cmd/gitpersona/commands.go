package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/weikxs/gitpersona/internal/app"
	"github.com/weikxs/gitpersona/internal/credential"
	"github.com/weikxs/gitpersona/internal/shell"
)

// runFunc runs a command after its flags are parsed; args are the remaining
// positional arguments.
type runFunc func(ctx context.Context, env app.Env, args []string) int

// command describes one subcommand. bind registers the command's own flags
// on fs and returns the function that runs it.
type command struct {
	synopsis    string
	description string
	maxArgs     int
	bind        func(fs *flag.FlagSet) runFunc
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

var commands = map[string]command{
	"ui": {
		synopsis:    "ui [--no-open]",
		description: "Open the configuration page in your browser. It runs only on this computer (127.0.0.1),\nthe link works once, and the page stops after 15 minutes without use.",
		bind: func(fs *flag.FlagSet) runFunc {
			noOpen := fs.Bool("no-open", false, "print the link without opening a browser")
			return func(ctx context.Context, env app.Env, _ []string) int {
				return fail(env.Stderr, runUI(ctx, env, !*noOpen, ""))
			}
		},
	},
	"setup": {
		synopsis: "setup [--yes] [--no-shell] [--keep-old-rules] [--default <profile> | --no-default] [--print | --web [--no-open]]",
		description: "Find what you already have and set GitPersona up from it in one step: one profile per GitHub\n" +
			"account gh is signed in to (public name and email, or the noreply address), your existing includeIf\n" +
			"identities with their SSH keys and signing, directories from the remotes and emails of the\n" +
			"repositories in your usual code folders, SSH aliases from ~/.ssh/config (keys are never opened),\n" +
			"pins for repositories that differ from their folder, and your login shell. It shows the result and\n" +
			"asks before writing; everything is backed up first. --web shows it in your browser instead.",
		bind: func(fs *flag.FlagSet) runFunc {
			var opts app.SetupOptions
			fs.BoolVar(&opts.Print, "print", false, "print the configuration instead of writing it")
			fs.BoolVar(&opts.Yes, "yes", false, "apply without asking")
			fs.BoolVar(&opts.NoShell, "no-shell", false, "do not add the shell integration")
			fs.BoolVar(&opts.KeepOldRules, "keep-old-rules", false, "keep the includeIf rules the profiles take over")
			fs.BoolVar(&opts.NoDefault, "no-default", false, "set no default profile; your global Git identity stays outside the directories")
			fs.StringVar(&opts.Default, "default", "", "make this profile the default, also when it overrides your own includeIf rules")
			web := fs.Bool("web", false, "set up in the browser: confirm identities, choose directories, review, apply")
			noOpen := fs.Bool("no-open", false, "with --web, print the link without opening a browser")
			return func(ctx context.Context, env app.Env, _ []string) int {
				if *web {
					if opts.Print {
						return fail(env.Stderr, errors.New("choose either --print or --web"))
					}
					section := "setup"
					if _, err := os.Stat(filepath.Join(env.ConfigDir, "config.toml")); err == nil {
						fmt.Fprintln(env.Stdout, "You already have a configuration, so the page opens on the overview instead of setup.")
						section = ""
					}
					return fail(env.Stderr, runUI(ctx, env, !*noOpen, section))
				}
				return fail(env.Stderr, app.Setup(ctx, env, opts))
			}
		},
	},
	"apply": {
		synopsis: "apply [--dry-run] [--yes]",
		description: "Generate the Git configuration for every profile from ~/.config/gitpersona/config.toml\n" +
			"and include it from your global Git configuration. Changed files are backed up first.",
		bind: func(fs *flag.FlagSet) runFunc {
			var opts app.ApplyOptions
			fs.BoolVar(&opts.DryRun, "dry-run", false, "show what would change without writing anything")
			fs.BoolVar(&opts.Yes, "yes", false, "allow removing generated files of profiles that no longer exist")
			return func(ctx context.Context, env app.Env, _ []string) int {
				return fail(env.Stderr, app.Apply(ctx, env, opts))
			}
		},
	},
	"whoami": {
		synopsis:    "whoami [--json] [--check]",
		description: "Show which profile Git uses in the current directory, why, and whether Git agrees.",
		bind: func(fs *flag.FlagSet) runFunc {
			var opts app.WhoamiOptions
			fs.BoolVar(&opts.JSON, "json", false, "print machine-readable JSON")
			fs.BoolVar(&opts.Check, "check", false, "exit with status 1 when Git does not use the profile")
			return func(ctx context.Context, env app.Env, _ []string) int {
				return fail(env.Stderr, app.WhoamiWith(ctx, env, opts))
			}
		},
	},
	"auth": {
		synopsis: "auth login|logout|status [--profile <profile>]",
		description: "Manage the tokens of profiles on hosts other than GitHub (GitLab, Bitbucket, others), one per\n" +
			"account, like gh does for GitHub. login saves a token in your own credential store (the system\n" +
			"keychain or Git Credential Manager; never a file in clear text); logout removes it; status says\n" +
			"which profiles have one. GitPersona keeps no copy. Without --profile, the current directory's\n" +
			"profile is used. login reads the token hidden from the terminal, or from standard input.",
		maxArgs: 1,
		bind: func(fs *flag.FlagSet) runFunc {
			var opts app.AuthOptions
			fs.StringVar(&opts.Profile, "profile", "", "the profile (default: the current directory's)")
			return func(ctx context.Context, env app.Env, args []string) int {
				switch firstArg(args) {
				case "login":
					return fail(env.Stderr, app.AuthLogin(ctx, env, opts))
				case "logout":
					return fail(env.Stderr, app.AuthLogout(ctx, env, opts))
				case "status", "":
					return fail(env.Stderr, app.AuthStatus(ctx, env))
				}
				return fail(env.Stderr, fmt.Errorf("%w: auth takes login, logout or status", errUsage))
			}
		},
	},
	"check-ssh": {
		synopsis: "check-ssh [--profile <profile>]",
		description: "Connect to each profile's host with its SSH key and check which account the host greets\n" +
			"(GitHub, GitLab, Bitbucket). It runs only when you ask: ssh uses the key, as for a push, and\n" +
			"GitPersona never reads it. Exits with status 1 when a key signs in as another account.",
		bind: func(fs *flag.FlagSet) runFunc {
			var opts app.CheckSSHOptions
			fs.StringVar(&opts.Profile, "profile", "", "check only this profile")
			return func(ctx context.Context, env app.Env, _ []string) int {
				return fail(env.Stderr, app.CheckSSH(ctx, env, opts))
			}
		},
	},
	"use": {
		synopsis:    "use <profile>",
		description: "Make repositories in the current directory use a profile (moving the directory from\nanother profile if needed), keep your comments in config.toml, and apply the change.",
		maxArgs:     1,
		bind: func(*flag.FlagSet) runFunc {
			return func(ctx context.Context, env app.Env, args []string) int {
				return fail(env.Stderr, app.Use(ctx, env, firstArg(args)))
			}
		},
	},
	"remove": {
		synopsis:    "remove [directory]",
		description: "Stop assigning a directory (the current one by default) to a profile, and apply the change.",
		maxArgs:     1,
		bind: func(*flag.FlagSet) runFunc {
			return func(ctx context.Context, env app.Env, args []string) int {
				return fail(env.Stderr, app.Remove(ctx, env, firstArg(args)))
			}
		},
	},
	"list": {
		synopsis:    "list [--json]",
		description: "List every profile with its directories and pinned repositories.",
		bind: func(fs *flag.FlagSet) runFunc {
			asJSON := fs.Bool("json", false, "print machine-readable JSON")
			return func(ctx context.Context, env app.Env, _ []string) int {
				return fail(env.Stderr, app.List(ctx, env, *asJSON))
			}
		},
	},
	"pin": {
		synopsis:    "pin <profile>",
		description: "Bind the current repository to a profile, overriding directory rules, and apply the change.\nWorktrees follow their main repository; pin that instead.",
		maxArgs:     1,
		bind: func(*flag.FlagSet) runFunc {
			return func(ctx context.Context, env app.Env, args []string) int {
				return fail(env.Stderr, app.Pin(ctx, env, firstArg(args)))
			}
		},
	},
	"unpin": {
		synopsis:    "unpin",
		description: "Remove the pin of the current repository and apply the change.",
		bind: func(*flag.FlagSet) runFunc {
			return func(ctx context.Context, env app.Env, _ []string) int {
				return fail(env.Stderr, app.Unpin(ctx, env))
			}
		},
	},
	"rollback": {
		synopsis:    "rollback [<name>|latest] [--yes]",
		description: "Without a name, list the backups. With a name (or latest), restore the files of that backup;\nthe current state is backed up first, so the rollback can be undone too.",
		maxArgs:     1,
		bind: func(fs *flag.FlagSet) runFunc {
			yes := fs.Bool("yes", false, "restore without asking")
			return func(ctx context.Context, env app.Env, args []string) int {
				return fail(env.Stderr, app.Rollback(ctx, env, firstArg(args), *yes))
			}
		},
	},
	"uninstall": {
		synopsis: "uninstall [--yes]",
		description: "Remove the GitPersona block from your global Git configuration and from your shell startup files,\n" +
			"put back the includeIf rules setup or doctor --fix took over, and delete the generated files.\n" +
			"Your config.toml and the backups are kept.",
		bind: func(fs *flag.FlagSet) runFunc {
			yes := fs.Bool("yes", false, "uninstall without asking")
			return func(ctx context.Context, env app.Env, _ []string) int {
				return fail(env.Stderr, app.Uninstall(ctx, env, *yes))
			}
		},
	},
	"init": {
		synopsis:    "init <fish|zsh|bash>",
		description: "Print the shell integration script. install-shell loads it from your shell's startup file;\nyou can also add `gitpersona init fish | source` (fish) or `eval \"$(gitpersona init zsh)\"` yourself.",
		maxArgs:     1,
		bind: func(*flag.FlagSet) runFunc {
			return func(_ context.Context, env app.Env, args []string) int {
				kind, err := shell.Parse(firstArg(args))
				if err != nil {
					return fail(env.Stderr, fmt.Errorf("%w: %w", errUsage, err))
				}
				app.InitScript(env, kind)
				return exitOK
			}
		},
	},
	"install-shell": {
		synopsis:    "install-shell [--shell fish,zsh,bash] [--dry-run]",
		description: "Add the shell integration (prompt display, gh switching) to your shell's startup file.\nWithout --shell, your login shell is detected from the system user record.",
		bind: func(fs *flag.FlagSet) runFunc {
			shells := fs.String("shell", "", "comma-separated shells to set up")
			dryRun := fs.Bool("dry-run", false, "show what would change without writing anything")
			return func(ctx context.Context, env app.Env, _ []string) int {
				var kinds []shell.Kind
				for _, name := range strings.FieldsFunc(*shells, func(r rune) bool { return r == ',' }) {
					kind, err := shell.Parse(strings.TrimSpace(name))
					if err != nil {
						return fail(env.Stderr, fmt.Errorf("%w: %w", errUsage, err))
					}
					kinds = append(kinds, kind)
				}
				return fail(env.Stderr, app.InstallShell(ctx, env, kinds, *dryRun))
			}
		},
	},
	"shells": {
		synopsis:    "shells [--json]",
		description: "Show your default (login) shell, the shell you are using right now, and every supported shell\ninstalled, with whether the GitPersona integration is set up in each.",
		bind: func(fs *flag.FlagSet) runFunc {
			asJSON := fs.Bool("json", false, "print machine-readable JSON")
			return func(ctx context.Context, env app.Env, _ []string) int {
				return fail(env.Stderr, app.Shells(ctx, env, *asJSON))
			}
		},
	},
	"prompt": {
		synopsis:    "prompt [--format FORMAT]",
		description: "Print the account Git uses here, for shell prompts. Fast: it never runs git or uses the network.\nFORMAT may use {account}, {profile} and {host} (default {account}).",
		bind: func(fs *flag.FlagSet) runFunc {
			format := fs.String("format", "", "output format")
			return func(_ context.Context, env app.Env, _ []string) int {
				app.Prompt(env, *format)
				return exitOK
			}
		},
	},
	"doctor": {
		synopsis:    "doctor [--fix] [--json]",
		description: "Check the configuration, the generated files, the signed-in accounts and the shell integration,\nand explain how to fix problems. --fix repairs what it safely can (it never removes files).",
		bind: func(fs *flag.FlagSet) runFunc {
			var opts app.DoctorOptions
			fs.BoolVar(&opts.Fix, "fix", false, "repair what can be repaired, then check again")
			fs.BoolVar(&opts.JSON, "json", false, "print machine-readable JSON")
			return func(ctx context.Context, env app.Env, _ []string) int {
				if !app.DoctorWith(ctx, env, opts) {
					return exitFailure
				}
				return exitOK
			}
		},
	},
	"credential": {
		synopsis:    "credential --provider P --host H --account A <get|store|erase>",
		description: "Git credential helper. Git runs this itself; you do not need to call it.",
		maxArgs:     1,
		bind: func(fs *flag.FlagSet) runFunc {
			var t credential.Target
			fs.StringVar(&t.Provider, "provider", "", "hosting provider")
			fs.StringVar(&t.Host, "host", "", "host name")
			fs.StringVar(&t.Account, "account", "", "account name")
			return func(ctx context.Context, env app.Env, args []string) int {
				if len(args) != 1 || t.Provider == "" || t.Host == "" || t.Account == "" {
					return fail(env.Stderr, fmt.Errorf("%w: gitpersona credential --provider P --host H --account A <get|store|erase>", errUsage))
				}
				return fail(env.Stderr, app.Credential(ctx, env, args[0], t))
			}
		},
	},
}
