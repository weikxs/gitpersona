#!/bin/sh
# Drafts the release notes of one version from its section of CHANGELOG.md,
# in the layout of .github/release-notes/TEMPLATE.md:
#
#   scripts/release-notes.sh v0.2.0 > .github/release-notes/v0.2.0.md
#
# The draft fills in what the changelog knows (date, summary, upgrading
# steps, new features, changes, fixes, security, downloads, links) and
# leaves TODO comments where a person writes: the theme, who the release is
# for and what you can do now. Sections with nothing to say are left out.
# The release workflow publishes the edited file, not this draft. Fails when
# the changelog has no section for the version.
#
# usage: scripts/release-notes.sh <tag> [CHANGELOG.md]
set -eu

tag=${1:?usage: release-notes.sh <tag> [changelog]}
changelog=${2:-CHANGELOG.md}
version=${tag#v}
repo=https://github.com/weikxs/gitpersona

# trim drops leading and trailing blank lines.
trim() { sed -e '/./,$!d' | sed -e ':a' -e '/^\n*$/{$d;N;ba' -e '}'; }

# section prints the lines under "### $1" in this version's section.
section() {
	awk -v version="$version" -v want="$1" '
		/^## \[/ { inside = index($0, "## [" version "]") == 1; next }
		!inside { next }
		/^### / { current = substr($0, 5); next }
		current == want { print }
	' "$changelog" | trim
}

# block prints a heading and the CHANGELOG category, unless it is empty.
block() {
	body=$(section "$2")
	if [ -n "$body" ]; then printf '%s\n%s\n\n' "$1" "$body"; fi
}

heading=$(command grep -m1 "^## \[$version\]" "$changelog" || true)
if [ -z "$heading" ]; then
	echo "release-notes: CHANGELOG has no section for $version" >&2
	exit 1
fi
date=${heading##* - }
summary=$(awk -v version="$version" '
	/^## \[/ { inside = index($0, "## [" version "]") == 1; next }
	inside && /^### / { exit }
	inside { print }
' "$changelog" | trim)
# The version released before this one: the next section below it.
previous=$(awk -v version="$version" '
	/^## \[/ { if (found) { sub(/^## \[/, ""); sub(/\].*/, ""); print; exit } if (index($0, "## [" version "]") == 1) found = 1 }
' "$changelog")
upgrading=$(section Upgrading)
removed=$(section Removed)
deprecated=$(section Deprecated)

cat <<EOF
# GitPersona $tag — <!-- TODO: one-line theme -->

Released on \`$date\`.

> GitPersona gives each directory its own Git identity: commits, pushes, \`gh\` and \`glab\` use the account that
> belongs to the folder you are in, in every tool that runs Git, and \`gitpersona verify\` proves no repository uses
> the wrong one.

## Summary

${summary:-<!-- TODO: two or three sentences: what this release is about and what changes for you. -->}

## Who this release is for

<!-- TODO: the person this release helps most, specifically. -->

EOF
if [ -n "$upgrading$removed$deprecated" ]; then
	printf '## Before you upgrade\n\n'
	if [ -n "$upgrading" ]; then printf '%s\n\n' "$upgrading"; fi
	block "### Removed" Removed
	block "### Deprecated" Deprecated
fi
cat <<EOF
## What you can do now

<!-- TODO: 3–6 short paragraphs, one per capability, from the reader's side. -->

EOF
if [ -n "$(section Added)$(section Changed)" ]; then
	printf "## What's new\n\n"
	block "### New" Added
	block "### Changed" Changed
fi
block "## Bug fixes" Fixed
cat <<EOF
## Privacy and security

EOF
security=$(section Security)
if [ -n "$security" ]; then printf '%s\n' "$security"; fi
cat <<EOF
- **Never stores tokens.** GitHub tokens stay in \`gh\`; other hosts' tokens stay in your system keychain or Git
  Credential Manager. GitPersona keeps no copy and never prints them.
- **Never reads your keys.** Private key files are only checked to exist; ssh uses them, GitPersona never opens them.
- **Local only.** The web console listens only on 127.0.0.1, behind a link that works once, and stops after 15
  minutes without use. There is no telemetry.
- **Only when you ask.** It connects to the network only to read your accounts' public names and emails during
  setup, and to the SSH hosts when you run \`check-ssh\` or \`verify --ssh\`.

## Install

- Script (macOS and Linux, fully automatic): \`curl -fsSL https://raw.githubusercontent.com/weikxs/gitpersona/main/install.sh | sh\`
- Homebrew (macOS and Linux): \`brew install weikxs/tap/gitpersona\`, or \`brew upgrade gitpersona\`
- Go 1.26+: \`go install github.com/weikxs/gitpersona/cmd/gitpersona@$tag\`
- A package or archive below; check it against \`checksums.txt\`

| File | Platform |
|---|---|
| \`gitpersona_${version}_darwin_arm64.tar.gz\` | macOS, Apple silicon |
| \`gitpersona_${version}_darwin_amd64.tar.gz\` | macOS, Intel |
| \`gitpersona_${version}_linux_amd64.deb\` / \`_arm64.deb\` | Debian, Ubuntu (x86_64 / arm64) |
| \`gitpersona_${version}_linux_amd64.rpm\` / \`_arm64.rpm\` | Fedora, RHEL (x86_64 / arm64) |
| \`gitpersona_${version}_linux_amd64.tar.gz\` / \`_arm64.tar.gz\` | Any Linux, Alpine and Arch too (x86_64 / arm64) |

Then sign in to each GitHub account once with \`gh auth login\`, and run \`gitpersona setup --web\`.

## Compatibility

- macOS 12 or newer on Apple silicon and Intel; Linux on x86_64 and arm64 (glibc or musl: Debian, Ubuntu, Fedora,
  Arch, Alpine and others). Windows is planned.
- Git 2.31 or newer (2.36 or newer if you have your own \`includeIf "hasconfig:…"\` rules).
- For GitHub profiles, the GitHub CLI 2.40 or newer; for other hosts, a credential helper such as the system keychain
  or Git Credential Manager.
- The web console is in English and Simplified Chinese; the command line is in English.

## Notes

- The macOS binary is not signed or notarized by Apple; Homebrew and the install script remove the quarantine flag.
  By hand: **Open Anyway** in System Settings → Privacy & Security. See
  [SECURITY.md]($repo/blob/$tag/SECURITY.md#release-binaries-and-macos).
EOF
if [ -n "$previous" ]; then
	printf -- '- **Full changelog:** [v%s...%s](%s/compare/v%s...%s) · [CHANGELOG.md](%s/blob/%s/CHANGELOG.md)\n' \
		"$previous" "$tag" "$repo" "$previous" "$tag" "$repo" "$tag"
else
	printf -- '- **Full changelog:** [CHANGELOG.md](%s/blob/%s/CHANGELOG.md)\n' "$repo" "$tag"
fi
cat <<EOF
- Documentation: [README]($repo#readme) · [简体中文]($repo/blob/main/README.zh-CN.md)
- Feedback and problems: [issues]($repo/issues).
EOF
