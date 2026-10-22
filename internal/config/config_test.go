package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const home = "/tmp/home"

func TestLoadParsesAndAppliesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join("testdata", "valid.toml"))
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Version != CurrentVersion || cfg.Default != "acme" || len(cfg.Profiles) != 3 || len(cfg.Pins) != 1 {
		t.Fatalf("Load() = %+v, want version 1, default acme, 3 profiles, 1 pin", cfg)
	}
	personal := cfg.Profiles[0]
	if personal.Provider != ProviderGitHub || personal.Host != DefaultGitHubHost {
		t.Errorf("personal provider/host = %q/%q, want the GitHub defaults", personal.Provider, personal.Host)
	}
	if personal.Signing.Method != SigningSSH || personal.Signing.Key != "~/.ssh/id_ed25519_personal.pub" {
		t.Errorf("personal signing = %+v", personal.Signing)
	}
	if !reflect.DeepEqual(personal.Dirs, []string{"~/Projects/personal", "~/Projects/acme/oss"}) {
		t.Errorf("personal dirs = %v", personal.Dirs)
	}
	if cfg.Profiles[2].SSHKey != "~/.ssh/id_ed25519_globex" {
		t.Errorf("globex ssh_key = %q", cfg.Profiles[2].SSHKey)
	}
	if err := cfg.Validate(home, false); err != nil {
		t.Errorf("Validate() on the sample config: %v", err)
	}
}

func TestLoadMissingFileReturnsErrNotFound(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "absent.toml"))

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Load() error = %v, want ErrNotFound", err)
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	path := writeFile(t, "version = 1\n[[profile]]\nid = \"a\"\nemial = \"typo@example.com\"\n")

	_, err := Load(path)

	if err == nil || !strings.Contains(err.Error(), "emial") {
		t.Fatalf("Load() error = %v, want it to name the unknown key", err)
	}
}

func TestLoadRejectsInvalidTOML(t *testing.T) {
	path := writeFile(t, "version = = 1\n")

	if _, err := Load(path); err == nil {
		t.Fatal("Load() of invalid TOML succeeded, want an error")
	}
}

func TestSaveRoundTripsWithPrivateMode(t *testing.T) {
	original, err := Load(filepath.Join("testdata", "valid.toml"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "gitpersona", "config.toml")

	if err := Save(path, original); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() after Save() error: %v", err)
	}

	if !reflect.DeepEqual(original, reloaded) {
		t.Fatalf("round trip changed the config:\n got  %+v\n want %+v", reloaded, original)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o, want 600", info.Mode().Perm())
	}
}

func TestValidateReportsEveryProblem(t *testing.T) {
	cfg := Config{
		Version: 2,
		Default: "missing",
		Profiles: []Profile{
			{ID: "Bad ID", Provider: "svn", Host: "", Account: "", Name: "", Email: "not-an-email"},
			{ID: "dup", Provider: ProviderGitHub, Host: DefaultGitHubHost, Account: "a", Name: "A", Email: "a@example.com",
				Signing: Signing{Method: "pgp"}, Dirs: []string{"relative/dir"}},
			{ID: "dup", Provider: ProviderGitHub, Host: DefaultGitHubHost, Account: "b", Name: "B", Email: "b@example.com",
				Signing: Signing{Method: SigningGPG}},
		},
		Pins: []Pin{{Repo: "~/r", Profile: "nobody"}, {Repo: "", Profile: "dup"}},
	}

	err := cfg.Validate(home, false)

	if err == nil {
		t.Fatal("Validate() = nil, want errors")
	}
	for _, want := range []string{
		"unsupported config version 2",
		`default profile "missing" does not exist`,
		`profile "Bad ID": id must`,
		`unsupported provider "svn"`,
		"host is required",
		"account is required",
		"name is required",
		`email "not-an-email" is not valid`,
		`unsupported signing method "pgp"`,
		"must be absolute or start with ~/",
		`duplicate profile id "dup"`,
		"signing key is required",
		`pin "~/r": profile "nobody" does not exist`,
		"pin: path is empty",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Validate() error is missing %q\nfull error:\n%v", want, err)
		}
	}
}

func TestValidateRejectsSameDirectoryInTwoProfiles(t *testing.T) {
	cfg := Config{
		Version: CurrentVersion,
		Profiles: []Profile{
			validProfile("personal", "~/Projects/shared"),
			validProfile("acme", "~/Projects/Shared/"),
		},
	}

	sensitive := cfg.Validate(home, false)
	insensitive := cfg.Validate(home, true)

	if sensitive != nil {
		t.Errorf("case-sensitive Validate() = %v, want nil (different directories)", sensitive)
	}
	if insensitive == nil || !strings.Contains(insensitive.Error(), "is assigned to both") {
		t.Errorf("case-insensitive Validate() = %v, want a duplicate directory error", insensitive)
	}
}

func TestValidateRejectsDuplicatePins(t *testing.T) {
	cfg := Config{
		Version:  CurrentVersion,
		Profiles: []Profile{validProfile("personal")},
		Pins:     []Pin{{Repo: "~/r", Profile: "personal"}, {Repo: "~/r/", Profile: "personal"}},
	}

	err := cfg.Validate(home, false)

	if err == nil || !strings.Contains(err.Error(), "pinned more than once") {
		t.Fatalf("Validate() = %v, want a duplicate pin error", err)
	}
}

func TestProfileByID(t *testing.T) {
	cfg := Config{Profiles: []Profile{validProfile("personal"), validProfile("acme")}}

	got, ok := cfg.ProfileByID("acme")
	_, missing := cfg.ProfileByID("globex")

	if !ok || got.ID != "acme" || missing {
		t.Fatalf("ProfileByID() = %+v, %v; missing lookup found = %v", got, ok, missing)
	}
}

func validProfile(id string, dirs ...string) Profile {
	return Profile{
		ID: id, Provider: ProviderGitHub, Host: DefaultGitHubHost,
		Account: id + "-alex", Name: "Alex", Email: id + "@example.com", Dirs: dirs,
	}
}

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
