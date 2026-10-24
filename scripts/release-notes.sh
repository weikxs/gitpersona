#!/bin/sh
# Prints the release notes of one version, built from its section of
# CHANGELOG.md, in the order large projects use: what GitPersona is, the
# summary, then upgrading steps, security, new features, changes,
# deprecations, removals and bug fixes (empty ones are left out), downloads
# and links. The release workflow passes them to GoReleaser, so a release
# never lists commit hashes or author emails. Fails when the version has no
# section.
#
# usage: scripts/release-notes.sh v0.2.0 [CHANGELOG.md]
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
	if [ -n "$body" ]; then printf '## %s\n%s\n\n' "$1" "$body"; fi
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

cat <<EOF
# GitPersona $tag

Released on $date.

> GitPersona gives each directory its own Git identity: commits, pushes, \`gh\` and \`glab\` use the account that
> belongs to the folder you are in, in every tool that runs Git, and \`gitpersona verify\` proves no repository uses
> the wrong one.

EOF
if [ -n "$summary" ]; then printf '## Summary\n%s\n\n' "$summary"; fi
block "Upgrading" Upgrading
block "Security" Security
block "New Features" Added
block "Changes" Changed
block "Deprecated" Deprecated
block "Removed" Removed
block "Bug Fixes" Fixed
cat <<EOF
## Install
- Homebrew (macOS and Linux): \`brew install weikxs/tap/gitpersona\`, or \`brew upgrade gitpersona\`
- Go 1.26+: \`go install github.com/weikxs/gitpersona/cmd/gitpersona@$tag\`
- A package or archive below; check it against \`checksums.txt\`

| File | Platform |
|---|---|
| \`gitpersona_${version}_darwin_arm64.tar.gz\` | macOS, Apple silicon |
| \`gitpersona_${version}_darwin_amd64.tar.gz\` | macOS, Intel |
| \`gitpersona_${version}_linux_amd64.deb\` / \`_arm64.deb\` | Debian, Ubuntu (x86_64 / arm64) |
| \`gitpersona_${version}_linux_amd64.rpm\` / \`_arm64.rpm\` | Fedora, RHEL (x86_64 / arm64) |
| \`gitpersona_${version}_linux_amd64.apk\` / \`_arm64.apk\` | Alpine (x86_64 / arm64) |
| \`gitpersona_${version}_linux_amd64.pkg.tar.zst\` / \`_arm64.pkg.tar.zst\` | Arch (x86_64 / arm64) |
| \`gitpersona_${version}_linux_amd64.tar.gz\` / \`_arm64.tar.gz\` | Any Linux (x86_64 / arm64) |

## Notes
- The macOS binary is not signed by Apple yet; Homebrew removes the quarantine flag. See [SECURITY.md]($repo/blob/$tag/SECURITY.md).
- Windows is planned.
EOF
if [ -n "$previous" ]; then
	printf -- '- **Full changelog:** [%s...%s](%s/compare/v%s...%s) · [CHANGELOG.md](%s/blob/%s/CHANGELOG.md)\n' \
		"v$previous" "$tag" "$repo" "$previous" "$tag" "$repo" "$tag"
else
	printf -- '- **Full changelog:** [CHANGELOG.md](%s/blob/%s/CHANGELOG.md)\n' "$repo" "$tag"
fi
