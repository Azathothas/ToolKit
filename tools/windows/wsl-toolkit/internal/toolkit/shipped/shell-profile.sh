# shell-profile.sh - the one interactive-shell mechanism this tree owns.
#
# WHAT IT DOES, AND IT IS ONE THING: an INTERACTIVE login shell whose working
# directory is a Windows drive that WSL mounted for it moves to the account's
# home and says so once. A directory this tool GRANTED is not touched, because
# that is the directory the caller asked to work in.
#
# ⚠ THE EXTENSION IS `.sh` ON PURPOSE, and it is a constraint rather than a
# label. CI runs `shellcheck -s sh` over every tracked `*.sh`, and POSIX sh is
# exactly this file's requirement: bash, dash, ash and FreeBSD sh all read it.
# Naming it `profile` or `shell-profile` would put it outside the one check that
# proves the requirement. So: no arrays, no `local`, no `[[`, no `$'...'`.
#
# ⛔ IT FETCHES NOTHING, EVER. A profile that updates itself from the network
# puts a fetch in front of every shell start and makes its own content
# untrackable, and a base with interop off and no configured egress cannot
# reach the network at all.
#
# ⛔ NO ALIASES AND NO PROMPT. An alias for a tool the toolset did not install is
# an error on every shell start, and a prompt is taste rather than a tool.
#
# `PATH` for the user prefix is NOT here. `bootstrap.sh` writes that line, and
# one fact has one home.
#
# `bootstrap.sh` installs this file and adds the line that reads it. By hand:
#   install -m 0644 shell-profile.sh ~/.local/share/wsl-toolkit/shell-profile.sh
# then in ~/.profile, on ONE line:
#   if [ -r "$HOME/.local/share/wsl-toolkit/shell-profile.sh" ]; then . "$HOME/.local/share/wsl-toolkit/shell-profile.sh"; fi

# Everything is inside one function so that `return` is legal whether this file
# is sourced or read by a shell that dislikes a bare top-level `return`, and so
# that the early exits read as one list rather than as nested ifs.
wsl_toolkit_profile_main() {
  # Sourced twice in one shell, the second time does nothing. ⚠ A PLAIN
  # VARIABLE AND NOT AN EXPORTED ONE: a nested login shell is a new shell and
  # decides for itself, and exporting this would silence it.
  if [ -n "${WSL_TOOLKIT_PROFILE:-}" ]; then
    return 0
  fi
  WSL_TOOLKIT_PROFILE=1

  # ⛔ INTERACTIVE, NOT LOGIN, AND THE DIFFERENCE IS THE WHOLE GUARD. This
  # tool's own `distro run -c` and `matrix -c` send every command to a LOGIN
  # shell, so a profile that moved a login shell would change the working
  # directory of every command any caller runs from a Windows drive - silently,
  # and after their `cd`. `$-` carries `i` only for a shell a person is typing
  # at, which is the shell this guard is about.
  case "$-" in
    *i*) ;;
    *)   return 0 ;;
  esac

  # Outside WSL there is no Windows drive to leave, and this costs one variable
  # read on every other system.
  if [ -z "${WSL_DISTRO_NAME:-}" ]; then
    wtp_release=''
    if [ -r /proc/sys/kernel/osrelease ]; then
      read -r wtp_release < /proc/sys/kernel/osrelease || wtp_release=''
    fi
    case "$wtp_release" in
      *[Mm]icrosoft*) ;;
      *)              return 0 ;;
    esac
  fi

  # ⭐ A SHELL STARTED THERE ON PURPOSE STAYS THERE. `base shell --here` asks
  # for the caller's Windows directory by name, and marks its shell through
  # WSLENV so this file can tell that shell from one that merely inherited the
  # directory. Without the mark the two are identical from in here.
  if [ -n "${WSL_TOOLKIT_HERE:-}" ]; then
    return 0
  fi

  wtp_pwd=${PWD:-}
  if [ -z "$wtp_pwd" ]; then
    wtp_pwd=$(pwd 2>/dev/null) || wtp_pwd=''
  fi
  if [ -z "$wtp_pwd" ]; then
    return 0
  fi

  # ⚠ THE AUTOMOUNT ROOT IS CONFIGURABLE, so reading it is the difference
  # between a guard and a guard that can never fire. /etc/wsl.conf is a few
  # lines, and this runs only for an interactive shell inside WSL.
  wtp_root=/mnt
  if [ -r /etc/wsl.conf ]; then
    wtp_in=0
    while IFS= read -r wtp_line || [ -n "$wtp_line" ]; do
      wtp_line=${wtp_line#"${wtp_line%%[![:blank:]]*}"}
      wtp_line=${wtp_line%"${wtp_line##*[![:blank:]]}"}
      case "$wtp_line" in
        '#'*|';'*) continue ;;
        '[automount]') wtp_in=1; continue ;;
        '['*']')       wtp_in=0; continue ;;
      esac
      if [ "$wtp_in" = 0 ]; then
        continue
      fi
      case "$wtp_line" in
        root|root[![:alnum:]_]*) ;;
        *) continue ;;
      esac
      case "$wtp_line" in
        *=*) ;;
        *)   continue ;;
      esac
      wtp_value=${wtp_line#*=}
      wtp_value=${wtp_value#"${wtp_value%%[![:blank:]]*}"}
      wtp_value=${wtp_value%"${wtp_value##*[![:blank:]]}"}
      case "$wtp_value" in
        '"'*'"') wtp_value=${wtp_value#'"'}; wtp_value=${wtp_value%'"'} ;;
      esac
      if [ -n "$wtp_value" ]; then
        wtp_root=$wtp_value
      fi
    done < /etc/wsl.conf
  fi
  # A trailing slash is allowed in the file and would build a prefix that
  # matches nothing.
  while :; do
    case "$wtp_root" in
      /) break ;;
      */) wtp_root=${wtp_root%/} ;;
      *)  break ;;
    esac
  done
  wtp_prefix="$wtp_root/"
  if [ "$wtp_root" = / ]; then
    wtp_prefix=/
  fi

  # ⛔ ONE DIRECTORY LEVEL, AND THAT IS DELIBERATE. WSL automounts a Windows
  # drive at <root>/<letter>; /mnt/wsl and /mnt/wslg are its own tmpfs and are
  # not a Windows drive, and a grant this tool writes lives somewhere else
  # entirely. Matching the whole of <root> would move a shell out of the
  # directory it was granted.
  case "$wtp_pwd" in
    "$wtp_prefix"?|"$wtp_prefix"?/*) ;;
    *) return 0 ;;
  esac

  if [ -z "${HOME:-}" ] || [ ! -d "$HOME" ]; then
    return 0
  fi
  if [ "$wtp_pwd" = "$HOME" ]; then
    return 0
  fi
  cd "$HOME" || return 0
  printf 'wsl-toolkit: moved out of %s, a Windows drive this guest mounted, to %s. A shell started with base shell --here stays where it was.\n' "$wtp_pwd" "$HOME" >&2
  return 0
}

wsl_toolkit_profile_main
unset wtp_release wtp_pwd wtp_root wtp_prefix wtp_in wtp_line wtp_value
unset -f wsl_toolkit_profile_main
:
