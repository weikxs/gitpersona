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
