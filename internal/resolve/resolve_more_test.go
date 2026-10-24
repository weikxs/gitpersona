package resolve

import (
	"testing"

	"github.com/weikxs/gitpersona/internal/config"
)

func TestGitDirIgnoresPinsWhoseRepositoryPathIsInvalid(t *testing.T) {
	cfg := demoConfig()
	cfg.Pins = []config.Pin{{Repo: "relative/repo", Profile: "globex"}}

	got := GitDir(cfg, home+"/Projects/personal/blog/.git", Options{Home: home})

	if got.Reason != ReasonRule || got.Profile.ID != "personal" {
		t.Fatalf("GitDir() = %+v, want the directory rule", got)
	}
}
