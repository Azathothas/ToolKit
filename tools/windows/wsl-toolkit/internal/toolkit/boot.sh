# The boot script's text. provision.sh and engines.sh each write
# /usr/local/lib/wsl-toolkit/boot.sh as three lines of the base's own, then this
# text whole: `#!/bin/sh`, `seal=` and `account=`. One text, so a base an earlier
# release built gets the script a new build gets. WSL-68, WSL-178.
#
# seal and account are those two lines.
# shellcheck disable=SC2154
# >>> the boot script: begin
# Written by wsl-toolkit. WSL runs this as root at every start of this
# distribution, named by `command` in /etc/wsl.conf's [boot] section. seal is
# base.shared_tmpfs, and account is the engine's account.
set -u
log=/run/wsl-toolkit-boot.log
{
  printf 'boot %s\n' "$(date -u +%FT%TZ 2>/dev/null)"
  # ⛔ THE RESOLVER IS REFRESHED BEFORE THE UNMOUNT, AND A BAD READ CHANGES
  # NOTHING. The shared file is not always populated this early - measured empty
  # on 2026-09-17 - so a refresh that comes back short leaves the working file
  # alone. A resolver that works beats a fresher one that does not. WSL-68.
  if [ "$seal" = off ]; then
    # ⛔ AND THE SHARED FILE IS WAITED FOR, ten seconds at most. Read once, a start
    # that came before WSL wrote it kept an empty or old resolver for the whole
    # boot. WSL-188.
    w=0
    while [ ! -s /mnt/wsl/resolv.conf ] && [ "$w" -lt 10 ]; do
      sleep 1
      w=$((w + 1))
    done
    if [ -s /mnt/wsl/resolv.conf ]; then
      t=$(mktemp /etc/.resolv.XXXXXX 2>/dev/null) || t=
      if [ -n "$t" ]; then
        if grep -E '^(nameserver|search|options|domain) ' /mnt/wsl/resolv.conf > "$t" 2>/dev/null && [ -s "$t" ]; then
          chmod 0644 "$t" && mv "$t" /etc/resolv.conf && printf 'resolver refreshed\n'
        else
          rm -f "$t"
          printf 'resolver kept, the shared file gave nothing usable\n'
        fi
      fi
    else
      printf 'resolver kept, no shared file to read yet\n'
    fi
    if ! grep -q ' /mnt/wsl ' /proc/mounts 2>/dev/null; then
      printf 'shared tmpfs was not mounted\n'
    elif umount /mnt/wsl 2>/dev/null || umount -l /mnt/wsl 2>/dev/null; then
      printf 'shared tmpfs unmounted\n'
    else
      printf 'shared tmpfs NOT unmounted\n'
    fi
  fi
  # ⛔ THE ENGINE'S RUN STATE IS CHECKED AS ITS ACCOUNT, with the check `run`
  # sends and its lock, which engines.sh writes beside this. A record another
  # boot or an earlier start of the base left is cleared before a login shell or
  # a herdr pane meets it. Root removes nothing in the account's directory.
  # WSL-178.
  rs=/usr/local/lib/wsl-toolkit/run-state.sh
  if [ ! -f "$rs" ]; then
    printf 'run state not checked: %s is not written yet, and base ensure writes it\n' "$rs"
  elif command -v runuser >/dev/null 2>&1; then
    runuser -u "$account" -- /bin/sh "$rs" </dev/null || printf 'run state check exited %s\n' "$?"
  elif command -v su >/dev/null 2>&1; then
    su -s /bin/sh -c "/bin/sh $rs" "$account" </dev/null || printf 'run state check exited %s\n' "$?"
  else
    printf 'run state not checked: neither runuser nor su is here to become %s\n' "$account"
  fi
} > "$log" 2>&1
# ⛔ ALWAYS 0. WSL treats a failing boot command as a start it should complain
# about, and a base that will not start is worse than one whose tmpfs is open.
# The log above is where the answer is, and `base doors` is what reads the result.
exit 0
# <<< the boot script: end
