# Contributing to GitPersona

Thanks for helping! This page is the short version; the full rules are in
[docs/development/standards.md](docs/development/standards.md).

## Before you write code

- **Bugs and documentation fixes:** welcome straight away. Open an issue with the bug template (it asks for
  `gitpersona version`, `git version`, your system and what happened), or send a pull request that links one.
- **Features, new commands or flags, changes to the generated Git configuration:** open an issue first and wait for
  a maintainer's go-ahead. Pull requests for features without one may be closed — this saves your time.
- **Looking for something to do?** Issues labelled `good first issue` or `help wanted` are open for anyone; comment
  to claim one. `needs-design` means the behavior is not decided yet: propose a design on the issue.
- **Security problems:** never in a public issue; see [SECURITY.md](SECURITY.md).

## Set up

Requirements: Go 1.26+, Git, Make. Optional: Docker (for the install-script and Linux tests), Chrome (for the
web-console tests), `golangci-lint`.

```sh
git clone https://github.com/weikxs/gitpersona.git
cd gitpersona
make hooks     # install the pre-commit and commit-msg hooks
make check     # format, vet, privacy check, tests with coverage (at least 95%)
make lint      # golangci-lint
make e2e       # the web console in headless Chrome
test/install/docker-test.sh   # install.sh in Linux containers
```

Run `make` to see every target. Tests never install anything on your machine and never touch your real home
directory, accounts or keys.

## Workflow

1. Create a branch from `main`: `feat/<topic>`, `fix/<topic>`, `docs/<topic>`, …
2. Write a failing test first, then the code that makes it pass.
3. Keep the pull request to one purpose; split unrelated fixes, refactors and features.
4. Update `CHANGELOG.md` under **Unreleased** for anything users notice; update the README in both languages, the
   `--help` text and the console texts (English and Chinese) when behavior changes.
5. Run `make check` and `make lint`, then open a pull request with the template. CI must be green and a code owner
   must approve; it is squash-merged.

## Commit messages

[Conventional Commits 1.0](https://www.conventionalcommits.org/en/v1.0.0/), enforced by the `commit-msg` hook:

```text
<type>(<optional scope>): <imperative summary, at most 72 characters>

<why the change is needed>

Fixes #123
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`.
Mark breaking changes with `!` and a `BREAKING CHANGE:` footer. Squash "fix typo" and "address review" commits.
Commits carry no tool or bot attribution trailers.

## Using AI tools

You may use AI tools, but you are the author and responsible for every line:

- you understand the change and can explain it in your own words, and you have run and checked it yourself;
- say in the pull request when AI wrote a substantial part, and which;
- write issues, reviews and replies yourself; no autonomous agents opening pull requests or issues;
- reproduce any problem an AI tool found before you report it, and say that AI was used.

## Privacy rule (important)

GitPersona is about identities, so examples are everywhere. **Never put real account names, emails,
organization names or local paths in code, tests, docs, screenshots, issues or AI prompts.** Use the demo
identities (`octo-alex`, `acme-alex`, `globex-alex`, `demo-work`, `demo-personal`) and demo paths
(`~/Projects/…`, `~/code/…`). `make privacy` and CI enforce this.

## Code of conduct

By participating you agree to follow the [Code of Conduct](CODE_OF_CONDUCT.md).
