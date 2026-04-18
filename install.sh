#!/bin/sh
# install.sh — download and install the latest ana release.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/highperformance-tech/ana-cli/main/install.sh | sh
#
# Environment overrides:
#   INSTALL_DIR   Where to place the ana binary (default: /usr/local/bin).
#   ANA_VERSION   Pin a specific release tag (default: latest).
#
# Requires: curl (or wget), tar, sha256sum or shasum, uname.
# Windows is not supported — download the .zip from the releases page instead.

set -eu

REPO="highperformance-tech/ana-cli"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
ANA_VERSION="${ANA_VERSION:-latest}"

log() {
	printf '==> %s\n' "$*"
}

die() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

detect_os() {
	case "$(uname -s)" in
		Darwin) echo darwin ;;
		Linux)  echo linux ;;
		MINGW*|MSYS*|CYGWIN*)
			die "install.sh does not support Windows — download the .zip from https://github.com/${REPO}/releases"
			;;
		*) die "unsupported OS: $(uname -s)" ;;
	esac
}

detect_arch() {
	case "$(uname -m)" in
		x86_64|amd64) echo amd64 ;;
		arm64|aarch64) echo arm64 ;;
		*) die "unsupported architecture: $(uname -m)" ;;
	esac
}

fetch() {
	# fetch URL -> stdout. Prefers curl; falls back to wget.
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$1"
	elif command -v wget >/dev/null 2>&1; then
		wget -qO- "$1"
	else
		die "curl or wget required"
	fi
}

fetch_file() {
	# fetch_file URL DEST. Downloads to DEST, failing on HTTP errors.
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL -o "$2" "$1"
	elif command -v wget >/dev/null 2>&1; then
		wget -q -O "$2" "$1"
	else
		die "curl or wget required"
	fi
}

resolve_version() {
	if [ "$ANA_VERSION" != "latest" ]; then
		echo "$ANA_VERSION"
		return
	fi
	# The /releases/latest endpoint returns a JSON blob whose "tag_name" is
	# the release tag (e.g. v0.1.0). We use a minimal sed/awk pipeline to
	# avoid a jq dependency.
	tag=$(fetch "https://api.github.com/repos/${REPO}/releases/latest" \
		| sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' \
		| head -n1)
	[ -n "$tag" ] || die "could not resolve latest release tag"
	echo "$tag"
}

verify_checksum() {
	# verify_checksum ARCHIVE_PATH CHECKSUMS_PATH
	archive="$1"
	checksums="$2"
	archive_name=$(basename "$archive")
	expected=$(grep " $archive_name\$" "$checksums" | awk '{print $1}')
	[ -n "$expected" ] || die "no checksum entry for $archive_name"
	if command -v sha256sum >/dev/null 2>&1; then
		actual=$(sha256sum "$archive" | awk '{print $1}')
	elif command -v shasum >/dev/null 2>&1; then
		actual=$(shasum -a 256 "$archive" | awk '{print $1}')
	else
		die "sha256sum or shasum required"
	fi
	if [ "$expected" != "$actual" ]; then
		die "checksum mismatch: expected $expected, got $actual"
	fi
}

main() {
	os=$(detect_os)
	arch=$(detect_arch)
	tag=$(resolve_version)
	# GoReleaser strips the leading v from {{ .Version }} when templating the
	# archive name, so a v0.1.0 tag produces ana_0.1.0_linux_amd64.tar.gz.
	version="${tag#v}"
	archive="ana_${version}_${os}_${arch}.tar.gz"
	base="https://github.com/${REPO}/releases/download/${tag}"

	log "installing ana ${tag} for ${os}/${arch}"

	tmpdir=$(mktemp -d)
	trap 'rm -rf "$tmpdir"' EXIT

	log "downloading ${archive}"
	fetch_file "${base}/${archive}" "${tmpdir}/${archive}"
	log "downloading checksums.txt"
	fetch_file "${base}/checksums.txt" "${tmpdir}/checksums.txt"

	log "verifying checksum"
	verify_checksum "${tmpdir}/${archive}" "${tmpdir}/checksums.txt"

	log "extracting"
	tar -xzf "${tmpdir}/${archive}" -C "$tmpdir" ana

	# Install target may require sudo when INSTALL_DIR is not user-writable.
	if [ -w "$INSTALL_DIR" ]; then
		install_cmd=""
	elif command -v sudo >/dev/null 2>&1; then
		install_cmd="sudo"
		log "using sudo to install to $INSTALL_DIR"
	else
		die "$INSTALL_DIR is not writable and sudo is unavailable; set INSTALL_DIR to a writable path"
	fi

	mkdir -p "$INSTALL_DIR" 2>/dev/null || $install_cmd mkdir -p "$INSTALL_DIR"
	$install_cmd install -m 0755 "${tmpdir}/ana" "${INSTALL_DIR}/ana"

	log "installed to ${INSTALL_DIR}/ana"
	"${INSTALL_DIR}/ana" --version || true
}

main "$@"
