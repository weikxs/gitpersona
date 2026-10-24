package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/weikxs/gitpersona/internal/resolve"
)

// VerifyOptions controls Verify.
type VerifyOptions struct {
	Dirs []string // extra folders to check besides the profiles' folders and pins
	JSON bool     // print a VerifyReport
	All  bool     // also list repositories that are as expected
	// SSH asks each SSH host which account a push signs in as (it connects).
	SSH bool
}

// errVerifyProblems is returned when a repository would commit or push as
// someone else; the report already says which.
var errVerifyProblems = errors.New("some repositories would commit or push as another identity; see above")

// Verify checks every repository: who commits and who pushes there.
func Verify(ctx context.Context, e Env, opts VerifyOptions) error {
	report, err := e.VerifyRepos(ctx, opts.Dirs, opts.SSH)
	if err != nil {
		return err
	}
	if opts.JSON {
		enc := json.NewEncoder(e.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return err
		}
	} else {
		printVerify(e, report, opts.All)
	}
	if report.Counts.Problems > 0 {
		return errVerifyProblems
	}
	return nil
}

var verdictMarks = map[string]string{VerdictOK: "✓", VerdictNote: "·", VerdictProblem: "✗", VerdictUnconfirmed: "?", VerdictUnmanaged: "-"}

func printVerify(e Env, r VerifyReport, all bool) {
	for _, c := range r.Repositories {
		if !all && c.Verdict == VerdictOK {
			continue
		}
		printRepoCheck(e, c)
	}
	if len(r.Missing) > 0 {
		fmt.Fprintf(e.Stdout, "not found: %s\n", strings.Join(r.Missing, ", "))
	}
	if !r.Complete {
		fmt.Fprintln(e.Stdout, "the scan stopped early or did not go deep enough; some repositories may be missing")
	}
	c := r.Counts
	fmt.Fprintf(e.Stdout, "%d repositories in %s: %d as expected, %d with notes, %d with problems, %d unconfirmed, %d outside your profiles\n",
		len(r.Repositories), strings.Join(r.Scanned, ", "), c.OK, c.Notes, c.Problems, c.Unconfirmed, c.Unmanaged)
	if c.Unconfirmed > 0 {
		fmt.Fprintln(e.Stdout, "(`gitpersona verify --ssh` asks the SSH hosts which account the unconfirmed ones push as)")
	}
	if !all && c.OK > 0 {
		fmt.Fprintln(e.Stdout, "(`gitpersona verify --all` lists every repository; `gitpersona ui` shows them on a page)")
	}
}

func printRepoCheck(e Env, c RepoCheck) {
	where := "no profile: Git uses your own settings"
	if c.Profile != "" {
		where = fmt.Sprintf("%s · %s", c.Profile, reasonWithMatch(c.Reason, c.Match))
	}
	fmt.Fprintf(e.Stdout, "%s %s  (%s)\n", verdictMarks[c.Verdict], c.Path, where)
	fmt.Fprintf(e.Stdout, "    author  %s\n", orNothing(c.Author))
	for _, rc := range c.Remotes {
		as := rc.PushAs
		if as == "" {
			as = "an account GitPersona cannot tell"
		}
		fmt.Fprintf(e.Stdout, "    push    %s → %s as %s (%s)\n", rc.Name, rc.Address, as, rc.Via)
	}
	for _, f := range c.Findings {
		fmt.Fprintf(e.Stdout, "    %-7s %s\n", f.Level, f.Text)
	}
}

func reasonWithMatch(reason, match string) string {
	text := resolve.Reason(reason).Describe()
	if match != "" {
		text += " " + match
	}
	return text
}
