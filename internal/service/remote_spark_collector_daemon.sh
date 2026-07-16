#!/bin/sh
# MineOps 远端 Spark 守护进程：通过受控 tmux Console 周期采集 TPS/MSPT 原始响应并写入有界 Spool。
set -u

data_dir=${1:?data directory required}
tmux_session=${2:?tmux session required}
spark_command=${3:?spark command required}
interval=${4:-15}
maximum_spool_bytes=${5:-4194304}

case "$interval" in *[!0-9]*|'') interval=15 ;; esac
case "$maximum_spool_bytes" in *[!0-9]*|'') maximum_spool_bytes=4194304 ;; esac
[ "$interval" -ge 5 ] || interval=5
[ "$interval" -le 3600 ] || interval=3600
[ "$maximum_spool_bytes" -ge 65536 ] || maximum_spool_bytes=65536

spool_path="$data_dir/spark.spool"
pid_path="$data_dir/spark.pid"
baseline_path="$data_dir/.spark-baseline.$$"
capture_path="$data_dir/.spark-capture.$$"
response_path="$data_dir/.spark-response.$$"
batch_path="$data_dir/.spark-batch.$$"

cleanup() {
    rm -f "$baseline_path" "$capture_path" "$response_path" "$batch_path"
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

while tmux has-session -t "$tmux_session:0.0" 2>/dev/null; do
    started_at=$(date +%s)
    tmux capture-pane -p -J -S -2000 -t "$tmux_session:0.0" > "$baseline_path" 2>/dev/null || break
    tmux send-keys -t "$tmux_session:0.0" -l -- "$spark_command" 2>/dev/null || break
    tmux send-keys -t "$tmux_session:0.0" Enter 2>/dev/null || break

    previous_signature=""
    stable_reads=0
    attempt=0
    : > "$response_path"
    while [ "$attempt" -lt 20 ]; do
        sleep 1
        tmux has-session -t "$tmux_session:0.0" 2>/dev/null || break 2
        tmux capture-pane -p -J -S -2000 -t "$tmux_session:0.0" > "$capture_path" 2>/dev/null || break 2
        if ! cmp -s "$baseline_path" "$capture_path"; then
            awk '
                index($0, "TPS from last 5s, 10s, 1m, 5m, 15m:") {
                    response = ""
                    found = 1
                }
                found { response = response $0 ORS }
                END { if (found) printf "%s", response }
            ' "$capture_path" > "$response_path"
        fi
        response_size=$(wc -c < "$response_path" 2>/dev/null | tr -d ' ') || response_size=0
        case "$response_size" in *[!0-9]*|'') response_size=0 ;; esac
        response_signature=$(cksum "$response_path" 2>/dev/null | awk '{print $1 ":" $2}') || response_signature=""
        if [ "$response_size" -gt 0 ] && [ "$response_signature" = "$previous_signature" ]; then
            stable_reads=$((stable_reads + 1))
        else
            stable_reads=0
        fi
        [ "$stable_reads" -ge 1 ] && break
        previous_signature=$response_signature
        attempt=$((attempt + 1))
    done

    if [ -s "$response_path" ]; then
        {
            printf 'spark_batch_start=%s\n' "$started_at"
            printf 'spark_raw_begin\n'
            cat "$response_path"
            printf '\nspark_raw_end\n'
            printf 'spark_batch_end=%s\n' "$started_at"
        } > "$batch_path"
        cat "$batch_path" >> "$spool_path"
    fi

    spool_bytes=$(wc -c < "$spool_path" 2>/dev/null | tr -d ' ') || spool_bytes=0
    case "$spool_bytes" in *[!0-9]*|'') spool_bytes=0 ;; esac
    if [ "$spool_bytes" -gt "$maximum_spool_bytes" ]; then
        trimmed_path="$data_dir/.spark-trim.$$"
        aligned_path="$data_dir/.spark-aligned.$$"
        tail -c "$maximum_spool_bytes" "$spool_path" > "$trimmed_path"
        awk 'found || /^spark_batch_start=/ { found=1; print }' "$trimmed_path" > "$aligned_path"
        mv -f "$aligned_path" "$spool_path"
        rm -f "$trimmed_path"
    fi

    finished_at=$(date +%s)
    elapsed=$((finished_at - started_at))
    delay=$((interval - elapsed))
    [ "$delay" -gt 0 ] || delay=1
    sleep "$delay"
done
