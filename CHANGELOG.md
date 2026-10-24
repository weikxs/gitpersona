# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Security

- Built with Go 1.26.10, which fixes 46 vulnerabilities in the Go standard library that v0.1.0 (built with Go 1.26.0)
  contained in code GitPersona can reach, mostly in `net/http`, `crypto/tls`, `crypto/x509` and `html/template`
  (GO-2026-4599 to GO-2026-6809; `govulncheck` lists them). GitPersona's own code and its dependencies had none.
  The web console only listens on 127.0.0.1, which limits the exposure, but every user should upgrade.

### Added

- `install.sh`: one command installs GitPersona on macOS and Linux
  (`curl -fsSL https://raw.githubusercontent.com/weikxs/gitpersona/main/install.sh | sh`). It picks the build for the
  system and CPU, checks it against the release's `checksums.txt`, installs to `~/.local/bin` (or `/usr/local/bin` as
  root) without `sudo`, removes the macOS quarantine flag and adds the directory to PATH in the shell's startup file
  once. `--version`, `--dir` and `--no-modify-path` change that; running it again upgrades. CI runs it on Apple
  silicon and Intel Macs and on Linux x86_64 and arm64.

### Changed

- Release builds use the newest Go 1.26 patch release instead of Go 1.26.0, so Go's security fixes reach the
  binaries; v0.1.0 was built with Go 1.26.0.
- Every release file carries signed build provenance: `gh attestation verify <file> --repo weikxs/gitpersona` shows
  it was built by this repository's release workflow from the tag.
- Each release has hand-written release notes in `.github/release-notes/vX.Y.Z.md`, following
  `.github/release-notes/TEMPLATE.md`: who the release is for, what you can do now, what's new by theme, privacy and
  security, install with a table of downloads by platform, compatibility. The release page shows that file instead
  of a list of commits with their hashes and author emails; a release stops when it is missing or unfinished.
  `scripts/release-notes.sh` drafts one from this changelog.

## [0.1.0] - 2026-10-24

The first release: macOS (Apple silicon and Intel) and Linux (x86_64 and arm64).

### Security

- GitHub profiles no longer chain the user's own credential helpers, which could store a copy of the `gh` token.
- `doctor` checks sign-in with `gh auth status` and never fetches a token; private keys are never read.

### Added

- `doctor` shows the Git version and fails below Git 2.31, the oldest GitPersona supports. On Git before 2.36 it says
  that an `includeIf "hasconfig:remote.*.url:…"` rule never applies (that Git ignores such rules), instead of where it
  does not apply.
- Linux packages for x86_64 and arm64: `.deb`, `.rpm`, `.apk` and Arch (`.pkg.tar.zst`), installing
  `/usr/bin/gitpersona`. CI now runs on Apple silicon and Intel Macs, Linux x86_64 and arm64, and in Debian, Alpine,
  Fedora and Ubuntu 22.04 containers.
- `gitpersona verify` and the page's **Repositories** section check every repository in the profiles' folders and
  pins (and folders you name): the author its commits use and the account each remote pushes as (HTTPS: the account
  the credential helper answers for; SSH: the profile that owns the key ssh offers first), next to the profile that
  should decide. It flags commits or pushes as another identity, tokens written in remote URLs, and SSH connections
  shared between keys; it reads only Git and SSH settings. `--ssh` (a checkbox on the page) signs in to each SSH host
  once, as a push would, so the host names the account; ssh uses the key, GitPersona never reads it.
- GitLab, Bitbucket and other hosts with several accounts each, like `gh` on GitHub: a profile names its account for
  its host and your own credential helper (system keychain, Git Credential Manager) answers with that account's
  token. `gitpersona auth login|logout|status` saves, removes or checks a profile's token there (hidden input, never
  in clear text); `doctor` warns when no helper, or the plain-text `store` helper, would keep them.
- `gitpersona setup` (and the page) turns existing GitLab, Bitbucket and other-host identities into profiles, also
  without `gh`: those whose `includeIf` file names the account for the host, or whose repositories all push to one
  such host as one user. A rule that names accounts for several hosts stays yours.
- `gitpersona check-ssh` connects to each profile's host with its SSH key (`ssh -T`, batch mode, only that key) and
  reports which account GitHub, GitLab or Bitbucket says it is. It runs only when you ask; ssh uses the key,
  GitPersona never opens it.
- `doctor` warns when only the `cache` helper keeps the tokens of a host, and finds helpers set for a URL pattern
  such as `credential.https://*.example.com.helper`, and the system-wide helper many installations set. A helper
  set only for `https://<user>@host` does not count: Git uses it only for remotes whose URL names that user.
  `auth login` refuses to save a token where a push would not find it (a helper set only for the account, or
  `credential.useHttpPath`) or in clear text, including `store` set for every host.
- `glab` follows your directories: in a GitLab profile's directory it runs with that account's token, which goes only
  to that one process.
- One-step setup: `gitpersona setup` and `setup --web` look at what you already have — the accounts signed in to gh,
  existing `includeIf` identities with their SSH keys and signing, the remotes and emails of the repositories in your
  usual code folders, and SSH host aliases in `~/.ssh/config` (key files are only checked to exist, never opened) — and
  organize them into profiles, directory rules, pins, the default profile and the login shell's integration. The page
  opens on the finished proposal ("Your setup is ready"; one click applies it, "Adjust step by step" changes it); the
  command line shows it and asks once (`--yes`, `--no-shell`, `--keep-old-rules`). The `includeIf` rules the profiles
  fully replace are removed in the same backup; rules whose files also set other things (such as `url.*.insteadOf`)
  stay, and setup says why. A first setup takes seconds instead of minutes.
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

### Changed

- The README shows every section of the console (English and Chinese, light and dark) and the terminal output of
  `setup`, `verify` and `doctor`, with a diagram of how it works and a troubleshooting table. The pictures come from
  a demo home with made-up accounts, and `make screenshots` fails if one would show a local path.
- `signing = { method = "none" }` now also makes `git commit -S` refuse, instead of signing with a key from your
  global settings or another profile.
- Your own `includeIf` rules and GitPersona have a clear boundary: GitPersona decides in the directories you give to
  profiles and in pins, your rules everywhere else, as before. While you keep identity rules of your own, `setup`
  proposes no default profile, which would override them everywhere (`--default <profile>` sets one anyway and lists
  the rules it overrides). The folders of rules that do more than a profile can (x509 signing, extra `ssh` options, a
  key used only for tags) stay with those rules. `doctor` says which of your rules a profile overrides; profiles reset
  signing and the SSH command when your rules set them, so a profile's folder inside one of your rules never uses its
  key. `uninstall` puts back the rules setup or `doctor --fix` removed, before the section that followed them; `whoami` shows the author Git
  really uses and where it comes from. GitPersona's own settings are read from your dotfile includes and
  `~/.config/git/config` too.
- A configuration whose GPG signing key is a file is refused (it made every commit fail); use `method = "ssh"`.

### Fixed

- `doctor` shows the paths of `gh` with `~` (where gh was found, and a gh that is not GitPersona's), like every other
  path, instead of the full path of the home folder.
- On the page, a backup's **Restore** button stays beside it when the list of files is long.
- Whether paths ignore case is asked of the file system holding your home folder (read only), not assumed from the
  system: on a Mac formatted with case-sensitive APFS, `~/Work` and `~/work` are no longer one folder.
- On Linux, doctor suggests a credential helper that is installed (Git Credential Manager or libsecret's), and
  otherwise says which to install, instead of always `libsecret`.
- A profile's SSH command turns off connection sharing (`-o ControlPath=none`): with `ControlMaster` in
  `~/.ssh/config`, a push could reuse a connection another key had opened and push as that key's account.
- `check-ssh` and `verify --ssh` read the host's greeting even when ssh's debug output splits it (it did in about
  one connection in three, which reported that the host named no account).
- `check-ssh` recognizes GitHub accounts with an underscore (enterprise managed users, `name_short`).
- A repository in a profile's folder that names another signed-in account for pushes (`credential.<url>.username`,
  or `user@` in the remote URL) pushes as that account with its own `gh` token, instead of getting no token at all;
  when `gh` has no token for it, the helper stays silent as before.
- `doctor` warns that one of your remote-URL `includeIf` rules does not apply in the profiles' folders only when a
  repository there matches it, and names one of them.
- An empty `GH_TOKEN` or `GITHUB_TOKEN` in the shell no longer keeps `gh` from getting the profile's token.
- Your rules that set no identity but a signing key, `tag.gpgsign`, `gpg.format` or an SSH command keep their folder
  like the rules that do more than a profile can; `use`, `pin` and `doctor` warn when a profile still covers one.
- Resets are written per folder: a profile includes its reset (signing, SSH command) only in the folders and pins that
  another profile's key or one of your rules can reach, also through symbolic links, so `GIT_SSH` keeps working
  everywhere else; the default resets only under your rules that set an identity.
- Whether one of your rules applies is judged in Git's real read order across `~/.config/git/config`,
  `~/.gitconfig` and the files they include: a rule a later global identity or rule overrides never applied, and setup
  and `doctor` say so instead of protecting it. Branch and remote rules (`onbranch`, `hasconfig`) that set an identity
  are announced as not applying in the profiles' folders.
- A repository of another signed-in account is pinned even when only its owner says so, so it pushes with that
  account's credentials.
- `uninstall` puts a rule back before the next rule that followed it when the section after it is gone; `remove`
  shows `~`; `whoami`'s author follows `GIT_AUTHOR_EMAIL`.
- `includeIf` rules in `~/.config/git/config` and in files your global configuration includes count as your own rules
  (no default is proposed over them, `doctor` reports overrides, the same identity in several files is one); a plain
  one a profile takes over is reported as no longer needed, since GitPersona does not edit those files.
- Setup does not give a profile a folder that contains one of your rules that does more than a profile can; it lists
  the profile folders inside a kept rule ("Except in …"), and announces folders where a rule of yours never applied
  (a later rule of yours overrode it) but will after setup. `use` warns when it overrides such a rule, and setup says
  when the default's SSH key applies outside your directories. `whoami` names the file the author's email comes from.
- The GitPersona block keeps the line endings of a CRLF `~/.gitconfig`. `rollback` recreates folders that are gone
  (for example after `uninstall`) and reads every saved file before it starts writing.
- Setup ties an existing `includeIf` identity that names no account to the right one by what is on disk: the key of
  that account's SSH alias, or the repositories in its folders or that set its email. The folders of existing rules
  are scanned too, along with `~/Developer`, `~/workspace` and `~/Documents/GitHub`. An identity it still cannot tie
  is shown with the profile that would override it, and on the page you can choose its account.
- Removing old `includeIf` rules (`doctor --fix`, the page's button, setup) only removes a rule when a profile with
  the same email covers its folder; before, a rule whose identity no profile had could be deleted. `doctor` now warns
  when a profile overrides such a rule, and reports rules kept on purpose as fine.
- Setup pins repositories of another account in subfolders without a rule of their own, a scanned folder that is a
  repository itself, and repositories kept inside other repositories; an https remote that names its user
  (`https://user@github.com/...`) counts as evidence. Reasons name every SSH alias the repositories use.
- Setup says when looking for repositories stopped at its time limit (now 10 seconds) instead of silently showing
  partial counts, and says which keys of an existing setup it left out because their files are missing.
- Declining the shell integration that setup offers (`setup --no-shell`, or unticking every shell on the page) writes
  `shell_integration = false`, so `doctor` and `doctor --fix` no longer push it.
- A profile resets signing and the SSH command only where another profile's (or one of your rules') key could
  otherwise reach its repositories: under the default, or inside another profile's folder, also through symbolic
  links. Elsewhere it leaves them alone, so `git commit -S` never uses another identity's key, and `GIT_SSH` keeps
  working where nothing needs a reset.
- Setup no longer trusts weak clues: a single repository an account owns no longer ties an
  existing identity to that account, the strongest clue wins when two identities point to one account, and a pin for
  a repository outside every rule follows the default you choose.
- `includeIf` rules written with the home's absolute path are shown and stored as `~/...`.
- The preview on the "Your setup is ready" page follows the boxes you tick or untick.
- An existing identity that takes `gpg.format` or `commit.gpgsign` from your global settings (also from files they
  include, such as dotfiles) is imported with that signing; `gpg.format = ssh` was read as GPG, which made every
  commit fail. `doctor` fails on a GPG key that is a file path.
- The folder of an identity setup cannot tie to an account gets no rule or pin from the repositories in it, which
  would otherwise replace that identity; folders inside it that clearly belong to an account still get their rule.
- An SSH alias is tied to the account whose existing key it uses, or else to the account most of its repositories
  belong to (one vote each); an alias used for several accounts' repositories is tied to none. Reasons say whose
  alias a repository uses.
- Suggested rules inside a rule for the same profile are merged into it.
- An existing identity without `user.name` keeps your global Git name instead of the GitHub login; setup says where
  each name comes from. Identities with the same email but another author name are no longer merged, and an old rule
  is removed only when its name matches too (or you ticked its removal on the page).
- Setup names the default profile's identity, has `--no-default`, says when folders were too deep to look in (it now
  looks five levels deep) or when a folder was cut off by the time limit, and `--print` lists what it leaves out.
- `doctor` warns when `git commit -S` in a profile without signing would use another profile's SSH key (the default
  or an enclosing directory's; Git cannot unset a key).
- Identities whose SSH signing key is written as the key itself (`ssh-ed25519 …` or `key::…`, as with an SSH agent or
  a password manager) are imported; the configuration accepts such keys, and refuses a GPG key that is a file.
- `gitdir:…/**/` rules are read as their folder; a single repository directly in a scanned folder is pinned instead
  of making the whole folder a rule; when the scan hits its time limit it names the folders it did not finish.
- Commands without a configuration show `~` and suggest `gitpersona setup`; identities without a name show the global
  name; the setup page suggests the shells only while they lack the integration.
- `doctor` without a configuration suggests `gitpersona setup`; the page's Profiles and Review & apply sections
  point to setup until there is a configuration; long checkbox labels wrap beside the box at phone width.
- `signing = { method = "none" }` turns commit and tag signing off even when you sign everything globally; before,
  it did nothing and commits still tried to sign.
- The configuration page compares a directory typed as an absolute path with the stored `~/` form: moving a
  directory from another profile always asks first, duplicates are caught in the setup steps, and messages show `~`.
- Error messages on the page (for example a `config.toml` that does not parse) show the home directory as `~`; with a
  broken configuration the page explains how to fix it or restore a backup instead of showing an empty Profiles view.
- The setup steps follow the browser's Back and Forward buttons and survive a reload.
- Setup step 1 names the identity used outside your directories today and offers "No default" to keep it; a name
  taken from the login (no public name on GitHub) says so.
- Hand-made `includeIf` identity rules can be removed from the page or with `doctor --fix`, after a backup; only
  rules a profile fully replaces are removed, and comments (including GitPersona's block markers) are kept.
- `~/.gitconfig` includes the rules as `~/.config/...` instead of an absolute path, so a dotfiles repository works on
  every machine (the next `apply` updates it).
- Pinning a repository reached through a symlink stores the path as you reached it.
- The Shells page lists the startup files it changes, with a preview, before writing them.
- After a failed change caused by an edit outside the page, the page shows the configuration as it now is.
- Plainer words: backups say what they were taken before ("before adding a directory to work"), success is shown
  once, and the `gh` check names the shell that started GitPersona. The "already open" message says how to stop a
  page whose tab or terminal is gone. Buttons on profile cards name their profile for screen readers, and
  `/static/` no longer lists files.
- Directory suggestions include folders whose repositories belong to an organization: the setup page lists them
  with their owner so you can choose the profile. Symlinked folders such as `~/code` are scanned, and folders that do
  not exist are named instead of reported as having no suggestions.
- Saving a profile whose SSH key or SSH signing key file does not exist is refused, and `doctor` reports it, instead of
  every commit failing later. GitPersona only checks that the file is there; it never opens it.
- `doctor` warns about directory rules for folders that do not exist and about hand-made `includeIf` identity rules
  left from before GitPersona.
- The configuration page asks before moving a directory from another profile, explains what removing the default
  profile changes, reports a restored backup as a restore, and writes nothing (and makes no backup) when a save
  changes nothing.
- Setup page: Enter in a field adds the directory or scans instead of skipping the step; a profile you leave out no
  longer blocks Next; duplicate directories are caught when added; old messages are cleared.
- Opening a second page no longer ends the first page's session, and only one page can run at a time, so two pages
  never edit the configuration together. After Close the page offers nothing that would fail.
- Diffs on the page show your home directory as `~`, so screenshots do not reveal it.
- New keys are added after a profile's last key and aligned like the others.
- Keyboard focus returns to the button or the result after an action; plainer wording on the Shells page; doctor
  advice links to the matching part of the page; `setup --web` with an existing configuration opens the overview.
- The configuration page checks the format of profile and account names before sending them: the browser had
  ignored the field patterns, because a `-` in a character class must be escaped in the patterns browsers use.
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

[Unreleased]: https://github.com/weikxs/gitpersona/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/weikxs/gitpersona/releases/tag/v0.1.0
