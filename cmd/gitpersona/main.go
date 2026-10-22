// Command gitpersona switches Git identity, credentials and CLI accounts
// automatically by directory.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/weikxs/gitpersona/internal/version"
)

const (
	exitOK    = 0
	exitUsage = 2
)

const usage = `GitPersona: Git profiles that follow your directories.

Usage:
  gitpersona <command>

Commands:
  version     Print build information
  help        Show this help

Flags:
  -h, --help     Show this help
  -v, --version  Print build information
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is main without process side effects, so it can be tested directly.
func run(args []string, stdout, stderr io.Writer) int {
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
	default:
		fmt.Fprintf(stderr, "gitpersona: unknown command %q\nRun 'gitpersona help' for usage.\n", args[0])
		return exitUsage
	}
}
