#!/bin/sh
# Runs install.sh in Linux containers against a published release: several
# distributions and both CPUs, as root and as a user, with curl and with
# wget only, upgrading, every shell's PATH setup, and the failures (a wrong
# checksum, an unknown release, an unsupported system or CPU). Nothing is
# installed on the machine running it. One container at a time, one CPU each.
#
# usage: test/install/docker-test.sh [release tag, default v0.1.0]
set -eu

TAG="${1:-v0.1.0}"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
DOCKER="docker run --rm --cpus 1 --memory 1g -v $ROOT/install.sh:/install.sh:ro"
passed=0
failed=0

# check NAME IMAGE PLATFORM SCRIPT: runs SCRIPT with sh in IMAGE; SCRIPT
# ends with "ok" when every expectation held.
check() {
	name=$1 image=$2 platform=$3 script=$4
	out=$($DOCKER --platform "$platform" "$image" sh -c "$script" 2>&1) || true
	if [ "$(printf '%s\n' "$out" | tail -n 1)" = ok ]; then
		passed=$((passed + 1))
		printf 'PASS  %s\n' "$name"
	else
		failed=$((failed + 1))
		printf 'FAIL  %s\n%s\n\n' "$name" "$out" | sed 's/^/      /'
	fi
}

# Helpers inside the containers.
APT_CURL='apt-get -qq update >/dev/null && apt-get -qq install -y curl ca-certificates >/dev/null 2>&1'
USER_SETUP='useradd -m -s /bin/bash dev'
AS_DEV='su dev -c'

check "debian 12 arm64, user, curl, latest release, PATH in ~/.bashrc, upgrade keeps one PATH line" \
	debian:12 linux/arm64 "$APT_CURL && $USER_SETUP && $AS_DEV '
		set -e
		SHELL=/bin/bash sh /install.sh
		test -x ~/.local/bin/gitpersona
		~/.local/bin/gitpersona version | grep -q \"^gitpersona ${TAG#v} \"
		grep -q \"export PATH=\\\"\$HOME/.local/bin:\\\$PATH\\\"\" ~/.bashrc
		SHELL=/bin/bash sh /install.sh >/dev/null
		test \$(grep -c \"Added by the GitPersona installer\" ~/.bashrc) = 1
		bash -ic \"gitpersona version\" 2>/dev/null | grep -q gitpersona
		echo ok'"

check "ubuntu 24.04 amd64 (emulated), root, curl, --version, lands in /usr/local/bin" \
	ubuntu:24.04 linux/amd64 "$APT_CURL && sh /install.sh --version $TAG >/tmp/log && test -x /usr/local/bin/gitpersona &&
		gitpersona version | grep -q \"${TAG#v}\" && ! grep -q 'added' /tmp/log && echo ok"

check "alpine 3.20 arm64, root, busybox wget only, latest release" \
	alpine:3.20 linux/arm64 "! command -v curl >/dev/null && sh /install.sh && gitpersona version | grep -q \"${TAG#v}\" && echo ok"

check "fedora 41 amd64 (emulated), root, curl" \
	fedora:41 linux/amd64 "sh /install.sh --version ${TAG#v} && /usr/local/bin/gitpersona version | grep -q \"${TAG#v}\" && echo ok"

check "zsh, fish and other shells get their own startup file (root, --dir off PATH); --no-modify-path changes none" \
	alpine:3.20 linux/arm64 "set -e
		export HOME=/tmp/h1; SHELL=/bin/zsh sh /install.sh --dir \$HOME/.local/bin >/dev/null; grep -q '.local/bin' /tmp/h1/.zshrc
		export HOME=/tmp/h2; SHELL=/usr/bin/fish sh /install.sh --dir \$HOME/.local/bin >/dev/null; grep -q 'fish_add_path -g \"/tmp/h2/.local/bin\"' /tmp/h2/.config/fish/conf.d/gitpersona-path.fish
		export HOME=/tmp/h3; SHELL=/bin/dash sh /install.sh --dir \$HOME/.local/bin >/dev/null; grep -q '.local/bin' /tmp/h3/.profile
		export HOME=/tmp/h4; SHELL=/bin/zsh sh /install.sh --no-modify-path --dir ~/tools >/tmp/log; test -x /tmp/h4/tools/gitpersona; test ! -e /tmp/h4/.zshrc
		grep -q 'not on your PATH' /tmp/log
		echo ok"

check "a wrong checksum stops before anything is installed" \
	alpine:3.20 linux/arm64 "mkdir /fake && printf '#!/bin/sh\necho 0000000000000000000000000000000000000000000000000000000000000000  \$1\n' >/fake/sha256sum && chmod +x /fake/sha256sum &&
		PATH=/fake:\$PATH sh /install.sh --version $TAG >/tmp/log 2>&1; test \$? -ne 0 && grep -q 'checksum mismatch' /tmp/log &&
		test ! -e /usr/local/bin/gitpersona && echo ok"

check "an unknown release and a malformed tag fail with a clear message" \
	alpine:3.20 linux/arm64 "sh /install.sh --version v99.0.0 >/tmp/a 2>&1; a=\$?; sh /install.sh --version latest-ish >/tmp/b 2>&1; b=\$?
		test \$a -ne 0 && grep -q 'could not download' /tmp/a && test \$b -ne 0 && grep -q 'not a release tag' /tmp/b &&
		test ! -e /usr/local/bin/gitpersona && echo ok"

check "an unsupported system or CPU fails with a clear message" \
	alpine:3.20 linux/arm64 "mkdir /fake && printf '#!/bin/sh\ncase \$1 in -s) echo FreeBSD;; *) echo riscv64;; esac\n' >/fake/uname && chmod +x /fake/uname &&
		PATH=/fake:\$PATH sh /install.sh >/tmp/a 2>&1; grep -q 'FreeBSD is not supported' /tmp/a &&
		printf '#!/bin/sh\ncase \$1 in -s) echo Linux;; *) echo riscv64;; esac\n' >/fake/uname &&
		PATH=/fake:\$PATH sh /install.sh >/tmp/b 2>&1; grep -q 'riscv64 CPUs are not supported' /tmp/b && echo ok"

check "--help works when the script comes from a pipe" \
	alpine:3.20 linux/arm64 "sh -s -- --help </install.sh | grep -q 'Run it again to upgrade' && echo ok"

printf '\n%d passed, %d failed\n' "$passed" "$failed"
[ "$failed" = 0 ]
