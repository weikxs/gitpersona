// Command gitpersona switches Git identity, credentials and CLI accounts
// automatically by directory.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/weikxs/gitpersona/internal/app"
	"github.com/weikxs/gitpersona/internal/credential"
	"github.com/weikxs/gitpersona/internal/version"
)

const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2
)

const usage = `GitPersona: Git profiles that follow your directories.

Usage:
  gitpersona <command> [flags]

Commands:
  apply       Generate Git configuration from the GitPersona configuration
  whoami      Show which profile applies here and why (--json for scripts)
  doctor      Check the setup and explain how to fix problems
  version     Print build information
  help        Show this help

Git runs "gitpersona credential" itself; you do not need to call it.

Flags:
  -h, --help     Show this help
  -v, --version  Print build information
`

// errUsage marks command-line mistakes, which exit with status 2.
var errUsage = errors.New("usage error")

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is main without process side effects, so it can be tested directly.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, usage)
		return exitOK
	}

	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return exitOK
	case "version", "-v", "--version":
		fmt.Fprintln(stdout, version.Current())
		return exitOK
	case "apply", "whoami", "doctor", "credential":
		env, err := app.FromOS(stdin, stdout, stderr)
		if err != nil {
			return fail(stderr, err)
		}
		return runCommand(context.Background(), env, args[0], args[1:])
	default:
		fmt.Fprintf(stderr, "gitpersona: unknown command %q\nRun 'gitpersona help' for usage.\n", args[0])
		return exitUsage
	}
}

func runCommand(ctx context.Context, env app.Env, name string, args []string) int {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	asJSON := fs.Bool("json", false, "print machine-readable JSON")
	var target credential.Target
	if name == "credential" {
		fs.StringVar(&target.Provider, "provider", "", "hosting provider")
		fs.StringVar(&target.Host, "host", "", "host name")
		fs.StringVar(&target.Account, "account", "", "account name")
	}
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	var err error
	switch name {
	case "apply":
		err = app.Apply(ctx, env)
	case "whoami":
		err = app.Whoami(ctx, env, *asJSON)
	case "doctor":
		if !app.Doctor(ctx, env) {
			return exitFailure
		}
	case "credential":
		err = credentialCommand(ctx, env, fs.Args(), target)
	}
	return fail(env.Stderr, err)
}

func credentialCommand(ctx context.Context, env app.Env, args []string, target credential.Target) error {
	if len(args) != 1 || target.Provider == "" || target.Host == "" || target.Account == "" {
		return fmt.Errorf("%w: gitpersona credential --provider P --host H --account A <get|store|erase>", errUsage)
	}
	return app.Credential(ctx, env, args[0], target)
}

// fail prints err and returns the matching exit status.
func fail(stderr io.Writer, err error) int {
	if err == nil {
		return exitOK
	}
	fmt.Fprintf(stderr, "gitpersona: %v\n", err)
	if errors.Is(err, errUsage) {
		return exitUsage
	}
	return exitFailure
}
