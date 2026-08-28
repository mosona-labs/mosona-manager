#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C LANG=C

CPU_INTERVAL="${CPU_INTERVAL:-1}"
NET_INTERVAL="${NET_INTERVAL:-3}"
DISK_IO_INTERVAL="${DISK_IO_INTERVAL:-1.0}"
SECTOR_SIZE="${SECTOR_SIZE:-512}"

if [ -d /dev/shm ] && mkdir -p /dev/shm 2>/dev/null; then
  tmpdir="$(mktemp -d -p /dev/shm)"
else
  tmpdir="$(mktemp -d)"
fi
trap 'status=$?; rm -rf "$tmpdir"; exit "$status"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# CPU task
cpu_task() {
  local out="$1"
  local u1 n1 s1 id1 iw1 ir1 si1 st1
  local u2 n2 s2 id2 iw2 ir2 si2 st2
  read -r _ u1 n1 s1 id1 iw1 ir1 si1 st1 _ < /proc/stat
  total1=$((u1 + n1 + s1 + id1 + iw1 + ir1 + si1 + st1))
  idle1=$((id1 + iw1))
  sleep "$CPU_INTERVAL"
  read -r _ u2 n2 s2 id2 iw2 ir2 si2 st2 _ < /proc/stat
  total2=$((u2 + n2 + s2 + id2 + iw2 + ir2 + si2 + st2))
  idle2=$((id2 + iw2))
  td=$((total2 - total1))
  idd=$((idle2 - idle1))
  if (( td <= 0 )); then
    printf "cpu=0.00\n" > "$out"
  else
    awk -v id="$idd" -v td="$td" 'BEGIN{printf "cpu=%.2f\n", (1 - id/td)*100}'
  fi > "$out"
}

# MEM task
mem_task() {
  local out="$1"
  awk '
    /^MemTotal:/     {t=$2}
    /^MemAvailable:/ {a=$2}
    /^SwapTotal:/    {st=$2}
    /^SwapFree:/     {sf=$2}
    END {
      if (t=="") t=0;
      if (a=="") a=0;
      if (st=="") st=0;
      if (sf=="") sf=0;
      used = t - a;
      swap_used = st - sf;
      printf "mem_total_mb=%.2f\nmem_used_mb=%.2f\nswap_total_mb=%.2f\nswap_used_mb=%.2f\n", t/1024.0, used/1024.0, st/1024.0, swap_used/1024.0
    }' /proc/meminfo > "$out"
}

# Disk space task (multi-disk)
disk_space_task() {
  local out="$1"
  local min_size_bytes=$((5 * 1024 * 1024 * 1024))
  local first=1

  printf 'disks=[' > "$out"

  { df -B1 -x tmpfs -x devtmpfs -x squashfs -x overlay -x iso9660 -x efivarfs -x autofs 2>/dev/null || true; } \
    | awk 'NR>1 && $6 !~ /^\/(boot|snap\/|sys|proc|dev|run)/ { print $2, $3, $6 }' \
    | while read -r total used mp; do
        if [ "$mp" != "/" ] && [ "$total" -lt "$min_size_bytes" ]; then
          continue
        fi
        total_gb=$(awk -v t="$total" 'BEGIN{printf "%.2f", t/1024/1024/1024}')
        used_gb=$(awk -v u="$used" 'BEGIN{printf "%.2f", u/1024/1024/1024}')
        if [ "$first" -eq 1 ]; then
          first=0
        else
          printf ',' >> "$out"
        fi
        printf '{"mp":"%s","total_gb":%s,"used_gb":%s}' "$mp" "$total_gb" "$used_gb" >> "$out"
      done

  printf ']\n' >> "$out"
}

# NET task
net_task() {
  local out="${1:-}"
  if [ -z "${out}" ]; then
    printf '%s\n' "net_task: missing output file argument" >&2
    return 1
  fi

  local r1 t1 r2 t2

  read -r r1 t1 < <(awk 'NR>2 && $1 !~ /^(lo|veth|docker|br-):/ {rx += $2; tx += $10} END {printf "%.0f %.0f\n", rx, tx}' /proc/net/dev)

  sleep "$NET_INTERVAL"

  read -r r2 t2 < <(awk 'NR>2 && $1 !~ /^(lo|veth|docker|br-):/ {rx += $2; tx += $10} END {printf "%.0f %.0f\n", rx, tx}' /proc/net/dev)

  local rx_delta tx_delta
  rx_delta=$((r2 - r1))
  tx_delta=$((t2 - t1))

  if (( rx_delta < 0 )); then
    rx_delta=$((r2 + 4294967296 - r1))
  fi
  if (( tx_delta < 0 )); then
    tx_delta=$((t2 + 4294967296 - t1))
  fi
  if (( rx_delta < 0 )); then rx_delta=0; fi
  if (( tx_delta < 0 )); then tx_delta=0; fi

  awk -v rxd="$rx_delta" -v txd="$tx_delta" -v r2="$r2" -v t2="$t2" -v s="$NET_INTERVAL" 'BEGIN{
    printf "rx_kib_s=%.2f\n", rxd/1024.0/s;
    printf "tx_kib_s=%.2f\n", txd/1024.0/s;
    printf "rx_total_mb=%.2f\n", r2/1024/1024;
    printf "tx_total_mb=%.2f\n", t2/1024/1024;
  }' > "$out"
}

# Disk IO task
disk_io_task() {
  local out="$1"
  local sample="$DISK_IO_INTERVAL"
  read -r rsec1 wsec1 rops1 wops1 < <(
    awk '{
      dev=$3
      if (dev ~ /^(ram|loop|fd)/) next
      rsec += $6
      wsec += $10
      rops += $4
      wops += $8
    }
    END {printf "%.0f %.0f %.0f %.0f\n", rsec+0, wsec+0, rops+0, wops+0}' /proc/diskstats)
  sleep "$sample"
  read -r rsec2 wsec2 rops2 wops2 < <(
    awk '{
      dev=$3
      if (dev ~ /^(ram|loop|fd)/) next
      rsec += $6
      wsec += $10
      rops += $4
      wops += $8
    }
    END {printf "%.0f %.0f %.0f %.0f\n", rsec+0, wsec+0, rops+0, wops+0}' /proc/diskstats)
  rsec_delta=$((rsec2 - rsec1))
  wsec_delta=$((wsec2 - wsec1))
  rops_delta=$((rops2 - rops1))
  wops_delta=$((wops2 - wops1))
  rbytes=$((rsec_delta * SECTOR_SIZE))
  wbytes=$((wsec_delta * SECTOR_SIZE))
  awk -v rb="$rbytes" -v wb="$wbytes" -v s="$sample" -v ro="$rops_delta" -v wo="$wops_delta" \
    'BEGIN {
      if (s<=0) s=1;
      printf "disk_read_kib_s=%.2f\n", rb/s/1024.0;
      printf "disk_write_kib_s=%.2f\n", wb/s/1024.0;
      printf "disk_read_iops=%.2f\n", ro/s;
      printf "disk_write_iops=%.2f\n", wo/s;
    }' > "$out"
}

# TCP/UDP connections task
tcp_udp_task() {
  local out="$1"

  local tcp_total
  tcp_total=$(awk 'NR>1' /proc/net/tcp /proc/net/tcp6 2>/dev/null | wc -l)

  local udp_total
  udp_total=$(awk 'NR>1' /proc/net/udp /proc/net/udp6 2>/dev/null | wc -l)

  {
    printf "tcp_total=%d\n" "$tcp_total"
    printf "udp_total=%d\n" "$udp_total"
  } > "$out"
}

while true; do
  cpu_f="$tmpdir/cpu"
  mem_f="$tmpdir/mem"
  disk_space_f="$tmpdir/dspace"
  net_f="$tmpdir/net"
  diskio_f="$tmpdir/dio"
  tcp_udp_f="$tmpdir/tcpudp"

  : > "$cpu_f" "$mem_f" "$disk_space_f" "$net_f" "$diskio_f" "$tcp_udp_f"

  cpu_task "$cpu_f" & pid_cpu=$!
  mem_task "$mem_f" & pid_mem=$!
  disk_space_task "$disk_space_f" & pid_dspace=$!
  net_task "$net_f" & pid_net=$!
  disk_io_task "$diskio_f" & pid_dio=$!
  tcp_udp_task "$tcp_udp_f" & pid_tcpudp=$!

  wait "$pid_cpu" "$pid_mem" "$pid_dspace" "$pid_net" "$pid_dio" "$pid_tcpudp"

  for result_f in "$cpu_f" "$mem_f" "$disk_space_f" "$net_f" "$diskio_f" "$tcp_udp_f"; do
    cat "$result_f"
    printf '\n'
  done | awk '
    BEGIN { printf "{"; first=1 }
    /^[[:space:]]*$/ { next }
    {
      gsub(/^[[:space:]]+|[[:space:]]+$/,"")
      idx = index($0, "=")
      if (idx == 0) next
      k = substr($0, 1, idx-1)
      v = substr($0, idx+1)
      if (!first) printf ", "; first=0
      if (substr(v, 1, 1) == "[") {
        printf "\"%s\": %s", k, v
      } else {
        printf "\"%s\": %s", k, v
      }
    }
    END { print "}" }'

  sleep 0.1
done
