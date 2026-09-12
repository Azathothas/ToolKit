#!/bin/sh
# Install the common user-level tools for a provider base.
# Run as the unprivileged base account, never as root.
set -eu

say() { printf '  * %s\n' "$*" >&2; }
die() { printf 'bootstrap: %s\n' "$*" >&2; exit 3; }

CODEGRAPH_VERSION=1.5.0
CODEGRAPH_PACKAGE=@colbymchenry/codegraph
CODEGRAPH_INTEGRITY='sha512-/l1JMVOQ9WGQLrc/IIuAg7Igr944t79/oNCJTcnGkYtIeQx2XFIqI0ho+9Les/Yu4zKfmPU17hIUshD6yP1fKw=='

for tool in git jq node npm rg tmux; do
  command -v "$tool" >/dev/null 2>&1 || die "$tool is absent; use base.toolset=developer and run base ensure first"
done
[ "$(id -u)" -ne 0 ] || die "run this as the base account, not through base shell --root"

case "$(uname -m)" in
  x86_64)
    PLATFORM_PACKAGE=@colbymchenry/codegraph-linux-x64
    PLATFORM_INTEGRITY='sha512-wT1O7NkQoV6zPiq36d1qBqoIHL2ZRnOkFMjQ7JJwWS7KHnP32qHCd/pPaSwl8cNRlGYkYSaX3WVDpz2BaV+Now=='
    ;;
  aarch64|arm64)
    PLATFORM_PACKAGE=@colbymchenry/codegraph-linux-arm64
    PLATFORM_INTEGRITY='sha512-Bq5Ggb7PoLF7mnOiGl/UxMrDgrPCAQUMsl7BpMkAxq8FulvVEp7E/GrIQih2YB32vWr18KpN2i2VoRLAHLIDUw=='
    ;;
  *) die "CodeGraph $CODEGRAPH_VERSION has no pinned package here for $(uname -m)" ;;
esac

work=$(mktemp -d "${TMPDIR:-/tmp}/wsl-toolkit-bootstrap.XXXXXX")
cleanup() { rm -rf "$work"; }
trap cleanup 0 1 2 15

fetch_verified_package() {
  package=$1
  integrity=$2
  package_dir="$work/$(printf '%s' "$package" | tr '/@' '__')"
  mkdir "$package_dir"
  manifest="$package_dir/pack.json"
  if ! npm pack "$package@$CODEGRAPH_VERSION" --ignore-scripts --pack-destination "$package_dir" --json > "$manifest"; then
    die "npm could not fetch $package@$CODEGRAPH_VERSION"
  fi
  set -- "$package_dir"/*.tgz
  [ "$#" -eq 1 ] && [ -f "$1" ] || die "npm did not write exactly one archive for $package"
  archive=$1
  filename=$(basename "$archive")
  actual=$(node - "$archive" <<'NODE'
const crypto = require('crypto');
const fs = require('fs');
const bytes = fs.readFileSync(process.argv[2]);
process.stdout.write('sha512-' + crypto.createHash('sha512').update(bytes).digest('base64'));
NODE
  ) || die "could not hash $filename"
  [ "$actual" = "$integrity" ] || die "$package@$CODEGRAPH_VERSION failed its pinned SHA-512 check"
  say "verified $package@$CODEGRAPH_VERSION"
  printf '%s\n' "$archive"
}

main_archive=$(fetch_verified_package "$CODEGRAPH_PACKAGE" "$CODEGRAPH_INTEGRITY")
platform_archive=$(fetch_verified_package "$PLATFORM_PACKAGE" "$PLATFORM_INTEGRITY")

mkdir -p "$HOME/.local/bin"
npm install --global --prefix "$HOME/.local" --ignore-scripts --omit=optional \
  "$platform_archive" "$main_archive" >/dev/null

profile=$HOME/.profile
# shellcheck disable=SC2016
path_line='export PATH="$HOME/.local/bin:$PATH"'
touch "$profile"
if ! grep -Fqx "$path_line" "$profile"; then
  printf '\n# Added by the wsl-toolkit provider-base example.\n%s\n' "$path_line" >> "$profile"
fi
PATH="$HOME/.local/bin:$PATH"
export PATH

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
install -m 0644 "$script_dir/tmux.conf" "$HOME/.tmux.conf"

codegraph version
tmux -V
printf 'bootstrap-complete\n'
