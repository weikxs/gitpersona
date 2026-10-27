// Package shell detects the user's shell and produces the integration
// scripts for fish, zsh and bash.
package shell

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/weikxs/gitpersona/internal/i18n"
)

// Kind is a supported shell.
type Kind string

// Supported shells.
const (
	Fish    Kind = "fish"
	Zsh     Kind = "zsh"
	Bash    Kind = "bash"
	Unknown Kind = ""
)

// All lists the supported shells.
var All = []Kind{Fish, Zsh, Bash}

// Runner runs a program and returns its standard output.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Detected is the user's login shell and where that answer came from.
type Detected struct {
	Path   string
	Kind   Kind
	Source string // "dscl", "getent" or "$SHELL"
}

// LoginShell returns the shell the user logs in with. The system's user
// record wins over $SHELL, because terminals, IDEs and automation tools
// often run commands in another shell and set $SHELL to it.
func LoginShell(ctx context.Context, goos, user string, run Runner, getenv func(string) string) Detected {
	switch goos {
	case "darwin":
		if out, err := run(ctx, "dscl", ".", "-read", "/Users/"+user, "UserShell"); err == nil {
			if _, path, ok := strings.Cut(strings.TrimSpace(string(out)), "UserShell:"); ok {
				return detected(strings.TrimSpace(path), "dscl")
			}
		}
	default:
		if out, err := run(ctx, "getent", "passwd", user); err == nil {
			fields := strings.Split(strings.TrimSpace(string(out)), ":")
			if len(fields) >= 7 {
				return detected(fields[6], "getent")
			}
		}
	}
	return detected(getenv("SHELL"), "$SHELL")
}

func detected(path, source string) Detected {
	return Detected{Path: path, Kind: KindOf(path), Source: source}
}

// KindOf returns the shell kind of a shell executable path. A leading "-"
// (login shells in ps output) is ignored.
func KindOf(path string) Kind {
	name := strings.TrimPrefix(filepath.Base(path), "-")
	for _, k := range All {
		if name == string(k) {
			return k
		}
	}
	return Unknown
}

// Parse turns a shell name given by the user into a Kind.
func Parse(name string) (Kind, error) {
	if k := KindOf(name); k != Unknown {
		return k, nil
	}
	return Unknown, i18n.Errorf("unsupported shell %q (supported: fish, zsh, bash)", name)
}

// maxAncestors bounds the walk up the process tree.
const maxAncestors = 16

// CurrentShell returns the shell the user is running right now: the nearest
// ancestor of process pid that is a supported shell. It can differ from the
// login shell, for example when zsh is started from fish.
func CurrentShell(ctx context.Context, pid int, run Runner) Detected {
	ppid, _, ok := processInfo(ctx, pid, run)
	for range maxAncestors {
		if !ok || ppid <= 1 {
			return Detected{}
		}
		// One ps per process: the parent's answer also names its parent.
		grandparent, comm, found := processInfo(ctx, ppid, run)
		if found && KindOf(comm) != Unknown {
			return Detected{Path: comm, Kind: KindOf(comm), Source: "process " + strconv.Itoa(ppid)}
		}
		ppid, ok = grandparent, found
	}
	return Detected{}
}

// processInfo returns the parent pid and command of a process.
func processInfo(ctx context.Context, pid int, run Runner) (int, string, bool) {
	out, err := run(ctx, "ps", "-o", "ppid=,comm=", "-p", strconv.Itoa(pid))
	if err != nil {
		return 0, "", false
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return 0, "", false
	}
	ppid, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, "", false
	}
	return ppid, strings.Join(fields[1:], " "), true
}

// Installed is a supported shell present on the system.
type Installed struct {
	Kind Kind
	Path string
}

// InstalledShells returns the supported shells listed in /etc/shells whose
// executable exists, one per kind, in the order of All.
func InstalledShells(etcShells string, exists func(string) bool) []Installed {
	found := map[Kind]string{}
	for _, line := range strings.Split(etcShells, "\n") {
		path := strings.TrimSpace(line)
		if path == "" || strings.HasPrefix(path, "#") {
			continue
		}
		if k := KindOf(path); k != Unknown && found[k] == "" && exists(path) {
			found[k] = path
		}
	}
	var out []Installed
	for _, k := range All {
		if p, ok := found[k]; ok {
			out = append(out, Installed{Kind: k, Path: p})
		}
	}
	return out
}
