#!/bin/sh
# engines.sh: what the base's container engines need beyond what provision.sh
# builds. `base ensure` runs it on every base after provisioning, so a base an
# earlier release built is brought up to date with no rebuild. WSL-130, WSL-136.
#
# It runs as root, delivered on stdin. TK_USER names the account and TK_SYSTEMD
# says whether the base is configured to run systemd. TK_ROOT_ENGINE,
# TK_RUNTIME_DIR and TK_KVM are the configuration's opt-in settings, each true or
# false. TK_RUN_PREFIX names the engine's run directory, which the account's uid
# ends, and TK_RUN_STATE_B64 is the check `run` sends for a stale one.
# TK_BOOT_B64 is the text of the script WSL runs at every start. WSL-178.
# TK_MOUNT_HELPER_B64 is mount-helper.sh's text, which this runs. WSL-198.
#
# ⭐ EVERY FILE HERE BELONGS TO ONE SETTING, and that setting turned off removes
# it. The cgroup delegation and the run state's upkeep are the parts that are not
# opt-in: one makes a limit the engine accepts a limit the kernel enforces, and
# the other keeps the engine able to start after a restart of WSL.
#
# ⛔ THE DELEGATION NEVER FAILS THE ENSURE. A base whose user manager does not
# answer still runs containers, and `base status --probe` reports what it can
# account for. An opt-in setting that cannot be applied does fail it, naming why.
set -u

: "${TK_USER:?TK_USER is required}"
: "${TK_SYSTEMD:?TK_SYSTEMD is required}"
: "${TK_ROOT_ENGINE:?TK_ROOT_ENGINE is required}"
: "${TK_RUNTIME_DIR:?TK_RUNTIME_DIR is required}"
: "${TK_KVM:?TK_KVM is required}"
: "${TK_RUN_PREFIX:?TK_RUN_PREFIX is required}"
: "${TK_RUN_STATE_B64:?TK_RUN_STATE_B64 is required}"
: "${TK_BOOT_B64:?TK_BOOT_B64 is required}"
: "${TK_MOUNT_HELPER_B64:?TK_MOUNT_HELPER_B64 is required}"

say() { printf '  * %s\n' "$*"; }
die() { printf 'engines: %s\n' "$*" >&2; exit 3; }

# same_file A B is true when B exists and holds A's bytes. ⚠ NOT cmp, for the
# reason binfmt.sh gives: a base with no toolset carries no diffutils.
same_file() {
  [ -f "$2" ] || return 1
  [ "$(sha256sum < "$1")" = "$(sha256sum < "$2")" ]
}

# install_file TMP DEST MODE moves a written candidate into place when it
# differs from what is there, and is true only when it moved it.
install_file() {
  chmod "$3" "$1" || return 1
  if same_file "$1" "$2"; then
    rm -f "$1"
    return 1
  fi
  mv -f "$1" "$2"
}

uid=$(id -u "$TK_USER" 2>/dev/null) || die "the account $TK_USER does not exist"
home=$(getent passwd "$TK_USER" 2>/dev/null | cut -d: -f6) || home=
[ -n "$home" ] || home=/home/$TK_USER

# >>> as the account: begin
# as_account SCRIPT NAME ARG...: SCRIPT run by sh as the account. NAME is its $0,
# and each ARG is one of its positional parameters. So a file in its home is
# written by its owner, and not by root following whatever that directory holds.
# It goes through runuser, or through su where there is no runuser.
#
# ⛔ NOT setpriv. BusyBox's has no --reuid, so every ensure on alpine printed its
# usage, and root then removed the account's file itself. su is grants.sh's form,
# its options before the account, and no ARG begins with a dash. WSL-188, WSL-199.
as_account() {
  aa_script=$1
  shift
  if command -v runuser >/dev/null 2>&1; then
    runuser -u "$TK_USER" -- /bin/sh -c "$aa_script" "$@"
  elif command -v su >/dev/null 2>&1; then
    su -s /bin/sh -c "$aa_script" "$TK_USER" "$@"
  else
    return 127
  fi
}
# <<< as the account: end

systemd_up=false
if [ "$TK_SYSTEMD" = true ] && [ -d /run/systemd/system ]; then
  systemd_up=true
fi

# -- cgroup delegation, WSL-136 --------------------------------------------------
# ⭐ MEASURED ON AN ARCH BASE WITH SYSTEMD. WSL starts a command in a cgroup of
# its own the account cannot write, and the rootless engine then asks the
# account's user manager for a cgroup per container, under user@UID.service.
# systemd delegates cpu, memory and pids there by default; this adds cpuset and
# io, and a daemon-reload hands them to a running manager. Lingering keeps the
# manager running at boot with nobody logged in.
#
# ⛔ THE ENGINE IS SWITCHED TO THE SYSTEMD MANAGER ONLY WHERE THE ACCOUNT'S
# SESSION BUS ANSWERS. With no bus, podman prints four warnings on every call
# and falls back to cgroupfs, so the switch would add noise and change nothing.
# The switch is a drop-in beside provision.sh's own, 10-wsl-toolkit.conf, which
# keeps cgroupfs as the default this one overrides by sorting after it.
DELEGATE=/etc/systemd/system/user@.service.d/50-wsl-toolkit-delegate.conf
MANAGER_REL=.config/containers/containers.conf.d/50-wsl-toolkit-cgroup.conf

# write_manager writes or removes the account's drop-in, as the account.
write_manager() {
  # $1 and $2 are expanded by the inner shell, which is the point.
  # shellcheck disable=SC2016
  as_account '
    set -eu
    f=$1/$2
    if [ "$3" = remove ]; then rm -f "$f"; exit 0; fi
    mkdir -p "${f%/*}"
    t=$(mktemp "$f.XXXXXX")
    printf "%s\n" "# Written by wsl-toolkit: the user manager holds a cgroup per container. WSL-136." "[engine]" "cgroup_manager = \"systemd\"" > "$t"
    chmod 0644 "$t"
    mv -f "$t" "$f"
  ' wtk-manager "$home" "$MANAGER_REL" "$1"
}

if [ "$systemd_up" = true ]; then
  mkdir -p "${DELEGATE%/*}" || die "could not create ${DELEGATE%/*}"
  tmp=$(mktemp "$DELEGATE.XXXXXX") || die "no temporary file beside $DELEGATE"
  printf '%s\n' '# Written by wsl-toolkit: the controllers a rootless engine sets limits with. WSL-136.' \
    '[Service]' 'Delegate=cpu cpuset io memory pids' > "$tmp"
  if install_file "$tmp" "$DELEGATE" 0644; then
    systemctl daemon-reload >/dev/null 2>&1 || say "⚠ systemctl daemon-reload failed, so the delegation waits for the next start"
  fi
  if [ ! -e "/var/lib/systemd/linger/$TK_USER" ]; then
    if ! loginctl enable-linger "$TK_USER" >/dev/null 2>&1; then
      mkdir -p /var/lib/systemd/linger && : > "/var/lib/systemd/linger/$TK_USER"
    fi
  fi
  # ⚠ --no-block, and the wait below is bounded: a manager that cannot start
  # must not hold the ensure.
  systemctl start --no-block "user@$uid.service" >/dev/null 2>&1 || :
  # ⚠ THE WAIT IS ONLY WHERE A SESSION BUS CAN APPEAR. A base with no user
  # dbus.socket, such as Debian without dbus-user-session, never gets one, and
  # waited 10 seconds in every ensure for it.
  i=10
  for unit in /etc/systemd/user/dbus.socket /usr/lib/systemd/user/dbus.socket /lib/systemd/user/dbus.socket; do
    if [ -e "$unit" ]; then i=0; break; fi
  done
  while [ ! -S "/run/user/$uid/bus" ] && [ "$i" -lt 10 ]; do sleep 1; i=$((i+1)); done
  delegated=$(cat "/sys/fs/cgroup/user.slice/user-$uid.slice/user@$uid.service/cgroup.controllers" 2>/dev/null) || delegated=
  if [ -S "/run/user/$uid/bus" ] && systemctl is-active --quiet "user@$uid.service" && write_manager write; then
    say "cgroup delegation: ${delegated:-none read} to $TK_USER's user manager, which holds a cgroup per container"
  else
    write_manager remove || :
    say "⚠ cgroup delegation: $TK_USER's user manager has no session bus, so its engine keeps cgroupfs and a limit can go unenforced"
  fi
else
  # ⚠ A BASE THAT NO LONGER RUNS SYSTEMD keeps no drop-in pointing its engine at
  # a manager it does not have.
  write_manager remove || rm -f "$home/$MANAGER_REL"
  rm -f "$DELEGATE"
  say "cgroup delegation: none, because this base does not run systemd"
fi

# -- the engine's run state, WSL-178 -----------------------------------------------
# ⛔ THE RUN STATE IS PER BOOT AND IT LIVES ON THE DISK. podman keeps it under
# $TK_RUN_PREFIX$uid, and /tmp is no tmpfs here: the default base has no /tmp
# mount. So every restart of WSL's VM left a record naming another boot, and
# podman refused every container until something cleared it. A job clears it
# first, WSL-137; a login shell, a herdr pane and podman typed by hand did not.
# So the job's own check runs at every start too, as the account, under its lock.
# ⚠ Measured on a scratch Arch base: with systemd, tmp.mount puts a tmpfs on /tmp,
# so the unit finds a current record there. Without systemd, /tmp is on the disk,
# and the boot script found another boot's record and cleared it.
#
# ⛔ ROOT REMOVES NOTHING IN THE ACCOUNT'S DIRECTORY. The account owns that
# directory and its name in /tmp, so a path root checked could be a symlink by
# the time root removed it. The unit and the boot script run the check as the
# account, which is what `run` does.
#
# ⛔ AND THE CLEANER LEAVES IT. Arch's tmp.conf ages /tmp at 10 days, which can
# remove the state of a container that runs longer. The x line keeps that age
# off it. A boot's own removal of /tmp, where a distribution's tmp.conf asks for
# one, is not age, and it stays.
RUN_STATE=/usr/local/lib/wsl-toolkit/run-state.sh
RUN_UNIT=wsl-toolkit-run-state.service
RUN_UNIT_PATH=/etc/systemd/system/$RUN_UNIT
TMPFILES=/etc/tmpfiles.d/wsl-toolkit.conf
mkdir -p "${RUN_STATE%/*}" || die "could not create ${RUN_STATE%/*}"
tmp=$(mktemp "$RUN_STATE.XXXXXX") || die "no temporary file beside $RUN_STATE"
printf '%s' "$TK_RUN_STATE_B64" | base64 -d > "$tmp" || die "could not write $RUN_STATE"
install_file "$tmp" "$RUN_STATE" 0755 || :
mkdir -p "${TMPFILES%/*}" || die "could not create ${TMPFILES%/*}"
tmp=$(mktemp "$TMPFILES.XXXXXX") || die "no temporary file beside $TMPFILES"
printf '%s\n' "# Written by wsl-toolkit: the engine's run state lives as long as its containers. WSL-178." \
  "x $TK_RUN_PREFIX*" > "$tmp"
install_file "$tmp" "$TMPFILES" 0644 || :
if [ "$systemd_up" = true ]; then
  tmp=$(mktemp "$RUN_UNIT_PATH.XXXXXX") || die "no temporary file for $RUN_UNIT"
  cat > "$tmp" <<UNIT_FILE
[Unit]
Description=wsl-toolkit: clear the engine run state a restart of WSL left naming another boot
After=local-fs.target

[Service]
Type=oneshot
User=$TK_USER
TimeoutStartSec=5min
ExecStart=-/bin/sh $RUN_STATE

[Install]
WantedBy=multi-user.target
UNIT_FILE
  if install_file "$tmp" "$RUN_UNIT_PATH" 0644; then
    systemctl daemon-reload >/dev/null 2>&1 || say "⚠ systemctl daemon-reload failed, so $RUN_UNIT waits for the next start"
  fi
  if systemctl enable "$RUN_UNIT" >/dev/null 2>&1; then
    say "run state: $RUN_UNIT clears another boot's record at every start, and $TMPFILES keeps the cleaner off it"
  else
    say "⚠ run state: systemctl could not enable $RUN_UNIT, so only a job clears another boot's record"
  fi
elif [ -f "$RUN_UNIT_PATH" ]; then
  systemctl disable "$RUN_UNIT" >/dev/null 2>&1 || :
  rm -f "$RUN_UNIT_PATH"
fi

# -- the boot script, WSL-178 ------------------------------------------------------
# ⛔ EVERY ENSURE PUTS IT IN PLACE, NOT ONLY PROVISIONING. A base that verifies is
# never provisioned again, so the default base an earlier release built had no
# boot line. Only a job cleared another start's state there, and its herdr panes,
# its `base shell` and podman typed by hand met that state first. The script
# runs the check above on every base, and the unit runs it too where systemd does.
#
# ⭐ NOTHING RESTARTS FOR IT. WSL reads /etc/wsl.conf at a start, which is when the
# script is needed. The line written here is where provisioning writes it, so the
# next provisioning finds wsl.conf unchanged and restarts nothing for it either.
#
# ⛔ THE SEAL IS THE BASE'S OWN, NEVER THE CONFIGURATION'S. Closing /mnt/wsl needs
# the resolver provisioning writes first, so this never seals or unseals a base.
# It keeps the seal the script there names, and a sealed base an earlier release
# built names seal-boot.sh. WSL-68.
BOOT=/usr/local/lib/wsl-toolkit/boot.sh
OLD_BOOT=/usr/local/lib/wsl-toolkit/seal-boot.sh
BOOT_LINE="command=$BOOT"
WSL_CONF=/etc/wsl.conf
seal=$(sed -n '2s/^seal=//p' "$BOOT" 2>/dev/null) || seal=
if [ -z "$seal" ]; then
  if grep -qxF "command=$OLD_BOOT" "$WSL_CONF" 2>/dev/null; then seal=off; else seal=on; fi
fi
if [ "$seal" != on ] && [ "$seal" != off ]; then
  say "⚠ boot script: $BOOT names the seal '$seal', so it was left as it is"
else
  tmp=$(mktemp "$BOOT.XXXXXX") || die "no temporary file beside $BOOT"
  {
    printf '#!/bin/sh\nseal=%s\naccount=%s\n' "$seal" "$TK_USER"
    printf '%s' "$TK_BOOT_B64" | base64 -d
  } > "$tmp" || die "could not write $BOOT"
  grep -qx '# <<< the boot script: end' "$tmp" ||
    { rm -f "$tmp"; die "the boot script handed to engines.sh was cut short"; }
  install_file "$tmp" "$BOOT" 0755 || :
  # ⚠ THE LINE GOES WHERE PROVISIONING PUTS IT: over the old script's line, or
  # over the blank line an earlier release left after systemd= in [boot]. A file
  # this tool did not write, or one naming another boot command, is left alone.
  if grep -qxF "$BOOT_LINE" "$WSL_CONF" 2>/dev/null; then
    say "boot script: /etc/wsl.conf names $BOOT, which runs at every start"
  elif [ "$(head -n 1 "$WSL_CONF" 2>/dev/null)" != '# Written by wsl-toolkit.' ]; then
    say "⚠ boot script: /etc/wsl.conf is not this tool's, so it names no boot script until base recreate writes one"
  elif grep '^command=' "$WSL_CONF" 2>/dev/null | grep -qvxF "command=$OLD_BOOT"; then
    say "⚠ boot script: /etc/wsl.conf names another boot command, so it was left as it is"
  else
    tmp=$(mktemp "$WSL_CONF.XXXXXX") || die "no temporary file beside $WSL_CONF"
    awk -v line="$BOOT_LINE" -v old="command=$OLD_BOOT" '
      function put() { if (!done) { print line; done = 1 } }
      /^\[/ { if (inboot) put(); inboot = ($0 == "[boot]"); after = 0; print; next }
      inboot && !done && $0 == old { put(); next }
      inboot && !done && after && $0 == "" { put(); after = 0; next }
      { after = (inboot && $0 ~ /^systemd=/); print }
      END { if (inboot) put(); if (!done) { print ""; print "[boot]"; print line } }
    ' "$WSL_CONF" > "$tmp" || die "could not write $WSL_CONF"
    if grep -qxF "$BOOT_LINE" "$tmp" && chmod 0644 "$tmp" && mv -f "$tmp" "$WSL_CONF"; then
      say "boot script: /etc/wsl.conf names $BOOT from the next start, and nothing restarts for it"
    else
      rm -f "$tmp"
      say "⚠ boot script: /etc/wsl.conf could not be given the line that names $BOOT"
    fi
  fi
  # ⛔ THE OLD NAME GOES ONLY ONCE NOTHING NAMES IT. Removed first, a base whose
  # wsl.conf could not be rewritten would start naming a script that was gone,
  # and a sealed one would keep /mnt/wsl mounted with nothing logged.
  if ! grep -qF "$OLD_BOOT" "$WSL_CONF" 2>/dev/null; then
    rm -f "$OLD_BOOT"
  fi
fi

# -- WSL's mount helper, WSL-198 --------------------------------------------------
# >>> the mount helper's install: begin
# ⛔ EVERY ENSURE PUTS IT IN PLACE, where /bin/mount is BusyBox's. grants.sh does
# too, but only when a grant changes. So an alpine base an earlier release granted
# would go on mounting no grant at a start. mount-helper.sh says why.
#
# ⚠ IT NEVER FAILS THE ENSURE. A helper that could not be written is named, and a
# restart without it mounts no grant, which the verifier then names.
if mh_file=$(mktemp 2>/dev/null); then
  if printf '%s' "$TK_MOUNT_HELPER_B64" | base64 -d > "$mh_file" 2>/dev/null; then
    sh "$mh_file" || say "⚠ mount helper: its install exited $?"
  else
    say "⚠ mount helper: its payload could not be decoded, so it was not written"
  fi
  rm -f "$mh_file"
else
  say "⚠ mount helper: no temporary file for its install, so it was not written"
fi
# <<< the mount helper's install: end

# -- root's engine, WSL-130 ------------------------------------------------------
# ⛔ ROOT'S PODMAN WANTS THE SYSTEMD CGROUP MANAGER BY DEFAULT, AND THAT NEEDS A
# SYSTEM BUS. A base with systemd and no bus fails root's every run with `crun:
# cannot open sd-bus`, which pg-toolkit measured. So root gets cgroupfs where
# there is no bus or no systemd, and the file event log the account has.
# ⚠ binfmt.sh does not need this: it passes both on its own command line,
# WSL-168. This is for what the operator runs as root, `base exec --root`.
ROOT_CONF=/etc/containers/containers.conf.d/50-wsl-toolkit-root.conf
if [ "$TK_ROOT_ENGINE" = true ]; then
  manager=systemd
  if [ "$systemd_up" != true ] || [ ! -S /run/dbus/system_bus_socket ]; then
    manager=cgroupfs
  fi
  mkdir -p "${ROOT_CONF%/*}" || die "could not create ${ROOT_CONF%/*}"
  tmp=$(mktemp "$ROOT_CONF.XXXXXX") || die "no temporary file beside $ROOT_CONF"
  {
    printf '%s\n' "# Written by wsl-toolkit: root's engine, set up as the account's is. WSL-130."
    printf '%s\n' '[engine]'
    if [ "$manager" = cgroupfs ]; then printf '%s\n' 'cgroup_manager = "cgroupfs"'; fi
    printf '%s\n' 'events_logger = "file"'
  } > "$tmp"
  install_file "$tmp" "$ROOT_CONF" 0644 || :
  command -v podman >/dev/null 2>&1 || die "base.root_engine is on and podman is not installed"
  got=$(podman info --format '{{.Host.CgroupManager}} {{.Host.EventLogger}}' 2>/dev/null) ||
    die "base.root_engine is on and root's podman does not answer podman info"
  say "root engine: $got"
else
  rm -f "$ROOT_CONF"
fi

# -- the account's runtime directory, WSL-130 ------------------------------------
# ⛔ THE ENGINE'S RUN STATE IS ONE DIRECTORY, AND A SHELL THAT NAMES ANOTHER IS
# REFUSED BY IT. A login shell gets XDG_RUNTIME_DIR from WSL, and podman there
# answered `database configuration mismatch` against the state this tool's jobs
# keep. The account's login shells now name the one the jobs use.
PROFILE=/etc/profile.d/wsl-toolkit-runtime-dir.sh
if [ "$TK_RUNTIME_DIR" = true ]; then
  mkdir -p "${PROFILE%/*}" || die "could not create ${PROFILE%/*}"
  tmp=$(mktemp "$PROFILE.XXXXXX") || die "no temporary file beside $PROFILE"
  cat > "$tmp" <<PROFILE_FILE
# Written by wsl-toolkit: the account's engine keeps its run state here, so its
# login shells name the same directory its jobs do. WSL-130.
if [ "\$(id -u 2>/dev/null)" = "$uid" ]; then
  _wtk_run=$TK_RUN_PREFIX$uid
  (umask 077; mkdir "\$_wtk_run") 2>/dev/null || :
  if [ -d "\$_wtk_run" ] && [ ! -L "\$_wtk_run" ] && [ "\$(stat -c %u "\$_wtk_run" 2>/dev/null)" = "$uid" ]; then
    chmod 700 "\$_wtk_run" 2>/dev/null || :
    XDG_RUNTIME_DIR=\$_wtk_run; export XDG_RUNTIME_DIR
  fi
  unset _wtk_run
fi
PROFILE_FILE
  install_file "$tmp" "$PROFILE" 0644 || :
  say "runtime directory: $TK_RUN_PREFIX$uid in $TK_USER's login shells"
else
  rm -f "$PROFILE"
fi

# -- /dev/kvm for the account, WSL-130 -------------------------------------------
# ⛔ THE NODE'S MODE IS SET AT EVERY BOOT AND A HAND FIX DOES NOT SURVIVE ONE.
# podbox measured /dev/kvm as `crw------- root root` and set it by hand. So the
# account joins the kvm group and a unit opens the node to that group at each
# start. ⚠ It never narrows what the node already allows: a base whose own rule
# makes it 0666 keeps 0666.
KVM_UNIT=wsl-toolkit-kvm.service
KVM_PATH=/etc/systemd/system/$KVM_UNIT
if [ "$TK_KVM" = true ]; then
  [ "$systemd_up" = true ] || die "base.kvm keeps /dev/kvm usable through a systemd unit, and this base does not run systemd"
  [ -c /dev/kvm ] || die "/dev/kvm is not in this base: WSL exposes it only with nested virtualization on in .wslconfig"
  getent group kvm >/dev/null 2>&1 || groupadd -r kvm || die "could not create the kvm group"
  if ! id -nG "$TK_USER" | tr ' ' '\n' | grep -qx kvm; then
    usermod -aG kvm "$TK_USER" 2>/dev/null || gpasswd -a "$TK_USER" kvm >/dev/null 2>&1 ||
      die "could not add $TK_USER to the kvm group"
    say "added $TK_USER to the kvm group"
  fi
  tmp=$(mktemp "$KVM_PATH.XXXXXX") || die "no temporary file for $KVM_UNIT"
  cat > "$tmp" <<'UNIT_FILE'
[Unit]
Description=wsl-toolkit: keep /dev/kvm open to the kvm group after each boot
After=systemd-tmpfiles-setup-dev.service systemd-udev-trigger.service
ConditionPathExists=/dev/kvm

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=chgrp kvm /dev/kvm
ExecStart=chmod g+rw /dev/kvm

[Install]
WantedBy=multi-user.target
UNIT_FILE
  if install_file "$tmp" "$KVM_PATH" 0644; then
    systemctl daemon-reload || die "systemctl daemon-reload failed"
  fi
  systemctl enable "$KVM_UNIT" >/dev/null 2>&1 || die "systemctl could not enable $KVM_UNIT"
  systemctl restart "$KVM_UNIT" || die "systemctl could not start $KVM_UNIT"
  say "kvm: /dev/kvm $(stat -c '%A %U:%G' /dev/kvm), $TK_USER in kvm, $KVM_UNIT enabled"
elif [ -f "$KVM_PATH" ]; then
  systemctl disable "$KVM_UNIT" >/dev/null 2>&1 || :
  rm -f "$KVM_PATH"
  systemctl daemon-reload >/dev/null 2>&1 || :
  say "kvm: $KVM_UNIT removed. $TK_USER keeps its kvm group, which may predate this tool"
fi

printf 'engines-complete\n'
