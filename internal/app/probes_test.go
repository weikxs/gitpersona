package app

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/weikxs/gitpersona/internal/provider"
)

// probeCounter answers dscl and ps like a Mac where gitpersona (500) runs
// in zsh (400), passes everything else to the real programs, and counts
// each command line.
type probeCounter struct {
	mu    sync.Mutex
	calls map[string]int
}

func (p *probeCounter) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	line := strings.Join(append([]string{name}, args...), " ")
	p.mu.Lock()
	if p.calls == nil {
		p.calls = map[string]int{}
	}
	p.calls[line]++
	p.mu.Unlock()
	switch {
	case name == "dscl":
		return []byte("UserShell: /bin/zsh\n"), nil
	case name == "ps" && args[len(args)-1] == "500":
		return []byte(" 400 /usr/local/bin/gitpersona\n"), nil
	case name == "ps" && args[len(args)-1] == "400":
		return []byte(" 1 -zsh\n"), nil
	}
	return provider.ExecRunner(ctx, name, args...)
}

func (p *probeCounter) count(prefix string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for line, c := range p.calls {
		if strings.HasPrefix(line, prefix) {
			n += c
		}
	}
	return n
}

func TestStateAsksAboutTheSystemOncePerProcess(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	counter := &probeCounter{}
	env.Run = counter.run
	shellsRead := 0
	env = env.withProbes(systemFacts{
		goos:     "darwin",
		username: func() string { return "alex" },
		getenv:   func(string) string { return "" },
		etcShells: func() string {
			shellsRead++
			return "/bin/zsh\n"
		},
		pid: 500,
	})

	first := State(context.Background(), env)
	second := State(context.Background(), env)

	if first.Shells.Default.Kind != "zsh" || second.Shells.Current.Kind != "zsh" {
		t.Fatalf("shells = %+v / %+v, want zsh", first.Shells, second.Shells)
	}
	for prefix, want := range map[string]int{"dscl": 1, "ps -o ppid=,comm= -p 500": 1, "ps -o ppid=,comm= -p 400": 1, "git version": 1} {
		if got := counter.count(prefix); got != want {
			t.Errorf("%q ran %d times for two page states, want %d", prefix, got, want)
		}
	}
	if shellsRead != 1 {
		t.Errorf("/etc/shells was read %d times, want once", shellsRead)
	}
}

func TestProbesAskAgainAfterACancelledCall(t *testing.T) {
	asked := 0
	probe := memoize(func(_ context.Context) int {
		asked++
		return asked
	})
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	_ = probe(cancelled)
	got := probe(context.Background())
	again := probe(context.Background())

	if got != 2 || again != 2 {
		t.Fatalf("probe() = %d then %d, want the first complete answer (2) kept", got, again)
	}
}
