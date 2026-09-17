# shell-profile.sh - the one interactive-shell mechanism this tree owns.
#
# WHAT IT DOES, AND EVERY PART IS FOR AN INTERACTIVE SHELL ONLY:
#
#   1. an interactive login shell whose working directory is a Windows drive that
#      WSL mounted for it moves to the account's home and says so once. A
#      directory this tool GRANTED is not touched, because that is the directory
#      the caller asked to work in;
#   2. duplicate entries are taken out of the PATH it inherited;
#   3. shell history is given a home that survives, when nothing else gave it one.
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
# ⭐ WHAT WAS TAKEN FROM THE TWO REFERENCES, AND WHAT WAS NOT. `errandsh` is a
# PYTHON program that replaces a login shell to give pty-less SSH sessions echo,
# line editing and history; almost none of it is profile material. What it is
# right about, and what is here, is that history should survive a session and
# that every behaviour should have one named way to turn it off. The second
# reference is a 1,603-line bash profile whose PATH helpers drop duplicates; the
# de-duplication is here and nothing else is, because prompt colours, `rvm`
# state and a self-update path are the three things this file may not have.
#
# ⛔ NOTHING HERE ADDS A DIRECTORY TO `PATH`. `bootstrap.sh` writes the line for
# the user prefix and one fact has one home. This file only removes a repeat of
# something already there.
#
# ⭐ ONE SWITCH TURNS ALL OF IT OFF: `WSL_TOOLKIT_NO_PROFILE=1`.
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

  # ⭐ ONE NAMED WAY TO TURN THE WHOLE FILE OFF, which is what `errandsh` gets
  # right about its own behaviour: every thing it does has one variable that
  # stops it. A caller who wants the shell exactly as the image left it sets
  # this, and nothing below runs.
  if [ -n "${WSL_TOOLKIT_NO_PROFILE:-}" ]; then
    return 0
  fi

  # ⛔ INTERACTIVE, NOT LOGIN, AND THE DIFFERENCE IS THE WHOLE GUARD. This
  # tool's own `distro run -c` and `matrix -c` send every command to a LOGIN
  # shell, so a profile that changed one would change what every caller's
  # command sees - silently, and after their own setup. `$-` carries `i` only
  # for a shell a person is typing at, which is the shell this file is about.
  case "$-" in
    *i*) ;;
    *)   return 0 ;;
  esac

  # -- PATH, de-duplicated and nothing else -------------------------------------
  # ⭐ THE ONE IDEA WORTH TAKING from the reference profile's four PATH helpers.
  # A login shell inside a login shell runs /etc/profile again, and /etc/profile
  # appends - so /usr/local/bin appears twice in a nested shell, and the list a
  # person reads to work out which binary wins gets longer every time. The first
  # occurrence of each directory keeps its place, so which binary wins does not
  # change.
  #
  # ⛔ AN EMPTY ELEMENT MEANS THE CURRENT DIRECTORY and it is dropped. `PATH=/bin:`
  # searches `.` for every command typed, which is the oldest way to run someone
  # else's program by accident. Dropping it is the only change here that alters
  # what a command resolves to, and it is deliberate.
  if [ -n "${PATH:-}" ]; then
    wtp_new=''
    wtp_rest=$PATH
    while [ -n "$wtp_rest" ]; do
      case "$wtp_rest" in
        *:*) wtp_one=${wtp_rest%%:*}; wtp_rest=${wtp_rest#*:} ;;
        *)   wtp_one=$wtp_rest; wtp_rest='' ;;
      esac
      if [ -z "$wtp_one" ]; then
        continue
      fi
      case ":$wtp_new:" in
        *":$wtp_one:"*) continue ;;
      esac
      if [ -z "$wtp_new" ]; then
        wtp_new=$wtp_one
      else
        wtp_new=$wtp_new:$wtp_one
      fi
    done
    if [ -n "$wtp_new" ] && [ "$wtp_new" != "$PATH" ]; then
      PATH=$wtp_new
      export PATH
    fi
  fi

  # -- history that survives the session ----------------------------------------
  # ⭐ THE ONE IDEA WORTH TAKING from `errandsh`, whose reason to exist is that a
  # session without a pty loses its history. A base is long-lived and is attached
  # to again and again, so a history that dies with the pane is a real loss.
  #
  # ⛔ EVERY VALUE IS SET ONLY WHEN NOTHING SET IT. The account's own choice wins,
  # including a choice to send history nowhere.
  #
  # ⚠ THESE ARE PLAIN VARIABLES AND NOTHING ELSE. `shopt -s histappend` is bash's
  # and is not POSIX, so it is not here; a shell that does not know these names
  # ignores them, which is the degrade this file is required to make.
  if [ -n "${HOME:-}" ] && [ -d "$HOME" ]; then
    if [ -z "${HISTFILE:-}" ]; then
      HISTFILE=$HOME/.sh_history
      export HISTFILE
    fi
    if [ -z "${HISTSIZE:-}" ]; then
      HISTSIZE=10000
      export HISTSIZE
    fi
    if [ -z "${HISTFILESIZE:-}" ]; then
      HISTFILESIZE=20000
      export HISTFILESIZE
    fi
    # ⚠ bash reads this one and the others ignore it. `ignoreboth` keeps a repeat
    # and a line typed with a leading space out of the file.
    if [ -z "${HISTCONTROL:-}" ]; then
      HISTCONTROL=ignoreboth
      export HISTCONTROL
    fi
  fi

  # -- the Windows drive an interactive shell should not be sitting on ----------

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
unset wtp_release wtp_pwd wtp_root wtp_prefix wtp_in wtp_line wtp_value wtp_new wtp_rest wtp_one
unset -f wsl_toolkit_profile_main
:
