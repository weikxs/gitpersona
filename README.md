# GitPersona

English | [简体中文](README.zh-CN.md)

**Git profiles that follow your directories.**

Work in `~/Projects/acme` and you commit, push and open pull requests as your work account.
Switch to `~/Projects/personal` and you are your personal account again. No commands to run, nothing to remember.

> **Status: early development, not released yet.** GitHub profiles, the command line, the configuration web page, shell integration and `gh` switching work; GitLab/Bitbucket support is in progress.

## Why

Many developers use several Git accounts on one machine: personal, work, and client accounts.
Git, `gh` and `glab` each assume one global "current account", so it is easy to commit with the wrong email,
push with the wrong token, or open a pull request as the wrong user.

## What GitPersona will do

- **One directory, one identity.** Bind directories to a profile; every repository inside follows it.
- **One rule set for everything:** commit author and email, HTTPS credentials, SSH key, commit signing,
  and the `gh` / `glab` account.
- **Works everywhere:** fish, zsh and bash, IDEs and GUI Git clients, scripts and non-interactive shells.
  It uses Git's own `includeIf` instead of rewriting your global config.
- **Never stores tokens.** Credentials stay in `gh`, `glab`, the system keychain and ssh-agent.
- **Configure in a minute.** A local web page discovers the accounts you are signed in to and suggests
  rules from the repositories you already have; you just confirm.
- **Repo pins** for the odd repository that lives outside its usual directory.

```text
~/Projects/personal/**      → octo-alex    (personal, GitHub)
~/Projects/acme/**          → acme-alex    (work, GitHub)
~/Projects/clients/globex/** → globex-alex (client, self-hosted GitLab)
everything else             → acme-alex    (default)
```

## Quick start

1. Sign in to every GitHub account with the GitHub CLI: `gh auth login` (once per account).
2. Run `gitpersona setup` (or `gitpersona setup --web` for the same in your browser). It looks at what you already
   have and puts it together in one step:
   - one profile per account `gh` is signed in to, with its public name and email (or its noreply address);
   - your existing `includeIf` identities, with their SSH keys and signing, which the profiles take over (rules that
     also set other things, such as `url.*.insteadOf`, stay);
   - directories from the remotes and emails of the repositories in your usual code folders (`~/code`, `~/Projects`, …);
   - SSH host aliases from `~/.ssh/config`, such as `git@github-work:…`, and their keys (key files are never opened);
   - pins for repositories that differ from their folder, and the integration for your login shell.

   It shows the result and asks once; everything is backed up first. Folders that belong to an organization it
   cannot attribute are listed for you to choose. In the browser, "Adjust step by step" changes anything before applying.
   Or describe your profiles yourself:

   ```toml
   version = 1
   default = "acme"            # used everywhere no rule matches (optional)

   [[profile]]
   id      = "personal"
   account = "octo-alex"       # your account on the host
   name    = "Alex Octo"       # commit author name
   email   = "1000001+octo-alex@users.noreply.github.com"
   dirs    = ["~/Projects/personal"]

   [[profile]]
   id      = "acme"
   account = "acme-alex"
   name    = "Alex (Acme)"
   email   = "1000002+acme-alex@users.noreply.github.com"
   dirs    = ["~/Projects/acme"]
   ```

3. Run `gitpersona apply`, then `gitpersona doctor`. In any repository, `gitpersona whoami` shows who you are.

## Configuration reference

| Key | Required | Meaning |
|---|---|---|
| `version` | yes | Always `1` |
| `default` | no | Profile id used where no rule or pin matches |
| `shell_integration` | no | `false` records that you do not want GitPersona in your shells, so `doctor` does not suggest it (`setup --no-shell` writes it) |
| `[[profile]] id` | yes | Short name: lowercase letters, digits, `-` |
| `provider` | no | `github` (default), `gitlab`, `bitbucket` or `generic`; tokens for the last three live in your own credential store, one per account (`gitpersona auth login`) |
| `host` | no | Defaults to `github.com` for GitHub |
| `account`, `name`, `email` | yes | Account on the host, commit author name and email |
| `dirs` | no | Directories whose repositories use this profile; the most specific directory wins |
| `ssh_key` | no | Path of the SSH private key Git should use for this profile |
| `signing` | no | `{ method = "ssh", key = "~/.ssh/key.pub" }`, `{ method = "gpg", key = "<key id>" }`, or `{ method = "none" }` |
| `[[pin]] repo`, `profile` | — | Bind one repository to a profile, overriding directory rules |

Paths must be absolute or start with `~/`.

## Commands

| Command | What it does |
|---|---|
| `gitpersona apply [--dry-run] [--yes]` | Generate the Git configuration; `--dry-run` previews, `--yes` allows removing files of deleted profiles |
| `gitpersona whoami [--json] [--check]` | Show which profile Git uses here and why; `--check` fails when Git disagrees |
| `gitpersona use <profile>` | Make the current directory use a profile (keeps your comments in config.toml) |
| `gitpersona remove [dir]` | Stop assigning a directory to a profile |
| `gitpersona pin <profile>` / `unpin` | Bind the current repository to a profile, or remove the pin |
| `gitpersona list [--json]` | List profiles, directories and pins |
| `gitpersona doctor [--fix] [--json]` | Check the setup and explain how to fix problems; `--fix` repairs what it safely can |
| `gitpersona auth login\|logout\|status [--profile <id>]` | Save, remove or check the token of a GitLab, Bitbucket or other profile in your own credential store (the system keychain or Git Credential Manager), one per account |
| `gitpersona shells [--json]` | Show your default shell, the shell you use right now, and installed shells |
| `gitpersona install-shell [--shell fish,zsh,bash]` | Add prompt display (and gh switching) to your shells; defaults to your login shell |
| `gitpersona prompt [--format]` | Print the account Git uses here, for prompts (`{account}`, `{profile}`, `{host}`) |
| `gitpersona rollback [<name>\|latest] [--yes]` | List backups, or restore one (the current state is backed up first) |
| `gitpersona uninstall [--yes]` | Remove GitPersona from your Git configuration and put back the `includeIf` rules it took over; keeps config.toml |
| `gitpersona setup [--yes] [--no-shell] [--keep-old-rules] [--default <profile> \| --no-default] [--print \| --web]` | Set GitPersona up from what you already have in one step (accounts, existing identities, repositories, SSH aliases, shell); asks before writing unless `--yes`; `--web` shows it in the browser, where you can also adjust it step by step |
| `gitpersona ui [--no-open]` | Open the configuration page in your browser: profiles, directories, pins, a diff preview before applying, health checks, shells and backups. It listens only on 127.0.0.1, the link works once, and it stops after 15 minutes without use |
| `gitpersona help <command>` | Show the flags of a command |

Prompt and plugin-manager setups (tide, Powerlevel10k, Starship, fisher, oh-my-zsh): see [docs/integrations.md](docs/integrations.md).

## How it behaves

- **Precedence:** a pin, then the most specific directory rule, then `default`. Git applies the rules by the
  location of the repository's `.git` directory, so a **worktree follows its main repository**. Outside any
  repository only `default` applies; `whoami` also tells you which profile new clones will get.
- **Your own `includeIf` rules:** GitPersona's rules come after yours, so in the directories you give to profiles
  (and in pins) the profile decides; everywhere else your own rules work exactly as before. A default profile applies
  everywhere, so while you keep identity rules of your own, `setup` proposes none (`--default <profile>` sets one
  anyway, and says which of your rules it overrides). Rules that do more than a profile can — x509 signing, an SSH
  command with a port or proxy, a key used only for tags, with or without an identity — keep their folders, and so do
  the folders around them. A rule that never applies today (a later rule or global identity of yours overrides it,
  in the order Git reads `~/.config/git/config`, `~/.gitconfig` and the files they include) is not protected.
  Profiles reset signing and the SSH command only where another identity's key could otherwise reach them. A
  repository of another signed-in account is pinned to it, so it pushes with that account. `setup` removes the
  rules a profile fully takes over; `uninstall` puts them back before the section (or rule) that followed them.
  `doctor` says which of your rules a profile overrides.
- **Credentials:** for GitHub profiles, GitPersona is the only credential helper for that host and borrows the
  token `gh` already holds, so no other helper stores a copy. If an account is not signed in to `gh`, Git asks
  for a password; `gitpersona doctor` tells you which account to sign in. For GitLab, Bitbucket and other hosts, your
  own credential helper (the system keychain, Git Credential Manager) keeps one token per account and host, as `gh`
  does for GitHub: `gitpersona auth login` saves it there (hidden input; never a file in clear text), and each
  profile only names its account, so the right token answers. GitPersona keeps no copy.
- **gh follows your directories too:** with the shell integration, running `gh` in a profile's directory uses that
  profile's account. GitPersona puts the account's token only into the environment of that one `gh` process; it is
  not printed or stored. It leaves `gh` alone when you set `GH_TOKEN`/`GITHUB_TOKEN`, use `GH_HOST` for another host,
  or no GitHub profile applies. `glab` follows your directories the same way: in a GitLab profile's directory it runs
  with that account's token from your credential store (`GITLAB_TOKEN`, only for that process), unless you set a token
  or another host yourself.
- **Shells:** your default shell is read from the system user record (not `$SHELL`, which terminals, IDEs and
  tools often change), the shell you use right now from the process tree, and installed shells from `/etc/shells`.
  fish gets its own file in `conf.d`; zsh uses `~/.zshenv`; bash uses `~/.bashrc`. In tide add the
  `gitpersona` item; in Powerlevel10k add the `gitpersona` segment; elsewhere call `gitpersona_prompt`.
- **Privacy:** GitPersona never opens, reads, lists or uses your private keys, and never prints or stores tokens.
- **Your files:** it adds one marked block to your global Git configuration (following a symlinked
  `~/.gitconfig` and `GIT_CONFIG_GLOBAL`), backs up everything it changes, and keeps the newest 20 backups in
  `~/.config/gitpersona/backups/`.

## Install

Not released yet. Releases will be published through Homebrew:

```sh
brew install weikxs/tap/gitpersona
```

To build from source (Go 1.25 or newer):

```sh
git clone https://github.com/weikxs/gitpersona.git
cd gitpersona
make build
./bin/gitpersona version
```

## Contributing

Contributions are welcome. Please read [CONTRIBUTING.md](CONTRIBUTING.md) and the
[development standards](docs/development/standards.md) first.

## Security

Please report vulnerabilities privately; see [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE)
