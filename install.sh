#!/bin/sh
# vynsh installer for macOS and Linux.
#
#   curl -fsSL https://raw.githubusercontent.com/jaraslaukunin/vynsh/main/install.sh | sh
#
# Environment overrides:
#   VYNSH_VERSION      release to install, e.g. 0.3.0-beta (default: latest)
#   VYNSH_INSTALL_DIR  where to put the binary (default: /usr/local/bin, or
#                      ~/.local/bin when that is not writable)
#   VYNSH_REPO         GitHub repository (default: jaraslaukunin/vynsh)
#   VYNSH_BASE_URL     override the download base URL (mirrors, local testing)
#   VYNSH_NO_VERIFY    set to 1 to skip the checksum check (not recommended)
set -eu

REPO="${VYNSH_REPO:-jaraslaukunin/vynsh}"
VERSION="${VYNSH_VERSION:-latest}"
INSTALL_DIR="${VYNSH_INSTALL_DIR:-}"
NO_VERIFY="${VYNSH_NO_VERIFY:-0}"

say() { printf '%s\n' "$*"; }
fail() { printf 'error: %s\n' "$*" >&2; exit 1; }

# ---- detect the platform -------------------------------------------------
os=$(uname -s 2>/dev/null || echo unknown)
case "$os" in
	Darwin) os=darwin ;;
	Linux) os=linux ;;
	*) fail "unsupported operating system: $os (vynsh supports macOS, Linux and Windows)" ;;
esac

arch=$(uname -m 2>/dev/null || echo unknown)
case "$arch" in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) fail "unsupported architecture: $arch" ;;
esac

# ---- locate a downloader -------------------------------------------------
if command -v curl >/dev/null 2>&1; then
	download() { curl -fsSL "$1" -o "$2"; }
	fetch() { curl -fsSL "$1"; }
elif command -v wget >/dev/null 2>&1; then
	download() { wget -qO "$2" "$1"; }
	fetch() { wget -qO- "$1"; }
else
	fail "curl or wget is required"
fi

# ---- resolve the release -------------------------------------------------
# Release archives are named after the version (vynsh_0.3.0-beta_darwin_arm64.tar.gz),
# so "latest" has to be resolved to a concrete tag first. GitHub's
# /releases/latest skips pre-releases, so fall back to the release list.
api="https://api.github.com/repos/$REPO/releases"

resolve_tag() {
	json=$(fetch "$api/latest" 2>/dev/null || true)
	tag=$(printf '%s' "$json" | grep -o '"tag_name": *"[^"]*"' | head -n1 | sed 's/.*"\([^"]*\)"$/\1/')
	if [ -z "$tag" ]; then
		json=$(fetch "$api?per_page=20" 2>/dev/null || true)
		# The list is newest first and may contain drafts; take the first tag.
		tag=$(printf '%s' "$json" | grep -o '"tag_name": *"[^"]*"' | head -n1 | sed 's/.*"\([^"]*\)"$/\1/')
	fi
	printf '%s' "$tag"
}

if [ -n "${VYNSH_BASE_URL:-}" ]; then
	base="$VYNSH_BASE_URL"
elif [ "$VERSION" = "latest" ]; then
	tag=$(resolve_tag)
	[ -n "$tag" ] || fail "could not find a release in $REPO
  * check your network, or pick a version: VYNSH_VERSION=0.3.0-beta sh install.sh
  * releases: https://github.com/$REPO/releases"
	VERSION=${tag#v}
	base="https://github.com/$REPO/releases/download/$tag"
else
	base="https://github.com/$REPO/releases/download/v$VERSION"
fi
asset="vynsh_${VERSION}_${os}_${arch}.tar.gz"

tmp=$(mktemp -d 2>/dev/null || mktemp -d -t vynsh)
trap 'rm -rf "$tmp"' EXIT INT TERM

say "downloading $asset"
if ! download "$base/$asset" "$tmp/$asset"; then
	fail "could not download $base/$asset
  * check that version $VERSION exists (see https://github.com/$REPO/releases)
  * or install from source: go install github.com/$REPO/cmd/vynsh@latest"
fi

# ---- verify the checksum -------------------------------------------------
if [ "$NO_VERIFY" != "1" ]; then
	if fetch "$base/SHA256SUMS" >"$tmp/SHA256SUMS" 2>/dev/null; then
		want=$(grep " $asset\$" "$tmp/SHA256SUMS" | awk '{print $1}' | head -n1)
		if [ -n "$want" ]; then
			if command -v sha256sum >/dev/null 2>&1; then
				got=$(sha256sum "$tmp/$asset" | awk '{print $1}')
			elif command -v shasum >/dev/null 2>&1; then
				got=$(shasum -a 256 "$tmp/$asset" | awk '{print $1}')
			else
				got=""
			fi
			if [ -n "$got" ] && [ "$got" != "$want" ]; then
				fail "checksum mismatch for $asset (expected $want, got $got)"
			fi
			[ -n "$got" ] && say "checksum ok"
		fi
	else
		say "note: SHA256SUMS is not available for this release, skipping verification"
	fi
fi

# ---- unpack --------------------------------------------------------------
tar -xzf "$tmp/$asset" -C "$tmp" || fail "could not unpack $asset"
bin="$tmp/vynsh"
[ -f "$bin" ] || bin=$(find "$tmp" -type f -name vynsh | head -n1)
[ -n "$bin" ] && [ -f "$bin" ] || fail "the archive does not contain a vynsh binary"

# ---- pick the install directory -----------------------------------------
if [ -z "$INSTALL_DIR" ]; then
	if [ -w /usr/local/bin ] 2>/dev/null; then
		INSTALL_DIR=/usr/local/bin
	else
		INSTALL_DIR="$HOME/.local/bin"
	fi
fi
mkdir -p "$INSTALL_DIR" || fail "cannot create $INSTALL_DIR"
install -m 0755 "$bin" "$INSTALL_DIR/vynsh" 2>/dev/null || {
	cp "$bin" "$INSTALL_DIR/vynsh" && chmod 0755 "$INSTALL_DIR/vynsh"
}

say ""
say "✓ vynsh installed to $INSTALL_DIR/vynsh"
"$INSTALL_DIR/vynsh" version 2>/dev/null | sed 's/^/  /' || true

case ":$PATH:" in
	*":$INSTALL_DIR:"*) ;;
	*)
		say ""
		say "Add it to your PATH:"
		say "  export PATH=\"$INSTALL_DIR:\$PATH\""
		say "For that to stick, add the line to ~/.zshrc (macOS) or ~/.bashrc (Linux)."
		;;
esac
say ""
say "Try it:  vynsh report    or    vynsh tui"
