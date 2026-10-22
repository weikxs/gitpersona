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
	"path/filepath"
	"strings"

	"github.com/weikxs/gitpersona/internal/app"
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
  setup       Create a first configuration from your signed-in GitHub accounts
  apply       Generate Git configuration from the GitPersona configuration
  whoami      Show which profile applies here and why
  use         Make the current directory use a profile
  remove      Stop assigning a directory to a profile
  pin         Bind the current repository to a profile
  unpin       Remove the pin of the current repository
  list        List profiles, directories and pins
  doctor      Check the setup and explain how to fix problems
  shells      Show your default shell, the current shell and installed shells
  install-shell  Add prompt display and gh switching to your shell
  init        Print the shell integration script
  prompt      Print the account Git uses here (for prompts)
  rollback    List backups or restore one
  uninstall   Remove GitPersona from your Git configuration
  version     Print build information
  help        Show help for a command

Run "gitpersona help <command>" for the flags of a command.
Git runs "gitpersona credential" itself; you do not need to call it.
`

// errUsage marks command-line mistakes, which exit with status 2.
var errUsage = errors.New("usage error")

func main() {
	// Installed as `gh` in the shim directory, gitpersona runs the real gh
	// with the account of the current directory.
	if filepath.Base(os.Args[0]) == "gh" {
		os.Exit(runGHShim(os.Args[1:]))
	}
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func runGHShim(args []string) int {
	env, err := app.FromOS(os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		return fail(os.Stderr, err)
	}
	return app.GHShim(context.Background(), env, args)
}

// run is main without process side effects, so it can be tested directly.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, usage)
		return exitOK
	}

	name, rest := args[0], args[1:]
	switch name {
	case "-h", "--help":
		fmt.Fprint(stdout, usage)
		return exitOK
	case "help":
		return help(rest, stdout, stderr)
	case "version", "-v", "--version":
		if len(rest) > 0 {
			return unexpected(stderr, name, rest[0])
		}
		fmt.Fprintln(stdout, version.Current())
		return exitOK
	}

	cmd, ok := commands[name]
	if !ok {
		fmt.Fprintf(stderr, "gitpersona: unknown command %q\nRun 'gitpersona help' for usage.\n", name)
		return exitUsage
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	runCmd := cmd.bind(fs)
	positional, err := parseInterspersed(fs, rest)
	switch {
	case errors.Is(err, flag.ErrHelp):
		printCommandHelp(stdout, cmd, fs)
		return exitOK
	case err != nil:
		fmt.Fprintf(stderr, "gitpersona %s: %v\nRun 'gitpersona help %s' for usage.\n", name, err, name)
		return exitUsage
	}
	if len(positional) > cmd.maxArgs {
		return unexpected(stderr, name, positional[cmd.maxArgs])
	}

	env, err := app.FromOS(stdin, stdout, stderr)
	if err != nil {
		return fail(stderr, err)
	}
	return runCmd(context.Background(), env, positional)
}

// parseInterspersed parses flags anywhere among the arguments, so both
// `rollback latest --yes` and `rollback --yes latest` work. Everything after
// "--" is positional.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		// fs.Parse consumed "--" if it was next; anything left starts with a positional.
		if len(args) > len(rest) && args[len(args)-len(rest)-1] == "--" {
			return append(positional, rest...), nil
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

func help(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, usage)
		return exitOK
	}
	if len(args) > 1 {
		return unexpected(stderr, "help", args[1])
	}
	cmd, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(stderr, "gitpersona: unknown command %q\nRun 'gitpersona help' for usage.\n", args[0])
		return exitUsage
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	cmd.bind(fs)
	printCommandHelp(stdout, cmd, fs)
	return exitOK
}

func printCommandHelp(w io.Writer, cmd command, fs *flag.FlagSet) {
	fmt.Fprintf(w, "Usage: gitpersona %s\n\n%s\n", cmd.synopsis, cmd.description)
	var flags strings.Builder
	fs.VisitAll(func(f *flag.Flag) {
		fmt.Fprintf(&flags, "  --%-10s %s\n", f.Name, f.Usage)
	})
	if flags.Len() > 0 {
		fmt.Fprintf(w, "\nFlags:\n%s", flags.String())
	}
}

func unexpected(stderr io.Writer, command, arg string) int {
	fmt.Fprintf(stderr, "gitpersona %s: unexpected argument %q\nRun 'gitpersona help %s' for usage.\n", command, arg, command)
	return exitUsage
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
