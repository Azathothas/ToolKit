#!/bin/sh
# bootstrap.sh - bring a Unix userland up to a named set of tools.
#
# THE DEFECT IT EXISTS TO CATCH: a guest that was set up by hand. Every
# hand-built userland is built slightly differently, and the difference is found
# weeks later inside a job that fails for a reason nobody wrote down. This
# installs a NAMED set from the system's own package manager, reports what it
# actually resolved, and names every package it could not get.
#
# WHERE IT RUNS: Linux with apk, apt, dnf, emerge, pacman, tdnf, xbps, yum or
# zypper; FreeBSD and DragonFly with pkg; NetBSD with pkgin; OpenBSD with
# pkg_add. As root, through passwordless sudo, or as an unprivileged account
# through nix, which it finds where a login shell would and never installs. It
# names what it looked for when it finds none.
#
# ⛔ IT DEPENDS ON THE SHELL AND THE PACKAGE MANAGER, AND ON ALMOST NOTHING
# ELSE. Not awk, not sed, not grep, not tr, not find, not install, not dirname.
# That is not minimalism for its own sake: measured across thirteen images on
# 2026-09-12, Photon carries neither `awk` nor `tr`, openSUSE carries neither
# `awk` nor `find`, and Void and Rocky 8 carry no `find`. A bootstrap whose job is
# to install the missing tools cannot require them to already be there. `uname`
# and `id` are the exceptions, and both are in every one of those images and in
# every BSD base system.
#
# ⭐ MOST OF WHAT THIS INSTALLS NEEDS NO DIGEST LOGIC HERE AT ALL, and that is the
# first answer rather than a fallback. A distribution package arrives through a
# package manager that already checks a signature over its own index, so `pacman`,
# `apt`, `dnf`, `apk` and the rest are trusted to do the job they exist for. The
# verification below is for the two cases where nothing else would do it: an npm
# package, and a tarball from a GitHub release.
#
# ⛔ IT IS POSIX sh AND IT IS CHECKED THAT WAY. This repository runs
# `shellcheck -s sh` over every tracked `.sh`, so there is no `local`, no `[[`,
# no array and no `a && b || c` here. The last one is the trap: a development
# host carries shellcheck 0.11.0, which permits it, and CI carries 0.9.0, which
# reports SC2015 and fails the job.
#
# ⚠ WHAT A RUN-TIME DIGEST PROVES IS TRANSPORT, NOT AUTHORSHIP. Where this
# verifies a download, the expected digest comes from the same place as the
# bytes, so whoever could replace one could replace the other. That is strictly
# weaker than a digest written into this file in advance, and it is the trade made
# on purpose: a digest written in here goes stale on the next upstream release,
# and a stale pin is a script nobody can run. `--expect-integrity` and
# `--expect-sha256` add the stronger check back for a caller who holds the value.
#
# EXIT CODES: 0 done, 1 something that was asked for could not be installed,
# 2 could not run at all. Read it from this process, unpiped.

set -eu

SELF=bootstrap
VERSION=1

# ---------------------------------------------------------------- reporting --

# Every message goes to stderr. Only the final report goes to stdout, so a caller
# may read the report through a pipe without the log arriving in it.
say()  { printf '%s: %s\n'     "$SELF" "$*" >&2; }
step() { printf '%s:   %s\n'   "$SELF" "$*" >&2; }
warn() { printf '%s: [!] %s\n' "$SELF" "$*" >&2; }
die()  { printf '%s: [-] %s\n' "$SELF" "$*" >&2; exit 2; }
fail() { printf '%s: [-] %s\n' "$SELF" "$*" >&2; FAILURES=$((FAILURES + 1)); }

FAILURES=0
SKIPPED=''
REQUESTED=''
CODEGRAPH_VERSION=''
UPSTREAM_DONE=''
UPSTREAM_FAILED=''

usage() {
  cat <<'USAGE'
usage: sh bootstrap.sh [options]

  --toolset NAME          minimal | developer | languages | agent.
                          Default developer.
  --with LIST             comma-separated logical names to add.
  --without LIST          comma-separated logical names to leave out.
  --provider NAME         force the package manager instead of detecting one.
  --user-provider NAME    nix | none. Default: nix when it is installed, on PATH
                          or in its own profile locations. A new profile
                          installs with flakes; GITHUB_TOKEN, when set, reaches
                          nix only through NIX_CONFIG.
  --no-upstream           do not fall back to an upstream tarball for a tool the
                          system packages do not carry.
  --codegraph SPEC        a version, `latest`, or `none`. Default: `latest` for
                          the agent toolset and `none` otherwise.
  --expect-integrity SRI  require this exact `sha512-...` for the CodeGraph
                          package, on top of the registry's own value.
  --expect-sha256 HEX     require this exact digest for every upstream tarball,
                          on top of the digest the release publishes.
  --tmux-config PATH      install this file as ~/.tmux.conf. Default: the
                          tmux.conf beside this script, when there is one.
  --no-tmux-config        install no tmux configuration.
  --prefix DIR            where user-level installs go. Default ~/.local.
  --dry-run               print what would be run and change nothing.
  --json                  write the final report as one JSON object.
  --list-providers        print the package managers this file knows, and exit.
  --list-names            print the logical names and their table row, and exit.
  --version               print this file's contract version, and exit.
  -h, --help              this text.

`--with` and `--without` take LOGICAL names, never a system's own package name:
the point of a logical name is that it survives the system spelling it
differently, or not carrying it at all.
USAGE
}

# ⛔ THE BLOCK BETWEEN THE TWO MARKER LINES BELOW HAS A SECOND COPY. It is copied
# byte for byte into tools/windows/wsl-toolkit/internal/toolkit/packages.sh, so
# that the base provisioner of that tool resolves its own `developer` names from
# one table, one lookup and one detection, and a distribution that renames a
# package is fixed once. ⚠ So an edit here reaches that executable as well as every
# caller of this file. Edit the block HERE. The gate's `package-table` check
# refuses the copy disagreeing, and this rewrites the copy:
#
#   sh scripts/common/check.sh package-table --fix
#
# ⚠ Everything inside it must stand alone: it may define functions and set
# variables, and it may not call anything defined outside the markers.

# >>> shared package table: begin
# ------------------------------------------------------------- the packages --

# ⭐ THE TABLE IS THE FEATURE. One row per logical name: the default package
# name, then only the places that spell it differently. Adding a distribution
# means adding values here and changing no code, and a name spelled the same
# everywhere needs no override at all.
#
#   KEY=VALUE       a package manager, `nix`, or `os:ID` from /etc/os-release
#   a|b=VALUE       several keys share one value
#   VALUE,VALUE     several packages for one logical name
#   -               this place does not carry it, or its base system already
#                   has it. The run reports the name as skipped.
#
# ⚠ `os:` BEATS THE MANAGER, and it has to. Alpine, Chimera and Wolfi all use apk
# and disagree about half of these names; Fedora and Rocky 8 both use dnf and
# Rocky has no ripgrep without EPEL. A table keyed on the manager alone cannot
# say either thing. ⚠ A `nix` value is a nixpkgs attribute, the same on every
# system, so nix is looked up with no `os:` key at all.
#
# ⚠ EVERY VALUE BELOW WAS READ OFF THE SYSTEM ON 2026-09-12, FreeBSD included.
# The Linux rows came from the thirteen images in this repository own catalogue;
# the `os:freebsd` rows came from FreeBSD 15.1 through `wsl-toolkit bsd run
# --network`, which fetches this file by raw URL and then queries `pkg` for each
# name. The `nix` values were evaluated in nixpkgs on 2026-09-14, every attribute the
# agent toolset resolves. ⛔ NetBSD and OpenBSD are the exception and say so in the
# header: `pkgin` and `pkg_add` are written and have never been run.
package_table() {
  cat <<'TABLE'
bash bash nix=bashInteractive
ca-certificates ca-certificates os:freebsd=ca_root_nss nix=cacert
build - apk=build-base os:chimera=base-devel apt=build-essential pacman=base-devel xbps=base-devel dnf|yum|zypper=gcc,gcc-c++,make tdnf=gcc,make emerge=sys-devel/gcc,sys-devel/make nix=gcc,gnumake
coreutils coreutils os:chimera=chimerautils os:rocky=-
curl curl
fd fd apt=fd-find dnf|yum=fd-find os:rocky=- tdnf=- os:chimera=- os:freebsd=fd-find
file file os:freebsd=-
git git
jq jq
less less os:freebsd=-
node nodejs zypper=nodejs-default os:freebsd=node
npm npm zypper=npm-default emerge=- tdnf=- xbps=- os:chimera=- os:void=- nix=-
openssh openssh-client pacman|xbps=openssh os:chimera=openssh dnf|yum|tdnf|zypper=openssh-clients emerge=net-misc/openssh os:freebsd=- nix=openssh
procps procps pacman|xbps=procps-ng dnf|yum|tdnf=procps-ng emerge=sys-process/procps os:freebsd=-
ripgrep ripgrep tdnf=- os:rocky=- os:chimera=-
sudo sudo nix=-
tar tar os:chimera=libarchive-progs os:wolfi=- os:freebsd=- nix=gnutar
tmux tmux
unzip unzip os:freebsd=-
xz xz apt=xz-utils emerge=app-arch/xz-utils os:freebsd=-
go go apt=golang dnf|yum=golang emerge=dev-lang/go
nim nim apt=- dnf|yum=- tdnf=- os:wolfi=- os:chimera=- os:rocky=-
python python3 pacman=python os:chimera=python emerge=dev-lang/python
rust rust apt=rustc emerge=dev-lang/rust nix=rustc
cargo cargo pacman=- tdnf=- emerge=- os:wolfi=- os:photon=- os:freebsd=-
powershell - apk=powershell os:wolfi=powershell tdnf=powershell os:chimera=- os:freebsd=powershell nix=powershell
TABLE
}

toolset_names() {
  case "$1" in
    minimal)
      printf 'ca-certificates curl git tar\n' ;;
    developer)
      printf 'bash ca-certificates coreutils build curl file git jq less node npm openssh procps ripgrep tar tmux unzip xz\n' ;;
    languages)
      printf 'bash build cargo go nim python rust powershell\n' ;;
    agent)
      printf 'bash ca-certificates coreutils build cargo curl fd file git go jq less nim node npm openssh powershell procps python ripgrep rust tar tmux unzip xz\n' ;;
    *)
      return 1 ;;
  esac
}

# ------------------------------------------------------------ small helpers --

have() { command -v "$1" >/dev/null 2>&1; }

# ⚠ `command -v` answers about a shell function and an alias too, and what this
# asks about is always an executable. A non-interactive `sh` has no aliases and
# this file defines no function named after a tool, so the simple form is right
# HERE and would not be inside somebody's interactive shell.

# split_on SEPARATORS STRING -> the string with each separator replaced by a
# space. This is `tr`'s job, and Photon has no `tr`.
split_on() {
  so_seps=$1
  so_in=$2
  so_out=''
  while [ -n "$so_in" ]; do
    so_head=${so_in%%[!"$so_seps"]*}
    if [ -n "$so_head" ]; then
      so_in=${so_in#"$so_head"}
      so_out="$so_out "
      continue
    fi
    so_word=${so_in%%["$so_seps"]*}
    so_out="$so_out$so_word"
    so_in=${so_in#"$so_word"}
  done
  printf '%s' "$so_out"
}

commas_to_spaces() { split_on ',' "$1"; }

# in_list ITEM LIST, where LIST is separated by spaces, commas or pipes.
in_list() {
  case " $(split_on ',|' "$2") " in
    *" $1 "*) return 0 ;;
  esac
  return 1
}

# package_row LOGICAL -> everything on its row after the name, or nothing.
package_row() {
  package_table | {
    while read -r pr_name pr_rest; do
      if [ "$pr_name" = "$1" ]; then
        printf '%s' "$pr_rest"
        return 0
      fi
    done
    return 1
  }
}

# package_for LOGICAL [PROVIDER OS_ID] -> the package name(s) to install here, or
# nothing when this place does not carry it. The provider and the system are the
# caller's PROVIDER and OS_ID unless the call names them, and an empty OS_ID matches
# no `os:` key.
package_for() {
  pf_row=$(package_row "$1") || return 1
  # ⚠ OS_ID AND PROVIDER BELONG TO THE CALLER, which sets both before its first
  # lookup. Read alone, the copy of this block has neither, and PROVIDER looks
  # like a misspelling of the list below.
  # shellcheck disable=SC2153
  pf_want_provider=${2-$PROVIDER}
  pf_want_os=${3-$OS_ID}
  pf_default=''
  pf_os=''
  pf_provider=''
  pf_first=1
  for pf_field in $pf_row; do
    if [ "$pf_first" = 1 ]; then
      pf_default=$pf_field
      pf_first=0
      continue
    fi
    pf_key=${pf_field%%=*}
    pf_value=${pf_field#*=}
    if [ -n "$pf_want_os" ] && in_list "os:$pf_want_os" "$pf_key"; then
      pf_os=$pf_value
    elif in_list "$pf_want_provider" "$pf_key"; then
      pf_provider=$pf_value
    fi
  done
  # ⚠ THE ORDER IS THE WHOLE POINT: the distribution wins over its package
  # manager, and the manager wins over the default. Reversing the first two makes
  # every apk distribution Alpine.
  pf_answer=$pf_default
  if [ -n "$pf_provider" ]; then
    pf_answer=$pf_provider
  fi
  if [ -n "$pf_os" ]; then
    pf_answer=$pf_os
  fi
  if [ "$pf_answer" = '-' ]; then
    printf ''
    return 0
  fi
  commas_to_spaces "$pf_answer"
}

# ------------------------------------------------------ platform and rights --

# The caller reads this list to validate a named provider; nothing inside the
# block does.
# shellcheck disable=SC2034
PROVIDERS='apk apt dnf emerge pacman pkg pkg_add pkgin tdnf xbps yum zypper'

# ⭐ THE KERNEL DECIDES THE FAMILY, then the family decides what to look for. A
# bare search for `pkg` on PATH is wrong in both directions: pkgsrc puts one on
# some Linux machines, and a FreeBSD jail can have a Linux emulation layer.
detect_provider() {
  case "$(uname -s)" in
    FreeBSD|DragonFly)
      if have pkg; then printf 'pkg'; return 0; fi
      printf ''
      return 0
      ;;
    NetBSD)
      if have pkgin; then printf 'pkgin'; return 0; fi
      printf ''
      return 0
      ;;
    OpenBSD)
      if have pkg_add; then printf 'pkg_add'; return 0; fi
      printf ''
      return 0
      ;;
  esac
  # ⚠ ORDER MATTERS AND IT IS NOT ALPHABETICAL. A distribution can carry more
  # than one: Fedora keeps a `yum` that forwards to `dnf`, and an openSUSE image
  # carries rpm tools that are not its package manager. The native one is tried
  # before anything that forwards to it.
  if have apk;          then printf 'apk';     return 0; fi
  if have pacman;       then printf 'pacman';  return 0; fi
  if have apt-get;      then printf 'apt';     return 0; fi
  if have zypper;       then printf 'zypper';  return 0; fi
  if have dnf;          then printf 'dnf';     return 0; fi
  if have tdnf;         then printf 'tdnf';    return 0; fi
  if have yum;          then printf 'yum';     return 0; fi
  if have xbps-install; then printf 'xbps';    return 0; fi
  if have emerge;       then printf 'emerge';  return 0; fi
  printf ''
}

detect_os_id() {
  # ⚠ OpenBSD and NetBSD HAVE NO /etc/os-release, and FreeBSD does. Without the
  # kernel fallback the two that do not would both be `unknown`, and every
  # `os:openbsd` row in the table would be dead.
  if [ -r /etc/os-release ]; then
    # shellcheck disable=SC1091
    . /etc/os-release
    if [ -n "${ID:-}" ]; then
      printf '%s' "$ID"
      return 0
    fi
  fi
  case "$(uname -s)" in
    FreeBSD)   printf 'freebsd' ;;
    NetBSD)    printf 'netbsd' ;;
    OpenBSD)   printf 'openbsd' ;;
    DragonFly) printf 'dragonfly' ;;
    Darwin)    printf 'darwin' ;;
    *)         printf 'unknown' ;;
  esac
}
# <<< shared package table: end

USER_PROVIDERS='nix'

# ---------------------------------------------------------------------- nix --

# ⭐ NIX IS LOADED, NEVER INSTALLED. Ruled by the operator on 2026-09-14: a Nix
# already on the machine is found even where this shell's PATH does not reach it,
# flakes are set up for the account and used for a new profile, and every Nix
# command this run starts gets the same settings. When Nix is absent the run says
# so and installs nothing, because installing Nix means running a remote script.
NIX_PROFILE_LINK=''
NIX_ROUTE=''
NIX_FLAKES_CONFIG=''
NIX_NEWLINE='
'

# nix_profile_link -> the account's profile link: the XDG state location when Nix
# made one there, and ~/.nix-profile otherwise, the order Nix's own profile script
# uses.
nix_profile_link() {
  npl_state="${XDG_STATE_HOME:-$HOME/.local/state}/nix/profile"
  if [ -e "$npl_state" ]; then
    printf '%s' "$npl_state"
    return 0
  fi
  printf '%s' "$HOME/.nix-profile"
}

# load_nix_environment puts an installed Nix on this run's PATH, and a CA bundle
# in NIX_SSL_CERT_FILE, as a login shell's profile script would.
#
# ⚠ IT SOURCES NOTHING. Nix ships a different profile script for its single-user
# and daemon installs, each written for an interactive shell, and a script in a
# user's own profile can do anything. The locations below are what those scripts
# read, taken from the filesystem instead. Run again after an install, so a
# profile made by that install is on PATH for the report.
load_nix_environment() {
  NIX_PROFILE_LINK=$(nix_profile_link)
  for ln_dir in /run/current-system/sw/bin /nix/var/nix/profiles/default/bin "$NIX_PROFILE_LINK/bin"; do
    if [ ! -d "$ln_dir" ]; then
      continue
    fi
    case ":$PATH:" in
      *":$ln_dir:"*) ;;
      *) PATH="$ln_dir:$PATH" ;;
    esac
  done
  export PATH
  if [ -n "${NIX_SSL_CERT_FILE:-}" ]; then
    return 0
  fi
  for ln_cert in /etc/ssl/certs/ca-certificates.crt /etc/ssl/ca-bundle.pem \
      /etc/ssl/certs/ca-bundle.crt /etc/pki/tls/certs/ca-bundle.crt \
      "$NIX_PROFILE_LINK/etc/ssl/certs/ca-bundle.crt" \
      /nix/var/nix/profiles/default/etc/ssl/certs/ca-bundle.crt; do
    if [ -e "$ln_cert" ]; then
      NIX_SSL_CERT_FILE=$ln_cert
      export NIX_SSL_CERT_FILE
      return 0
    fi
  done
}

detect_user_provider() {
  if have nix-env; then printf 'nix'; return 0; fi
  printf ''
}

# nix_run COMMAND... runs one Nix command with this run's settings.
#
# ⭐ THE SETTINGS ARE THE RUN'S, NOT THE MACHINE'S. flakes and the nix command, a
# source build when a binary substitute fails, bounded network waits, and every
# package nixpkgs would otherwise refuse as broken, insecure, unfree or unsupported,
# as the operator ruled. A caller's own NIX_CONFIG comes last, so a line it sets
# wins.
#
# ⛔ A GITHUB TOKEN REACHES NIX THROUGH NIX_CONFIG AND NOTHING ELSE. It is read from
# GITHUB_TOKEN when that is set, given to the command's environment, and never
# printed, logged or written to a file. It lifts GitHub's rate limit on the flake
# registry's nixpkgs.
nix_run() {
  nr_config="experimental-features = nix-command flakes${NIX_NEWLINE}fallback = true${NIX_NEWLINE}connect-timeout = 20${NIX_NEWLINE}download-attempts = 5${NIX_NEWLINE}max-jobs = auto${NIX_NEWLINE}warn-dirty = false"
  if [ -n "${GITHUB_TOKEN:-}" ]; then
    nr_config="$nr_config${NIX_NEWLINE}access-tokens = github.com=$GITHUB_TOKEN"
  fi
  if [ -n "${NIX_CONFIG:-}" ]; then
    nr_config="$nr_config${NIX_NEWLINE}$NIX_CONFIG"
  fi
  NIXPKGS_ALLOW_BROKEN=1 NIXPKGS_ALLOW_INSECURE=1 NIXPKGS_ALLOW_UNFREE=1 \
    NIXPKGS_ALLOW_UNSUPPORTED_SYSTEM=1 NIX_PAGER=cat NIX_CONFIG=$nr_config "$@"
}

# nix_route -> flakes or channels: how this account's profile is installed into.
#
# ⛔ A PROFILE HAS ONE FORMAT, AND AN EXISTING ONE DECIDES. `nix profile` rewrites a
# profile nix-env made, after which nix-env refuses it, so a profile with a
# manifest.nix stays on channels. A new profile, or one `nix profile` made, gets
# flakes when this Nix has them.
nix_route() {
  if [ -e "$NIX_PROFILE_LINK/manifest.json" ]; then
    printf 'flakes'
    return 0
  fi
  if [ -e "$NIX_PROFILE_LINK/manifest.nix" ]; then
    printf 'channels'
    return 0
  fi
  if have nix && nix_run nix flake --help >/dev/null 2>&1; then
    printf 'flakes'
    return 0
  fi
  printf 'channels'
}

# ensure_nix_flakes_config enables flakes and the nix command for this account's
# later shells, in its own nix.conf, and says what it found.
#
# ⚠ A LINE THE ACCOUNT ALREADY HAS IS ITS CHOICE. A file that names experimental
# features without flakes is left as it is and the run says so; this run's own
# commands get flakes either way, through nix_run.
ensure_nix_flakes_config() {
  nf_dir="${XDG_CONFIG_HOME:-$HOME/.config}/nix"
  nf_file="$nf_dir/nix.conf"
  nf_named=0
  if [ -f "$nf_file" ]; then
    while IFS= read -r nf_line; do
      case "$nf_line" in
        *=*) ;;
        *) continue ;;
      esac
      # ⚠ SPLIT ON THE FIRST `=`, NOT ON SPACES. nix.conf takes `key=value` with no
      # space as well as `key = value`, and a key read by words missed the first.
      # shellcheck disable=SC2086
      set -- ${nf_line%%=*}
      case "${1:-}" in
        experimental-features|extra-experimental-features) ;;
        *) continue ;;
      esac
      nf_named=1
      if in_list flakes "${nf_line#*=}" && in_list nix-command "${nf_line#*=}"; then
        NIX_FLAKES_CONFIG=present
        return 0
      fi
    done < "$nf_file"
  fi
  if [ "$nf_named" = 1 ]; then
    warn "$nf_file names experimental features without both nix-command and flakes; it is left as it is"
    NIX_FLAKES_CONFIG=left
    return 0
  fi
  if [ "$DRY_RUN" = 1 ]; then
    step "would enable flakes and the nix command in $nf_file"
    NIX_FLAKES_CONFIG=added
    return 0
  fi
  mkdir -p "$nf_dir"
  printf '\n# Added by %s.\nexperimental-features = nix-command flakes\n' "$SELF" >> "$nf_file"
  step "enabled flakes and the nix command in $nf_file"
  NIX_FLAKES_CONFIG=added
}

# nix_refresh updates the channels the account owns, before a channels install.
# ⚠ Only its own: a daemon account installs from the channel Nix was set up with,
# which only that channel's owner can update.
nix_refresh() {
  if [ "$NIX_ROUTE" != channels ]; then
    return 0
  fi
  nr_channels=$(nix_run nix-channel --list 2>/dev/null) || nr_channels=''
  if [ -z "$nr_channels" ]; then
    step 'this account has no channel of its own, so the install reads the channel Nix was set up with'
    return 0
  fi
  if [ "$DRY_RUN" = 1 ]; then
    step 'would run: nix-channel --update'
    return 0
  fi
  step 'nix-channel --update'
  nix_run nix-channel --update >/dev/null 2>&1
}

# first_line COMMAND... -> the command's first line of output, or nothing. This
# is `head -1`'s job, done with the shell so that a userland with no coreutils
# can still report its versions.
first_line() {
  "$@" 2>/dev/null | {
    if read -r fl_line; then
      printf '%s' "$fl_line"
    fi
  }
}

detect_libc() {
  # ⚠ `ldd --version` writes to stdout on glibc, to stderr on musl, and exits 1
  # on musl doing it. Looking for the loader is the answer that does not depend
  # on which one is there.
  #
  # ⚠ THE MULTIARCH PATH IS NOT OPTIONAL. Debian and Ubuntu keep libc at
  # /lib/x86_64-linux-gnu/libc.so.6 and nothing at the four obvious paths, so a
  # probe without the glob below reports `unknown` on the two most common
  # distributions there are. Measured, 2026-09-12.
  case "$(uname -s)" in
    Linux) ;;
    *) printf 'libc'; return 0 ;;
  esac
  for dl_candidate in /lib/ld-musl-* /usr/lib/ld-musl-*; do
    if [ -e "$dl_candidate" ]; then
      printf 'musl'
      return 0
    fi
  done
  for dl_candidate in \
      /lib/libc.so.6 /lib64/libc.so.6 /usr/lib/libc.so.6 /usr/lib64/libc.so.6 \
      /lib/*-linux-gnu/libc.so.6 /usr/lib/*-linux-gnu/libc.so.6; do
    if [ -e "$dl_candidate" ]; then
      printf 'glibc'
      return 0
    fi
  done
  printf 'unknown'
}

detect_wsl() {
  if [ -n "${WSL_DISTRO_NAME:-}" ] || [ -n "${WSLENV:-}" ]; then
    printf 'yes'
    return 0
  fi
  if [ -r /proc/sys/kernel/osrelease ]; then
    dw_release=''
    read -r dw_release < /proc/sys/kernel/osrelease || dw_release=''
    case "$dw_release" in
      *[Mm]icrosoft*) printf 'yes'; return 0 ;;
    esac
  fi
  printf 'no'
}

# ⭐ THE PRIVILEGE ANSWER IS THREE-VALUED, and collapsing it to a boolean is what
# makes a bootstrap hang. `sudo` without `-n` waits on a terminal an unattended
# run does not have, so a password-requiring sudo is reported as no privilege
# rather than tried.
detect_privilege() {
  if [ "$(id -u)" = 0 ]; then
    printf 'root'
    return 0
  fi
  if have sudo && sudo -n true 2>/dev/null; then
    printf 'sudo'
    return 0
  fi
  printf 'none'
}

as_root() {
  case "$PRIVILEGE" in
    root) "$@" ;;
    sudo) sudo -n "$@" ;;
    *)    return 1 ;;
  esac
}

# run_root DESCRIPTION COMMAND... - announces it, honours --dry-run, reports.
# ⭐ LOUD IS FOR THE RETRY, AND IT IS THE DIFFERENCE BETWEEN A REPORT AND A
# SHRUG. "apk could not install openssh" names the package and says nothing about
# why; the package manager's own message names the conflicting dependency. The
# bulk call stays quiet because its output is a screen of progress lines nobody
# reads, and the retry only runs when something has already gone wrong.
LOUD=0

run_root() {
  rr_what=$1
  shift
  if [ "$DRY_RUN" = 1 ]; then
    step "would run: $*"
    return 0
  fi
  step "$rr_what"
  if [ "$LOUD" = 1 ]; then
    if as_root "$@" >/dev/null; then
      return 0
    fi
    return 1
  fi
  if as_root "$@" >/dev/null 2>&1; then
    return 0
  fi
  return 1
}

# ------------------------------------------------------- the package manager --

refresh_index() {
  case "$PROVIDER" in
    apk)    run_root 'refreshing the apk index' apk update ;;
    apt)    run_root 'refreshing the apt index' apt-get update -qq ;;
    zypper) run_root 'refreshing the zypper index' zypper --non-interactive refresh ;;
    xbps)   run_root 'refreshing the xbps index' xbps-install -Sy ;;
    pkg)    run_root 'refreshing the pkg catalogue' pkg update ;;
    pkgin)  run_root 'refreshing the pkgin index' pkgin -y update ;;
    # ⚠ ARCH IS THE ONE THAT MUST NOT BE REFRESHED ALONE. `pacman -Sy` followed
    # by an install is a partial upgrade, and it is how a fresh Arch rootfs ends
    # up with a libc newer than the packages linked against it. A live run on
    # 2026-09-12 failed exactly there, so the refresh here IS the full upgrade.
    pacman) run_root 'upgrading pacman, which must not be refreshed alone' pacman -Syu --noconfirm ;;
    # dnf, yum, tdnf, emerge and pkg_add read a remote index per transaction, or
    # want a tree sync this script will not start on a caller's behalf.
    *)      return 0 ;;
  esac
}

install_packages() {
  if [ "$#" -eq 0 ]; then
    return 0
  fi
  case "$PROVIDER" in
    apk)     run_root "installing $#" apk add --no-cache "$@" ;;
    apt)     run_root "installing $#" env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends "$@" ;;
    pacman)  run_root "installing $#" pacman -S --noconfirm --needed "$@" ;;
    dnf)     run_root "installing $#" dnf -y --setopt=install_weak_deps=False install "$@" ;;
    yum)     run_root "installing $#" yum -y install "$@" ;;
    tdnf)    run_root "installing $#" tdnf install -y "$@" ;;
    zypper)  run_root "installing $#" zypper --non-interactive install --no-recommends "$@" ;;
    xbps)    run_root "installing $#" xbps-install -Sy "$@" ;;
    emerge)  run_root "installing $#" emerge --quiet --noreplace "$@" ;;
    pkg)     run_root "installing $#" pkg install -y "$@" ;;
    pkgin)   run_root "installing $#" pkgin -y install "$@" ;;
    # ⚠ pkg_add IS INTERACTIVE BY DEFAULT over an ambiguous name, and -I is what
    # turns that into a refusal rather than a prompt nothing will answer.
    pkg_add) run_root "installing $#" pkg_add -I "$@" ;;
    *)       return 1 ;;
  esac
}

# ⛔ THE USER-LEVEL PROVIDER IS USED AND NEVER INSTALLED. Installing nix means
# piping a remote script into a shell, which docs/security/remote-ops.md and
# docs/consumers.md both refuse for a script in this tree. Detect it, use it, and
# say what is needed when it is absent.
#
# install_nix_packages ATTRIBUTE... installs nixpkgs attributes into the account's
# profile by its route, in one transaction.
install_nix_packages() {
  if [ "$#" -eq 0 ]; then
    return 0
  fi
  case "$NIX_ROUTE" in
    flakes)
      in_installables=''
      for in_attr in "$@"; do
        in_installables="$in_installables nixpkgs#$in_attr"
      done
      if [ "$DRY_RUN" = 1 ]; then
        step "would run: nix profile install --impure$in_installables"
        return 0
      fi
      step "nix profile install --impure$in_installables"
      # ⚠ --impure IS WHAT LETS THE NIXPKGS_ALLOW VARIABLES THROUGH. A flake
      # evaluates purely by default and reads no environment variable at all.
      # shellcheck disable=SC2086
      set -- nix profile install --impure $in_installables
      ;;
    *)
      if [ "$DRY_RUN" = 1 ]; then
        step "would run: nix-env -f <nixpkgs> -iA $*"
        return 0
      fi
      step "nix-env -f <nixpkgs> -iA $*"
      # ⚠ -f <nixpkgs> AND NOT nixpkgs.NAME. The attribute path through
      # ~/.nix-defexpr names the channel, and a NixOS account's channel is `nixos`;
      # the search path names nixpkgs on both.
      set -- nix-env -f '<nixpkgs>' -iA "$@"
      ;;
  esac
  if [ "$LOUD" = 1 ]; then
    nix_run "$@" >/dev/null
    return $?
  fi
  nix_run "$@" >/dev/null 2>&1
}

# ---------------------------------------------------------------- fetch and --
# ------------------------------------------------------------------- digest --

fetch_to() {
  # $1 destination, $2 url. curl, then wget, then BSD fetch.
  if have curl; then
    curl -fsSL --retry 3 --retry-delay 2 -o "$1" "$2"
    return $?
  fi
  if have wget; then
    wget -q -O "$1" "$2"
    return $?
  fi
  if have fetch; then
    fetch -q -o "$1" "$2"
    return $?
  fi
  return 1
}

redirect_target() {
  # The URL a redirect lands on, which is how a `latest` release names its tag
  # without anything here having to parse JSON.
  if have curl; then
    curl -fsSL -o /dev/null -w '%{url_effective}' "$1" 2>/dev/null
    return 0
  fi
  printf ''
}

# first_field COMMAND... -> the first whitespace-separated word of the first
# output line. `_` is the throwaway, which is the one name shellcheck does not
# report as unused.
first_field() {
  "$@" 2>/dev/null | {
    if read -r ff_word _; then
      printf '%s' "$ff_word"
    fi
  }
}

sha256_of_file() {
  # ⚠ FIVE CANDIDATES, AND EVERY ONE OF THEM IS ABSENT SOMEWHERE. sha256sum is
  # coreutils, so a BSD base has `sha256` instead; a minimal image may have only
  # openssl; and node arrives with the toolset rather than before it. Whichever
  # answers first is used, and the digest is printed either way.
  if have sha256sum; then first_field sha256sum "$1"; return 0; fi
  if have sha256;    then sha256 -q "$1" 2>/dev/null; return 0; fi
  if have shasum;    then first_field shasum -a 256 "$1"; return 0; fi
  if have openssl;   then first_field openssl dgst -sha256 -r "$1"; return 0; fi
  if have node; then
    node -e '
      const crypto = require("crypto");
      const fs = require("fs");
      process.stdout.write(crypto.createHash("sha256").update(fs.readFileSync(process.argv[1])).digest("hex"));
    ' "$1"
    return 0
  fi
  printf ''
}

# decode_utf16 FILE -> the file as text on stdout.
#
# ⚠ THE POWERSHELL RELEASE PUBLISHES ITS DIGESTS AS UTF-16LE WITH A BYTE ORDER
# MARK. Read as bytes every hex character is followed by a NUL, so a plain
# `read` loop matches nothing and a `grep` for the digest finds nothing. This is
# the one place in the file that needs a program beyond the shell, and it says
# so rather than silently skipping the check.
decode_utf16() {
  if have node; then
    node -e '
      const fs = require("fs");
      let text = fs.readFileSync(process.argv[1]).toString("utf16le");
      if (text.charCodeAt(0) === 0xfeff) { text = text.slice(1); }
      process.stdout.write(text.replace(/\r/g, ""));
    ' "$1"
    return 0
  fi
  if have iconv; then
    iconv -f UTF-16 -t UTF-8 "$1" 2>/dev/null
    return 0
  fi
  if have tr; then
    tr -d '\000\377\376\r' < "$1"
    return 0
  fi
  return 1
}

# ------------------------------------------------------- the upstream tarball --

# ⭐ ONE UPSTREAM ROUTE, AND IT EXISTS BECAUSE ONE TOOL NEEDS IT. Measured on
# 2026-09-12: of ten Linux images, only Alpine, Wolfi and Photon carry a
# `powershell` package at all. Every other tool the toolsets name is in the
# system repositories of nearly every one of them, so adding an upstream route
# for those would be a second install path with nothing to do.
install_powershell_upstream() {
  if [ "$UPSTREAM" = 0 ]; then
    warn 'powershell is not in this system'"'"'s packages and --no-upstream was passed'
    return 1
  fi
  case "$(uname -s)" in
    Linux) ;;
    *)
      warn "powershell publishes no $(uname -s) tarball this script can place"
      return 1
      ;;
  esac
  case "$ARCH" in
    x86_64|amd64)  ps_arch=x64 ;;
    aarch64|arm64) ps_arch=arm64 ;;
    *)
      warn "powershell publishes no Linux tarball for $ARCH"
      return 1
      ;;
  esac
  ps_flavour=$ps_arch
  if [ "$LIBC" = musl ] && [ "$ps_arch" = x64 ]; then
    ps_flavour=musl-x64
  fi

  ps_landed=$(redirect_target 'https://github.com/PowerShell/PowerShell/releases/latest')
  ps_tag=${ps_landed##*/}
  case "$ps_tag" in
    v[0-9]*) ;;
    *)
      warn 'could not resolve the current powershell release tag'
      return 1
      ;;
  esac
  ps_version=${ps_tag#v}
  ps_file="powershell-$ps_version-linux-$ps_flavour.tar.gz"
  ps_base="https://github.com/PowerShell/PowerShell/releases/download/$ps_tag"
  step "powershell resolves to $ps_version, $ps_file"

  if [ "$DRY_RUN" = 1 ]; then
    step "would install $ps_file below $PREFIX/share/powershell"
    return 0
  fi

  ps_work="$PREFIX/.bootstrap-work.$$"
  rm -rf "$ps_work"
  mkdir -p "$ps_work"
  if ! fetch_to "$ps_work/$ps_file" "$ps_base/$ps_file"; then
    warn "could not download $ps_base/$ps_file"
    rm -rf "$ps_work"
    return 1
  fi

  ps_actual=$(sha256_of_file "$ps_work/$ps_file")
  if [ -z "$ps_actual" ]; then
    warn 'no sha256 tool is present, so the powershell tarball cannot be verified'
    rm -rf "$ps_work"
    return 1
  fi
  step "powershell tarball sha256 $ps_actual"

  ps_expected=''
  if fetch_to "$ps_work/hashes.sha256" "$ps_base/hashes.sha256"; then
    if decode_utf16 "$ps_work/hashes.sha256" > "$ps_work/hashes.txt"; then
      # ⚠ A SUBSTRING MATCH, NOT A SUFFIX MATCH. The digest file is CRLF as well
      # as UTF-16, so every decoded line ends in a carriage return that `read`
      # does not strip. A `*"$ps_file")` pattern therefore matched nothing, the
      # run found no digest, and it refused to install a tarball it had already
      # downloaded and hashed correctly. The decoder now drops the carriage
      # returns and this pattern no longer depends on it having done so.
      while read -r ps_hex ps_name; do
        case "$ps_name" in
          *"$ps_file"*) ps_expected=$ps_hex; break ;;
        esac
      done < "$ps_work/hashes.txt"
    else
      warn 'the release digests are UTF-16 and none of node, iconv or tr is here to decode them'
    fi
  fi
  if [ -z "$ps_expected" ]; then
    warn 'the powershell release published no digest this run could read; refusing rather than installing unverified bytes'
    rm -rf "$ps_work"
    return 1
  fi
  if [ "$ps_actual" != "$ps_expected" ]; then
    fail "the powershell tarball does not match the digest its own release publishes"
    rm -rf "$ps_work"
    return 1
  fi
  step 'powershell matches the digest its release publishes'
  if [ -n "$EXPECT_SHA256" ]; then
    if [ "$ps_actual" != "$EXPECT_SHA256" ]; then
      fail 'the powershell tarball does not match the --expect-sha256 value'
      rm -rf "$ps_work"
      return 1
    fi
    step 'powershell matches the value you supplied'
  fi

  ps_home="$PREFIX/share/powershell"
  rm -rf "$ps_home"
  mkdir -p "$ps_home" "$PREFIX/bin"
  if ! tar -xzf "$ps_work/$ps_file" -C "$ps_home"; then
    fail 'could not unpack the powershell tarball'
    rm -rf "$ps_work"
    return 1
  fi
  chmod 0755 "$ps_home/pwsh"
  ln -sf "$ps_home/pwsh" "$PREFIX/bin/pwsh"
  rm -rf "$ps_work"
  UPSTREAM_DONE="$UPSTREAM_DONE powershell"
  return 0
}

install_upstream() {
  case "$1" in
    powershell) install_powershell_upstream ;;
    *)          return 1 ;;
  esac
}

# ⚠ A ROUTE THAT CANNOT EXIST HERE IS NOT A ROUTE THAT FAILED. PowerShell ships a
# Linux tarball and no BSD one, so on FreeBSD the name is reported as skipped -
# which is what it is - rather than as a delivery this run failed to make.
has_upstream_route() {
  case "$1" in
    powershell)
      case "$KERNEL" in
        Linux) return 0 ;;
      esac
      return 1
      ;;
  esac
  return 1
}

# ------------------------------------------------------------ the extra tool --

CODEGRAPH_MAIN='@colbymchenry/codegraph'

npm_registry_version() {
  # `latest` goes through the dist-tag, which is the only form that answers with
  # one bare version rather than a list.
  if [ "$2" = latest ]; then
    first_line npm view "$1" dist-tags.latest
    return 0
  fi
  printf '%s' "$2"
}

npm_registry_integrity() { first_line npm view "$1@$2" dist.integrity; }

# npm_packs_to_a_directory VERSION answers whether that npm has --pack-destination.
#
# ⛔ MEASURED, NOT READ. On 2026-09-15 npm 6.14.11 took the directory for a second
# package, wrote the archive into its working directory and exited 1, which read as a
# fetch that failed; npm 7.17.0 exited 254; npm 7.18.0 wrote the archive where it was
# told. WSL-87.
npm_packs_to_a_directory() {
  case "$1" in
    [0-6].*|7.[0-9].*|7.1[0-7].*) return 1 ;;
    [0-9]*.*) return 0 ;;
  esac
  return 1
}

sri_of_file() {
  # ⚠ node is the hashing tool here on purpose. sha512sum prints hex, the
  # registry publishes base64, and every converter between them is absent on some
  # userland this runs on. node cannot be absent: nothing reaches here without
  # npm.
  node -e '
    const crypto = require("crypto");
    const fs = require("fs");
    const bytes = fs.readFileSync(process.argv[1]);
    process.stdout.write("sha512-" + crypto.createHash("sha512").update(bytes).digest("base64"));
  ' "$1"
}

# ⛔ IT ANSWERS THROUGH A GLOBAL AND NOT THROUGH STDOUT, and that is not a style
# choice. Called as `x=$(fetch_verified_npm ...)` it runs in a SUBSHELL, so every
# `fail` inside it incremented a copy of the counter that died with the subshell.
# Measured on 2026-09-12: a run whose digest step never completed reported
# `failures=0` and exited 0. A guard that cannot make the process fail is not a
# guard, and this is the shape that hides it.
FETCHED_ARCHIVE=

fetch_verified_npm() {
  # $1 work dir, $2 package, $3 exact version. Answers in FETCHED_ARCHIVE.
  #
  # ⚠ THE ARGUMENTS ARE COPIED BEFORE ANYTHING ELSE, because `set --` below
  # replaces them. The first version read `$2` and `$3` after that line, so the
  # registry was asked for the integrity of `@` and answered nothing.
  fv_work=$1
  fv_package=$2
  fv_version=$3
  FETCHED_ARCHIVE=
  # ⛔ THE DIRECTORY IS NAMED WITHOUT read. Its name was read back from a pipe with no
  # trailing newline, and under dash with `set -e` that substitution ends before its
  # printf: every Debian, Ubuntu and Void run exited 1 with `npm did not write exactly
  # one archive into ` and no directory. Measured on 2026-09-15. WSL-87.
  fv_name=
  for fv_part in $(split_on '/@' "$fv_package"); do
    fv_name=${fv_name:+$fv_name-}$fv_part
  done
  fv_dir="$fv_work/$fv_name"
  mkdir -p "$fv_dir"
  if ! npm pack "$fv_package@$fv_version" --ignore-scripts --pack-destination "$fv_dir" >/dev/null 2>&1; then
    fail "npm could not fetch $fv_package@$fv_version"
    return 1
  fi
  # ⛔ NOT `[ ... ] && [ ... ] || fail`. That is SC2015 under the shellcheck CI
  # carries, and it reads as clean under the newer one on a development host.
  set -- "$fv_dir"/*.tgz
  if [ "$#" -ne 1 ] || [ ! -f "$1" ]; then
    fail "npm did not write exactly one archive into $fv_dir"
    return 1
  fi
  fv_archive=$1

  fv_actual=$(sri_of_file "$fv_archive")
  fv_expected=$(npm_registry_integrity "$fv_package" "$fv_version")
  if [ -z "$fv_expected" ]; then
    fail "the npm registry published no integrity value for $fv_package@$fv_version"
    return 1
  fi
  if [ "$fv_actual" != "$fv_expected" ]; then
    fail "$fv_package@$fv_version does not match the registry integrity value"
    return 1
  fi
  if [ -n "$EXPECT_INTEGRITY" ] && [ "$fv_package" = "$CODEGRAPH_MAIN" ]; then
    if [ "$fv_actual" != "$EXPECT_INTEGRITY" ]; then
      fail "$fv_package@$fv_version does not match the --expect-integrity value"
      return 1
    fi
    step "$fv_package@$fv_version matches the value you supplied"
  fi
  step "$fv_package@$fv_version matches the registry integrity value"
  FETCHED_ARCHIVE=$fv_archive
}

install_codegraph() {
  cg_spec=$1
  # ⚠ A DRY RUN MUST NOT REPORT A FAILURE FOR A TOOL THE REAL RUN WOULD HAVE
  # INSTALLED FIRST. node and npm arrive with the toolset and a dry run installs
  # nothing, so asserting them here made `--dry-run --toolset agent` exit 1 on
  # every clean userland.
  if [ "$DRY_RUN" = 1 ]; then
    step "would install codegraph@$cg_spec below $PREFIX, after node and npm"
    return 0
  fi
  for cg_tool in node npm; do
    if ! have "$cg_tool"; then
      fail "codegraph needs $cg_tool, which is absent"
      return 1
    fi
  done
  cg_npm=$(first_line npm --version)
  if ! npm_packs_to_a_directory "$cg_npm"; then
    fail "codegraph is fetched with npm pack --pack-destination, which needs npm 7.18.0 or later, and this npm answers ${cg_npm:-nothing}"
    return 1
  fi

  case "$ARCH" in
    x86_64|amd64)  cg_platform="$CODEGRAPH_MAIN-linux-x64" ;;
    aarch64|arm64) cg_platform="$CODEGRAPH_MAIN-linux-arm64" ;;
    *)
      fail "codegraph publishes no Linux package for $ARCH"
      return 1
      ;;
  esac

  cg_version=$(npm_registry_version "$CODEGRAPH_MAIN" "$cg_spec")
  if [ -z "$cg_version" ]; then
    fail "the npm registry answered no version for $CODEGRAPH_MAIN@$cg_spec"
    return 1
  fi
  step "codegraph resolves to $cg_version"

  cg_work="$PREFIX/.bootstrap-work-npm.$$"
  rm -rf "$cg_work"
  mkdir -p "$cg_work"
  # ⚠ TWO NAMED ARCHIVES RATHER THAN A LIST, because a list in one variable has
  # to be word-split to become two arguments, and a path with a space in it then
  # becomes three. There are exactly two packages, and they are named.
  if ! fetch_verified_npm "$cg_work" "$cg_platform" "$cg_version"; then
    rm -rf "$cg_work"
    return 1
  fi
  cg_platform_archive=$FETCHED_ARCHIVE
  if ! fetch_verified_npm "$cg_work" "$CODEGRAPH_MAIN" "$cg_version"; then
    rm -rf "$cg_work"
    return 1
  fi
  cg_main_archive=$FETCHED_ARCHIVE

  mkdir -p "$PREFIX/bin"
  # The platform archive is installed by hand and --omit=optional is what stops
  # npm reaching the network for the other architectures.
  if ! npm install --global --prefix "$PREFIX" --ignore-scripts --omit=optional \
      "$cg_platform_archive" "$cg_main_archive" >/dev/null 2>&1; then
    fail 'npm could not install the verified codegraph archives'
    rm -rf "$cg_work"
    return 1
  fi
  rm -rf "$cg_work"
  CODEGRAPH_VERSION=$cg_version
  return 0
}

# ------------------------------------------------------------- the dotfiles --

script_dir() {
  # ⚠ A script read from a pipe has no directory, and `dirname` of `sh` or of a
  # dash answers the working directory, which is not where this file lives.
  # Answering nothing is honest; the caller then names the file.
  case "$0" in
    */*) ;;
    *)   printf ''; return 0 ;;
  esac
  if [ ! -f "$0" ]; then
    printf ''
    return 0
  fi
  ( CDPATH='' cd -- "${0%/*}" && pwd )
}

install_tmux_config() {
  if [ "$TMUX_CONFIG" = none ]; then
    return 0
  fi
  if [ -z "$TMUX_CONFIG" ]; then
    it_dir=$(script_dir)
    if [ -n "$it_dir" ] && [ -f "$it_dir/tmux.conf" ]; then
      TMUX_CONFIG="$it_dir/tmux.conf"
    fi
  fi
  if [ -z "$TMUX_CONFIG" ]; then
    warn 'no tmux.conf beside this script: pass --tmux-config PATH or --no-tmux-config'
    SKIPPED="$SKIPPED tmux-config"
    return 0
  fi
  if [ ! -f "$TMUX_CONFIG" ]; then
    fail "the tmux configuration $TMUX_CONFIG does not exist"
    return 1
  fi
  if [ "$DRY_RUN" = 1 ]; then
    step "would install $TMUX_CONFIG as $HOME/.tmux.conf"
    return 0
  fi
  # `install` is coreutils, and coreutils is one of the things being installed.
  cp "$TMUX_CONFIG" "$HOME/.tmux.conf"
  chmod 0644 "$HOME/.tmux.conf"
  step "installed $HOME/.tmux.conf"
  return 0
}

# ⚠ DEBIAN AND UBUNTU RENAME THE BINARY RATHER THAN THE PACKAGE. `fd-find`
# installs `/usr/bin/fdfind`, because another Debian package already owns the name
# `fd`. Measured on 2026-09-12: the install succeeded, the report said `fd` was
# absent, and both were true. A link in the prefix makes the tool usable by the
# name every other distribution gives it.
#
# ⚠ FREEBSD PUTS NIM OFF PATH ALTOGETHER. Its `nim` package installs 978 files with
# the binaries in `/usr/local/nim/bin` and nothing in `/usr/local/bin`, so `command
# -v nim` exited 127 after an install that succeeded. Measured on 2026-09-13 in the
# FreeBSD 15.1 guest. The same link reaches it by its absolute path.
link_renamed_binaries() {
  if [ "$DRY_RUN" = 1 ]; then
    return 0
  fi
  # Whitespace-separated pairs, `logical:what the system actually installed`, the
  # second either a name on PATH or an absolute path.
  lr_pairs=$(printf '%s' 'fd:fdfind nim:/usr/local/nim/bin/nim')
  for lr_pair in $lr_pairs; do
    lr_want=${lr_pair%%:*}
    lr_actual=${lr_pair#*:}
    if have "$lr_want"; then
      continue
    fi
    if ! have "$lr_actual"; then
      continue
    fi
    mkdir -p "$PREFIX/bin"
    ln -sf "$(command -v "$lr_actual")" "$PREFIX/bin/$lr_want"
    step "linked $PREFIX/bin/$lr_want to this distribution's $lr_actual"
  done
}

install_path_line() {
  ip_line="export PATH=\"$PREFIX/bin:\$PATH\""
  ip_profile="$HOME/.profile"
  if [ "$DRY_RUN" = 1 ]; then
    step "would make sure $ip_profile carries $PREFIX/bin"
    return 0
  fi
  # `: >>` creates without truncating, which is `touch`'s job here, and it is a
  # shell redirection rather than a program that may be absent.
  : >> "$ip_profile"
  ip_present=0
  while read -r ip_existing; do
    if [ "$ip_existing" = "$ip_line" ]; then
      ip_present=1
      break
    fi
  done < "$ip_profile"
  if [ "$ip_present" = 0 ]; then
    printf '\n# Added by %s.\n%s\n' "$SELF" "$ip_line" >> "$ip_profile"
    step "added $PREFIX/bin to $ip_profile"
  fi
  PATH="$PREFIX/bin:$PATH"
  export PATH
}

# ---------------------------------------------------------------- arguments --

TOOLSET=developer
WITH=''
WITHOUT=''
PROVIDER=''
USER_PROVIDER=''
CODEGRAPH=''
EXPECT_INTEGRITY=''
EXPECT_SHA256=''
TMUX_CONFIG=''
PREFIX=''
DRY_RUN=0
JSON=0
UPSTREAM=1

need_value() {
  if [ "$#" -lt 2 ]; then
    die "$1 needs a value"
  fi
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --toolset)          need_value "$@"; TOOLSET=$2; shift 2 ;;
    --with)             need_value "$@"; WITH=$2; shift 2 ;;
    --without)          need_value "$@"; WITHOUT=$2; shift 2 ;;
    --provider)         need_value "$@"; PROVIDER=$2; shift 2 ;;
    --user-provider)    need_value "$@"; USER_PROVIDER=$2; shift 2 ;;
    --no-upstream)      UPSTREAM=0; shift ;;
    --codegraph)        need_value "$@"; CODEGRAPH=$2; shift 2 ;;
    --expect-integrity) need_value "$@"; EXPECT_INTEGRITY=$2; shift 2 ;;
    --expect-sha256)    need_value "$@"; EXPECT_SHA256=$2; shift 2 ;;
    --tmux-config)      need_value "$@"; TMUX_CONFIG=$2; shift 2 ;;
    --no-tmux-config)   TMUX_CONFIG=none; shift ;;
    --prefix)           need_value "$@"; PREFIX=$2; shift 2 ;;
    --dry-run)          DRY_RUN=1; shift ;;
    --json)             JSON=1; shift ;;
    --list-providers)   printf 'system: %s\nuser:   %s\n' "$PROVIDERS" "$USER_PROVIDERS"; exit 0 ;;
    --list-names)       package_table; exit 0 ;;
    --version)          printf '%s/%s\n' "$SELF" "$VERSION"; exit 0 ;;
    -h|--help)          usage; exit 0 ;;
    *)                  usage >&2; die "unknown argument $1" ;;
  esac
done

if ! toolset_names "$TOOLSET" >/dev/null; then
  die "unknown toolset $TOOLSET; it is one of minimal, developer, languages, agent"
fi
if [ -z "$PREFIX" ]; then
  PREFIX="$HOME/.local"
fi
# ⛔ THE PREFIX REACHES AN `rm -rf`, so it is constrained here rather than trusted
# at each call site. Two places remove a directory below it: the npm work
# directory and any previous unpacked PowerShell. A relative prefix would make
# those depend on the working directory, and `/` would make them reach into the
# system. RULES.md section 3: the containment check belongs in one place, because
# a guard applied at several call sites is one that will one day be applied at
# fewer.
case "$PREFIX" in
  /) die '--prefix / is refused: this removes directories below the prefix' ;;
  /*) ;;
  *) die "--prefix must be an absolute path, and $PREFIX is not" ;;
esac
if [ -z "$CODEGRAPH" ]; then
  case "$TOOLSET" in
    agent) CODEGRAPH=latest ;;
    *)     CODEGRAPH=none ;;
  esac
fi
if [ -n "$EXPECT_INTEGRITY" ]; then
  case "$EXPECT_INTEGRITY" in
    sha512-*) ;;
    *) die '--expect-integrity takes an npm integrity value, which begins sha512-' ;;
  esac
fi

# ------------------------------------------------------------------- the run --

OS_ID=$(detect_os_id)
KERNEL=$(uname -s)
ARCH=$(uname -m)
LIBC=$(detect_libc)
IS_WSL=$(detect_wsl)
PRIVILEGE=$(detect_privilege)

if [ -z "$PROVIDER" ]; then
  PROVIDER=$(detect_provider)
elif ! in_list "$PROVIDER" "$PROVIDERS"; then
  die "--provider $PROVIDER is not one of: $PROVIDERS"
fi
if [ "$USER_PROVIDER" != none ]; then
  load_nix_environment
fi
if [ -z "$USER_PROVIDER" ]; then
  USER_PROVIDER=$(detect_user_provider)
elif [ "$USER_PROVIDER" = none ]; then
  USER_PROVIDER=''
elif ! in_list "$USER_PROVIDER" "$USER_PROVIDERS"; then
  die "--user-provider $USER_PROVIDER is not one of: $USER_PROVIDERS none"
elif ! have nix-env; then
  # ⚠ A NAMED PROVIDER IS CHECKED FOR HERE, ONCE. Without this, forcing one that
  # is not installed reached the install loop and failed per package: measured on
  # 2026-09-12, a named provider on a box without it reported five failures for one
  # missing program.
  die "--user-provider nix was asked for and no Nix is installed: nix-env is not on PATH, in $NIX_PROFILE_LINK/bin or in /nix/var/nix/profiles/default/bin. This script never installs Nix; the installer is at https://nixos.org/download"
fi

say "$OS_ID on $KERNEL $ARCH, $LIBC, wsl=$IS_WSL, privilege=$PRIVILEGE"
say "package manager: ${PROVIDER:-none}, user-level provider: ${USER_PROVIDER:-none}"

if [ -z "$PROVIDER" ] && [ -z "$USER_PROVIDER" ]; then
  die "no package manager found; looked for $PROVIDERS and $USER_PROVIDERS"
fi
USE_SYSTEM=0
if [ -n "$PROVIDER" ] && [ "$PRIVILEGE" != none ]; then
  USE_SYSTEM=1
elif [ -z "$USER_PROVIDER" ]; then
  die "$PROVIDER needs root and this account has neither root nor passwordless sudo; nix, as a user-level provider, would give a path that needs neither"
fi
if [ "$USE_SYSTEM" = 0 ]; then
  NIX_ROUTE=$(nix_route)
  say "nix $(first_line nix_run nix-env --version), installing through $NIX_ROUTE"
  ensure_nix_flakes_config
fi

# Compose the request: the toolset, plus --with, minus --without.
WANTED=''
WANTED_N=0
for name in $(toolset_names "$TOOLSET") $(commas_to_spaces "$WITH"); do
  if in_list "$name" "$(commas_to_spaces "$WITHOUT")"; then
    continue
  fi
  if in_list "$name" "$WANTED"; then
    continue
  fi
  WANTED="$WANTED $name"
  WANTED_N=$((WANTED_N + 1))
done

# ⭐ EVERY NAME IS RESOLVED BEFORE ANYTHING IS INSTALLED, so a name this file has
# no row for is reported before the first transaction rather than half way
# through one.
SYSTEM_PACKAGES=''
USER_LEVEL=''
UPSTREAM_WANTED=''
for name in $WANTED; do
  if ! package_row "$name" >/dev/null; then
    fail "no row for the logical name $name; --list-names prints the ones there are"
    continue
  fi
  if [ "$USE_SYSTEM" = 1 ]; then
    resolved=$(package_for "$name")
    if [ -z "$resolved" ]; then
      if has_upstream_route "$name"; then
        step "$OS_ID's $PROVIDER does not carry $name; the upstream release will be used"
        UPSTREAM_WANTED="$UPSTREAM_WANTED $name"
        REQUESTED="$REQUESTED $name"
        continue
      fi
      warn "$OS_ID's $PROVIDER does not carry $name"
      SKIPPED="$SKIPPED $name"
      continue
    fi
    SYSTEM_PACKAGES="$SYSTEM_PACKAGES $resolved"
  else
    # ⚠ THE TABLE'S nix KEY, AND NO os: KEY. A nixpkgs attribute is the same on
    # every system, and an `os:` override names that system's own package. The
    # first drive installed logical names as attributes, and nixpkgs has no
    # `ca-certificates` and no `tar`.
    resolved=$(package_for "$name" nix '')
    if [ -z "$resolved" ]; then
      warn "nixpkgs, through nix, does not carry $name, or carries it inside another package"
      SKIPPED="$SKIPPED $name"
      continue
    fi
    USER_LEVEL="$USER_LEVEL $resolved"
  fi
  REQUESTED="$REQUESTED $name"
done
if [ "$FAILURES" -gt 0 ]; then
  die 'the request names something this file has no row for, so nothing was installed'
fi

if [ -n "$SYSTEM_PACKAGES" ]; then
  if ! refresh_index; then
    warn "the $PROVIDER index refresh failed; the install below may be working from a stale list"
  fi
  # ⭐ ONE TRANSACTION FIRST, THEN ONE PACKAGE AT A TIME. The bulk call is what a
  # package manager is good at, and on Arch a per-package loop would be a
  # sequence of partial upgrades. ⚠ But a bulk call that fails installs NOTHING
  # and names nothing: measured on 2026-09-12, six of twelve images failed over
  # one absent package each and reported all eighteen as missing. The retry turns
  # that into partial success and a list of what is really absent.
  # shellcheck disable=SC2086
  if ! install_packages $SYSTEM_PACKAGES; then
    warn 'the single transaction failed; retrying one package at a time, with what the package manager says'
    LOUD=1
    for package in $SYSTEM_PACKAGES; do
      if ! install_packages "$package"; then
        fail "$PROVIDER could not install $package"
      fi
    done
    LOUD=0
  fi
fi
if [ -n "$USER_LEVEL" ]; then
  if ! nix_refresh; then
    warn 'the nix channel update failed; the install below may be working from a stale channel'
  fi
  # ⭐ THE SAME SHAPE AS THE SYSTEM ROUTE: one transaction, then one attribute at a
  # time with what nix says, so one missing attribute names itself rather than
  # failing every other one.
  # shellcheck disable=SC2086
  if ! install_nix_packages $USER_LEVEL; then
    warn 'the single nix transaction failed; retrying one attribute at a time, with what nix says'
    LOUD=1
    for attribute in $USER_LEVEL; do
      if ! install_nix_packages "$attribute"; then
        fail "nix could not install $attribute"
      fi
    done
    LOUD=0
  fi
  load_nix_environment
fi

install_path_line
link_renamed_binaries

# ⚠ A NAME THAT FAILED HERE IS REPORTED ONCE, NOT TWICE. It was already counted
# as requested, so leaving it in that list made the PATH probe below report it a
# second time as "installed without an error and not on PATH", which it was not.
for name in $UPSTREAM_WANTED; do
  if ! install_upstream "$name"; then
    fail "$name is not in $OS_ID's packages and its upstream release could not be installed either"
    UPSTREAM_FAILED="$UPSTREAM_FAILED $name"
  fi
done

if [ "$CODEGRAPH" != none ]; then
  install_codegraph "$CODEGRAPH" || true
fi

install_tmux_config || true

# ------------------------------------------------------------------ report ---

# ⭐ THE REPORT IS READ FROM THE MACHINE, NOT FROM WHAT WAS ASKED FOR. A line
# claiming a tool is present because an install command exited 0 is the class of
# claim this repository keeps finding to be false.
probe_for() {
  case "$1" in
    ripgrep)    printf 'rg' ;;
    openssh)    printf 'ssh' ;;
    procps)     printf 'ps' ;;
    rust)       printf 'rustc' ;;
    python)     if have python3; then printf 'python3'; else printf 'python'; fi ;;
    powershell) printf 'pwsh' ;;
    build)      if have cc; then printf 'cc'; else printf 'gcc'; fi ;;
    ca-certificates|coreutils) printf '' ;;
    *)          printf '%s' "$1" ;;
  esac
}

PRESENT=0
ABSENT=''
for name in $REQUESTED; do
  if in_list "$name" "$UPSTREAM_FAILED"; then
    continue
  fi
  probe=$(probe_for "$name")
  if [ -z "$probe" ]; then
    PRESENT=$((PRESENT + 1))
    continue
  fi
  if have "$probe"; then
    PRESENT=$((PRESENT + 1))
  else
    ABSENT="$ABSENT $name"
  fi
done

if [ "$DRY_RUN" = 1 ]; then
  ABSENT=''
fi
if [ -n "$ABSENT" ]; then
  fail "asked for, installed without an error, and not on PATH afterwards:$ABSENT"
fi

lead() { printf '%s' "${1# }"; }

if [ "$JSON" = 1 ]; then
  # ⚠ ONLY IDENTIFIERS, LISTS OF LOGICAL NAMES AND COUNTS REACH THIS OBJECT.
  # Every free-text message went to stderr and the version strings stay in the
  # text report, so there is nothing here that needs escaping this cannot do.
  printf '{"schema":"%s/%s"' "$SELF" "$VERSION"
  printf ',"os":"%s","kernel":"%s","arch":"%s","libc":"%s","wsl":"%s"' "$OS_ID" "$KERNEL" "$ARCH" "$LIBC" "$IS_WSL"
  printf ',"provider":"%s","user_provider":"%s","privilege":"%s"' "$PROVIDER" "$USER_PROVIDER" "$PRIVILEGE"
  printf ',"nix_route":"%s","nix_flakes_config":"%s"' "$NIX_ROUTE" "$NIX_FLAKES_CONFIG"
  printf ',"toolset":"%s","requested":%d,"present":%d' "$TOOLSET" "$WANTED_N" "$PRESENT"
  printf ',"skipped":"%s","absent":"%s","upstream":"%s"' "$(lead "$SKIPPED")" "$(lead "$ABSENT")" "$(lead "$UPSTREAM_DONE")"
  printf ',"codegraph":"%s","failures":%d}\n' "$CODEGRAPH_VERSION" "$FAILURES"
else
  printf 'os=%s\n'            "$OS_ID"
  printf 'kernel=%s\n'        "$KERNEL"
  printf 'arch=%s\n'          "$ARCH"
  printf 'libc=%s\n'          "$LIBC"
  printf 'wsl=%s\n'           "$IS_WSL"
  printf 'provider=%s\n'      "${PROVIDER:-none}"
  printf 'user_provider=%s\n' "${USER_PROVIDER:-none}"
  if [ -n "$NIX_ROUTE" ]; then
    printf 'nix_route=%s\n'         "$NIX_ROUTE"
    printf 'nix_flakes_config=%s\n' "$NIX_FLAKES_CONFIG"
  fi
  printf 'privilege=%s\n'     "$PRIVILEGE"
  printf 'toolset=%s\n'       "$TOOLSET"
  printf 'requested=%d\n'     "$WANTED_N"
  printf 'present=%d\n'       "$PRESENT"
  printf 'skipped=%s\n'       "$(lead "$SKIPPED")"
  printf 'absent=%s\n'        "$(lead "$ABSENT")"
  printf 'upstream=%s\n'      "$(lead "$UPSTREAM_DONE")"
  printf 'codegraph=%s\n'     "${CODEGRAPH_VERSION:-none}"
  # ⚠ `--version` IS NOT THE FLAG FOR ALL OF THEM, and the ones it is wrong for
  # do not fail loudly: `go --version` prints usage, `tmux --version` exits 1, and
  # `ssh --version` writes to stderr. The first report printed four empty values
  # beside tools it had just installed and verified were on PATH.
  for name in bash cc cargo curl fd git go jq less nim node npm pwsh python3 rg rustc ssh tar tmux unzip xz; do
    if ! have "$name"; then
      continue
    fi
    case "$name" in
      go)            value=$(first_line go version) ;;
      tmux)          value=$(first_line tmux -V) ;;
      unzip)         value=$(first_line unzip -v) ;;
      ssh)           value=$(ssh -V 2>&1 | { read -r line || line=""; printf '%s' "$line"; }) ;;
      *)             value=$(first_line "$name" --version) ;;
    esac
    # ⚠ An empty value reads as a tool that reported nothing, and the reader
    # cannot tell that from a tool this list asked the wrong question of.
    # FreeBSD base `unzip -v` writes nothing to stdout, so the line is omitted
    # rather than printed blank.
    if [ -n "$value" ]; then
      printf 'version.%s=%s\n' "$name" "$value"
    fi
  done
  printf 'failures=%d\n'      "$FAILURES"
fi

if [ "$FAILURES" -gt 0 ]; then
  exit 1
fi
exit 0
