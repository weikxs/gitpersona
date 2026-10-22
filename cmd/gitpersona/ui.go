package main

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/weikxs/gitpersona/internal/app"
	"github.com/weikxs/gitpersona/internal/web"
)

// runUI serves the configuration page until it is closed or idle. section
// is the part of the page to open first, such as "setup"; "" opens the overview.
func runUI(ctx context.Context, env app.Env, openBrowser bool, section string) error {
	ln, err := web.Listen()
	if err != nil {
		return fmt.Errorf("start the configuration page: %w", err)
	}
	srv, err := web.New(ln.Addr().String(), web.Handlers{})
	if err != nil {
		return err
	}
	srv.Register(web.API(env, srv.Stop))

	link := srv.URL()
	if section != "" {
		link += "#" + section
	}
	fmt.Fprintf(env.Stdout, "GitPersona configuration page (this link works once):\n  %s\n", link)
	fmt.Fprintln(env.Stdout, "Press Ctrl-C or use Close on the page to stop. It stops by itself after 15 minutes without use.")
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
	return exec.Command(name, url).Start()
}
