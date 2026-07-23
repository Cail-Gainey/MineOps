#!/bin/sh
# MineOps SSH Session 级远端监控守护进程。
# 周期热加载目标清单，由 collector.sh 为每个 Server 写入独立 Spool。
set -u

data_dir=${1:?data directory required}
collector_path=${2:?collector path required}
manifest_path=${3:?manifest path required}
interval=${4:-15}
maximum_spool_bytes=${5:-8388608}

case "$interval" in *[!0-9]*|'') interval=15 ;; esac
case "$maximum_spool_bytes" in *[!0-9]*|'') maximum_spool_bytes=8388608 ;; esac
[ "$interval" -ge 5 ] || interval=5
[ "$interval" -le 3600 ] || interval=3600
[ "$maximum_spool_bytes" -ge 65536 ] || maximum_spool_bytes=65536

pid_path="$data_dir/collector.pid"
error_path="$data_dir/collector.last-error"

cleanup() {
    if [ -f "$pid_path" ] && [ "$(cat "$pid_path" 2>/dev/null)" = "$$" ]; then
        rm -f "$pid_path"
    fi
}
stop_daemon() {
    cleanup
    trap - 0
    exit 0
}
trap cleanup 0
trap stop_daemon 1 2 15

umask 077
[ -d "$data_dir" ] && [ ! -L "$data_dir" ] || exit 1
chmod 700 "$data_dir" 2>/dev/null || true
printf '%s\n' "$$" > "$pid_path"

while :; do
    started_at=$(date +%s)
    if sh "$collector_path" "$manifest_path" "$data_dir" "$maximum_spool_bytes"; then
        rm -f "$error_path"
    else
        printf '%s\n' "$started_at" > "$error_path"
    fi

    finished_at=$(date +%s)
    elapsed=$((finished_at - started_at))
    delay=$((interval - elapsed))
    [ "$delay" -gt 0 ] || delay=5
    sleep "$delay"
done
