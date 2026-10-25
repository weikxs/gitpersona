<!--
  Release notes template. It has the layout of the v0.1.0 release
  (https://github.com/weikxs/gitpersona/releases/tag/v0.1.0, file v0.1.0.md);
  keep every release in the same order and voice.

  One file per release: .github/release-notes/vX.Y.Z.md. The release workflow
  publishes it, word for word, as the GitHub release page, and stops when the
  file is missing or still contains TODO. Draft one from the changelog:

      scripts/release-notes.sh vX.Y.Z > .github/release-notes/vX.Y.Z.md

  Voice: product communication for people deciding whether to install or
  upgrade, not a developer changelog (CHANGELOG.md is that). Speak to "you",
  say what the reader notices, not how it was built. No commit hashes, author
  emails, real names, companies or local paths; examples use the demo
  identities of docs/development/standards.md.

  Sections marked "(only when …)" are left out when they have nothing to say.
  Every other section is always there. Delete these comments when done.
-->

# GitPersona vX.Y.Z — One-line theme

Released on `YYYY-MM-DD`.

> GitPersona gives each directory its own Git identity: commits, pushes, `gh` and `glab` use the account that
> belongs to the folder you are in, in every tool that runs Git, and `gitpersona verify` proves no repository uses
> the wrong one.

## Summary

<!-- Two or three sentences: what this release is, what changes for you, which systems it runs on. -->

## Who this release is for

<!--
  One or two sentences naming the person it helps most, specifically, then an
  "If you have ever:" list of 3–5 real moments, then one sentence on how this
  release changes them. As in v0.1.0:

  You keep more than one Git account on one machine — ...

  If you have ever:

  - noticed a work commit signed with your personal email, after it was pushed
  - ...

  then GitPersona ...
-->

## Before you upgrade

<!-- (only when you must act) Breaking changes, removed or renamed flags, steps to run after upgrading, in order. -->

## What you can do now

<!--
  3–6 short paragraphs, one per capability. Each starts with a bold verb
  phrase and ends with the moment you notice it. As in v0.1.0:

  **Set everything up in one step.** Run `gitpersona setup --web`: ...
-->

## What's new

<!--
  Grouped by theme, one "###" per theme (v0.1.0: Identities that follow your
  folders, Setup, Checking repositories, Web console, Shells). Each item: a
  bold lead, what it does, and its boundary (what it does not do).
-->

### Theme

- **Lead.** What it does, and what it does not do.

## Bug fixes

<!-- (only when there are fixes) One line each, in the reader's words: what went wrong, and that it no longer does. -->

## Privacy and security

<!--
  Always present, even when nothing changed. Keep the four promises, then add
  this release's security fixes with their severity (and advisory ID).
-->

- **Never stores tokens.** GitHub tokens stay in `gh`; other hosts' tokens stay in your system keychain or Git
  Credential Manager. GitPersona keeps no copy and never prints them.
- **Never reads your keys.** Private key files are only checked to exist; ssh uses them, GitPersona never opens them.
- **Local only.** The web console listens only on 127.0.0.1, behind a link that works once, and stops after 15
  minutes without use. There is no telemetry.
- **Only when you ask.** It connects to the network only to read your accounts' public names and emails during
  setup, and to the SSH hosts when you run `check-ssh` or `verify --ssh`.

## Install

- Script (macOS and Linux, fully automatic): `curl -fsSL https://raw.githubusercontent.com/weikxs/gitpersona/main/install.sh | sh`
- Homebrew (macOS and Linux): `brew install weikxs/tap/gitpersona`, or `brew upgrade gitpersona`
- Go 1.26+: `go install github.com/weikxs/gitpersona/cmd/gitpersona@vX.Y.Z`
- A package or archive below; check it against `checksums.txt`

| File | Platform |
|---|---|
| `gitpersona_X.Y.Z_darwin_arm64.tar.gz` | macOS, Apple silicon |
| `gitpersona_X.Y.Z_darwin_amd64.tar.gz` | macOS, Intel |
| `gitpersona_X.Y.Z_linux_amd64.deb` / `_arm64.deb` | Debian, Ubuntu (x86_64 / arm64) |
| `gitpersona_X.Y.Z_linux_amd64.rpm` / `_arm64.rpm` | Fedora, RHEL (x86_64 / arm64) |
| `gitpersona_X.Y.Z_linux_amd64.apk` / `_arm64.apk` | Alpine (x86_64 / arm64) |
| `gitpersona_X.Y.Z_linux_amd64.pkg.tar.zst` / `_arm64.pkg.tar.zst` | Arch (x86_64 / arm64) |
| `gitpersona_X.Y.Z_linux_amd64.tar.gz` / `_arm64.tar.gz` | Any Linux (x86_64 / arm64) |

Then sign in to each GitHub account once with `gh auth login`, and run `gitpersona setup --web`.

## Compatibility

- macOS 12 or newer on Apple silicon and Intel; Linux on x86_64 and arm64 (glibc or musl: Debian, Ubuntu, Fedora,
  Arch, Alpine and others). Windows is planned.
- Git 2.31 or newer (2.36 or newer if you have your own `includeIf "hasconfig:…"` rules).
- For GitHub profiles, the GitHub CLI 2.40 or newer; for other hosts, a credential helper such as the system keychain
  or Git Credential Manager.
- The web console is in English and Simplified Chinese; the command line is in English.

## What's coming next

<!-- (only when there is a plan to share) One or two themes of the next release, without dates. -->

## Notes

- The macOS binary is not signed or notarized by Apple; Homebrew and the install script remove the quarantine flag.
  By hand: **Open Anyway** in System Settings → Privacy & Security. See
  [SECURITY.md](https://github.com/weikxs/gitpersona/blob/vX.Y.Z/SECURITY.md#release-binaries-and-macos).
- **Full changelog:** [vA.B.C...vX.Y.Z](https://github.com/weikxs/gitpersona/compare/vA.B.C...vX.Y.Z) ·
  [CHANGELOG.md](https://github.com/weikxs/gitpersona/blob/vX.Y.Z/CHANGELOG.md)
- Documentation: [README](https://github.com/weikxs/gitpersona#readme) ·
  [简体中文](https://github.com/weikxs/gitpersona/blob/main/README.zh-CN.md)
- Feedback and problems: [issues](https://github.com/weikxs/gitpersona/issues).
