#!/bin/sh
# engines.sh: what the base's container engines need beyond what provision.sh
# builds. `base ensure` runs it on every base after provisioning, so a base an
# earlier release built is brought up to date with no rebuild. WSL-130, WSL-136.
#
# It runs as root, delivered on stdin. TK_USER names the account and TK_SYSTEMD
# says whether the base is configured to run systemd. TK_ROOT_ENGINE,
# TK_RUNTIME_DIR and TK_KVM are the configuration's opt-in settings, each true or
# false. TK_RUN_PREFIX names the engine's run directory, which the account's uid
# ends, and TK_RUN_STATE_B64 is the check `run` sends for a stale one. WSL-178.
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

# as_account runs a command as the account, so a file in its home is written by
# its owner rather than by root following whatever that directory holds.
as_account() {
  if command -v runuser >/dev/null 2>&1; then
    runuser -u "$TK_USER" -- "$@"
  elif command -v setpriv >/dev/null 2>&1; then
    setpriv --reuid="$uid" --regid="$(id -g "$TK_USER")" --init-groups -- "$@"
  else
    return 127
  fi
}

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
# The switch is a drop-in beside provision.sh's containers.conf, which keeps
# cgroupfs as the default the drop-in overrides.
DELEGATE=/etc/systemd/system/user@.service.d/50-wsl-toolkit-delegate.conf
MANAGER_REL=.config/containers/containers.conf.d/50-wsl-toolkit-cgroup.conf

# write_manager writes or removes the account's drop-in, as the account.
write_manager() {
  # $1 and $2 are expanded by the inner shell, which is the point.
  # shellcheck disable=SC2016
  as_account /bin/sh -c '
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
else
  if [ -f "$RUN_UNIT_PATH" ]; then
    systemctl disable "$RUN_UNIT" >/dev/null 2>&1 || :
    rm -f "$RUN_UNIT_PATH"
  fi
  # ⚠ WITHOUT systemd THE BOOT SCRIPT RUNS IT, and only provisioning writes the
  # line in wsl.conf that names that script, so a base an earlier release built
  # says which it has.
  if grep -q '^command=/usr/local/lib/wsl-toolkit/boot.sh$' /etc/wsl.conf 2>/dev/null; then
    say "run state: the boot script clears another boot's record at every start"
  else
    say "⚠ run state: /etc/wsl.conf names no boot script, so only a job clears another boot's record until base recreate writes one"
  fi
fi

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
