package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/weikxs/gitpersona/internal/app"
	"github.com/weikxs/gitpersona/internal/i18n"
	"github.com/weikxs/gitpersona/internal/web"
)

// runUI serves the configuration page until it is closed or idle. section
// is the part of the page to open first, such as "setup"; "" opens the overview.
func runUI(ctx context.Context, env app.Env, openBrowser bool, section string) error {
	unlock, err := lockUI(env.ConfigDir)
	if err != nil {
		return err
	}
	defer unlock()
	ln, err := web.Listen()
	if err != nil {
		return fmt.Errorf("start the configuration page: %w", err)
	}
	system := i18n.System(runtime.GOOS, os.Getenv, func(name string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, args...).Output()
	})
	srv, err := web.New(ln.Addr().String(), web.Handlers{SystemLang: string(system)})
	if err != nil {
		return err
	}
	srv.Register(web.API(env, srv.Stop))

	link := srv.URL()
	if section != "" {
		link += "#" + section
	}
	fmt.Fprintf(env.Stdout, "GitPersona configuration page (this link works once):\n  %s\n", link)
	fmt.Fprintln(env.Stdout, "Press Ctrl-C or use Close on the page to stop. It stops by itself soon after the tab is closed, "+
		"or after 15 minutes without use.")
	if openBrowser {
		if err := open(link); err != nil {
			fmt.Fprintf(env.Stdout, "Could not open a browser (%v); open the link above yourself.\n", err)
		}
	}
	return srv.Serve(ctx, ln)
}

func open(url string) error {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	_, err := startAndReap(exec.Command(name, url))
	return err
}

// startAndReap starts cmd without waiting for it, and waits for it in the
// background so it does not stay behind as a zombie process while the page
// runs. The returned channel closes once it has been waited for.
func startAndReap(cmd *exec.Cmd) (<-chan struct{}, error) {
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	reaped := make(chan struct{})
	go func() {
		defer close(reaped)
		_ = cmd.Wait() // the browser opener's exit status tells nothing useful
	}()
	return reaped, nil
}
