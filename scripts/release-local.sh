#!/bin/sh
# Cut a BINNY release from this machine: cross-compile every target, package
# them the way .github/workflows/release.yml does, and publish to the public
# openbin-ai/binny-releases repo with the gh CLI.
#
#   scripts/release-local.sh v0.1.0-beta.1
#
# Needs: go, zip, gh (logged in with push access to binny-releases).
set -eu
ver="${1:?usage: release-local.sh <tag like v0.1.0-beta.1>}"
repo="openbin-ai/binny-releases"
out="$(mktemp -d)"
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
go vet ./... && go test ./...
for t in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
  os="${t%/*}"; arch="${t#*/}"; ext=""; [ "$os" = windows ] && ext=".exe"
  pkg="binny-$os-$arch"
  mkdir -p "$out/$pkg"
  GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$ver" -o "$out/$pkg/binny$ext" .
  cp README.md "$out/$pkg/"
  if [ "$os" = windows ]; then (cd "$out" && zip -qr "$pkg.zip" "$pkg"); else (cd "$out" && tar czf "$pkg.tar.gz" "$pkg"); fi
  echo "built $pkg"
done
printf '%s' "$ver" > "$out/VERSION"
# Not a prerelease on purpose: releases/latest/download/<asset> only resolves
# to full releases, and the installers depend on that URL.
gh release create "$ver" --repo "$repo" --title "BINNY $ver" --notes "BINNY $ver. Install: https://openbin.ai/#binny" \
  "$out"/binny-*.tar.gz "$out"/binny-*.zip "$out/VERSION"
echo "published $ver to https://github.com/$repo/releases/tag/$ver"
