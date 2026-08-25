#!/bin/sh
# Install plst, and the modules a fresh install needs to be useful.
#
# POSIX sh, because this is the one thing that runs before anything is known
# about the machine. No bashisms, no jq, no Go toolchain — curl or wget, tar, and
# a shell.
set -eu

REPO="${PLST_REPO:-esmarkowski/plasticity}"
# The modules installed by default. A loader with nothing loaded cannot do
# anything, and `plst harness` is the one people came for.
MODULES="${PLST_MODULES:-esmarkowski/plasticity-modules}"
BINDIR="${PLST_BINDIR:-$HOME/.local/bin}"

say() { printf '  %s\n' "$*" >&2; }
die() { printf 'install: %s\n' "$*" >&2; exit 1; }

# --- platform -----------------------------------------------------------------
os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) die "unsupported architecture: $arch" ;;
esac
case "$os" in
  darwin | linux) ;;
  *) die "unsupported system: $os — build from source with: go install github.com/$REPO/cmd/plst@latest" ;;
esac

# --- fetch --------------------------------------------------------------------
if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL "$1"; }
  download() { curl -fsSL -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -qO- "$1"; }
  download() { wget -qO "$2" "$1"; }
else
  die "neither curl nor wget is available"
fi

# The tag of the latest release, read out of the redirect rather than the API:
# /releases/latest redirects to the tag, which needs no JSON parsing and no token.
latest_tag() {
  url=$(fetch "https://api.github.com/repos/$1/releases/latest" 2>/dev/null |
    sed -n 's/.*"tag_name" *: *"\([^"]*\)".*/\1/p' | head -1)
  printf '%s' "$url"
}

install_release() {
  repo=$1
  tag=$(latest_tag "$repo") || tag=""
  [ -n "$tag" ] || return 1

  name=$(basename "$repo")
  version=${tag#v}
  asset="${name}_${version}_${os}_${arch}.tar.gz"
  url="https://github.com/$repo/releases/download/$tag/$asset"

  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT
  say "$repo $tag ($os/$arch)"
  download "$url" "$tmp/a.tar.gz" || { rm -rf "$tmp"; return 1; }
  tar -xzf "$tmp/a.tar.gz" -C "$tmp" || { rm -rf "$tmp"; return 1; }

  mkdir -p "$BINDIR"
  found=""
  for f in "$tmp"/plst "$tmp"/plst-*; do
    [ -f "$f" ] || continue
    install -m 0755 "$f" "$BINDIR/$(basename "$f")"
    found="$found $(basename "$f")"
  done
  rm -rf "$tmp"
  trap - EXIT
  [ -n "$found" ] || return 1
  say "installed$found into $BINDIR"
}

say "installing plst"
if ! install_release "$REPO"; then
  die "no release asset for $os/$arch in $REPO.
    Build from source instead:
      go install github.com/$REPO/cmd/plst@latest"
fi

# --- modules ------------------------------------------------------------------
# Through plst itself, so a module installed here is installed the same way a
# module installed later is, and lands wherever plst is configured to put them.
for m in $MODULES; do
  say "installing module $m"
  "$BINDIR/plst" install "$m" >/dev/null 2>&1 ||
    say "could not install $m — run: plst install $m"
done

# --- PATH ---------------------------------------------------------------------
case ":$PATH:" in
  *":$BINDIR:"*) ;;
  *)
    printf '\n%s\n' "$BINDIR is not on your PATH. Add it:" >&2
    printf '  %s\n' "export PATH=\"$BINDIR:\$PATH\"" >&2
    ;;
esac

printf '\n'
"$BINDIR/plst" || true
