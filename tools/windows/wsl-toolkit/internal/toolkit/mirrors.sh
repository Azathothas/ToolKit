#!/bin/sh
# The base's registry mirrors and engine proxy, from base.registry. WSL-196.
#
# It defines two functions and runs nothing. The provisioning payload carries it
# ahead of provision.sh, so a new base pulls through them from its first pull,
# and every owner's `base ensure` carries it ahead of engines.sh, so a base that
# already works takes a changed key with no rebuild. Both call
# apply_registry_settings.
#
# TK_REGISTRY_MIRRORS_B64 and TK_REGISTRY_PROXY_B64 are each drop-in's text, as
# the executable rendered it, and empty where the key names none.
#
# ⛔ A BASE WITHOUT THE KEY IS BYTE FOR BYTE UNCHANGED. An empty text removes a
# drop-in only where one is there, and says nothing where none is.
# ⚠ Both drop-ins are in /etc, so root's engine reads them as the account's does.

# >>> the registry settings: begin
# registry_drop_in PATH B64 WHAT writes PATH from B64, or removes it where B64 is
# empty, and says which.
registry_drop_in() {
  if [ -z "$2" ]; then
    [ -e "$1" ] || return 0
    rm -f "$1" || return 1
    say "$3: none, so $1 is removed"
    return 0
  fi
  mkdir -p "${1%/*}" || return 1
  printf '%s' "$2" | base64 -d > "$1.new" || { rm -f "$1.new"; return 1; }
  chmod 0644 "$1.new" || { rm -f "$1.new"; return 1; }
  mv -f "$1.new" "$1" || return 1
  say "$3: $1"
}

# apply_registry_settings writes or removes both drop-ins.
apply_registry_settings() {
  registry_drop_in /etc/containers/registries.conf.d/99-wsl-toolkit-mirrors.conf \
    "${TK_REGISTRY_MIRRORS_B64:-}" "registry mirrors" || return 1
  registry_drop_in /etc/containers/containers.conf.d/99-wsl-toolkit-proxy.conf \
    "${TK_REGISTRY_PROXY_B64:-}" "engine proxy" || return 1
}
# <<< the registry settings: end
