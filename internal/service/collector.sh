#!/bin/sh
# MineOps SSH 拉取采集脚本:读取远程 Linux 的 /proc 与 /sys,输出 key=value 指标行。
# 用法:sh -s -- <server_dir> <jar_name> <managed_pid> <managed_pgid>
# (脚本经 stdin 管道传入远程执行,无状态、不落盘)
# 速率类指标(CPU%/磁盘/网络/进程 CPU%)通过脚本内两次采样(间隔 1 秒)差分计算。
# 任何单项失败输出 err_<项>=<原因> 并继续其余项;整体始终 exit 0。
set -u

server_dir=${1:-}
jar_name=${2:-}
managed_pid=${3:-}
managed_pgid=${4:-}

echo "collector_version=1"
echo "ts=$(date +%s)"

if [ -z "$server_dir" ] || [ ! -d "$server_dir" ]; then
    echo "err_server_dir=server directory missing or not a directory"
    server_dir=""
fi

# 读取 /proc/stat 首行,输出 "total idle"(idle 含 iowait,与旧采集口径一致)。
read_cpu() {
    awk 'NR==1 && $1=="cpu" {
        total=0
        for (i=2; i<=NF; i++) total+=$i
        print total, $5+$6
        ok=1
    }
    END { if (!ok) exit 1 }' /proc/stat 2>/dev/null
}

# 汇总除 lo/docker*/veth*/br-* 外全部网卡的收发字节,输出 "rx tx"。
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

# 定位 server_dir 所在块设备并输出 diskstats 的 "读扇区 写扇区"。
read_disk() {
    [ -n "$server_dir" ] || return 1
    device_number=$(stat -c '%d' "$server_dir" 2>/dev/null) || return 1
    major=$(( (device_number >> 8) & 4095 ))
    minor=$(( (device_number & 255) | ((device_number >> 12) & 1048320) ))
    awk -v M="$major" -v N="$minor" '
        $1==M && $2==N { print $6, $10; ok=1; exit }
        END { if (!ok) exit 1 }' /proc/diskstats 2>/dev/null
}

# 判断 PID 是否对应真实 Java 可执行进程,避免把命令行参数中含 java 的 tmux 包装 Shell 误判为 Java。
is_java_pid() {
    [ -n "$1" ] && [ -r "/proc/$1/comm" ] || return 1
    process_name=$(cat "/proc/$1/comm" 2>/dev/null) || return 1
    case "$process_name" in
        java|javaw) return 0 ;;
        *) return 1 ;;
    esac
}

# 读取 /proc/<pid>/stat 的进程组 ID。右括号后相对字段 state=f1、ppid=f2、pgrp=f3。
read_proc_group() {
    awk '{
        if (!match($0, /^.*\)/)) exit 1
        n = split(substr($0, RLENGTH + 2), fields, " ")
        if (n < 3) exit 1
        print fields[3]
    }' "/proc/$1/stat" 2>/dev/null
}

# 定位 Minecraft Java 进程:优先接受真实 Java PID；tmux 托管 PID 通常是包装 Shell，
# 此时按同一进程组从全局可读的 stat/comm 定位 Java 子进程；最后再按 cwd 与 Jar 名回退扫描。
find_java_pid() {
    if is_java_pid "$managed_pid"; then
        echo "$managed_pid"
        return 0
    fi
    if [ -n "$managed_pgid" ]; then
        for process in /proc/[0-9]*; do
            process_pid=${process##*/}
            is_java_pid "$process_pid" || continue
            process_group=$(read_proc_group "$process_pid") || continue
            [ "$process_group" = "$managed_pgid" ] || continue
            echo "$process_pid"
            return 0
        done
    fi
    [ -n "$server_dir" ] || return 1
    for process in /proc/[0-9]*; do
        process_pid=${process##*/}
        is_java_pid "$process_pid" || continue
        cwd=$(readlink "$process/cwd" 2>/dev/null) || continue
        [ "$cwd" = "$server_dir" ] || continue
        cmdline=$(tr '\0' ' ' < "$process/cmdline" 2>/dev/null) || continue
        if [ -n "$jar_name" ]; then
            case "$cmdline" in *"$jar_name"*) ;; *) continue ;; esac
        fi
        echo "$process_pid"
        return 0
    done
    return 1
}

# 输出 /proc/<pid>/stat 中 ")" 之后的相对字段:state=f1 utime=f12 stime=f13 threads=f18 start=f20 rss=f22。
read_proc_stat() {
    awk '{
        if (!match($0, /^.*\)/)) exit 1
        n = split(substr($0, RLENGTH + 2), fields, " ")
        if (n < 22) exit 1
        print fields[12] + fields[13], fields[18], fields[20], fields[22]
    }' "/proc/$1/stat" 2>/dev/null
}

# ---------- 第一次采样 ----------
uptime1=$(awk '{print $1}' /proc/uptime 2>/dev/null) || uptime1=""
cpu1=$(read_cpu) || cpu1=""
net1=$(read_net)
disk1=$(read_disk) || disk1=""
pid=$(find_java_pid) || pid=""
proc1=""
if [ -n "$pid" ]; then
    proc1=$(read_proc_stat "$pid") || proc1=""
fi

sleep 1

# ---------- 第二次采样与差分 ----------
# 速率统一除以 /proc/uptime 实测窗口(0.01s 精度),避免 sleep 1 之外的读取耗时导致速率高估。
uptime2=$(awk '{print $1}' /proc/uptime 2>/dev/null) || uptime2=""
elapsed=$(echo "$uptime1 $uptime2" | awk '{
    window = $2 - $1
    if (window < 0.5 || window > 30) window = 1
    printf "%.2f", window
}')
[ -n "$elapsed" ] || elapsed=1
cpu2=$(read_cpu) || cpu2=""
if [ -n "$cpu1" ] && [ -n "$cpu2" ]; then
    echo "$cpu1 $cpu2" | awk '{
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

if [ -n "$server_dir" ]; then
    df_line=$(df -Pk "$server_dir" 2>/dev/null | awk 'NR==2 {print $2, $4}')
    if [ -n "$df_line" ]; then
        echo "$df_line" | awk '{
            printf "disk_total=%d\n", $1 * 1024
            printf "disk_used=%d\n", ($1 - $2) * 1024
        }'
    else
        echo "err_disk=df failed for server directory"
    fi
fi

disk2=$(read_disk) || disk2=""
if [ -n "$disk1" ] && [ -n "$disk2" ]; then
    echo "$disk1 $disk2" | awk -v window="$elapsed" '{
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

net2=$(read_net)
echo "$net1 $net2" | awk -v window="$elapsed" '{
    rx_delta = $3 - $1
    tx_delta = $4 - $2
    if (rx_delta < 0) rx_delta = 0
    if (tx_delta < 0) tx_delta = 0
    printf "net_rx_bps=%d\n", rx_delta / window
    printf "net_tx_bps=%d\n", tx_delta / window
}'

if [ -n "$pid" ]; then
    proc2=""
    proc2=$(read_proc_stat "$pid") || proc2=""
    if [ -n "$proc1" ] && [ -n "$proc2" ]; then
        start1=$(echo "$proc1" | awk '{print $3}')
        start2=$(echo "$proc2" | awk '{print $3}')
        if [ "$start1" = "$start2" ]; then
            echo "proc_running=1"
            echo "proc_pid=$pid"
            clock_ticks=$(getconf CLK_TCK 2>/dev/null) || clock_ticks=100
            page_size=$(getconf PAGESIZE 2>/dev/null) || page_size=4096
            uptime_seconds=$(awk '{print $1}' /proc/uptime 2>/dev/null) || uptime_seconds=0
            echo "$proc2" | awk -v ticks="$clock_ticks" -v page="$page_size" -v uptime="$uptime_seconds" '{
                rss_pages = $4
                if (rss_pages < 0) rss_pages = 0
                printf "proc_rss_bytes=%d\n", rss_pages * page
                printf "proc_threads=%d\n", $2
                process_uptime = uptime - $3 / ticks
                if (process_uptime < 0) process_uptime = 0
                printf "proc_uptime_seconds=%.2f\n", process_uptime
            }'
            echo "$proc1 $proc2" | awk -v ticks="$clock_ticks" -v window="$elapsed" '{
                cpu_delta = $5 - $1
                if (cpu_delta >= 0)
                    printf "proc_cpu_percent=%.2f\n", (cpu_delta / ticks) / window * 100
            }'
            fd_count=$(ls "/proc/$pid/fd" 2>/dev/null | wc -l | tr -d ' ')
            if [ -n "$fd_count" ] && [ "$fd_count" -gt 0 ]; then
                echo "proc_fds=$fd_count"
            fi
        else
            echo "proc_running=0"
        fi
    else
        echo "proc_running=0"
    fi
else
    echo "proc_running=0"
fi

exit 0
