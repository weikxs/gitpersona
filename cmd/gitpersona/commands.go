package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/weikxs/gitpersona/internal/app"
	"github.com/weikxs/gitpersona/internal/credential"
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
		synopsis:    "whoami [--json]",
		description: "Show which profile Git uses in the current directory, why, and whether Git agrees.",
		bind: func(fs *flag.FlagSet) runFunc {
			asJSON := fs.Bool("json", false, "print machine-readable JSON")
			return func(ctx context.Context, env app.Env, _ []string) int {
				return fail(env.Stderr, app.Whoami(ctx, env, *asJSON))
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
		synopsis:    "uninstall [--yes]",
		description: "Remove the GitPersona block from your global Git configuration and delete the generated files.\nYour config.toml and the backups are kept.",
		bind: func(fs *flag.FlagSet) runFunc {
			yes := fs.Bool("yes", false, "uninstall without asking")
			return func(ctx context.Context, env app.Env, _ []string) int {
				return fail(env.Stderr, app.Uninstall(ctx, env, *yes))
			}
		},
	},
	"doctor": {
		synopsis:    "doctor",
		description: "Check the configuration, the generated files and the signed-in accounts, and explain how to fix problems.",
		bind: func(*flag.FlagSet) runFunc {
			return func(ctx context.Context, env app.Env, _ []string) int {
				if !app.Doctor(ctx, env) {
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
