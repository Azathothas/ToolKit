#!/bin/sh
# The managed account's login shell: bash where the guest has bash. WSL-143.
#
# It defines two functions and runs nothing. The provisioning payload carries it
# ahead of provision.sh, which creates the account with wanted_login_shell, and
# every `base ensure` sends it as root with a call to apply_login_shell, which is
# what moves a base built by an older release.
#
# ⛔ WHY. `wsl.exe -d NAME -u USER` and an SSH session run the account's passwd
# shell, and provisioning created it with /bin/sh. So an attach had no line
# editing, no history and no login profile, on a base that has bash. Measured on
# the operator's base: passwd shell /bin/sh, bash 5.3.15 installed. Finding 44.

# wanted_login_shell prints the shell the account should have: bash by the
# spelling every distribution lists in /etc/shells where it is there, and /bin/sh
# on a guest with no bash at all.
wanted_login_shell() {
  for wls_candidate in /bin/bash /usr/bin/bash; do
    if [ -x "$wls_candidate" ]; then
      printf '%s\n' "$wls_candidate"
      return 0
    fi
  done
  if wls_found=$(command -v bash 2>/dev/null) && [ -n "$wls_found" ]; then
    printf '%s\n' "$wls_found"
    return 0
  fi
  printf '/bin/sh\n'
}

# apply_login_shell USER moves USER's login shell from /bin/sh to bash, and says
# what the shell is either way.
#
# ⛔ ONLY /bin/sh IS MOVED, because it is the one shell this tool ever set. An
# account somebody gave zsh or fish keeps it. ⚠ A shell that is not a program
# any more is replaced as well, since nothing can start it.
#
# ⚠ A LAST LINE WITH NO NEWLINE IS STILL READ: `read` fails on it and fills the
# fields anyway, so each loop also runs while a name came back.
apply_login_shell() {
  als_user=$1
  als_now=''
  als_found=no
  while IFS=: read -r als_name _als_password _als_uid _als_gid _als_gecos _als_home als_shell || [ -n "$als_name" ]; do
    if [ "$als_name" = "$als_user" ]; then
      als_now=$als_shell
      als_found=yes
      break
    fi
  done < /etc/passwd
  if [ "$als_found" != yes ]; then
    printf 'login-shell: /etc/passwd holds no entry for %s\n' "$als_user" >&2
    return 1
  fi
  als_want=$(wanted_login_shell)
  if [ -n "$als_now" ] && [ -x "$als_now" ] && [ ! -d "$als_now" ]; then
    case "$als_now" in
      */bash)
        printf 'login-shell %s\n' "$als_now"
        return 0
        ;;
    esac
    if [ "$als_now" != /bin/sh ]; then
      printf 'login-shell %s, chosen for this account and left as it is\n' "$als_now"
      return 0
    fi
    if [ "$als_want" = /bin/sh ]; then
      printf 'login-shell %s, and this guest has no bash\n' "$als_now"
      return 0
    fi
  fi
  if command -v usermod >/dev/null 2>&1; then
    usermod -s "$als_want" "$als_user" >/dev/null 2>&1 || :
  elif command -v chsh >/dev/null 2>&1; then
    chsh -s "$als_want" "$als_user" >/dev/null 2>&1 || :
  fi
  # ⛔ READ BACK, NOT TRUSTED. A tool that exits 0 having written nothing is the
  # shape this repository keeps finding.
  als_after=''
  while IFS=: read -r als_name _als_password _als_uid _als_gid _als_gecos _als_home als_shell || [ -n "$als_name" ]; do
    if [ "$als_name" = "$als_user" ]; then
      als_after=$als_shell
      break
    fi
  done < /etc/passwd
  if [ "$als_after" = "$als_want" ]; then
    printf 'login-shell %s, was %s\n' "$als_after" "${als_now:-nothing}"
    return 0
  fi
  printf 'login-shell: %s still has %s, and neither usermod nor chsh set %s\n' "$als_user" "${als_after:-nothing}" "$als_want" >&2
  return 1
}
