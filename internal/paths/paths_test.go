package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

const home = "/tmp/home"

func TestNormalize(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "expands bare tilde", in: "~", want: home},
		{name: "expands tilde prefix", in: "~/Projects/acme", want: home + "/Projects/acme"},
		{name: "removes trailing slash", in: "~/Projects/acme/", want: home + "/Projects/acme"},
		{name: "cleans dot segments", in: "/srv/./code/../repos", want: "/srv/repos"},
		{name: "keeps absolute path", in: "/srv/repos", want: "/srv/repos"},
		{name: "keeps spaces", in: "~/My Projects/acme", want: home + "/My Projects/acme"},
		{name: "does not expand tilde user form", in: "~other/x", wantErr: true},
		{name: "rejects relative path", in: "Projects/acme", wantErr: true},
		{name: "rejects empty path", in: "", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Normalize(tc.in, home)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("Normalize(%q) = %q, want an error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Normalize(%q) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestWithin(t *testing.T) {
	tests := []struct {
		name            string
		dir             string
		path            string
		caseInsensitive bool
		want            bool
	}{
		{name: "same directory", dir: "/p/acme", path: "/p/acme", want: true},
		{name: "child directory", dir: "/p/acme", path: "/p/acme/billing", want: true},
		{name: "deep child", dir: "/p/acme", path: "/p/acme/a/b/c", want: true},
		{name: "sibling with common prefix is outside", dir: "/p/work", path: "/p/work2/repo", want: false},
		{name: "parent is outside", dir: "/p/acme/billing", path: "/p/acme", want: false},
		{name: "unrelated", dir: "/p/acme", path: "/q/acme", want: false},
		{name: "case differs, sensitive", dir: "/p/Acme", path: "/p/acme/x", want: false},
		{name: "case differs, insensitive", dir: "/p/Acme", path: "/p/acme/x", caseInsensitive: true, want: true},
		{name: "root contains everything", dir: "/", path: "/p/acme", want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Within(tc.path, tc.dir, tc.caseInsensitive)

			if got != tc.want {
				t.Fatalf("Within(%q, %q, %v) = %v, want %v", tc.path, tc.dir, tc.caseInsensitive, got, tc.want)
			}
		})
	}
}

func TestSame(t *testing.T) {
	if !Same("/p/Acme", "/p/acme", true) {
		t.Error("Same should ignore case when insensitive")
	}
	if Same("/p/Acme", "/p/acme", false) {
		t.Error("Same should respect case when sensitive")
	}
}

func TestResolveSymlinksFollowsLinks(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	link := filepath.Join(root, "link")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDir, link); err != nil {
		t.Fatal(err)
	}
	wantReal, err := filepath.EvalSymlinks(realDir)
	if err != nil {
		t.Fatal(err)
	}

	got := ResolveSymlinks(filepath.Join(link, "missing", "child"))

	if want := filepath.Join(wantReal, "missing", "child"); got != want {
		t.Fatalf("ResolveSymlinks() = %q, want %q", got, want)
	}
}

func TestCaseInsensitiveFSMatchesPlatform(t *testing.T) {
	want := runtime.GOOS == "darwin"

	if got := CaseInsensitiveFS(); got != want {
		t.Fatalf("CaseInsensitiveFS() = %v on %s, want %v", got, runtime.GOOS, want)
	}
}

func TestHomeUsesHomeEnvironment(t *testing.T) {
	t.Setenv("HOME", "/tmp/home")

	got, err := Home()

	if err != nil || got != "/tmp/home" {
		t.Fatalf("Home() = %q, %v; want /tmp/home", got, err)
	}
}

func TestResolveSymlinksKeepsPathWithoutExistingPrefix(t *testing.T) {
	got := ResolveSymlinks("relative/missing")

	if got != "relative/missing" {
		t.Fatalf("ResolveSymlinks() = %q, want the input unchanged", got)
	}
}

func TestHomeFailsWhenUnset(t *testing.T) {
	t.Setenv("HOME", "")

	if _, err := Home(); err == nil {
		t.Fatal("Home() with HOME unset succeeded, want an error")
	}
}

func TestResolveSymlinksWithoutLinksReturnsSamePath(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if got := ResolveSymlinks(dir); got != dir {
		t.Fatalf("ResolveSymlinks(%s) = %s, want it unchanged", dir, got)
	}
}
