package config

import (
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/weikxs/gitpersona/internal/paths"
)

var (
	idPattern       = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
	accountPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)
	hostPattern     = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?(:[0-9]{1,5})?$`)
	validProviders  = []string{ProviderGitHub, ProviderGitLab, ProviderBitbucket, ProviderGeneric}
	validSigningSet = []string{"", SigningNone, SigningSSH, SigningGPG}
)

// Validate checks the whole configuration and reports every problem found,
// joined into one error. home expands "~"; caseInsensitive controls whether
// directories differing only in case count as the same directory.
func (c Config) Validate(home string, caseInsensitive bool) error {
	var errs []error
	switch c.Version {
	case CurrentVersion:
	case 0:
		errs = append(errs, fmt.Errorf("version is missing: add `version = %d` at the top of the file", CurrentVersion))
	default:
		errs = append(errs, fmt.Errorf("unsupported config version %d (expected %d)", c.Version, CurrentVersion))
	}
	if len(c.Profiles) == 0 {
		errs = append(errs, errors.New("no profiles are defined: add at least one [[profile]]"))
	}
	if c.Default != "" {
		if _, ok := c.ProfileByID(c.Default); !ok {
			errs = append(errs, fmt.Errorf("default profile %q does not exist", c.Default))
		}
	}

	seenIDs := map[string]bool{}
	for _, p := range c.Profiles {
		if seenIDs[p.ID] {
			errs = append(errs, fmt.Errorf("duplicate profile id %q", p.ID))
		}
		seenIDs[p.ID] = true
		errs = append(errs, validateProfile(p, home)...)
	}

	errs = append(errs, validateDirOwnership(c.Profiles, home, caseInsensitive)...)
	errs = append(errs, c.validatePins(home, caseInsensitive)...)
	return errors.Join(errs...)
}

func validateProfile(p Profile, home string) []error {
	var errs []error
	add := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("profile %q: "+format, append([]any{p.ID}, args...)...))
	}

	if !idPattern.MatchString(p.ID) {
		add("id must be 1-32 lowercase letters, digits or hyphens, starting with a letter or digit")
	}
	if !slices.Contains(validProviders, p.Provider) {
		add("unsupported provider %q (use one of %s)", p.Provider, strings.Join(validProviders, ", "))
	}
	switch {
	case p.Host == "":
		add("host is required")
	case !hostPattern.MatchString(p.Host):
		add("host %q is not a valid host name", p.Host)
	}
	switch {
	case p.Account == "":
		add("account is required")
	case !accountPattern.MatchString(p.Account):
		add("account %q may only contain letters, digits, '.', '_' and '-'", p.Account)
	}
	for _, f := range []struct{ name, value string }{
		{"name", p.Name}, {"email", p.Email}, {"ssh_key", p.SSHKey}, {"signing key", p.Signing.Key},
	} {
		if hasControl(f.value) {
			add("%s contains a control character such as a line break", f.name)
		}
	}
	if p.Name == "" {
		add("name is required")
	}
	if _, err := mail.ParseAddress(p.Email); err != nil || !strings.Contains(p.Email, "@") {
		add("email %q is not valid", p.Email)
	}
	if !slices.Contains(validSigningSet, p.Signing.Method) {
		add("unsupported signing method %q (use none, ssh or gpg)", p.Signing.Method)
	}
	if p.Signing.Enabled() && p.Signing.Key == "" {
		add("signing key is required when signing method is %q", p.Signing.Method)
	}
	if p.SSHKey != "" {
		if _, err := paths.Normalize(p.SSHKey, home); err != nil {
			add("ssh_key: %v", err)
		}
	}
	// An SSH signing key is a file; a GPG signing key is a key ID, not a path.
	if p.Signing.Method == SigningSSH && p.Signing.Key != "" {
		if _, err := paths.Normalize(p.Signing.Key, home); err != nil {
			add("signing key: %v", err)
		}
	}
	seen := map[string]bool{}
	for _, d := range p.Dirs {
		norm, err := paths.Normalize(d, home)
		switch {
		case hasControl(d):
			add("directory contains a control character such as a line break")
		case err != nil:
			add("directory: %v", err)
		case norm == "/":
			add("directory %q is the root directory; use the default profile instead", d)
		case seen[norm]:
			add("directory %q is listed twice", d)
		}
		seen[norm] = true
	}
	return errs
}

// hasControl reports whether s contains a control character. Such characters
// could break out of a Git configuration value.
func hasControl(s string) bool {
	return strings.ContainsFunc(s, unicode.IsControl)
}

// validateDirOwnership enforces that a directory belongs to one profile only.
func validateDirOwnership(profiles []Profile, home string, caseInsensitive bool) []error {
	var errs []error
	owners := map[string]string{}
	for _, p := range profiles {
		for _, d := range p.Dirs {
			norm, err := paths.Normalize(d, home)
			if err != nil {
				continue // reported by validateProfile
			}
			key := comparisonKey(norm, caseInsensitive)
			if owner, taken := owners[key]; taken && owner != p.ID {
				errs = append(errs, fmt.Errorf("directory %s is assigned to both %q and %q", d, owner, p.ID))
				continue
			}
			owners[key] = p.ID
		}
	}
	return errs
}

func (c Config) validatePins(home string, caseInsensitive bool) []error {
	var errs []error
	seen := map[string]bool{}
	for _, pin := range c.Pins {
		norm, err := paths.Normalize(pin.Repo, home)
		if hasControl(pin.Repo) {
			err = errors.New("path contains a control character such as a line break")
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("pin: %w", err))
			continue
		}
		if _, ok := c.ProfileByID(pin.Profile); !ok {
			errs = append(errs, fmt.Errorf("pin %q: profile %q does not exist", pin.Repo, pin.Profile))
		}
		key := comparisonKey(norm, caseInsensitive)
		if seen[key] {
			errs = append(errs, fmt.Errorf("pin %q: repository is pinned more than once", pin.Repo))
		}
		seen[key] = true
	}
	return errs
}

func comparisonKey(normalized string, caseInsensitive bool) string {
	if caseInsensitive {
		return strings.ToLower(normalized)
	}
	return normalized
}
