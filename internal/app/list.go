package app

import (
	"context"
	"encoding/json"
	"fmt"
)

// ListReport is the stable --json output of `gitpersona list`.
type ListReport struct {
	Default  string          `json:"default,omitempty"`
	Profiles []ListedProfile `json:"profiles"`
	Pins     []ListedPin     `json:"pins"`
}

// ListedProfile is one profile in ListReport.
type ListedProfile struct {
	ID       string   `json:"id"`
	Provider string   `json:"provider"`
	Host     string   `json:"host"`
	Account  string   `json:"account"`
	Name     string   `json:"name"`
	Email    string   `json:"email"`
	Dirs     []string `json:"dirs"`
}

// ListedPin is one pin in ListReport.
type ListedPin struct {
	Repo    string `json:"repo"`
	Profile string `json:"profile"`
}

// List prints every profile with its directories, and the pins.
func List(_ context.Context, e Env, asJSON bool) error {
	cfg, err := e.loadConfig()
	if err != nil {
		return err
	}
	report := ListReport{Default: cfg.Default, Profiles: []ListedProfile{}, Pins: []ListedPin{}}
	for _, p := range cfg.Profiles {
		dirs := append([]string{}, p.Dirs...)
		report.Profiles = append(report.Profiles, ListedProfile{ID: p.ID, Provider: p.Provider, Host: p.Host,
			Account: p.Account, Name: p.Name, Email: p.Email, Dirs: dirs})
	}
	for _, p := range cfg.Pins {
		report.Pins = append(report.Pins, ListedPin{Repo: p.Repo, Profile: p.Profile})
	}

	if asJSON {
		enc := json.NewEncoder(e.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	for _, p := range report.Profiles {
		label := p.ID
		if p.ID == report.Default {
			label += " (default)"
		}
		fmt.Fprintf(e.Stdout, "%s  %s on %s <%s>\n", label, p.Account, p.Host, p.Email)
		for _, d := range p.Dirs {
			fmt.Fprintf(e.Stdout, "  dir %s\n", d)
		}
		for _, pin := range report.Pins {
			if pin.Profile == p.ID {
				fmt.Fprintf(e.Stdout, "  pin %s\n", pin.Repo)
			}
		}
	}
	return nil
}
