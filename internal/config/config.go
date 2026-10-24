// Package config defines the GitPersona configuration file and reads and
// writes it. The file is the single source of truth; everything else is
// generated from it.
package config

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/weikxs/gitpersona/internal/fsutil"
	"github.com/weikxs/gitpersona/internal/i18n"
)

// CurrentVersion is the only configuration schema version this build reads.
const CurrentVersion = 1

// Providers.
const (
	ProviderGitHub    = "github"
	ProviderGitLab    = "gitlab"
	ProviderBitbucket = "bitbucket"
	ProviderGeneric   = "generic"
)

// DefaultGitHubHost is used when a GitHub profile does not name a host.
const DefaultGitHubHost = "github.com"

// Signing methods.
const (
	SigningNone = "none"
	SigningSSH  = "ssh"
	SigningGPG  = "gpg"
)

// ErrNotFound is returned by Load when the configuration file does not exist.
var ErrNotFound = i18n.Errorf("configuration file not found")

// Config is the whole configuration file.
type Config struct {
	Version int    `toml:"version"`
	Default string `toml:"default,omitempty"`
	// ShellIntegration false records that the user chose not to load
	// GitPersona in their shells, so doctor does not suggest it.
	ShellIntegration *bool     `toml:"shell_integration,omitempty"`
	Profiles         []Profile `toml:"profile,omitempty"`
	Pins             []Pin     `toml:"pin,omitempty"`
}

// Profile is one identity on one Git hosting service.
type Profile struct {
	ID       string   `toml:"id"`
	Provider string   `toml:"provider,omitempty"`
	Host     string   `toml:"host,omitempty"`
	Account  string   `toml:"account"`
	Name     string   `toml:"name"`
	Email    string   `toml:"email"`
	SSHKey   string   `toml:"ssh_key,omitempty"`
	Signing  Signing  `toml:"signing,omitempty"`
	Dirs     []string `toml:"dirs,omitempty"`
}

// Signing configures commit signing for a profile.
type Signing struct {
	Method string `toml:"method,omitempty" json:"method"`
	Key    string `toml:"key,omitempty" json:"key,omitempty"`
}

// Pin binds one repository to a profile, overriding directory rules.
type Pin struct {
	Repo    string `toml:"repo"`
	Profile string `toml:"profile"`
}

// Enabled reports whether commits should be signed.
func (s Signing) Enabled() bool {
	return s.Method == SigningSSH || s.Method == SigningGPG
}

// ShellIntegrationOff reports whether the user turned the shell
// integration off.
func (c Config) ShellIntegrationOff() bool {
	return c.ShellIntegration != nil && !*c.ShellIntegration
}

// ProfileByID returns the profile with the given id.
func (c Config) ProfileByID(id string) (Profile, bool) {
	i := slices.IndexFunc(c.Profiles, func(p Profile) bool { return p.ID == id })
	if i < 0 {
		return Profile{}, false
	}
	return c.Profiles[i], true
}

// Load reads and decodes the configuration file at path and fills in
// defaults. It rejects unknown keys so that typos are not silently ignored.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, i18n.Errorf("%w: %s", ErrNotFound, path)
	}
	if err != nil {
		return Config{}, i18n.Errorf("read %s: %w", path, err)
	}

	cfg, err := Parse(data)
	if err != nil {
		return Config{}, i18n.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// Parse decodes configuration text and fills in defaults. It rejects unknown
// keys so that typos are not silently ignored.
func Parse(data []byte) (Config, error) {
	var cfg Config
	meta, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return Config{}, err
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		return Config{}, i18n.Errorf("unknown keys: %s", strings.Join(keys, ", "))
	}
	return withDefaults(cfg), nil
}

// Save encodes cfg and writes it atomically to path with private permissions.
func Save(path string, cfg Config) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return i18n.Errorf("encode configuration: %w", err)
	}
	return fsutil.WriteAtomic(path, buf.Bytes(), fsutil.FilePerm)
}

func withDefaults(cfg Config) Config {
	profiles := make([]Profile, len(cfg.Profiles))
	for i, p := range cfg.Profiles {
		if p.Provider == "" {
			p.Provider = ProviderGitHub
		}
		if p.Host == "" && p.Provider == ProviderGitHub {
			p.Host = DefaultGitHubHost
		}
		profiles[i] = p
	}
	cfg.Profiles = profiles
	return cfg
}
