// SPDX-License-Identifier: 0BSD

package toolkit

// userEnvironmentPrelude prepares only process environment. It installs
// nothing and sources no account file. The directory order is a contract: a
// user-installed tool wins over a system copy, and the inherited PATH is kept
// after those entries with duplicates removed.
const userEnvironmentPrelude = `_wtk_uid=$(id -u) 2>/dev/null || { echo "wsl-toolkit: this guest has no id command" >&2; exit 2; }
_wtk_run=/tmp/wsl-toolkit-run-$_wtk_uid
(umask 077; mkdir "$_wtk_run") 2>/dev/null || :
if [ -L "$_wtk_run" ]; then echo "wsl-toolkit: $_wtk_run is a symlink and will not be used" >&2; exit 2; fi
if [ ! -d "$_wtk_run" ]; then echo "wsl-toolkit: could not create $_wtk_run" >&2; exit 2; fi
if command -v stat >/dev/null 2>&1; then
    _wtk_own=$(stat -c %u "$_wtk_run" 2>/dev/null) || _wtk_own=
    if [ -z "$_wtk_own" ]; then echo "wsl-toolkit: stat cannot read $_wtk_run" >&2; exit 2; fi
    if [ "$_wtk_own" != "$_wtk_uid" ]; then echo "wsl-toolkit: $_wtk_run belongs to uid $_wtk_own, not $_wtk_uid" >&2; exit 2; fi
fi
chmod 700 "$_wtk_run" || exit 2
XDG_RUNTIME_DIR=$_wtk_run; export XDG_RUNTIME_DIR
(umask 077; mkdir "$_wtk_run/tmp") 2>/dev/null || :
if [ -d "$_wtk_run/tmp" ] && [ ! -L "$_wtk_run/tmp" ]; then TMPDIR=$_wtk_run/tmp; export TMPDIR; fi
_wtk_path=
_wtk_add() {
    case ":$_wtk_path:" in *":$1:"*) return 0 ;; esac
    [ -d "$1" ] || return 0
    _wtk_path=${_wtk_path:+$_wtk_path:}$1
}
for _wtk_dir in "$HOME/.local/bin" "$HOME/bin" "$HOME/.cargo/bin" "$HOME/go/bin" "$HOME/.bun/bin" "$HOME/.deno/bin" "$HOME/.nix-profile/bin" /nix/var/nix/profiles/default/bin /usr/local/go/bin /usr/local/cargo/bin /usr/local/bin /usr/bin /bin /usr/local/sbin /usr/sbin /sbin; do _wtk_add "$_wtk_dir"; done
_wtk_old=$PATH
while [ -n "$_wtk_old" ]; do
    case "$_wtk_old" in *:*) _wtk_one=${_wtk_old%%:*}; _wtk_old=${_wtk_old#*:} ;; *) _wtk_one=$_wtk_old; _wtk_old= ;; esac
    [ -n "$_wtk_one" ] && _wtk_add "$_wtk_one"
done
PATH=$_wtk_path; export PATH
unset -f _wtk_add 2>/dev/null || :
unset _wtk_uid _wtk_run _wtk_own _wtk_path _wtk_dir _wtk_old _wtk_one
`
