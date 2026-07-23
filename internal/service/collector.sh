#!/bin/sh
# MineOps SSH Session 级基础指标采集脚本。
# 用法: sh collector.sh <manifest_path> <data_dir> <maximum_spool_bytes>
# 每个周期只读取一次主机计数器和 Java 进程清单，再为每个 Server 写入独立有界 Spool。
set -u

manifest_path=${1:?manifest path required}
data_dir=${2:?data directory required}
maximum_spool_bytes=${3:-8388608}

case "$maximum_spool_bytes" in *[!0-9]*|'') maximum_spool_bytes=8388608 ;; esac
[ "$maximum_spool_bytes" -ge 65536 ] || maximum_spool_bytes=65536

spool_dir="$data_dir/spool"
lock_root="$data_dir/locks"
work_dir="$data_dir/.sample.$$"
targets_path="$work_dir/targets"
java_inventory="$work_dir/java-processes"
host_metrics="$work_dir/host.metrics"
separator=$(printf '\037')
current_lock=""

release_spool_lock() {
    if [ -n "$current_lock" ]; then
        rm -f "$current_lock/owner"
        rmdir "$current_lock" 2>/dev/null || true
        current_lock=""
    fi
}

cleanup() {
    release_spool_lock
    rm -rf "$work_dir"
}
trap cleanup 0 1 2 15

umask 077
[ -d "$data_dir" ] && [ ! -L "$data_dir" ] || exit 1
for directory in "$spool_dir" "$lock_root"; do
    [ ! -L "$directory" ] || exit 1
    [ ! -e "$directory" ] || [ -d "$directory" ] || exit 1
    [ -e "$directory" ] || mkdir "$directory"
done
[ ! -e "$work_dir" ] || exit 1
mkdir "$work_dir"
chmod 700 "$data_dir" "$spool_dir" "$lock_root" "$work_dir" 2>/dev/null || true
: > "$targets_path"

if [ ! -f "$manifest_path" ] || [ -L "$manifest_path" ]; then
    exit 0
fi

{
    IFS= read -r manifest_header || exit 0
    [ "$manifest_header" = "manifest_version=1" ] || exit 1
    while IFS="$separator" read -r server_id server_dir jar_name managed_pid managed_pgid extra; do
        [ -z "$extra" ] || continue
        [ "${#server_id}" -eq 36 ] || continue
        case "$server_id" in *[!0-9a-f-]*) continue ;; esac
        case "$server_dir" in /*) ;; *) continue ;; esac
        case "$managed_pid" in ''|*[!0-9]*) managed_pid="" ;; *) [ "$managed_pid" -gt 0 ] || managed_pid="" ;; esac
        case "$managed_pgid" in ''|*[!0-9]*) managed_pgid="" ;; *) [ "$managed_pgid" -gt 0 ] || managed_pgid="" ;; esac
        [ ! -L "$server_dir" ] && [ -d "$server_dir" ] || continue
        target_path="$work_dir/target.$server_id"
        [ ! -e "$target_path" ] || continue
        {
            printf '%s\n' "$server_dir"
            printf '%s\n' "$jar_name"
            printf '%s\n' "$managed_pid"
            printf '%s\n' "$managed_pgid"
        } > "$target_path"
        printf '%s\n' "$server_id" >> "$targets_path"
    done
} < "$manifest_path"

[ -s "$targets_path" ] || exit 0

# 读取 /proc/stat 首行，输出 "total idle"，idle 包含 iowait。
read_cpu() {
    awk 'NR==1 && $1=="cpu" {
        total=0
        for (i=2; i<=NF; i++) total+=$i
        print total, $5+$6
        ok=1
    }
    END { if (!ok) exit 1 }' /proc/stat 2>/dev/null
}

# 汇总除 lo/docker*/veth*/br-* 外全部网卡的收发字节。
read_net() {
    total_rx=0
    total_tx=0
    for device in /sys/class/net/*; do
        [ -e "$device" ] || continue
        name=${device##*/}
        case "$name" in
            lo|docker*|veth*|br-*) continue ;;
        esac
        rx=$(cat "$device/statistics/rx_bytes" 2>/dev/null) || rx=0
        tx=$(cat "$device/statistics/tx_bytes" 2>/dev/null) || tx=0
        total_rx=$((total_rx + rx))
        total_tx=$((total_tx + tx))
    done
    echo "$total_rx $total_tx"
}

# 输出路径所在块设备的 major、minor。
read_device_number() {
    device_number=$(stat -c '%d' "$1" 2>/dev/null) || return 1
    major=$(( (device_number >> 8) & 4095 ))
    minor=$(( (device_number & 255) | ((device_number >> 12) & 1048320) ))
    echo "$major $minor"
}

# 输出指定块设备 diskstats 的 "读扇区 写扇区"。
read_disk_device() {
    awk -v M="$1" -v N="$2" '
        $1==M && $2==N { print $6, $10; ok=1; exit }
        END { if (!ok) exit 1 }' /proc/diskstats 2>/dev/null
}

is_java_pid() {
    [ -n "$1" ] && [ -r "/proc/$1/comm" ] || return 1
    process_name=$(cat "/proc/$1/comm" 2>/dev/null) || return 1
    case "$process_name" in
        java|javaw) return 0 ;;
        *) return 1 ;;
    esac
}

# /proc/<pid>/stat 右括号后的相对字段 state=f1、ppid=f2、pgrp=f3。
read_proc_group() {
    awk '{
        if (!match($0, /^.*\)/)) exit 1
        n = split(substr($0, RLENGTH + 2), fields, " ")
        if (n < 3) exit 1
        print fields[3]
    }' "/proc/$1/stat" 2>/dev/null
}

# 输出 cpu_ticks、threads、start_ticks、rss_pages。
read_proc_stat() {
    awk '{
        if (!match($0, /^.*\)/)) exit 1
        n = split(substr($0, RLENGTH + 2), fields, " ")
        if (n < 22) exit 1
        print fields[12] + fields[13], fields[18], fields[20], fields[22]
    }' "/proc/$1/stat" 2>/dev/null
}

# 一次扫描全部 Java 进程，后续目标只查询清单，不再重复遍历 /proc。
build_java_inventory() {
    : > "$java_inventory"
    for process in /proc/[0-9]*; do
        process_pid=${process##*/}
        is_java_pid "$process_pid" || continue
        process_group=$(read_proc_group "$process_pid") || continue
        process_cwd=$(readlink "$process/cwd" 2>/dev/null) || process_cwd=""
        process_cmdline=$(tr '\000' ' ' < "$process/cmdline" 2>/dev/null | tr '\037\r\n' '   ') || process_cmdline=""
        process_cwd=$(printf '%s' "$process_cwd" | tr '\037\r\n' '   ')
        printf '%s%s%s%s%s%s%s\n' "$process_pid" "$separator" "$process_group" "$separator" "$process_cwd" "$separator" "$process_cmdline" >> "$java_inventory"
    done
}

find_target_java_pid() {
    target_server_dir=$1
    target_jar_name=$2
    target_managed_pid=$3
    target_managed_pgid=$4

    if [ -n "$target_managed_pid" ]; then
        while IFS="$separator" read -r candidate_pid candidate_group candidate_cwd candidate_cmdline extra; do
            [ "$candidate_pid" = "$target_managed_pid" ] || continue
            echo "$candidate_pid"
            return 0
        done < "$java_inventory"
    fi
    if [ -n "$target_managed_pgid" ]; then
        while IFS="$separator" read -r candidate_pid candidate_group candidate_cwd candidate_cmdline extra; do
            [ "$candidate_group" = "$target_managed_pgid" ] || continue
            echo "$candidate_pid"
            return 0
        done < "$java_inventory"
    fi
    while IFS="$separator" read -r candidate_pid candidate_group candidate_cwd candidate_cmdline extra; do
        [ "$candidate_cwd" = "$target_server_dir" ] || continue
        if [ -n "$target_jar_name" ]; then
            case "$candidate_cmdline" in *"$target_jar_name"*) ;; *) continue ;; esac
        fi
        echo "$candidate_pid"
        return 0
    done < "$java_inventory"
    return 1
}

acquire_spool_lock() {
    lock_path=$1
    attempt=0
    while ! mkdir "$lock_path" 2>/dev/null; do
        owner_pid=$(cat "$lock_path/owner" 2>/dev/null || true)
        case "$owner_pid" in
            ''|*[!0-9]*) ;;
            *)
                if ! kill -0 "$owner_pid" 2>/dev/null; then
                    rm -f "$lock_path/owner"
                    rmdir "$lock_path" 2>/dev/null || true
                    continue
                fi
                ;;
        esac
        attempt=$((attempt + 1))
        [ "$attempt" -lt 30 ] || return 1
        sleep 1
    done
    printf '%s\n' "$$" > "$lock_path/owner"
    current_lock=$lock_path
    return 0
}

append_server_batch() {
    append_server_id=$1
    append_batch_path=$2
    append_spool_path="$spool_dir/$append_server_id.spool"
    append_lock_path="$lock_root/$append_server_id.lock"
    acquire_spool_lock "$append_lock_path" || return 1

    cat "$append_batch_path" >> "$append_spool_path"
    spool_bytes=$(wc -c < "$append_spool_path" 2>/dev/null | tr -d ' ') || spool_bytes=0
    case "$spool_bytes" in *[!0-9]*|'') spool_bytes=0 ;; esac
    if [ "$spool_bytes" -gt "$maximum_spool_bytes" ]; then
        trimmed_path="$work_dir/trim.$append_server_id"
        aligned_path="$work_dir/aligned.$append_server_id"
        tail -c "$maximum_spool_bytes" "$append_spool_path" > "$trimmed_path"
        awk 'found || /^batch_start=/ { found=1; print }' "$trimmed_path" > "$aligned_path"
        mv -f "$aligned_path" "$append_spool_path"
        rm -f "$trimmed_path"
    fi
    release_spool_lock
    return 0
}

started_at=$(date +%s)
uptime1=$(awk '{print $1}' /proc/uptime 2>/dev/null) || uptime1=""
cpu1=$(read_cpu) || cpu1=""
net1=$(read_net)
build_java_inventory

# 为每个目标解析一次设备和 Java PID；同一块设备的第一次采样只读取一次。
while IFS= read -r server_id; do
    target_path="$work_dir/target.$server_id"
    {
        IFS= read -r server_dir
        IFS= read -r jar_name
        IFS= read -r managed_pid
        IFS= read -r managed_pgid
    } < "$target_path"

    device_pair=$(read_device_number "$server_dir") || device_pair=""
    if [ -n "$device_pair" ]; then
        device_major=$(printf '%s\n' "$device_pair" | awk '{print $1}')
        device_minor=$(printf '%s\n' "$device_pair" | awk '{print $2}')
        device_key="${device_major}_${device_minor}"
        printf '%s\n' "$device_key" > "$work_dir/target-device.$server_id"
        if [ ! -f "$work_dir/device.$device_key.meta" ]; then
            printf '%s %s\n' "$device_major" "$device_minor" > "$work_dir/device.$device_key.meta"
            read_disk_device "$device_major" "$device_minor" > "$work_dir/device.$device_key.first" || : > "$work_dir/device.$device_key.first"
        fi
    fi

    target_pid=$(find_target_java_pid "$server_dir" "$jar_name" "$managed_pid" "$managed_pgid") || target_pid=""
    printf '%s\n' "$target_pid" > "$work_dir/pid.$server_id"
    if [ -n "$target_pid" ]; then
        read_proc_stat "$target_pid" > "$work_dir/proc.$server_id.first" || : > "$work_dir/proc.$server_id.first"
    else
        : > "$work_dir/proc.$server_id.first"
    fi
done < "$targets_path"

sleep 1

uptime2=$(awk '{print $1}' /proc/uptime 2>/dev/null) || uptime2=""
elapsed=$(printf '%s %s\n' "$uptime1" "$uptime2" | awk '{
    window = $2 - $1
    if (window < 0.5 || window > 300) window = 1
    printf "%.2f", window
}')
[ -n "$elapsed" ] || elapsed=1
cpu2=$(read_cpu) || cpu2=""
net2=$(read_net)

for device_meta in "$work_dir"/device.*.meta; do
    [ -f "$device_meta" ] || continue
    device_name=${device_meta##*/device.}
    device_key=${device_name%.meta}
    read -r device_major device_minor < "$device_meta"
    read_disk_device "$device_major" "$device_minor" > "$work_dir/device.$device_key.second" || : > "$work_dir/device.$device_key.second"
done

{
    echo "collector_version=2"
    echo "ts=$started_at"
    if [ -n "$cpu1" ] && [ -n "$cpu2" ]; then
        printf '%s %s\n' "$cpu1" "$cpu2" | awk '{
            delta_total = $3 - $1
            delta_idle = $4 - $2
            if (delta_total > 0 && delta_idle >= 0 && delta_idle <= delta_total)
                printf "cpu_percent=%.2f\n", (delta_total - delta_idle) / delta_total * 100
            else
                print "err_cpu=cpu counters did not advance"
        }'
    else
        echo "err_cpu=read /proc/stat failed"
    fi

    if [ -r /proc/meminfo ]; then
        awk '
            /^MemTotal:/     { total = $2 * 1024 }
            /^MemAvailable:/ { available = $2 * 1024 }
            /^SwapTotal:/    { swap_total = $2 * 1024 }
            /^SwapFree:/     { swap_free = $2 * 1024 }
            END {
                if (total > 0 && available > 0 && available <= total) {
                    printf "mem_total=%d\n", total
                    printf "mem_used=%d\n", total - available
                    printf "mem_percent=%.2f\n", (total - available) / total * 100
                } else {
                    print "err_memory=invalid /proc/meminfo values"
                }
                if (swap_free > swap_total) swap_free = swap_total
                printf "swap_used=%d\n", swap_total - swap_free
            }' /proc/meminfo
    else
        echo "err_memory=read /proc/meminfo failed"
    fi

    if [ -r /proc/loadavg ]; then
        awk '{ printf "load1=%s\n", $1 }' /proc/loadavg
    else
        echo "err_load=read /proc/loadavg failed"
    fi

    printf '%s %s\n' "$net1" "$net2" | awk -v window="$elapsed" '{
        rx_delta = $3 - $1
        tx_delta = $4 - $2
        if (rx_delta < 0) rx_delta = 0
        if (tx_delta < 0) tx_delta = 0
        printf "net_rx_bps=%d\n", rx_delta / window
        printf "net_tx_bps=%d\n", tx_delta / window
    }'
} > "$host_metrics"

while IFS= read -r server_id; do
    target_path="$work_dir/target.$server_id"
    {
        IFS= read -r server_dir
        IFS= read -r jar_name
        IFS= read -r managed_pid
        IFS= read -r managed_pgid
    } < "$target_path"
    batch_path="$work_dir/batch.$server_id"
    {
        echo "batch_start=$started_at"
        cat "$host_metrics"

        df_line=$(df -Pk "$server_dir" 2>/dev/null | awk 'NR==2 {print $2, $4}')
        if [ -n "$df_line" ]; then
            printf '%s\n' "$df_line" | awk '{
                printf "disk_total=%d\n", $1 * 1024
                printf "disk_used=%d\n", ($1 - $2) * 1024
            }'
        else
            echo "err_disk=df failed for server directory"
        fi

        device_key=$(cat "$work_dir/target-device.$server_id" 2>/dev/null || true)
        disk1=""
        disk2=""
        if [ -n "$device_key" ]; then
            disk1=$(cat "$work_dir/device.$device_key.first" 2>/dev/null || true)
            disk2=$(cat "$work_dir/device.$device_key.second" 2>/dev/null || true)
        fi
        if [ -n "$disk1" ] && [ -n "$disk2" ]; then
            printf '%s %s\n' "$disk1" "$disk2" | awk -v window="$elapsed" '{
                read_delta = ($3 - $1) * 512
                write_delta = ($4 - $2) * 512
                if (read_delta < 0) read_delta = 0
                if (write_delta < 0) write_delta = 0
                printf "disk_read_bps=%d\n", read_delta / window
                printf "disk_write_bps=%d\n", write_delta / window
            }'
        else
            echo "err_disk_io=block device not found in /proc/diskstats"
        fi

        target_pid=$(cat "$work_dir/pid.$server_id" 2>/dev/null || true)
        proc1=$(cat "$work_dir/proc.$server_id.first" 2>/dev/null || true)
        proc2=""
        if [ -n "$target_pid" ]; then
            proc2=$(read_proc_stat "$target_pid") || proc2=""
        fi
        if [ -n "$proc1" ] && [ -n "$proc2" ]; then
            start1=$(printf '%s\n' "$proc1" | awk '{print $3}')
            start2=$(printf '%s\n' "$proc2" | awk '{print $3}')
            if [ "$start1" = "$start2" ]; then
                echo "proc_running=1"
                echo "proc_pid=$target_pid"
                clock_ticks=$(getconf CLK_TCK 2>/dev/null) || clock_ticks=100
                page_size=$(getconf PAGESIZE 2>/dev/null) || page_size=4096
                uptime_seconds=$(awk '{print $1}' /proc/uptime 2>/dev/null) || uptime_seconds=0
                printf '%s\n' "$proc2" | awk -v ticks="$clock_ticks" -v page="$page_size" -v uptime="$uptime_seconds" '{
                    rss_pages = $4
                    if (rss_pages < 0) rss_pages = 0
                    printf "proc_rss_bytes=%d\n", rss_pages * page
                    printf "proc_threads=%d\n", $2
                    process_uptime = uptime - $3 / ticks
                    if (process_uptime < 0) process_uptime = 0
                    printf "proc_uptime_seconds=%.2f\n", process_uptime
                }'
                printf '%s %s\n' "$proc1" "$proc2" | awk -v ticks="$clock_ticks" -v window="$elapsed" '{
                    cpu_delta = $5 - $1
                    if (cpu_delta >= 0)
                        printf "proc_cpu_percent=%.2f\n", (cpu_delta / ticks) / window * 100
                }'
                fd_count=$(ls "/proc/$target_pid/fd" 2>/dev/null | wc -l | tr -d ' ')
                if [ -n "$fd_count" ] && [ "$fd_count" -gt 0 ]; then
                    echo "proc_fds=$fd_count"
                fi
            else
                echo "proc_running=0"
            fi
        else
            echo "proc_running=0"
        fi
        echo "batch_end=$started_at"
    } > "$batch_path"
    append_server_batch "$server_id" "$batch_path" || true
done < "$targets_path"

exit 0
