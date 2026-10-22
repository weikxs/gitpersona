package app

import (
	"context"
	"fmt"

	"github.com/weikxs/gitpersona/internal/config"
)

// ProfileChanges lists the fields to change; nil leaves a field as it is and
// an empty string removes an optional one.
type ProfileChanges struct {
	Account *string
	Name    *string
	Email   *string
	SSHKey  *string
	Signing *config.Signing
}

// AddProfile adds a profile, keeping comments, and applies it.
func AddProfile(ctx context.Context, e Env, p config.Profile) error {
	text, cfg, err := e.readConfigText()
	if err != nil {
		return err
	}
	if err := requireKeyFiles(p, e.Home); err != nil {
		return err
	}
	edited, err := config.AddProfile(text, p)
	if err != nil {
		return err
	}
	want := cfg
	want.Profiles = append(append([]config.Profile{}, cfg.Profiles...), withProfileDefaults(p))
	return e.saveEdited(ctx, "add profile "+p.ID, edited, want)
}

// UpdateProfile changes fields of a profile and applies it.
func UpdateProfile(ctx context.Context, e Env, id string, c ProfileChanges) error {
	text, cfg, err := e.readConfigText()
	if err != nil {
		return err
	}
	if _, ok := cfg.ProfileByID(id); !ok {
		return fmt.Errorf("unknown profile %q; choose one of: %s", id, profileIDs(cfg))
	}
	edited, want := text, cfg
	for key, value := range map[string]*string{"account": c.Account, "name": c.Name, "email": c.Email, "ssh_key": c.SSHKey} {
		if value == nil {
			continue
		}
		if edited, err = config.SetProfileField(edited, id, key, *value); err != nil {
			return err
		}
		want = withField(want, id, key, *value)
	}
	if c.Signing != nil {
		if edited, err = config.SetProfileSigning(edited, id, *c.Signing); err != nil {
			return err
		}
		want = withProfile(want, id, func(p *config.Profile) { p.Signing = *c.Signing })
	}
	if c.SSHKey != nil || c.Signing != nil {
		if p, ok := want.ProfileByID(id); ok {
			if err := requireKeyFiles(p, e.Home); err != nil {
				return err
			}
		}
	}
	return e.saveEdited(ctx, "update profile "+id, edited, want)
}

// RemoveProfile deletes a profile together with its pins (and the default,
// if it was the default), and applies the change.
func RemoveProfile(ctx context.Context, e Env, id string) error {
	text, cfg, err := e.readConfigText()
	if err != nil {
		return err
	}
	edited, err := config.RemoveProfile(text, id)
	if err != nil {
		return err
	}
	want := cfg
	want.Profiles = nil
	for _, p := range cfg.Profiles {
		if p.ID != id {
			want.Profiles = append(want.Profiles, p)
		}
	}
	for _, pin := range cfg.Pins {
		if pin.Profile != id {
			continue
		}
		if edited, err = config.RemovePin(edited, pin.Repo, e.editOptions()); err != nil {
			return err
		}
		want = withoutPin(want, pin.Repo, e.editOptions())
	}
	if cfg.Default == id {
		if edited, err = config.SetDefault(edited, ""); err != nil {
			return err
		}
		want.Default = ""
	}
	return e.saveEdited(ctx, "remove profile "+id, edited, want)
}

// SetDefaultProfile sets (or with "" clears) the default profile.
func SetDefaultProfile(ctx context.Context, e Env, id string) error {
	text, cfg, err := e.readConfigText()
	if err != nil {
		return err
	}
	if id != "" {
		if err := knownProfile(cfg, id, "default"); err != nil {
			return err
		}
	}
	edited, err := config.SetDefault(text, id)
	if err != nil {
		return err
	}
	want := cfg
	want.Default = id
	return e.saveEdited(ctx, "set default "+id, edited, want)
}

func withProfileDefaults(p config.Profile) config.Profile {
	if p.Provider == "" {
		p.Provider = config.ProviderGitHub
	}
	if p.Host == "" && p.Provider == config.ProviderGitHub {
		p.Host = config.DefaultGitHubHost
	}
	return p
}

func withField(cfg config.Config, id, key, value string) config.Config {
	return withProfile(cfg, id, func(p *config.Profile) {
		switch key {
		case "account":
			p.Account = value
		case "name":
			p.Name = value
		case "email":
			p.Email = value
		case "ssh_key":
			p.SSHKey = value
		}
	})
}

func withProfile(cfg config.Config, id string, change func(*config.Profile)) config.Config {
	out := cfg
	out.Profiles = append([]config.Profile{}, cfg.Profiles...)
	for i := range out.Profiles {
		if out.Profiles[i].ID == id {
			change(&out.Profiles[i])
		}
	}
	return out
}
