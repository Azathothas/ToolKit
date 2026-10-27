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
  # >>> the grants at start: begin
  # ⛔ EACH GRANT IS MOUNTED AS ITS LINE IN THE BLOCK SAYS, FIRST AND AT EVERY
  # START. WSL runs `mount -a` with no PATH, and BusyBox's mount looks for WSL's
  # mount.drvfs on PATH. musl's default has no /sbin, where WSL keeps it. Measured
  # on alpine: every grant failed with `No such device`, so a restart mounted
  # none. A line WSL did not mount is mounted here, with /sbin on PATH. WSL-198.
  #
  # ⛔ AND A READ-ONLY GRANT IS MADE READ-ONLY. BusyBox hands the helper no `ro`,
  # so a read-only grant came up writable. A bind remount makes the mount point
  # read-only and asks the filesystem nothing. Measured on alpine: it holds, and
  # the account's write is refused. One that will not go read-only is unmounted,
  # so nothing writes through it. WSL-198.
  #
  # ⚠ UNDER THE GRANT LOCK grants.sh takes, so a grant or a revoke made as the
  # base starts is not undone from a block read before it. WSL-179, WSL-198.
  #
  # ⭐ THE LAST LINE SAYS THE GRANTS WERE CHECKED, and the verifier waits for it
  # while a grant is not yet as asked. WSL does not wait for a [boot] command.
  gb_lock=/var/lib/wsl-toolkit/grants.lock
  if command -v flock >/dev/null 2>&1 && mkdir -p "${gb_lock%/*}" 2>/dev/null && true 2>/dev/null >>"$gb_lock"; then
    exec 9>>"$gb_lock"
    gb_waited=0
    until flock -n 9 2>/dev/null; do
      if [ "$gb_waited" -ge 120 ]; then
        printf 'grant lock held for 120s, so the grants are checked without it\n'
        break
      fi
      sleep 1
      gb_waited=$((gb_waited + 1))
    done
  fi
  # gb_mount ARG...: mount, with the directories WSL keeps its helpers in on PATH.
  # A subshell, so the run state's check below meets the PATH it always did.
  gb_mount() (
    PATH=${PATH:+$PATH:}/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
    export PATH
    mount "$@"
  )
  # gb_live TARGET: sets gb_mode to the first option of the live Windows mount at
  # TARGET, ro or rw, or to nothing. The last row wins, as the mount on top does.
  gb_live() {
    gb_mode=
    while read -r _gb_from gb_at gb_type gb_options _gb_rest; do
      [ "$gb_at" = "$1" ] || continue
      case "$gb_type:$gb_options" in
        drvfs:*|9p:*aname=drvfs*) gb_mode=${gb_options%%,*} ;;
      esac
    done < /proc/mounts
  }
  gb_in=false
  gb_mounted=0
  gb_readonly=0
  gb_failed=0
  if [ -f /etc/fstab ]; then
    while IFS= read -r gb_line || [ -n "$gb_line" ]; do
      case "$gb_line" in
        '# wsl-toolkit mounts begin') gb_in=true; continue ;;
        '# wsl-toolkit mounts end') gb_in=false; continue ;;
      esac
      [ "$gb_in" = true ] || continue
      # A target holds letters, digits and `/._-` alone, so the split is safe.
      set -f
      # shellcheck disable=SC2086
      set -- $gb_line
      set +f
      case "${1:-#}" in '#'*) continue ;; esac
      gb_target=${2:-}
      [ -n "$gb_target" ] || continue
      case ",${4:-}," in *,ro,*) gb_want=ro ;; *) gb_want=rw ;; esac
      gb_live "$gb_target"
      if [ -z "$gb_mode" ]; then
        if gb_mount "$gb_target"; then
          printf 'grant %s mounted, which WSL had not\n' "$gb_target"
          gb_mounted=$((gb_mounted + 1))
          gb_live "$gb_target"
        else
          printf 'grant %s NOT mounted\n' "$gb_target"
          gb_failed=$((gb_failed + 1))
          continue
        fi
      fi
      if [ "$gb_want" = ro ] && [ "$gb_mode" = rw ]; then
        if gb_mount -o remount,bind,ro "$gb_target"; then
          printf 'grant %s remounted read-only, which its mount had dropped\n' "$gb_target"
          gb_readonly=$((gb_readonly + 1))
        else
          umount "$gb_target" || :
          printf 'grant %s NOT made read-only, so it was unmounted\n' "$gb_target"
          gb_failed=$((gb_failed + 1))
        fi
      fi
    done < /etc/fstab
  fi
  # ⛔ WSL'S OWN mount -a IS GIVEN ITS HELPER WHERE BUSYBOX LOOKS, for the next
  # start. grants.sh links it before it writes a block. This is for a base an
  # earlier release granted, whose block nothing writes again. WSL-198.
  gb_helper=/sbin/mount.drvfs
  gb_link=/usr/local/bin/mount.drvfs
  gb_mount_bin=/bin/mount
  if [ -e "$gb_helper" ] && [ ! -e "$gb_link" ] && [ ! -L "$gb_link" ]; then
    case "$(readlink -f "$gb_mount_bin" 2>/dev/null)" in
      */busybox)
        if mkdir -p "${gb_link%/*}" && ln -s "$gb_helper" "$gb_link"; then
          printf 'linked %s to %s, so mount -a finds it at the next start\n' "$gb_link" "$gb_helper"
        fi
        ;;
    esac
  fi
  printf 'grants checked: %s mounted here, %s made read-only, %s failed\n' "$gb_mounted" "$gb_readonly" "$gb_failed"
  exec 9>&-
  # <<< the grants at start: end
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
