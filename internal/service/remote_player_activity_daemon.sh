#!/bin/sh
# MineOps 远端玩家活动守护进程：增量跟随受管 Server 日志并写入有界、可恢复 Spool。
set -u

data_dir=${1:?data directory required}
server_dir=${2:?server directory required}
log_path=${3:-$server_dir/logs/latest.log}
process_identity_id=${4:-unknown}
managed_pid=${5:-}
managed_pgid=${6:-}
interval=${7:-5}
maximum_spool_bytes=${8:-8388608}

case "$interval" in *[!0-9]*|'') interval=5 ;; esac
case "$maximum_spool_bytes" in *[!0-9]*|'') maximum_spool_bytes=8388608 ;; esac
[ "$interval" -ge 1 ] || interval=1
[ "$interval" -le 3600 ] || interval=3600
[ "$maximum_spool_bytes" -ge 65536 ] || maximum_spool_bytes=65536

spool_path="$data_dir/player-activity.spool"
pid_path="$data_dir/player-activity.pid"
offset_path="$data_dir/player-activity.offset"
inode_path="$data_dir/player-activity.inode"
sequence_path="$data_dir/player-activity.sequence"
temporary_path="$data_dir/.player-activity-batch.$$"
lines_path="$data_dir/.player-activity-lines.$$"
input_path="$data_dir/.player-activity-input.$$"

cleanup() {
    rm -f "$temporary_path" "$lines_path" "$input_path"
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

read_number() {
    value=$(cat "$1" 2>/dev/null || true)
    case "$value" in *[!0-9]*|'') value=${2:-0} ;; esac
    printf '%s\n' "$value"
}

process_alive() {
    # Empty identities are allowed for a test/degraded collector; production passes both PID and PGID.
    if [ -z "$managed_pid" ] && [ -z "$managed_pgid" ]; then return 0; fi
    if [ -n "$managed_pid" ] && ! kill -0 "$managed_pid" 2>/dev/null; then return 1; fi
    if [ -n "$managed_pgid" ]; then
        observed_pgid=$(ps -o pgid= -p "$managed_pid" 2>/dev/null | tr -d ' ' || true)
        [ -z "$observed_pgid" ] || [ "$observed_pgid" = "$managed_pgid" ] || return 1
    fi
    return 0
}

strip_ansi() {
    # POSIX sed cannot express every CSI sequence; this removes the common color/reset forms used by Minecraft logs.
    escape=$(printf '\033')
    printf '%s' "$1" | sed "s/${escape}\\[[0-9;]*m//g; s/\r//g"
}

emit_activity() {
    clean=$(strip_ansi "$1")
    event_type=
    player_name=
    case "$clean" in
        *" joined the game"*)
            event_type=join
            player_name=$(printf '%s\n' "$clean" | sed -nE 's/.*[^A-Za-z0-9_-]([A-Za-z0-9_-]{1,32}) joined the game.*/\1/p')
            ;;
        *" left the game"*|*" lost connection"*)
            event_type=leave
            player_name=$(printf '%s\n' "$clean" | sed -nE 's/.*[^A-Za-z0-9_-]([A-Za-z0-9_-]{1,32}) (left the game|lost connection).*/\1/p')
            ;;
        *" logged in with entity id"*)
            event_type=join
            player_name=$(printf '%s\n' "$clean" | sed -nE 's/.*[^A-Za-z0-9_-]([A-Za-z0-9_-]{1,32}) logged in with entity id.*/\1/p')
            ;;
    esac
    [ -n "$event_type" ] && [ -n "$player_name" ] || return 0
    case "$player_name" in *[!A-Za-z0-9_-]*) return 0 ;; esac
    sequence=$((sequence + 1))
    epoch=$(date +%s)
    # Keep one physical line per record. Raw evidence is bounded and pipe/newline sanitized for grammar safety.
    raw=$(printf '%s' "$clean" | tr '\t|\n' '   ' | cut -c 1-8192)
    printf 'player_activity_v1|%s|%s|%s|%s|%s|%s\n' "$process_identity_id" "$sequence" "$event_type" "$epoch" "$player_name" "$raw" >> "$lines_path"
}

offset=$(read_number "$offset_path" 0)
sequence=$(read_number "$sequence_path" 0)
last_inode=$(cat "$inode_path" 2>/dev/null || true)

while process_alive; do
    if [ ! -f "$log_path" ]; then
        sleep "$interval"
        continue
    fi
    size=$(wc -c < "$log_path" 2>/dev/null | tr -d ' ') || size=0
    case "$size" in *[!0-9]*|'') size=0 ;; esac
	current_inode=$(ls -di "$log_path" 2>/dev/null | awk '{print $1}' || true)
	if [ -n "$last_inode" ] && [ -n "$current_inode" ] && [ "$current_inode" != "$last_inode" ]; then offset=0; fi
	last_inode="$current_inode"
	[ "$size" -ge "$offset" ] || offset=0
    : > "$lines_path"
    if [ "$size" -gt "$offset" ]; then
        tail -c "+$((offset + 1))" "$log_path" > "$input_path" 2>/dev/null || :
        while IFS= read -r line; do
            emit_activity "$line"
        done < "$input_path"
        rm -f "$input_path"
        offset=$size
    fi
    if [ -s "$lines_path" ]; then
        batch_at=$(date +%s)
        {
            printf 'player_batch_start=%s\n' "$batch_at"
            cat "$lines_path"
            printf 'player_batch_end=%s\n' "$batch_at"
        } > "$temporary_path"
        cat "$temporary_path" >> "$spool_path"
        rm -f "$temporary_path"
    fi
    printf '%s\n' "$offset" > "$offset_path.tmp.$$"
    mv -f "$offset_path.tmp.$$" "$offset_path"
    printf '%s\n' "$sequence" > "$sequence_path.tmp.$$"
	mv -f "$sequence_path.tmp.$$" "$sequence_path"
	printf '%s\n' "$last_inode" > "$inode_path.tmp.$$"
	mv -f "$inode_path.tmp.$$" "$inode_path"

    spool_bytes=$(wc -c < "$spool_path" 2>/dev/null | tr -d ' ') || spool_bytes=0
    case "$spool_bytes" in *[!0-9]*|'') spool_bytes=0 ;; esac
    if [ "$spool_bytes" -gt "$maximum_spool_bytes" ]; then
        trim_path="$data_dir/.player-activity-trim.$$"
        aligned_path="$data_dir/.player-activity-aligned.$$"
        dropped_bytes=$((spool_bytes - maximum_spool_bytes))
        dropped_events=$(head -c "$dropped_bytes" "$spool_path" 2>/dev/null | grep -c '^player_activity_v1|' || true)
        case "$dropped_events" in *[!0-9]*|'') dropped_events=0 ;; esac
        [ "$dropped_events" -gt 0 ] || dropped_events=1
        tail -c "$maximum_spool_bytes" "$spool_path" > "$trim_path"
        awk 'found || /^player_batch_start=/ || /^player_drop_marker=/ { found=1; print }' "$trim_path" > "$aligned_path"
        mv -f "$aligned_path" "$spool_path"
        rm -f "$trim_path"
        # The marker is itself a complete line and remains in the Claim on a failed retry.
        printf 'player_drop_marker=%s|%s\n' "$(date +%s)" "$dropped_events" >> "$spool_path"
    fi
    sleep "$interval"
done
