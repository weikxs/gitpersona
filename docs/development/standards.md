# Development Standards

These rules apply to every change, by anyone, including the maintainers. **MUST** rules are enforced by tooling
or review; **SHOULD** rules need a reason in the pull request when broken. Where a tool enforces a rule, the
"Enforced by" column says which; "review" means a maintainer checks it.

They follow the practice of large open-source projects: issue-first contribution (GitHub CLI, uv, Terraform),
human responsibility for AI-assisted work (Terraform, uv, curl), focused commits and squash rules (Kubernetes,
curl), structured bug reports (Go), and the [OpenSSF Scorecard](https://scorecard.dev) checks for the supply chain.

## 1. Standards we follow

| Area | Standard |
|---|---|
| Versioning | [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html) |
| Commit messages | [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/) |
| Changelog | [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/) |
| Go style | [Effective Go](https://go.dev/doc/effective_go), [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments), `gofmt` |
| Shell style | POSIX `sh`, clean under [ShellCheck](https://www.shellcheck.net) |
| CLI behavior | [Command Line Interface Guidelines](https://clig.dev/) |
| Web console | [WCAG 2.2](https://www.w3.org/TR/WCAG22/) level AA, strict Content Security Policy |
| License | MIT, SPDX identifier `MIT` |
| Community | [Contributor Covenant 2.1](https://www.contributor-covenant.org/version/2/1/code_of_conduct/) |
| Security reporting | Private GitHub Security Advisories ([SECURITY.md](../../SECURITY.md)) |
| Supply chain | [OpenSSF Scorecard](https://scorecard.dev) checks |

## 2. Contribution process

- **Issue first.** Every change starts from an issue. Bugs and documentation fixes are welcome straight away.
  New features, new commands, new flags and changes to generated Git configuration need a maintainer's go-ahead
  on the issue **before** code is written; pull requests for features without one may be closed.
- **Labels:** `good first issue` and `help wanted` are open for anyone; comment on the issue to claim it.
  `needs-design` means the behavior is not decided yet: propose a design on the issue, not in code.
- **Bug reports** answer five questions (as Go asks): the output of `gitpersona version`, the system and CPU, what
  you did, what you expected, what happened, plus `gitpersona doctor --json` when relevant, with identities
  replaced by demo ones (§4).
- **One pull request, one purpose.** Do not mix a fix, a refactor and a feature; split them.
- **Review:** every change reaches `main` through a pull request with green CI, maintainers' changes included
  (the repository ruleset allows nothing else). Pull requests from contributors also need a code owner's approval
  (`.github/CODEOWNERS`).
- **Response times** are best effort; security reports follow [SECURITY.md](../../SECURITY.md).

## 3. AI-assisted contributions (MUST)

AI tools may help write code, tests and docs. The person who opens the pull request is its author and is
responsible for every line.

- **Understand it.** You must be able to explain the change, its edge cases and side effects in your own words. If
  you cannot, the pull request is not ready.
- **Verify it.** Run the tests and check the behavior yourself. AI output that "looks right" is not evidence.
- **No autonomous agents.** Pull requests and issues come from a person's own account, opened and answered by that
  person. Pull requests that look generated and unattended are closed.
- **Say so** in the pull request description when AI wrote a substantial part of the change, and what it did.
- **Talk in your own words.** Write issues, reviews and replies yourself. Translation help for non-native speakers
  is fine; keep it your own voice.
- **Security reports found with AI** must be reproduced by you before you report them, and say that AI was used.
- **Commits carry no tool or bot attribution** (no `Co-Authored-By` for tools, no "generated with" lines): the
  human author is the author.
- Everything else in these standards applies unchanged, privacy (§4) first: never paste real identities, tokens or
  private paths into an AI tool's prompt for this project.

## 4. Privacy and identity (MUST)

GitPersona is about identities, so this rule has no exceptions. Enforced by `make privacy` (pre-commit hook and
CI) and review.

- **Never commit real account names, emails, organization names, hostnames or local paths** — not in code,
  tests, fixtures, docs, screenshots, release notes, commit messages or issues.
- Use only these demo identities:

  | Demo identity | Role | Commit email | Used in |
  |---|---|---|---|
  | `octo-alex` | personal account on GitHub | `1000001+octo-alex@users.noreply.github.com` | docs, unit tests |
  | `acme-alex` | work account (fictional company Acme) on GitHub | `1000002+acme-alex@users.noreply.github.com` | docs, unit tests |
  | `globex-alex` | client account (fictional client Globex) on `gitlab.globex.example` | `alex@globex.example` | docs, unit tests |
  | `demo-work` | work account in the demo home | `worker@demo.example` | e2e tests, screenshots |
  | `demo-personal` | personal account in the demo home | `102+demo-personal@users.noreply.github.com` | e2e tests, screenshots |

- Demo paths: `~/Projects/…` in docs and unit tests, `~/code/…` in the e2e demo home.
- Reserved example domains only: `example.com`, `example.org`, `*.example`, `demo.example`
  ([RFC 2606](https://www.rfc-editor.org/rfc/rfc2606)).
- **Tests never touch the real home directory.** All test data lives under `/tmp`:
  - a literal demo home directory is `/tmp/home` (for example `/tmp/home/Projects/acme`);
  - tests that touch the file system use `t.TempDir()`; `make test` / `make cover` set
    `TMPDIR=/tmp/gitpersona-test`;
  - tests that run Git or GitPersona end to end set `HOME`, `XDG_CONFIG_HOME` and `GIT_CONFIG_GLOBAL` under `/tmp`
    **and change the working directory there (`t.Chdir`)**, so they never see the repository they run from.
- Real home directories (`/Users/<name>/`, `/home/<name>/`) are rejected by the privacy check without exception.
- **Pictures:** README pictures are drawn only by `make screenshots` from the demo home, with stand-ins for `gh`,
  the login shell and the process list, so they show nothing of the machine that makes them. It fails when a
  picture would show a local path.
- Each contributor may add their own real identifiers, one regular expression per line, to the git-ignored
  `.privacy-denylist`.

## 5. Security (MUST)

- **Never touch the user's keys.** Do not open, read, copy, back up or upload private key files; do not scan or
  list `~/.ssh`; do not query ssh-agent. ssh connects with a key only when the user asks (`check-ssh`,
  `verify --ssh`). Key paths are strings the user provides, written into Git configuration unchanged. Tests prove
  this by making the demo key unreadable (mode 000).
- Fetch a token in exactly two cases: when Git itself asks for a credential and the host and account match (hand
  it straight to Git), or when the user runs `gh` / `glab` in a directory that has a profile (put it only into the
  environment of that one child process). Never store, print, log or export it. Health checks and discovery use
  token-free status queries (`gh auth status --json hosts`), never `--show-token` or `gh auth token`.
- Never write tokens or passwords to disk, logs, backups, error messages or the web console.
- Run external programs with `exec.Command(name, args...)`; never build shell command strings.
- When the right identity cannot be determined, do nothing and fall through to Git's default behavior.
- Edit user files only between `# >>> gitpersona >>>` and `# <<< gitpersona <<<` markers, back them up first, and
  write atomically (temporary file, then rename). Files GitPersona creates are `0600`, directories `0700`.
- **Output shows paths with `~`**, never the full home directory.
- **The web console** listens only on 127.0.0.1, behind a one-time link and a CSRF token, stops after 15 minutes
  without use, sends a strict Content Security Policy, and loads nothing from outside (no CDN, font or analytics).
  Enforced by `internal/web` tests and the e2e tests.
- **No telemetry.** GitPersona sends nothing anywhere unless the user runs a command that needs the network.

## 6. Supply chain

| Rule | Level | Enforced by |
|---|---|---|
| Workflows declare least-privilege `permissions` (read-only by default; `contents: write` only for the release) | MUST | review |
| Dependencies are updated by Dependabot (Go modules and GitHub Actions, weekly) | MUST | `.github/dependabot.yml` |
| A new dependency needs a reason in the pull request: maintenance, MIT-compatible license, size; standard library first | MUST | review |
| `go.mod` / `go.sum` are tidy and committed | MUST | `make check`, release workflow |
| Release archives are published with SHA-256 `checksums.txt`; the install script and Homebrew verify them | MUST | GoReleaser, `install.sh` |
| Release binaries are built in CI only, never on a developer's machine | MUST | release workflow |
| Actions are pinned to a full commit SHA, with the version in a comment; Dependabot updates both | MUST | review, Dependabot |
| CI and releases build with the newest Go patch release (`1.26.x`, `check-latest`), so Go's security fixes ship; `go.mod` keeps the oldest Go that builds GitPersona | MUST | workflows |
| Known vulnerabilities reachable from the code, in Go or in a dependency, fail CI (`govulncheck`) | MUST | CI `govulncheck` |
| Every release file carries signed build provenance; check one with `gh attestation verify <file> --repo weikxs/gitpersona` | MUST | release workflow |
| `main` is protected: no force-push or deletion, changes only through pull requests with CI green | MUST | repository ruleset |

## 7. Code

- **Layout:** `cmd/gitpersona` is the entry point; all other code lives in `internal/` packages, one
  responsibility each. No `pkg/` until there is a public API.
- **Size:** functions under 50 lines; files under 400 lines (800 hard limit); nesting at most 4 levels — prefer
  early returns.
- **Pure core:** resolution and generation take values and return values. File system, process and network access
  stay at the edges, behind small interfaces, so the core is testable without I/O.
- **Errors:** return errors, do not panic. Wrap with context (`fmt.Errorf("load config: %w", err)`); compare with
  `errors.Is` / `errors.As`. Messages are lower case without trailing punctuation, and user-facing ones say what
  went wrong and what to do next.
- **Immutability:** do not mutate inputs; return new values. No package-level mutable state (build information set
  by `-ldflags` is the only exception).
- **No magic values:** name constants for exit codes, file modes, limits and timeouts.
- **Naming:** standard Go naming (`MixedCaps`, short receivers, no `Get` prefix, acronyms in one case: `URL`, `ID`).
- **Comments:** doc comments on every exported identifier; inline comments explain *why*, not *what*.
- **Logging:** when logging is added, `log/slog`, off by default, enabled with `--verbose`. Never log secrets.
- **Shell scripts** (`install.sh`, `scripts/`, `test/`): POSIX `sh` unless bash is needed, `set -eu`, quote every
  expansion, ShellCheck clean (CI). `install.sh` never uses `sudo`, verifies checksums before installing, and
  changes nothing but the binary and one marked PATH line.
- **Workflows** pass [actionlint](https://github.com/rhysd/actionlint) (run it in a container before pushing).

## 8. Command line and web console

| Rule | Detail |
|---|---|
| Output streams | Results go to stdout; errors, warnings and progress to stderr |
| Exit codes | `0` success, `1` failure (including `verify` finding a problem), `2` usage error |
| Machine output | Commands that report state support `--json`, with a stable schema |
| Color | Only when stdout is a terminal; respect `NO_COLOR` |
| Prompts | Never prompt when stdin is not a terminal; offer flags (`--yes`) instead |
| Destructive actions | Preview first, back up, and require confirmation (or `--yes`) |
| Languages | The web console is in English and Simplified Chinese: every interface text exists in both `i18n-en.js` and `i18n-zh.js`, and every message from Go has an entry in `internal/i18n`. Tests fail on a missing or unused key and on mismatched placeholders |
| Language choice | The system's language, then the browser's, then English; the switch lasts for the tab |
| Accessibility | WCAG 2.2 AA: keyboard reachable, visible focus, labels, contrast; works at 375px wide and in dark mode (e2e) |

## 9. Tests (MUST)

- **Test first:** write a failing test, make it pass, then refactor.
- **Coverage:** at least 95% overall; `make cover` fails below it.
- **Style:** table-driven tests with descriptive names; Arrange–Act–Assert.
- **Isolation:** no network, no real `$HOME`, no real accounts. Use `t.TempDir()`, `t.Setenv("HOME", …)`, and fake
  `gh` / `glab` / `ssh` executables placed first on `PATH`.
- **Never install anything on the developer's machine.** Tests that need other systems, Git versions or tools run
  in containers (`test/install/docker-test.sh`, CI's Linux matrix) or in CI; at most one container at a time and with
  a CPU limit, so the machine stays usable.
- **Integration tests** run the real `git` binary against temporary repositories.
- **End-to-end tests** (`test/e2e`, `make e2e`, `GITPERSONA_E2E=1`) drive the web console in headless Chrome
  against a built `gitpersona` and a demo home: user flows, both languages, no sideways scrolling at 375px or on a
  desktop, light and dark mode, and no script error or CSP violation.
- **Golden files** for generated configuration live in `testdata/`; update them only on purpose.
- **Platforms:** every change passes `-race` tests on Apple silicon and Intel Macs and on Linux x86_64 and arm64,
  plus Debian, Alpine, Fedora and Ubuntu 22.04 (the oldest supported Git) containers, and the install script on all
  four systems (CI).

## 10. Git workflow

- **Branching:** GitHub Flow. `main` is always releasable; work happens on short-lived branches (`feat/…`, `fix/…`,
  `docs/…`, `refactor/…`, `test/…`, `ci/…`, `chore/…`).
- **Commits:** Conventional Commits, subject at most 72 characters, imperative, enforced by the `commit-msg` hook
  (`make hooks`). One logical change per commit; the body says *why*. Reference issues in the body (`Fixes #123`).
- **Squash** fix-up commits ("address review", "fix typo") before merging; keep separate commits only for
  independent layers of one change. Pull requests are squash-merged with a Conventional Commit title.
- Commits are authored by the contributor's own account, with no tool or bot attribution trailers.
- **Never** force-push to `main` or rewrite published history; tags are never moved or deleted.

## 11. Versioning, changelog and releases

- SemVer. Before 1.0, minor versions may contain breaking changes; they are called out in the changelog and in the
  release notes' "Before you upgrade".
- Every user-visible change adds a line under **Unreleased** in `CHANGELOG.md` (Added / Changed / Deprecated /
  Removed / Fixed / Security), in the user's words. Internal refactors and dependency bumps without user impact
  get no entry; a bump that fixes a vulnerability users were exposed to does.
- **Release notes** are product communication, one file per release in `.github/release-notes/vX.Y.Z.md`, in the
  layout of `.github/release-notes/TEMPLATE.md`. Draft with `scripts/release-notes.sh vX.Y.Z`, then write the TODO
  parts. `CHANGELOG.md` stays the complete engineering record.
- **Release:** move Unreleased entries to a new version section, write the release notes, then tag `vX.Y.Z` on
  `main` and push the tag. The release workflow stops when the notes are missing or unfinished, runs the checks,
  builds the macOS and Linux archives and packages, publishes them with checksums and the notes, and updates the
  Homebrew tap. After a release, verify the release page, a download against `checksums.txt`, the install script
  and the tap.

## 12. Documentation

- Public docs are in English; `README.md` has a Simplified Chinese translation in `README.zh-CN.md` that is
  updated in the same pull request.
- Every command change updates the README and the command's `--help` text.
- Statements in the docs must be true of the code: a supported platform is tested in CI, a promise in "Privacy and
  security" is enforced by a test or a rule above.
- Product requirements and planning are internal and are not published (`docs/prd/` is git-ignored).

## 13. Definition of done

A change is done when:

- [ ] it starts from an issue, and a feature had a maintainer's go-ahead;
- [ ] tests were written first and pass with `-race` on macOS and Linux (CI);
- [ ] `make check` and `make lint` pass, coverage is at least 95%, scripts are ShellCheck clean;
- [ ] no real identities, local paths or secrets (`make privacy`);
- [ ] errors are handled and messages are actionable, in both languages where the console shows them;
- [ ] `CHANGELOG.md`, README (both languages) and help text are updated where relevant;
- [ ] the author understands and has verified every line, including AI-assisted parts;
- [ ] the pull request is reviewed and merged with a Conventional Commit title.
