package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/weikxs/gitpersona/internal/i18n"
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
var errVerifyProblems = i18n.Errorf("some repositories would commit or push as another identity; see above")

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
		fmt.Fprintln(e.Stdout, e.tr("not found: %s", strings.Join(r.Missing, ", ")))
	}
	if !r.Complete {
		fmt.Fprintln(e.Stdout, e.tr("the scan stopped early or did not go deep enough; some repositories may be missing"))
	}
	c := r.Counts
	fmt.Fprintln(e.Stdout, e.tr("%d repositories in %s: %d as expected, %d with notes, %d with problems, %d unconfirmed, %d outside your profiles",
		len(r.Repositories), strings.Join(r.Scanned, ", "), c.OK, c.Notes, c.Problems, c.Unconfirmed, c.Unmanaged))
	if c.Unconfirmed > 0 {
		fmt.Fprintln(e.Stdout, e.tr("(`gitpersona verify --ssh` asks the SSH hosts which account the unconfirmed ones push as)"))
	}
	if !all && c.OK > 0 {
		fmt.Fprintln(e.Stdout, e.tr("(`gitpersona verify --all` lists every repository; `gitpersona ui` shows them on a page)"))
	}
}

func printRepoCheck(e Env, c RepoCheck) {
	where := e.tr("no profile: Git uses your own settings")
	if c.Profile != "" {
		where = e.tr("%s · %s", c.Profile, e.reasonWithMatch(c.Reason, c.Match))
	}
	fmt.Fprintf(e.Stdout, "%s %s  (%s)\n", verdictMarks[c.Verdict], c.Path, where)
	fmt.Fprintln(e.Stdout, e.tr("    author  %s", e.orNothing(c.Author)))
	for _, rc := range c.Remotes {
		as := rc.PushAs
		if as == "" {
			as = e.tr("an account GitPersona cannot tell")
		}
		fmt.Fprintln(e.Stdout, e.tr("    push    %s → %s as %s (%s)", rc.Name, rc.Address, as, rc.Via))
	}
	for _, f := range c.Findings {
		fmt.Fprintf(e.Stdout, "    %-7s %s\n", f.Level, f.Text)
	}
}

// reasonWithMatch says why a profile applies (a reason code of resolve) and,
// for a rule or a pin, which one.
func (e Env) reasonWithMatch(reason, match string) string {
	text := e.describeReason(reason)
	if match != "" {
		text += " " + match
	}
	return text
}

// describeReason is resolve.Reason.Describe in e's language.
func (e Env) describeReason(reason string) string {
	switch resolve.Reason(reason) {
	case resolve.ReasonPin:
		return e.tr("pinned repository")
	case resolve.ReasonRule:
		return e.tr("directory rule")
	case resolve.ReasonDefault:
		return e.tr("default profile")
	default:
		return e.tr("no rule matches")
	}
}
