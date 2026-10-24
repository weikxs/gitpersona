package app

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/weikxs/gitpersona/internal/shell"
)

// ShellsReport is the stable --json output of `gitpersona shells`.
type ShellsReport struct {
	Default   DetectedShell    `json:"default"`
	Current   DetectedShell    `json:"current"`
	Installed []InstalledShell `json:"installed"`
}

// DetectedShell is a shell and how it was detected.
type DetectedShell struct {
	Kind   string `json:"kind,omitempty"`
	Path   string `json:"path,omitempty"`
	Source string `json:"source,omitempty"`
}

// InstalledShell is a supported shell on the system.
type InstalledShell struct {
	Kind       string `json:"kind"`
	Path       string `json:"path"`
	Integrated bool   `json:"integrated"`
}

// Shells reports the default (login) shell, the shell in use right now, and
// every supported shell installed, with its integration status.
func Shells(ctx context.Context, e Env, asJSON bool) error {
	report := e.shellsReport(ctx)
	if asJSON {
		enc := json.NewEncoder(e.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	fmt.Fprintln(e.Stdout, e.tr("default  %s", e.describe(report.Default, e.tr("your login shell"))))
	fmt.Fprintln(e.Stdout, e.tr("current  %s", e.describe(report.Current, e.tr("the shell running this command"))))
	fmt.Fprintln(e.Stdout, "\n"+e.tr("installed shells:"))
	for _, in := range report.Installed {
		status := e.tr("integration not installed")
		if in.Integrated {
			status = e.tr("integration installed")
		}
		fmt.Fprintf(e.Stdout, "  %-4s  %s  %s\n", in.Kind, in.Path, status)
	}
	return nil
}

func (e Env) shellsReport(ctx context.Context) ShellsReport {
	report := ShellsReport{
		Default:   detectedShell(e.loginShell(ctx)),
		Current:   detectedShell(e.currentShell(ctx)),
		Installed: []InstalledShell{},
	}
	for _, in := range e.installedShells() {
		_, ok := e.integrated(in.Kind)
		report.Installed = append(report.Installed, InstalledShell{Kind: string(in.Kind), Path: in.Path, Integrated: ok})
	}
	return report
}

func detectedShell(d shell.Detected) DetectedShell {
	return DetectedShell{Kind: string(d.Kind), Path: d.Path, Source: d.Source}
}

func (e Env) describe(d DetectedShell, role string) string {
	if d.Kind == "" {
		return e.tr("unknown")
	}
	return e.tr("%s  %s (%s, from %s)", d.Kind, d.Path, role, d.Source)
}
