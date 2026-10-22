package config

import (
	"strings"
	"testing"
)

var editOpts = EditOptions{Home: home}

const commented = `# My GitPersona setup
version = 1
default = "acme" # work by default

[[profile]]
id      = "personal"   # open source
account = "octo-alex"
name    = "Alex Octo"
email   = "1000001+octo-alex@users.noreply.github.com"
dirs    = [
  "~/Projects/personal", # main
  "~/Projects/acme/oss",
]

[[profile]]
id      = "acme"
account = "acme-alex"
name    = "Alex (Acme)"
email   = "1000002+acme-alex@users.noreply.github.com"
dirs    = ["~/Projects/acme"]

[[profile]]
id      = "globex"
account = "globex-alex"
name    = "Alex"
email   = "alex@globex.example"
`

func TestAddDirToSingleLineArray(t *testing.T) {
	got, err := AddDir(commented, "acme", "~/Projects/acme-tools", editOpts)

	assertEdit(t, got, err, `dirs    = ["~/Projects/acme", "~/Projects/acme-tools"]`)
}

func TestAddDirToMultilineArrayKeepsComments(t *testing.T) {
	got, err := AddDir(commented, "personal", "~/side projects", editOpts)

	assertEdit(t, got, err, "  \"~/Projects/acme/oss\",\n  \"~/side projects\",\n]")
	if !strings.Contains(got, `"~/Projects/personal", # main`) || !strings.Contains(got, "# My GitPersona setup") {
		t.Fatalf("comments were lost:\n%s", got)
	}
}

func TestAddDirWithoutDirsKeyAddsOne(t *testing.T) {
	got, err := AddDir(commented, "globex", "~/Projects/clients/globex", editOpts)

	assertEdit(t, got, err, "id      = \"globex\"\ndirs = [\"~/Projects/clients/globex\"]\n")
}

func TestAddDirEscapesQuotesAndBackslashes(t *testing.T) {
	got, err := AddDir(commented, "acme", `~/odd "name" \dir`, editOpts)

	assertEdit(t, got, err, `"~/odd \"name\" \\dir"]`)
}

func TestAddDirUnknownProfile(t *testing.T) {
	if _, err := AddDir(commented, "ghost", "~/x", editOpts); err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("AddDir() to a missing profile = %v", err)
	}
}

func TestRemoveDirFromMultilineArrayDropsItsLine(t *testing.T) {
	got, owner, err := RemoveDir(commented, "/tmp/home/Projects/personal", editOpts)

	assertEdit(t, got, err, "dirs    = [\n  \"~/Projects/acme/oss\",\n]")
	if owner != "personal" || strings.Contains(got, "# main") {
		t.Fatalf("owner = %q; the removed entry's comment should go with it:\n%s", owner, got)
	}
}

func TestRemoveLastDirRemovesTheKey(t *testing.T) {
	got, owner, err := RemoveDir(commented, "~/Projects/acme/", editOpts)

	if err != nil || owner != "acme" {
		t.Fatalf("RemoveDir() = %v, owner %q", err, owner)
	}
	if strings.Contains(got, "~/Projects/acme\"") || strings.Contains(got, "dirs    = []") {
		t.Fatalf("empty dirs key should be removed:\n%s", got)
	}
	assertParses(t, got)
}

func TestRemoveDirFromSingleLineArrayWithSeveralEntries(t *testing.T) {
	text := strings.Replace(commented, `dirs    = ["~/Projects/acme"]`, `dirs    = ["~/a", "~/Projects/acme", "~/b"]`, 1)

	got, _, err := RemoveDir(text, "~/Projects/acme", editOpts)

	assertEdit(t, got, err, `dirs    = ["~/a", "~/b"]`)
}

func TestRemoveDirNotConfigured(t *testing.T) {
	if _, _, err := RemoveDir(commented, "~/nowhere", editOpts); err == nil {
		t.Fatal("RemoveDir() of an unknown directory succeeded")
	}
}

func TestAddAndRemovePin(t *testing.T) {
	withPin, err := AddPin(commented, "~/Downloads/globex-hotfix", "globex", editOpts)
	assertEdit(t, withPin, err, "[[pin]]\nrepo    = \"~/Downloads/globex-hotfix\"\nprofile = \"globex\"\n")

	repinned, err := AddPin(withPin, "/tmp/home/Downloads/globex-hotfix", "acme", editOpts)
	assertEdit(t, repinned, err, "profile = \"acme\"")
	if strings.Count(repinned, "[[pin]]") != 1 {
		t.Fatalf("re-pinning should update the existing pin:\n%s", repinned)
	}

	unpinned, err := RemovePin(repinned, "~/Downloads/globex-hotfix", editOpts)
	if err != nil || unpinned != commented {
		t.Fatalf("RemovePin() should restore the original text exactly (err=%v):\n%s", err, unpinned)
	}
	if _, err := RemovePin(commented, "~/Downloads/globex-hotfix", editOpts); err == nil {
		t.Fatal("RemovePin() of a repository that is not pinned succeeded")
	}
}

func TestParseMatchesLoad(t *testing.T) {
	cfg, err := Parse([]byte(commented))

	if err != nil || len(cfg.Profiles) != 3 || cfg.Profiles[0].Dirs[1] != "~/Projects/acme/oss" {
		t.Fatalf("Parse() = %+v, %v", cfg, err)
	}
}

func assertEdit(t *testing.T, got string, err error, want string) {
	t.Helper()
	if err != nil {
		t.Fatalf("edit error: %v", err)
	}
	if !strings.Contains(got, want) {
		t.Fatalf("edited text does not contain %q:\n%s", want, got)
	}
	assertParses(t, got)
}

func assertParses(t *testing.T, text string) {
	t.Helper()
	if _, err := Parse([]byte(text)); err != nil {
		t.Fatalf("edited text is not valid configuration: %v\n%s", err, text)
	}
}

// TestEditsMatchIntendedSemantics applies sequences of edits to several
// layouts and checks every step against the intended configuration.
func TestEditsMatchIntendedSemantics(t *testing.T) {
	layouts := map[string]string{
		"commented multiline":  commented,
		"compact inline":       strings.ReplaceAll(strings.ReplaceAll(commented, "dirs    = [\n  \"~/Projects/personal\", # main\n  \"~/Projects/acme/oss\",\n]", `dirs = ["~/Projects/personal","~/Projects/acme/oss"]`), "    ", " "),
		"bracket on last line": strings.Replace(commented, "  \"~/Projects/acme/oss\",\n]", "  \"~/Projects/acme/oss\"]", 1),
		"literal strings":      strings.Replace(commented, `dirs    = ["~/Projects/acme"]`, `dirs    = ['~/Projects/acme', '~/Projects/acme-2']`, 1),
	}
	steps := []struct {
		add     bool
		profile string
		dir     string
	}{
		{true, "personal", "~/p1"}, {true, "acme", "~/a1"}, {true, "globex", "~/g1"},
		{false, "", "~/Projects/acme/oss"}, {true, "personal", "~/p2"}, {false, "", "~/p1"},
		{false, "", "~/g1"}, {false, "", "~/Projects/personal"}, {false, "", "~/p2"},
	}

	for name, text := range layouts {
		t.Run(name, func(t *testing.T) {
			want, err := Parse([]byte(text))
			if err != nil {
				t.Fatalf("layout does not parse: %v", err)
			}
			for _, s := range steps {
				if s.add {
					text, err = AddDir(text, s.profile, s.dir, editOpts)
					want = withDir(want, s.profile, s.dir)
				} else {
					var owner string
					text, owner, err = RemoveDir(text, s.dir, editOpts)
					want = withoutDir(want, owner, s.dir)
				}
				if err != nil {
					t.Fatalf("step %+v: %v\n%s", s, err, text)
				}
				got, err := Parse([]byte(text))
				if err != nil || !sameDirs(got, want) {
					t.Fatalf("step %+v: parsed %+v, want %+v\n%s", s, got.Profiles, want.Profiles, text)
				}
			}
			if strings.Contains(commented, "# My GitPersona setup") && !strings.Contains(text, "# My GitPersona setup") {
				t.Fatal("file comment lost")
			}
		})
	}
}

func withDir(cfg Config, id, dir string) Config {
	out := cfg
	out.Profiles = append([]Profile{}, cfg.Profiles...)
	for i, p := range out.Profiles {
		if p.ID == id {
			out.Profiles[i].Dirs = append(append([]string{}, p.Dirs...), dir)
		}
	}
	return out
}

func withoutDir(cfg Config, id, dir string) Config {
	out := cfg
	out.Profiles = append([]Profile{}, cfg.Profiles...)
	for i, p := range out.Profiles {
		if p.ID != id {
			continue
		}
		var kept []string
		for _, d := range p.Dirs {
			if d != dir {
				kept = append(kept, d)
			}
		}
		out.Profiles[i].Dirs = kept
	}
	return out
}

func sameDirs(a, b Config) bool {
	if len(a.Profiles) != len(b.Profiles) {
		return false
	}
	for i := range a.Profiles {
		if strings.Join(a.Profiles[i].Dirs, "|") != strings.Join(b.Profiles[i].Dirs, "|") {
			return false
		}
	}
	return true
}

func TestEditsHandleQuotedKeys(t *testing.T) {
	text := strings.Replace(commented, `id      = "acme"`, `"id"    = "acme"`, 1)
	text = strings.Replace(text, `dirs    = ["~/Projects/acme"]`, `'dirs'  = ["~/Projects/acme"]`, 1)

	got, err := AddDir(text, "acme", "~/a2", editOpts)

	assertEdit(t, got, err, `'dirs'  = ["~/Projects/acme", "~/a2"]`)
}

func TestEditsKeepCRLFLineEndings(t *testing.T) {
	crlf := strings.ReplaceAll(commented, "\n", "\r\n")

	got, err := AddDir(crlf, "globex", "~/g", editOpts)
	if err != nil {
		t.Fatal(err)
	}
	withPin, err := AddPin(got, "~/r", "acme", editOpts)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Count(withPin, "\n") != strings.Count(withPin, "\r\n") {
		t.Fatalf("mixed line endings after editing a CRLF file:\n%q", withPin)
	}
	assertParses(t, withPin)
}

func TestAddProfileAppendsABlockAfterTheLastProfile(t *testing.T) {
	p := Profile{ID: "client", Provider: ProviderGitHub, Host: DefaultGitHubHost, Account: "client-alex",
		Name: "Alex", Email: "alex@client.example", Dirs: []string{"~/Projects/client"}}

	got, err := AddProfile(commented, p)

	assertEdit(t, got, err, "[[profile]]\nid      = \"client\"\naccount = \"client-alex\"\nname    = \"Alex\"\nemail   = \"alex@client.example\"\ndirs    = [\"~/Projects/client\"]\n")
	if !strings.Contains(got, "# My GitPersona setup") {
		t.Fatal("comments lost")
	}
	if _, err := AddProfile(commented, Profile{ID: "acme"}); err == nil {
		t.Fatal("adding a duplicate profile id succeeded")
	}
}

func TestSetProfileFieldReplacesOrAddsTheKey(t *testing.T) {
	renamed, err := SetProfileField(commented, "acme", "name", "Alex A.")
	assertEdit(t, renamed, err, `name    = "Alex A."`)

	withHost, err := SetProfileField(commented, "globex", "host", "gitlab.globex.example")
	assertEdit(t, withHost, err, "id      = \"globex\"\nhost = \"gitlab.globex.example\"")

	cleared, err := SetProfileField(commented, "acme", "name", "")
	if err != nil || strings.Contains(cleared, `name    = "Alex (Acme)"`) {
		t.Fatalf("an empty value should remove the key: %v\n%s", err, cleared)
	}
	if _, err := SetProfileField(commented, "acme", "dirs", "x"); err == nil {
		t.Fatal("dirs must be edited with AddDir/RemoveDir")
	}
}

func TestRemoveProfileDropsItsBlockAndComments(t *testing.T) {
	got, err := RemoveProfile(commented, "personal")

	if err != nil || strings.Contains(got, "octo-alex") || strings.Contains(got, "# main") || !strings.Contains(got, "# work by default") {
		t.Fatalf("RemoveProfile() = %v\n%s", err, got)
	}
	assertParses(t, got)
}

func TestSetDefault(t *testing.T) {
	changed, err := SetDefault(commented, "personal")
	assertEdit(t, changed, err, `default = "personal" # work by default`)

	cleared, err := SetDefault(commented, "")
	if err != nil || strings.Contains(cleared, "default =") {
		t.Fatalf("SetDefault(\"\") should remove the key: %v\n%s", err, cleared)
	}
	added, err := SetDefault(cleared, "acme")
	assertEdit(t, added, err, "version = 1\ndefault = \"acme\"\n")
}

func TestSetProfileSigning(t *testing.T) {
	signed, err := SetProfileSigning(commented, "acme", Signing{Method: SigningSSH, Key: "~/.ssh/acme.pub"})
	assertEdit(t, signed, err, `signing = { method = "ssh", key = "~/.ssh/acme.pub" }`)

	changed, err := SetProfileSigning(signed, "acme", Signing{Method: SigningGPG, Key: "ABC123"})
	assertEdit(t, changed, err, `signing = { method = "gpg", key = "ABC123" }`)

	off, err := SetProfileSigning(changed, "acme", Signing{})
	if err != nil || strings.Contains(off, "signing") {
		t.Fatalf("removing signing: %v\n%s", err, off)
	}
	table := commented + "\n[profile.signing]\nmethod = \"ssh\"\n"
	if _, err := SetProfileSigning(strings.Replace(table, "", "", 1), "ghost", Signing{}); err == nil {
		t.Fatal("unknown profile accepted")
	}
}
