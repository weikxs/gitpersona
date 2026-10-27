package app

import (
	"context"
	"fmt"
	"strings"
)

// gitVersion is a Git release, such as 2.34.1.
type gitVersion struct{ major, minor, patch int }

var (
	// minGit is the oldest Git GitPersona supports.
	minGit = gitVersion{2, 31, 0}
	// remoteConditionsGit added includeIf "hasconfig:remote.*.url:…"; older
	// Git ignores such rules.
	remoteConditionsGit = gitVersion{2, 36, 0}
)

func (v gitVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.patch)
}

// atLeast reports whether v is other or newer.
func (v gitVersion) atLeast(other gitVersion) bool {
	if v.major != other.major {
		return v.major > other.major
	}
	if v.minor != other.minor {
		return v.minor > other.minor
	}
	return v.patch >= other.patch
}

// parseGitVersion reads `git version` output: "git version 2.34.1", and the
// forms vendors add to it ("2.39.5 (Apple Git-154)", "2.45.2.windows.1").
func parseGitVersion(out string) (gitVersion, bool) {
	fields := strings.Fields(out)
	if len(fields) < 3 || fields[0] != "git" || fields[1] != "version" {
		return gitVersion{}, false
	}
	var v gitVersion
	n, _ := fmt.Sscanf(fields[2], "%d.%d.%d", &v.major, &v.minor, &v.patch)
	if n < 2 {
		return gitVersion{}, false
	}
	return v, true
}

// gitVersion asks Git for its version; false when it cannot be told.
func (e Env) gitVersion(ctx context.Context) (gitVersion, bool) {
	if e.probes != nil {
		return e.probes.gitVersion(ctx)
	}
	out, err := e.Run(ctx, "git", "version")
	if err != nil {
		return gitVersion{}, false
	}
	return parseGitVersion(string(out))
}

// checkGitVersion reports the Git version, and fails below the oldest one
// GitPersona supports.
func (e Env) checkGitVersion(ctx context.Context) check {
	v, ok := e.gitVersion(ctx)
	switch {
	case !ok:
		return check{warn: true, label: e.tr("could not tell which version of Git this is"),
			advice: e.tr("run `git version`; GitPersona needs Git %s or newer", minGit.short())}
	case !v.atLeast(minGit):
		return check{label: e.tr("Git %s is older than %s, the oldest GitPersona supports", v, minGit.short()),
			advice: e.tr("update Git (for example with your package manager or Homebrew)")}
	}
	return check{ok: true, label: e.tr("Git %s", v)}
}

// short is the version without the patch number ("2.36").
func (v gitVersion) short() string {
	return fmt.Sprintf("%d.%d", v.major, v.minor)
}
