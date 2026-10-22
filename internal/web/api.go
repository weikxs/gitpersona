package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/weikxs/gitpersona/internal/app"
	"github.com/weikxs/gitpersona/internal/backup"
	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/discover"
	"github.com/weikxs/gitpersona/internal/paths"
	"github.com/weikxs/gitpersona/internal/provider"
	"github.com/weikxs/gitpersona/internal/shell"
)

// maxBody bounds request bodies; the page only sends small JSON objects.
const maxBody = 64 << 10

// defaultRoots are scanned for repositories when the page does not choose.
var defaultRoots = []string{"~/Projects", "~/projects", "~/code", "~/src", "~/dev", "~/work", "~/repos", "~/git"}

// envelope is every API response: ok with data, or an error; log carries
// what the command printed.
type envelope struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
	Log   string `json:"log,omitempty"`
}

// DiscoverReport is what discovery suggests.
type DiscoverReport struct {
	Roots       []string              `json:"roots"`
	Existing    []discover.Identity   `json:"existing"`
	Suggestions []discover.Suggestion `json:"suggestions"`
}

// BackupView is one backup on the page.
type BackupView struct {
	Name    string    `json:"name"`
	Created time.Time `json:"created"`
	Command string    `json:"command"`
	Files   []string  `json:"files"`
}

// API returns the page's API routes, running commands with env; stop ends
// the server when the page closes.
func API(env app.Env, stop func()) map[string]http.HandlerFunc {
	a := api{env: env, stop: stop}
	return map[string]http.HandlerFunc{
		"GET /api/state":            a.handle(a.state),
		"GET /api/preview":          a.handle(a.preview),
		"POST /api/apply":           a.handle(a.apply),
		"GET /api/setup":            a.handle(a.proposeSetup),
		"POST /api/setup/preview":   a.handle(a.previewSetup),
		"POST /api/setup":           a.handle(a.setup),
		"GET /api/discover":         a.handle(a.discover),
		"GET /api/resolve":          a.handle(a.resolve),
		"POST /api/profiles":        a.handle(a.addProfile),
		"PATCH /api/profiles/{id}":  a.handle(a.updateProfile),
		"DELETE /api/profiles/{id}": a.handle(a.removeProfile),
		"POST /api/default":         a.handle(a.setDefault),
		"POST /api/dirs":            a.handle(a.addDir),
		"DELETE /api/dirs":          a.handle(a.removeDir),
		"POST /api/pins":            a.handle(a.addPin),
		"DELETE /api/pins":          a.handle(a.removePin),
		"GET /api/backups":          a.handle(a.backups),
		"POST /api/rollback":        a.handle(a.rollback),
		"POST /api/shells":          a.handle(a.installShells),
		"POST /api/close":           a.handle(a.close),
	}
}

type api struct {
	env  app.Env
	stop func()
}

// call is one API request with its own output buffer.
type call struct {
	ctx context.Context
	r   *http.Request
	env app.Env
	log *bytes.Buffer
}

type handlerFunc func(c call) (any, error)

func (a api) handle(h handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		log := &bytes.Buffer{}
		env := a.env
		env.Stdout, env.Stderr = log, log
		data, err := h(call{ctx: r.Context(), r: r, env: env, log: log})
		resp := envelope{OK: err == nil, Data: data, Log: log.String()}
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			resp.Error = err.Error()
			w.WriteHeader(http.StatusBadRequest)
		}
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func (c call) decode(into any) error {
	if err := json.NewDecoder(c.r.Body).Decode(into); err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}
	return nil
}

// at returns the env with the working directory set to a user-given path,
// which must be absolute or start with ~/.
func (c call) at(dir string) (app.Env, error) {
	norm, err := paths.Normalize(dir, c.env.Home)
	if err != nil {
		return c.env, err
	}
	env := c.env
	env.WorkDir = norm
	return env, nil
}

func (a api) state(c call) (any, error)   { return app.State(c.ctx, c.env), nil }
func (a api) preview(c call) (any, error) { return app.Preview(c.ctx, c.env) }

func (a api) apply(c call) (any, error) {
	var req struct {
		Yes bool `json:"yes"`
	}
	if err := c.decode(&req); err != nil {
		return nil, err
	}
	return nil, app.Apply(c.ctx, c.env, app.ApplyOptions{Yes: req.Yes, Command: "apply (web)"})
}

func (a api) proposeSetup(c call) (any, error) { return app.ProposeSetup(c.ctx, c.env) }

func (a api) previewSetup(c call) (any, error) {
	var plan app.SetupPlan
	if err := c.decode(&plan); err != nil {
		return nil, err
	}
	return app.PreviewSetup(c.ctx, c.env, plan)
}

// setup writes the confirmed plan, or without profiles the configuration
// `gitpersona setup` would write.
func (a api) setup(c call) (any, error) {
	var plan app.SetupPlan
	if err := c.decode(&plan); err != nil {
		return nil, err
	}
	if len(plan.Profiles) == 0 {
		return nil, app.Setup(c.ctx, c.env, app.SetupOptions{})
	}
	return nil, app.ApplySetup(c.ctx, c.env, plan)
}

func (a api) discover(c call) (any, error) {
	roots := defaultRoots
	if q := c.r.URL.Query().Get("roots"); q != "" {
		roots = strings.Split(q, ",")
	}
	report := DiscoverReport{Roots: []string{}, Existing: []discover.Identity{}, Suggestions: []discover.Suggestion{}}
	var abs []string
	for _, root := range roots {
		norm, err := paths.Normalize(strings.TrimSpace(root), c.env.Home)
		if err != nil {
			return nil, err
		}
		if info, err := os.Stat(norm); err == nil && info.IsDir() {
			report.Roots = append(report.Roots, c.env.Display(norm))
			abs = append(abs, norm)
		}
	}
	if existing, err := discover.Existing(c.ctx, c.env.Home, c.env.GlobalGitConfigPath(), c.env.Run); err == nil && existing != nil {
		for _, id := range existing {
			id.Source = c.env.Display(id.Source)
			report.Existing = append(report.Existing, id)
		}
	}
	var logins []string
	if gh, err := c.env.LookPath("gh"); err == nil {
		accounts, _ := provider.GitHub{GH: gh, Run: c.env.Run}.Accounts(c.ctx, config.DefaultGitHubHost)
		for _, acc := range accounts {
			logins = append(logins, acc.Login)
		}
	}
	for _, s := range discover.Scan(c.ctx, abs, logins, discover.DefaultScanLimits) {
		s.Dir = c.env.Display(s.Dir)
		report.Suggestions = append(report.Suggestions, s)
	}
	return report, nil
}

func (a api) resolve(c call) (any, error) {
	env, err := c.at(c.r.URL.Query().Get("dir"))
	if err != nil {
		return nil, err
	}
	return app.WhoamiReportFor(c.ctx, env)
}

type profileRequest struct {
	ID      string          `json:"id"`
	Account *string         `json:"account"`
	Name    *string         `json:"name"`
	Email   *string         `json:"email"`
	SSHKey  *string         `json:"ssh_key"`
	Signing *config.Signing `json:"signing"`
}

func (a api) addProfile(c call) (any, error) {
	var req profileRequest
	if err := c.decode(&req); err != nil {
		return nil, err
	}
	p := config.Profile{ID: req.ID}
	if req.Account != nil {
		p.Account = *req.Account
	}
	if req.Name != nil {
		p.Name = *req.Name
	}
	if req.Email != nil {
		p.Email = *req.Email
	}
	if req.SSHKey != nil {
		p.SSHKey = *req.SSHKey
	}
	if req.Signing != nil {
		p.Signing = *req.Signing
	}
	return nil, app.AddProfile(c.ctx, c.env, p)
}

func (a api) updateProfile(c call) (any, error) {
	var req profileRequest
	if err := c.decode(&req); err != nil {
		return nil, err
	}
	return nil, app.UpdateProfile(c.ctx, c.env, c.r.PathValue("id"), app.ProfileChanges{
		Account: req.Account, Name: req.Name, Email: req.Email, SSHKey: req.SSHKey, Signing: req.Signing,
	})
}

func (a api) removeProfile(c call) (any, error) {
	return nil, app.RemoveProfile(c.ctx, c.env, c.r.PathValue("id"))
}

func (a api) setDefault(c call) (any, error) {
	var req struct {
		ID string `json:"id"`
	}
	if err := c.decode(&req); err != nil {
		return nil, err
	}
	return nil, app.SetDefaultProfile(c.ctx, c.env, req.ID)
}

type dirRequest struct {
	Dir     string `json:"dir"`
	Repo    string `json:"repo"`
	Profile string `json:"profile"`
}

func (a api) addDir(c call) (any, error) {
	var req dirRequest
	if err := c.decode(&req); err != nil {
		return nil, err
	}
	env, err := c.at(req.Dir)
	if err != nil {
		return nil, err
	}
	return nil, app.Use(c.ctx, env, req.Profile)
}

func (a api) removeDir(c call) (any, error) {
	var req dirRequest
	if err := c.decode(&req); err != nil {
		return nil, err
	}
	if _, err := c.at(req.Dir); err != nil {
		return nil, err
	}
	return nil, app.Remove(c.ctx, c.env, req.Dir)
}

func (a api) addPin(c call) (any, error) {
	var req dirRequest
	if err := c.decode(&req); err != nil {
		return nil, err
	}
	env, err := c.at(req.Repo)
	if err != nil {
		return nil, err
	}
	return nil, app.Pin(c.ctx, env, req.Profile)
}

func (a api) removePin(c call) (any, error) {
	var req dirRequest
	if err := c.decode(&req); err != nil {
		return nil, err
	}
	env, err := c.at(req.Repo)
	if err != nil {
		return nil, err
	}
	return nil, app.Unpin(c.ctx, env)
}

func (a api) backups(c call) (any, error) {
	infos, err := backup.List(c.env.BackupDir())
	if err != nil {
		return nil, err
	}
	views := []BackupView{}
	for _, info := range infos {
		v := BackupView{Name: filepath.Base(info.Dir), Created: info.Manifest.Created, Command: info.Manifest.Command, Files: []string{}}
		for _, e := range info.Manifest.Entries {
			v.Files = append(v.Files, c.env.Display(e.Path))
		}
		views = append(views, v)
	}
	return views, nil
}

func (a api) rollback(c call) (any, error) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.decode(&req); err != nil {
		return nil, err
	}
	if req.Name == "" {
		return nil, fmt.Errorf("choose a backup")
	}
	return nil, app.Rollback(c.ctx, c.env, req.Name, true)
}

func (a api) installShells(c call) (any, error) {
	var req struct {
		Shells []string `json:"shells"`
	}
	if err := c.decode(&req); err != nil {
		return nil, err
	}
	var kinds []shell.Kind
	for _, name := range req.Shells {
		k, err := shell.Parse(name)
		if err != nil {
			return nil, err
		}
		kinds = append(kinds, k)
	}
	return nil, app.InstallShell(c.ctx, c.env, kinds, false)
}

func (a api) close(_ call) (any, error) {
	if a.stop != nil {
		go a.stop()
	}
	return nil, nil
}
