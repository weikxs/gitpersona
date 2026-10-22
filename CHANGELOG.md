# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

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
- Project scaffold: Go module, `gitpersona version` / `help` commands, Makefile, CI, release configuration,
  privacy check, contribution and development standards.
