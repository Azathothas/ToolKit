#!/bin/sh
# ⛔ GENERATED FILE. DO NOT EDIT IT.
#
# This is the shared package table from scripts/common/bootstrap.sh, copied byte
# for byte from its begin marker line to its end marker line, for the base
# provisioner to resolve its developer names from. ⚠ Nothing reads it yet;
# wiring the provisioner to it is the rest of WSL-70. Edit the block in
# bootstrap.sh, then rewrite this with:
#
#   sh scripts/common/check.sh package-table --fix
#
# The gate's package-table check refuses this file disagreeing with that block.

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
