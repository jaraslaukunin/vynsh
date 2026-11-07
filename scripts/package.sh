#!/bin/sh
# Package the cross-compiled binaries in dist/ into the archives that the
# install scripts download, plus SHA256SUMS.
#
#   VERSION=0.3.0-beta scripts/package.sh
#
# Produces, for every platform:
#   dist/vynsh_<version>_<os>_<arch>.tar.gz   (macOS, Linux)
#   dist/vynsh_<version>_windows_<arch>.zip   (Windows)
#   dist/SHA256SUMS
set -eu

VERSION="${VERSION:-0.3.0-beta}"
DIST="${DIST:-dist}"
PLATFORMS="${PLATFORMS:-darwin-amd64 darwin-arm64 windows-amd64 windows-arm64 linux-amd64 linux-arm64}"

[ -d "$DIST" ] || { echo "error: $DIST does not exist; run 'make release' first" >&2; exit 1; }

cd "$DIST"

for platform in $PLATFORMS; do
	os=${platform%%-*}
	arch=${platform##*-}
	src="vynsh-$platform"
	[ "$os" = "windows" ] && src="$src.exe"
	[ -f "$src" ] || { echo "skipping $src (not built)"; continue; }

	if [ "$os" = "windows" ]; then
		archive="vynsh_${VERSION}_${os}_${arch}.zip"
		rm -f "$archive"
		cp "$src" "vynsh.exe"
		if command -v zip >/dev/null 2>&1; then
			zip -q -9 "$archive" "vynsh.exe"
		elif command -v powershell >/dev/null 2>&1; then
			powershell -NoProfile -Command "Compress-Archive -Path vynsh.exe -DestinationPath '$archive' -Force"
		else
			echo "warning: no zip tool found, skipping $archive" >&2
			rm -f "vynsh.exe"
			continue
		fi
		rm -f "vynsh.exe"
	else
		archive="vynsh_${VERSION}_${os}_${arch}.tar.gz"
		rm -f "$archive"
		cp "$src" "vynsh"
		chmod 0755 "vynsh"
		# COPYFILE_DISABLE keeps macOS from adding ._ AppleDouble entries.
		COPYFILE_DISABLE=1 tar -czf "$archive" "vynsh"
		rm -f "vynsh"
	fi
	echo "packaged $archive"
done

# ---- checksums -----------------------------------------------------------
if command -v sha256sum >/dev/null 2>&1; then
	sha256sum vynsh_* >SHA256SUMS
elif command -v shasum >/dev/null 2>&1; then
	shasum -a 256 vynsh_* >SHA256SUMS
else
	echo "warning: no sha256sum/shasum found, SHA256SUMS not written" >&2
	exit 0
fi
echo "wrote SHA256SUMS"
cat SHA256SUMS
