// Package version reports build information injected at link time.
package version

import "fmt"

// Overridden by the release build via -ldflags "-X ...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// Info describes one build of gitpersona.
type Info struct {
	Version string
	Commit  string
	Date    string
}

// Current returns the build information of the running binary.
func Current() Info {
	return Info{Version: version, Commit: commit, Date: date}
}

func (i Info) String() string {
	return fmt.Sprintf("gitpersona %s (commit %s, built %s)", i.Version, i.Commit, i.Date)
}
