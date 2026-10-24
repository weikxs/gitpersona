#!/bin/sh
# GitPersona installer for macOS and Linux.
#
#   curl -fsSL https://raw.githubusercontent.com/weikxs/gitpersona/main/install.sh | sh
#
# It picks the build for this system and CPU, downloads it from the GitHub
# release, checks it against the release's checksums.txt, installs the
# gitpersona binary and makes sure the install directory is on PATH. Nothing
# else is changed, and nothing runs as root unless you run the script as root.
#
# Options (or the environment variables in brackets):
#   --version <tag>     install this release, such as v0.1.0 [GITPERSONA_VERSION]; default: the latest
#   --dir <directory>   install here [GITPERSONA_INSTALL_DIR]; default: ~/.local/bin, or /usr/local/bin as root
#   --no-modify-path    do not add the directory to your shell's startup file [GITPERSONA_NO_MODIFY_PATH=1]
#   --help              show this help
#
# Run it again to upgrade. Uninstall: `gitpersona uninstall`, then delete the binary.
set -eu

REPO="weikxs/gitpersona"
BASE_URL="${GITPERSONA_BASE_URL:-https://github.com/$REPO/releases}"
PATH_MARK="# Added by the GitPersona installer"

say() { printf 'gitpersona-install: %s\n' "$*"; }
fail() {
	printf 'gitpersona-install: error: %s\n' "$*" >&2
	exit 1
}

usage() {
	cat <<'EOF'
GitPersona installer for macOS and Linux.

  curl -fsSL https://raw.githubusercontent.com/weikxs/gitpersona/main/install.sh | sh
  curl -fsSL https://raw.githubusercontent.com/weikxs/gitpersona/main/install.sh | sh -s -- [options]

Options (or environment variables):
  --version <tag>     install this release, such as v0.1.0 (GITPERSONA_VERSION); default: the latest
  --dir <directory>   install here (GITPERSONA_INSTALL_DIR); default: ~/.local/bin, or /usr/local/bin as root
  --no-modify-path    do not add the directory to your shell's startup file (GITPERSONA_NO_MODIFY_PATH=1)
  --help              show this help

Run it again to upgrade.
EOF
}

parse_args() {
	version="${GITPERSONA_VERSION:-}"
	install_dir="${GITPERSONA_INSTALL_DIR:-}"
	modify_path=1
	[ "${GITPERSONA_NO_MODIFY_PATH:-}" = "1" ] && modify_path=0
	while [ $# -gt 0 ]; do
		case "$1" in
		--version) [ $# -ge 2 ] || fail "--version needs a tag, such as v0.1.0"; version="$2"; shift 2 ;;
		--version=*) version="${1#*=}"; shift ;;
		--dir) [ $# -ge 2 ] || fail "--dir needs a directory"; install_dir="$2"; shift 2 ;;
		--dir=*) install_dir="${1#*=}"; shift ;;
		--no-modify-path) modify_path=0; shift ;;
		-h | --help) usage; exit 0 ;;
		*) fail "unknown option: $1 (see --help)" ;;
		esac
	done
	case "$version" in
	"" | v[0-9]*.[0-9]*.[0-9]*) ;;
	[0-9]*.[0-9]*.[0-9]*) version="v$version" ;;
	*) fail "not a release tag: $version (expected something like v0.1.0)" ;;
	esac
}

detect_platform() {
	case "$(uname -s)" in
	Darwin) os=darwin ;;
	Linux) os=linux ;;
	*) fail "$(uname -s) is not supported yet; GitPersona runs on macOS and Linux" ;;
	esac
	case "$(uname -m)" in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) fail "$(uname -m) CPUs are not supported; GitPersona runs on x86_64 and arm64" ;;
	esac
	# An x86_64 shell under Rosetta on an Apple silicon Mac: use the native build.
	if [ "$os" = darwin ] && [ "$arch" = amd64 ] &&
		[ "$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)" = 1 ]; then
		arch=arm64
	fi
}

detect_tools() {
	if command -v curl >/dev/null 2>&1; then
		downloader=curl
	elif command -v wget >/dev/null 2>&1; then
		downloader=wget
	else
		fail "curl or wget is needed to download GitPersona"
	fi
	if command -v sha256sum >/dev/null 2>&1; then
		hasher="sha256sum"
	elif command -v shasum >/dev/null 2>&1; then
		hasher="shasum -a 256"
	else
		fail "sha256sum or shasum is needed to check the download"
	fi
	command -v tar >/dev/null 2>&1 || fail "tar is needed to unpack the download"
}

# download URL FILE
download() {
	if [ "$downloader" = curl ]; then
		curl --proto '=https' --tlsv1.2 -fsSL --retry 3 -o "$2" "$1"
	else
		wget -q -O "$2" "$1"
	fi
}

# latest_version prints the tag of the newest release, read from where
# releases/latest redirects (no API call, so no rate limit).
latest_version() {
	if [ "$downloader" = curl ]; then
		url=$(curl --proto '=https' --tlsv1.2 -fsSLI -o /dev/null -w '%{url_effective}' "$BASE_URL/latest")
	else
		url=$(wget -S --spider "$BASE_URL/latest" 2>&1 | sed -n 's/^ *[Ll]ocation: *//p' | tail -n 1 | tr -d '\r')
	fi
	tag="${url##*/}"
	case "$tag" in
	v[0-9]*.[0-9]*.[0-9]*) printf '%s\n' "$tag" ;;
	*) fail "could not find the latest release at $BASE_URL/latest" ;;
	esac
}

choose_dir() {
	if [ -z "$install_dir" ]; then
		if [ "$(id -u)" = 0 ]; then install_dir=/usr/local/bin; else install_dir="$HOME/.local/bin"; fi
	fi
	case "$install_dir" in
	"~"/*) install_dir="$HOME/${install_dir#"~/"}" ;;
	/*) ;;
	*) install_dir="$(pwd)/$install_dir" ;;
	esac
	mkdir -p "$install_dir" || fail "cannot create $install_dir"
	[ -w "$install_dir" ] || fail "$install_dir is not writable; choose another with --dir, or run as root"
}

# fetch_and_verify downloads the archive and checksums.txt into $tmp and
# stops unless the archive's SHA-256 matches its line in checksums.txt.
fetch_and_verify() {
	file="gitpersona_${version#v}_${os}_${arch}.tar.gz"
	say "downloading $file ($version)"
	download "$BASE_URL/download/$version/$file" "$tmp/$file" ||
		fail "could not download $file; is $version a GitPersona release?"
	download "$BASE_URL/download/$version/checksums.txt" "$tmp/checksums.txt" ||
		fail "could not download checksums.txt for $version"
	expected=$(awk -v f="$file" '$2 == f || $2 == "*" f { print $1 }' "$tmp/checksums.txt")
	[ -n "$expected" ] || fail "checksums.txt has no entry for $file"
	actual=$(cd "$tmp" && $hasher "$file" | awk '{ print $1 }')
	[ "$expected" = "$actual" ] || fail "checksum mismatch for $file (expected $expected, got $actual); nothing was installed"
	say "checksum OK"
	tar -xzf "$tmp/$file" -C "$tmp" gitpersona || fail "could not unpack $file"
}

install_binary() {
	chmod 755 "$tmp/gitpersona"
	if [ "$os" = darwin ]; then
		xattr -d com.apple.quarantine "$tmp/gitpersona" 2>/dev/null || true
	fi
	# Copy next to the target, then rename, so a running gitpersona is never
	# left half-written.
	cp "$tmp/gitpersona" "$install_dir/.gitpersona.new" || fail "cannot write to $install_dir"
	mv -f "$install_dir/.gitpersona.new" "$install_dir/gitpersona"
	"$install_dir/gitpersona" version >/dev/null 2>&1 || fail "the installed binary does not run on this system"
	say "installed $("$install_dir/gitpersona" version) to $install_dir/gitpersona"
}

on_path() {
	case ":${PATH:-}:" in *":$1:"*) return 0 ;; esac
	return 1
}

# startup_file prints the file the login shell reads for PATH.
startup_file() {
	case "$(basename "${SHELL:-sh}")" in
	zsh) printf '%s\n' "${ZDOTDIR:-$HOME}/.zshrc" ;;
	bash)
		if [ "$os" = darwin ]; then printf '%s\n' "$HOME/.bash_profile"; else printf '%s\n' "$HOME/.bashrc"; fi
		;;
	fish) printf '%s\n' "${XDG_CONFIG_HOME:-$HOME/.config}/fish/conf.d/gitpersona-path.fish" ;;
	*) printf '%s\n' "$HOME/.profile" ;;
	esac
}

ensure_path() {
	if on_path "$install_dir"; then return 0; fi
	if [ "$modify_path" = 0 ]; then
		say "$install_dir is not on your PATH; add it to use \`gitpersona\` by name"
		return 0
	fi
	rc=$(startup_file)
	if [ -f "$rc" ] && grep -qF "$PATH_MARK" "$rc"; then
		say "$rc already adds the install directory to PATH; open a new terminal"
		return 0
	fi
	mkdir -p "$(dirname "$rc")"
	case "$rc" in
	*.fish) line="fish_add_path -g \"$install_dir\"" ;;
	*) line="export PATH=\"$install_dir:\$PATH\"" ;;
	esac
	printf '\n%s\n%s\n' "$PATH_MARK" "$line" >>"$rc"
	say "added $install_dir to PATH in $rc; open a new terminal (or run: $line)"
}

# warn_shadowed says when another gitpersona comes first on PATH.
warn_shadowed() {
	found=$(command -v gitpersona 2>/dev/null || true)
	if [ -n "$found" ] && [ "$found" != "$install_dir/gitpersona" ]; then
		say "note: $found comes first on your PATH; remove it or put $install_dir first"
	fi
}

main() {
	parse_args "$@"
	detect_platform
	detect_tools
	[ -n "$version" ] || version=$(latest_version)
	choose_dir
	tmp=$(mktemp -d 2>/dev/null || mktemp -d -t gitpersona)
	trap 'rm -rf "$tmp"' EXIT INT TERM
	fetch_and_verify
	install_binary
	ensure_path
	warn_shadowed
	say "done. Next: sign in to each GitHub account with \`gh auth login\`, then run \`gitpersona setup --web\`"
}

main "$@"
