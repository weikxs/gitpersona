package web

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/weikxs/gitpersona/internal/app"
	"github.com/weikxs/gitpersona/internal/provider"
)

func TestTheOverviewReusesItsCheckWhileNothingChanged(t *testing.T) {
	var gits atomic.Int64
	w := newAPIWorldWith(t, func(e *app.Env) {
		e.Run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			if filepath.Base(name) == "git" {
				gits.Add(1)
			}
			return provider.ExecRunner(ctx, name, args...)
		}
	})
	auto := map[string]any{"dirs": []string{}, "ssh": false, "auto": true}
	check := func() app.VerifyReport {
		t.Helper()
		var report app.VerifyReport
		decode(t, w.call(t, "POST", "/api/verify", auto), &report)
		return report
	}

	first := check()
	if len(first.Repositories) == 0 || first.Total != len(first.Repositories) || first.Limited {
		t.Fatalf("first check = %+v", first)
	}
	before := gits.Load()
	second := check()

	if started := gits.Load() - before; started != 0 {
		t.Errorf("the second overview load started %d git processes, want 0", started)
	}
	if len(second.Repositories) != len(first.Repositories) {
		t.Errorf("the reused check differs: %+v", second)
	}

	// A repository's settings change (the command line, by hand): check again.
	repoConfig := filepath.Join(w.home, "Projects", "personal", "blog", ".git", "config")
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(repoConfig, later, later); err != nil {
		t.Fatal(err)
	}
	before = gits.Load()
	check()
	if gits.Load() == before {
		t.Error("a changed .git/config should be checked again")
	}

	// A change through the page drops the kept check.
	w.call(t, "POST", "/api/apply", map[string]bool{"yes": true})
	before = gits.Load()
	check()
	if gits.Load() == before {
		t.Error("after a change on the page the overview should check again")
	}
}
