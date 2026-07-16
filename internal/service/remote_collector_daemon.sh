#!/bin/sh
# MineOps 远端监控守护进程：周期执行 collector.sh，并把完整批次写入有界 Spool。
set -u

data_dir=${1:?data directory required}
collector_path=${2:?collector path required}
server_dir=${3:?server directory required}
jar_name=${4:-}
managed_pid=${5:-}
managed_pgid=${6:-}
interval=${7:-15}
maximum_spool_bytes=${8:-8388608}

case "$interval" in *[!0-9]*|'') interval=15 ;; esac
case "$maximum_spool_bytes" in *[!0-9]*|'') maximum_spool_bytes=8388608 ;; esac
[ "$interval" -ge 5 ] || interval=5
[ "$interval" -le 3600 ] || interval=3600
[ "$maximum_spool_bytes" -ge 65536 ] || maximum_spool_bytes=65536

spool_path="$data_dir/metrics.spool"
pid_path="$data_dir/collector.pid"
temporary_path="$data_dir/.batch.$$"

cleanup() {
    rm -f "$temporary_path"
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
mkdir -p "$data_dir"
printf '%s\n' "$$" > "$pid_path"

while :; do
    started_at=$(date +%s)
    {
        printf 'batch_start=%s\n' "$started_at"
        sh "$collector_path" "$server_dir" "$jar_name" "$managed_pid" "$managed_pgid"
        printf 'batch_end=%s\n' "$started_at"
    } > "$temporary_path"
    cat "$temporary_path" >> "$spool_path"
    rm -f "$temporary_path"

    spool_bytes=$(wc -c < "$spool_path" 2>/dev/null | tr -d ' ') || spool_bytes=0
    case "$spool_bytes" in *[!0-9]*|'') spool_bytes=0 ;; esac
    if [ "$spool_bytes" -gt "$maximum_spool_bytes" ]; then
        trimmed_path="$data_dir/.trim.$$"
        aligned_path="$data_dir/.aligned.$$"
        tail -c "$maximum_spool_bytes" "$spool_path" > "$trimmed_path"
        awk 'found || /^batch_start=/ { found=1; print }' "$trimmed_path" > "$aligned_path"
        mv -f "$aligned_path" "$spool_path"
        rm -f "$trimmed_path"
    fi

    finished_at=$(date +%s)
    elapsed=$((finished_at - started_at))
    delay=$((interval - elapsed))
    [ "$delay" -gt 0 ] || delay=1
    sleep "$delay"
done
