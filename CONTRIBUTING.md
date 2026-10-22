# Contributing to GitPersona

Thanks for helping! This page is the short version; the full rules are in
[docs/development/standards.md](docs/development/standards.md).

## Set up

Requirements: Go 1.25+, Git, Make. Optional: `golangci-lint` and `goreleaser` (`brew install golangci-lint goreleaser`).

```sh
git clone https://github.com/weikxs/gitpersona.git
cd gitpersona
make hooks     # install the pre-commit and commit-msg hooks
make check     # format, vet, privacy check, tests with coverage
make e2e       # the configuration page in headless Chrome (needs Chrome)
```

Run `make` to see every target.

## Workflow

1. Open or pick an issue, and say you are working on it.
2. Create a branch from `main`: `feat/<topic>`, `fix/<topic>`, `docs/<topic>`, …
3. Write a failing test first, then the code that makes it pass.
4. Keep the change focused. Update `CHANGELOG.md` under **Unreleased** for anything users notice.
5. Run `make check` and `make lint`, then open a pull request using the template.

## Commit messages

[Conventional Commits 1.0](https://www.conventionalcommits.org/en/v1.0.0/), enforced by the `commit-msg` hook:

```text
<type>(<optional scope>): <imperative summary, at most 72 characters>
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`.
Mark breaking changes with `!` and a `BREAKING CHANGE:` footer.

## Privacy rule (important)

GitPersona is about identities, so examples are everywhere. **Never put real account names, emails,
organization names or local paths in code, tests, docs, screenshots or issues.** Use the demo identities:
`octo-alex`, `acme-alex`, `globex-alex`, and paths under `~/Projects/`. `make privacy` and CI enforce this.

## Code of conduct

By participating you agree to follow the [Code of Conduct](CODE_OF_CONDUCT.md).
