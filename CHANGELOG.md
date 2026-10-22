# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Security

- GitHub profiles no longer chain the user's own credential helpers, which could store a copy of the `gh` token.
- `doctor` checks sign-in with `gh auth status` and never fetches a token; private keys are never read.

### Added

- `use`, `remove`, `pin`, `unpin` and `list [--json]`: edit the configuration from the command line while keeping
  your comments and layout; every edit is verified, backed up and applied. `doctor` warns about pins to missing repositories.
- `apply --dry-run` and `apply --yes`; per-command help (`gitpersona help <command>`).
- Quick start, configuration reference and behavior notes in the README.
- `gitpersona apply`: generates per-profile Git configuration and a rules index of `includeIf` entries
  (default profile, then directory rules from least to most specific, then repository pins), adds one
  include to `~/.gitconfig` inside a marked block, backs up every file it changes, and does nothing when
  everything is already up to date.
- `gitpersona whoami` (and `--json`): shows which profile applies to the current repository and why,
  and warns when Git currently uses a different email.
- `gitpersona doctor`: checks the configuration, whether generated files are current, and whether each
  GitHub account is signed in to `gh`; never prints tokens.
- Git credential helper for GitHub: answers HTTPS requests with the profile's account using the token
  `gh` already holds, and stays silent otherwise so the user's own helpers still work.
- Directory rules work when a directory is reached through a symbolic link.

### Fixed

- A symlinked `~/.gitconfig` is updated through the link; `GIT_CONFIG_GLOBAL` is honored; a read-only file is refused.
- Upgrading gitpersona with a package manager no longer breaks the credential helper.
- `whoami` reports exactly what Git applies in worktrees and outside repositories.
- Line breaks and other control characters in the configuration are rejected instead of breaking Git.
- `apply` asks for `--yes` before removing files and keeps only the newest 20 backups.
- `apply` keeps the GitPersona block last, so later `git config --global` settings or `gh auth setup-git`
  cannot silently override profiles; `doctor` flags settings after the block.
- `whoami` works in bare repositories and inside `.git`, and warns about `GIT_AUTHOR_EMAIL` /
  `GIT_COMMITTER_EMAIL` and repository-local `user.email` with the exact fix; `doctor` checks the current repository.
- Directory names containing `[`, `*` or `?` match literally.
- `doctor` no longer reports accounts as signed out when `gh` cannot be queried.
- Blank names, emails with angle brackets and a `.pub` file as `ssh_key` are rejected.
- Project scaffold: Go module, `gitpersona version` / `help` commands, Makefile, CI, release configuration,
  privacy check, contribution and development standards.
