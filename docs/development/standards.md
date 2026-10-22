# Development Standards

These rules apply to every change. "MUST" rules are enforced by tooling or review; "SHOULD" rules need a
reason in the pull request when broken.

## 1. Standards we follow

| Area | Standard |
|---|---|
| Versioning | [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html) |
| Commit messages | [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/) |
| Changelog | [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/) |
| Go style | [Effective Go](https://go.dev/doc/effective_go), [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments), `gofmt` / `goimports` |
| License | MIT, SPDX identifier `MIT` |
| Community | [Contributor Covenant 2.1](https://www.contributor-covenant.org/version/2/1/code_of_conduct/) |
| Security reporting | Private GitHub Security Advisories ([SECURITY.md](../../SECURITY.md)) |
| Web UI (later milestones) | [WCAG 2.2](https://www.w3.org/TR/WCAG22/) level AA |
| CLI behavior | [Command Line Interface Guidelines](https://clig.dev/) |

## 2. Privacy and identity (MUST)

GitPersona is about identities, so this rule has no exceptions.

- **Never commit real account names, emails, organization names, hostnames or local paths** — not in code,
  tests, fixtures, docs, screenshots, commit messages or issues.
- Use only these demo identities:

  | Demo identity | Role | Commit email |
  |---|---|---|
  | `octo-alex` | personal account on GitHub | `1000001+octo-alex@users.noreply.github.com` |
  | `acme-alex` | work account (fictional company Acme) on GitHub | `1000002+acme-alex@users.noreply.github.com` |
  | `globex-alex` | client account (fictional client Globex) on `gitlab.globex.example` | `alex@globex.example` |

- Use demo paths under `~/Projects/` (`~/Projects/personal`, `~/Projects/acme`, `~/Projects/clients/globex`).
- **Tests never touch the real home directory.** All test data lives under `/tmp`:
  - a literal demo home directory is `/tmp/home` (for example `/tmp/home/Projects/acme`);
  - tests that touch the file system use `t.TempDir()`, and `make test` / `make cover` set
    `TMPDIR=/tmp/gitpersona-test`, so those directories are created under `/tmp` too;
  - tests that run Git or GitPersona end to end set `HOME` (and `XDG_CONFIG_HOME`) to a directory under `/tmp`.
- Real home directories (`/Users/<name>/`, `/home/<name>/`) are rejected by the privacy check without exception.
- Use reserved example domains only: `example.com`, `example.org`, `*.example` ([RFC 2606](https://www.rfc-editor.org/rfc/rfc2606)).
- `make privacy` (pre-commit hook and CI) blocks local paths, private keys and tokens. Each contributor may
  add their own real identifiers, one regular expression per line, to the git-ignored `.privacy-denylist`.

## 3. Security (MUST)

- **Never touch the user's keys.** Do not open, read, copy, back up or upload private key files; do not
  scan or list `~/.ssh`; do not query ssh-agent; do not run ssh with a user's key to "test" it. Key paths are
  strings the user provides and are written into Git configuration unchanged. Tests prove this by making
  the demo key unreadable (mode 000).
- Fetch a token in exactly two cases: when Git itself asks for a credential and the host and account match
  (hand it straight to Git), or when the user runs `gh` / `glab` in a directory that has a profile (put it only
  into the environment of that one child process). Never store, print, log or export it. Health checks and discovery use token-free status queries (`gh auth status --json hosts`),
  never `--show-token` or `gh auth token`.
- Never write tokens or passwords to disk, logs, backups, error messages or the web UI.
- Run external programs with `exec.Command(name, args...)`; never build shell command strings.
- When the right identity cannot be determined, do nothing and fall through to Git's default behavior.
- Edit user files only between `# >>> gitpersona >>>` and `# <<< gitpersona <<<` markers, back them up
  first, and write atomically (temporary file, then rename).
- Files GitPersona creates are `0600`, directories `0700`.

## 4. Code

- **Layout:** `cmd/gitpersona` is the entry point; all other code lives in `internal/` packages, one
  responsibility each (see the architecture design). No `pkg/` until there is a public API.
- **Size:** functions under 50 lines; files under 400 lines (800 hard limit); nesting at most 4 levels —
  prefer early returns.
- **Pure core:** resolution and generation logic takes values and returns values. File system, process and
  network access stay at the edges, behind small interfaces, so the core is testable without I/O.
- **Errors:** return errors, do not panic. Wrap with context using `fmt.Errorf("load config: %w", err)`;
  compare with `errors.Is` / `errors.As`. Messages are lower case without trailing punctuation.
  User-facing messages say what went wrong and what to do next.
- **Immutability:** do not mutate inputs; return new values. Avoid package-level mutable state
  (build information set by `-ldflags` is the only exception).
- **No magic values:** name constants for exit codes, file modes, limits and timeouts.
- **Naming:** standard Go naming (`MixedCaps`, short receivers, no `Get` prefix, acronyms in one case: `URL`, `ID`).
- **Comments:** doc comments on every exported identifier. Inline comments explain *why*, not *what*.
- **Logging:** when logging is added, use `log/slog`, off by default and enabled with a `--verbose` flag. Never log secrets.
- **Dependencies:** standard library first. A new dependency needs a reason in the pull request
  (maintenance, license compatible with MIT, size). Run `go mod tidy`.

## 5. Command-line behavior

| Rule | Detail |
|---|---|
| Output streams | Results go to stdout; errors, warnings and progress to stderr |
| Exit codes | `0` success, `1` failure, `2` usage error |
| Machine output | Commands that report state support `--json`, with a stable schema |
| Color | Only when stdout is a terminal; respect `NO_COLOR` |
| Prompts | Never prompt when stdin is not a terminal; offer flags instead |
| Destructive actions | Preview first, back up, and require confirmation (or `--yes`) |

## 6. Tests (MUST)

- **Test first:** write a failing test, make it pass, then refactor.
- **Coverage:** at least 80% overall (`make cover` fails below it); the resolver and the generator aim for 95%.
- **Style:** table-driven tests with descriptive names; Arrange–Act–Assert.
- **Isolation:** no network, no real `$HOME`, no real accounts. Use `t.TempDir()`, `t.Setenv("HOME", …)`,
  and fake `gh` / `glab` / `ssh` executables placed first on `PATH`.
- **Integration tests** run the real `git` binary against temporary repositories.
- **Golden files** for generated configuration live in `testdata/`; update them only on purpose.
- Run with `-race`. Tests must pass on macOS and Linux.

## 7. Git workflow

- **Branching:** GitHub Flow. `main` is always releasable; work happens on short-lived branches
  (`feat/…`, `fix/…`, `docs/…`, `refactor/…`, `test/…`, `ci/…`, `chore/…`).
- **Commits:** Conventional Commits, enforced by the `commit-msg` hook. One logical change per commit.
  Commits are authored only by the contributor's own account; do not add tool or bot attribution trailers.
- **Pull requests:** use the template, link the issue, keep them small. CI must be green; at least one
  approval from a code owner; squash-merge with a Conventional Commit title.
- **Never** force-push to `main` or rewrite published history.

## 8. Versioning and releases

- SemVer. Before 1.0, minor versions may contain breaking changes; they are called out in the changelog.
- Every user-visible change adds a line under **Unreleased** in `CHANGELOG.md`
  (Added / Changed / Deprecated / Removed / Fixed / Security).
- Release: move Unreleased entries to a new version section, tag `vX.Y.Z` on `main`, push the tag.
  The release workflow runs the checks, then GoReleaser builds macOS and Linux binaries (amd64, arm64),
  publishes checksums, and updates the Homebrew tap.

## 9. Documentation

- Public docs are in English; `README.md` has a Simplified Chinese translation in `README.zh-CN.md`
  that is kept in sync in the same pull request.
- Every command change updates the README and the command's `--help` text.
- Product requirements and planning are internal and are not published (`docs/prd/` is git-ignored).

## 10. Definition of done

A change is done when:

- [ ] tests were written first and pass with `-race` on macOS and Linux;
- [ ] `make check` and `make lint` pass, coverage is at least 80%;
- [ ] no real identities, local paths or secrets (`make privacy`);
- [ ] errors are handled and messages are actionable;
- [ ] `CHANGELOG.md`, README and help text are updated where relevant;
- [ ] the pull request is reviewed and merged with a Conventional Commit title.
