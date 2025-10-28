#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C LANG=C

CPU_INTERVAL="${CPU_INTERVAL:-0.5}"
NET_INTERVAL="${NET_INTERVAL:-5}"
IFACE_EXCLUDE_REGEX="${IFACE_EXCLUDE_REGEX:-^(lo|docker[0-9]*|veth.*|br-.*|virbr.*|vmnet.*|tun.*|tap.*|tailscale.*|wg.*)$}"

get_cpu_usage() {
  local u1 n1 s1 id1 iw1 ir1 si1 st1
  local u2 n2 s2 id2 iw2 ir2 si2 st2
  local total1 total2 idle_all1 idle_all2 idle_delta total_delta

  read -r _ u1 n1 s1 id1 iw1 ir1 si1 st1 _ < /proc/stat
  total1=$((u1 + n1 + s1 + id1 + iw1 + ir1 + si1 + st1))
  idle_all1=$((id1 + iw1))

  sleep "$CPU_INTERVAL"

  read -r _ u2 n2 s2 id2 iw2 ir2 si2 st2 _ < /proc/stat
  total2=$((u2 + n2 + s2 + id2 + iw2 + ir2 + si2 + st2))
  idle_all2=$((id2 + iw2))

  idle_delta=$((idle_all2 - idle_all1))
  total_delta=$((total2 - total1))

  if (( total_delta <= 0 )); then
    printf "0.00\n"
    return
  fi

  awk -v id="$idle_delta" -v td="$total_delta" 'BEGIN { printf "%.2f\n", (1 - id/td) * 100 }'
}

read_mem_mb() {
  awk '
    /^MemTotal:/     {t=$2}
    /^MemAvailable:/ {a=$2}
    END {
      if (t=="") t=0;
      if (a=="") a=0;
      used = t - a;
      printf "%.2f %.2f\n", t/1024.0, used/1024.0
    }' /proc/meminfo
}

read_net_bytes() {
  awk -v re="$IFACE_EXCLUDE_REGEX" -F'[: ]+' '
    /:/ {
      if ($1 ~ re) next;
      rx += $3;  tx += $11
    }
    END { printf "%d %d\n", rx+0, tx+0 }
  ' /proc/net/dev
}

read_disk_gb() {
  df -B1 / 2>/dev/null | awk 'NR==2 {printf "%.2f %.2f\n", $2/1024/1024/1024, $3/1024/1024/1024}'
}

trap 'exit 0' INT TERM

while true; do
  cpu_usage="$(get_cpu_usage)"

  read -r mem_total_mb mem_used_mb < <(read_mem_mb)
  read -r disk_total_gb disk_used_gb < <(read_disk_gb)

  read -r rx1 tx1 < <(read_net_bytes)
  sleep "$NET_INTERVAL"
  read -r rx2 tx2 < <(read_net_bytes)

  rx_kib_s=$(awk -v a="$rx1" -v b="$rx2" -v s="$NET_INTERVAL" 'BEGIN {printf "%.2f", (b-a)/1024.0/s}')
  tx_kib_s=$(awk -v a="$tx1" -v b="$tx2" -v s="$NET_INTERVAL" 'BEGIN {printf "%.2f", (b-a)/1024.0/s}')

  rx_total_mb=$(awk -v v="$rx2" 'BEGIN {printf "%.2f", v/1024/1024}')
  tx_total_mb=$(awk -v v="$tx2" 'BEGIN {printf "%.2f", v/1024/1024}')

  printf '{"cpu": %.2f, "mem_total_mb": %.2f, "mem_used_mb": %.2f, "disk_total_gb": %.2f, "disk_used_gb": %.2f, "rx_kib_s": %.2f, "tx_kib_s": %.2f, "rx_total_mb": %.2f, "tx_total_mb": %.2f}\n' \
    "$cpu_usage" "$mem_total_mb" "$mem_used_mb" "$disk_total_gb" "$disk_used_gb" "$rx_kib_s" "$tx_kib_s" "$rx_total_mb" "$tx_total_mb"
done
