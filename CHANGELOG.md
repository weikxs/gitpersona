# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Security

- GitHub profiles no longer chain the user's own credential helpers, which could store a copy of the `gh` token.
- `doctor` checks sign-in with `gh auth status` and never fetches a token; private keys are never read.

### Added

- `setup --web` and the page's setup steps: confirm one profile per signed-in account (reusing an existing `includeIf`
  setup), choose directories with suggestions from repository remotes, review every file including `config.toml`,
  then apply in one backup and see the health checks.
- `ui`: a local configuration page for profiles, directories and pins, with a diff preview before applying,
  health checks, shells and backups. It listens only on 127.0.0.1, uses a one-time link, CSRF protection and a strict
  Content Security Policy, loads nothing from outside, works at phone width and in dark mode.
- `use`, `remove`, `pin`, `unpin` and `list [--json]`: edit the configuration from the command line while keeping
  your comments and layout; every edit is verified, backed up and applied. `doctor` warns about pins to missing repositories.
- `rollback` lists backups and restores one (each command's changes are one backup, so a rollback undoes the whole
  operation); `uninstall` removes GitPersona from your Git configuration and keeps `config.toml`.
- Shell integration: `shells` shows the default (login) shell, the shell in use right now and the installed shells;
  `install-shell` adds the integration (to the login shell by default); `init <shell>` prints it; `prompt` prints the
  account for prompts in a few milliseconds, with items for tide and Powerlevel10k. `doctor` checks both the default
  and the current shell, and `uninstall` removes the integration.
- `gh` follows your directories: with the shell integration, `gh` runs as the account of the current directory's
  profile; the token goes only to that one `gh` process.
- `doctor --fix` repairs what it safely can (regenerates files, moves the block last, installs the login shell's
  integration; never removes files) and `doctor --json`; `whoami --check` fails when Git does not use the profile.
- `setup` creates a first configuration from the GitHub accounts `gh` is signed in to, with each account's public
  name and email (its noreply address when the account shows no public email).
- Plugins for fisher (`conf.d/gitpersona.fish`) and oh-my-zsh / zinit / antidote (`gitpersona.plugin.zsh`), and
  prompt recipes for tide, Powerlevel10k, Starship and bash in `docs/integrations.md`.
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

- Profiles without signing restore your own global signing settings instead of turning signing off.
- The oldest backup (the state before GitPersona) is never pruned; backups record which command made them.
- `uninstall` restores startup files exactly, including ones without a final newline; edits keep CRLF line
  endings and accept quoted keys.
- In a profile's directory, `gh` refuses to act as another account when the profile's account is not signed in;
  `gh auth` commands pass through untouched.
- The shell integration keeps GitPersona first on `PATH` even after macOS `path_helper`; bash no longer creates a
  `~/.bash_profile` that hides `~/.profile`.
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
