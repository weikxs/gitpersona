package config

import (
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"slices"
	"strings"

	"github.com/weikxs/gitpersona/internal/paths"
)

var (
	idPattern       = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
	validProviders  = []string{ProviderGitHub, ProviderGitLab, ProviderBitbucket, ProviderGeneric}
	validSigningSet = []string{"", SigningNone, SigningSSH, SigningGPG}
)

// Validate checks the whole configuration and reports every problem found,
// joined into one error. home expands "~"; caseInsensitive controls whether
// directories differing only in case count as the same directory.
func (c Config) Validate(home string, caseInsensitive bool) error {
	var errs []error
	if c.Version != CurrentVersion {
		errs = append(errs, fmt.Errorf("unsupported config version %d (expected %d)", c.Version, CurrentVersion))
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
	if p.Host == "" {
		add("host is required")
	}
	if p.Account == "" {
		add("account is required")
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
	for _, d := range p.Dirs {
		if _, err := paths.Normalize(d, home); err != nil {
			add("directory: %v", err)
		}
	}
	return errs
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
