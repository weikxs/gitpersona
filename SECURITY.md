# Security Policy

GitPersona handles developer identities and decides which credentials Git uses, so we treat security
reports as the highest priority.

## Supported versions

Until 1.0, only the latest release receives security fixes. After 1.0, the latest minor release of the
current major version is supported.

## Reporting a vulnerability

**Do not open a public issue.** Report privately through
[GitHub Security Advisories](https://github.com/weikxs/gitpersona/security/advisories/new).

Please include:

- the affected version (`gitpersona version`) and operating system;
- steps to reproduce, with real account names, emails and paths replaced by placeholders;
- the impact you expect (for example, a token reaching the wrong host).

## What to expect

| Step | Target |
|---|---|
| Acknowledge the report | within 3 days |
| Initial assessment | within 7 days |
| Fix released for confirmed critical issues | as soon as possible, normally within 30 days |

We credit reporters in the release notes unless they prefer to stay anonymous.

## Design commitments

- GitPersona never opens, reads, copies or uploads private keys and never queries ssh-agent. It only checks
  that the key files you configure exist; `setup` reads `~/.ssh/config` for host aliases. Key paths are passed to
  Git unchanged. ssh uses a key only for your own Git operations and for `gitpersona check-ssh`, which runs only
  when you start it.
- GitPersona fetches a token only when Git asks for a credential for a matching host and account, when you run
  `gh` / `glab` in a directory with a profile (the token goes only to that one process), or when you run
  `gitpersona auth status`. It never writes tokens or passwords to disk, logs, backups or the web UI;
  `gitpersona auth login` hands the token you type straight to your own credential helper and refuses the
  plain-text `store` helper.
- The configuration web UI listens on `127.0.0.1` only, opens with a one-time link, uses CSRF protection and a
  strict Content Security Policy, loads nothing from outside, and stops after 15 minutes without use.
- When GitPersona cannot determine the right identity, it does nothing and leaves Git's default behavior.

## Release binaries and macOS

Release binaries are built by GitHub Actions from the tagged source, with checksums in `checksums.txt`.
They are not yet signed or notarized by Apple. The Homebrew cask therefore removes the quarantine attribute from
the installed `gitpersona` binary after installing it (`xattr -dr com.apple.quarantine`), as GoReleaser documents
for unsigned binaries; otherwise macOS would refuse to run it. If you prefer to keep Gatekeeper's check, download
the archive from the releases page, verify it against `checksums.txt`, and install the binary yourself.
