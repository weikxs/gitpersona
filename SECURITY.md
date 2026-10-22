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

- GitPersona never opens, reads, copies or uploads private keys, never scans `~/.ssh`, and never queries
  ssh-agent. Key paths you configure are passed to Git unchanged.
- GitPersona fetches a token only when Git asks for a credential for a matching host and account, and never
  writes tokens or passwords to disk, logs, backups or the web UI.
- The configuration web UI listens on `127.0.0.1` only and requires a one-time key.
- When GitPersona cannot determine the right identity, it does nothing and leaves Git's default behavior.
