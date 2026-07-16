#!/usr/bin/env sh
set -eu

if [ -z "${APPIMAGE:-}" ]; then
  echo "APPIMAGE is unset; this process is not running from an AppImage." >&2
  exit 2
fi
target=$(readlink -f "$APPIMAGE")
runtime=$(readlink -f /proc/self/exe)
parent=$(dirname "$target")
printf 'APPIMAGE=%s\nruntime=%s\ntarget_parent_writable=%s\n' "$target" "$runtime" "$(test -w "$parent" && echo true || echo false)"
test "$target" != "$runtime"
test -f "$target"
test -w "$parent"
