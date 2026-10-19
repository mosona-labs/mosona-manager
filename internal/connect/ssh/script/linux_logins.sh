#!/bin/sh
set -eu
export LC_ALL=C LANG=C TZ=UTC0

report_stage() {
  if [ "${LOGIN_PROBE_PROGRESS:-}" = 1 ]; then
    printf 'login_probe_stage=%s\n' "$1"
  fi
}

WTMP_RECORDS="${1:-${LOGIN_WTMP_RECORDS:-}}"
SINCE_EPOCH="${2:-${LOGIN_SINCE_EPOCH:-}}"
LAST_LINES="${3:-${LOGIN_LAST_LINES:-50}}"
case "$LAST_LINES" in
  ''|*[!0-9]*) LAST_LINES=50 ;;
esac
if [ "$LAST_LINES" -le 0 ]; then
  LAST_LINES=50
fi
case "$SINCE_EPOCH" in
  ''|*[!0-9]*) SINCE_EPOCH='' ;;
esac

report_stage clock
server_epoch="$(date +%s 2>/dev/null)" || server_epoch=''
case "$server_epoch" in
  ''|*[!0-9]*) server_epoch='' ;;
esac

client_ip="${SSH_CONNECTION:-}"
printf 'self_ip=%s\n' "${client_ip%% *}"

report_stage tempdir
if [ -d /dev/shm ]; then
  tmpdir="$(mktemp -d -p /dev/shm 2>/dev/null)" || tmpdir="$(mktemp -d)"
else
  tmpdir="$(mktemp -d)"
fi
cleanup() {
  rm -rf "$tmpdir"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

raw="$tmpdir/raw"
wtmpbin="$tmpdir/wtmp.bin"

WTMP="${LOGIN_WTMP:-/var/log/wtmp}"
WTMP_TAIL="${LOGIN_WTMP_TAIL:-256}"
case "$WTMP_TAIL" in
  ''|*[!0-9]*) WTMP_TAIL=256 ;;
esac

probe_wtmp() {
  command -v utmpdump >/dev/null 2>&1 || return 1
  report_stage wtmp
  printf '%s\n' '[7] [00001] [ts/0] [probe   ] [pts/0       ] [192.0.2.1           ] [192.0.2.1      ] [2026-01-01T00:00:00,000000+00:00]' > "$tmpdir/layout.txt"
  utmpdump -r "$tmpdir/layout.txt" > "$tmpdir/layout.bin" 2>/dev/null || return 1
  WTMP_REC="$(stat -c %s "$tmpdir/layout.bin" 2>/dev/null)" || return 1
  case "$WTMP_REC" in
    384|400) ;;
    *) return 1 ;;
  esac
  size="$(stat -c %s "$WTMP" 2>/dev/null)" || return 1
  case "$size" in
    ''|*[!0-9]*) return 1 ;;
  esac

  [ $((size % WTMP_REC)) -eq 0 ] || return 1
  total=$((size / WTMP_REC))
  skip=0
  case "$WTMP_RECORDS" in
    ''|*[!0-9]*) ;;
    *) skip=$WTMP_RECORDS ;;
  esac
  if [ "$skip" -le 0 ] || [ "$skip" -gt "$total" ]; then

    if [ "$total" -gt "$WTMP_TAIL" ]; then
      skip=$((total - WTMP_TAIL))
    else
      skip=0
    fi
  fi
  : > "$wtmpbin"
  if [ "$skip" -lt "$total" ]; then
    dd if="$WTMP" of="$wtmpbin" bs=$WTMP_REC skip="$skip" count="$((total - skip))" 2>/dev/null || return 1
  fi
  report_stage parse
  utmpdump "$wtmpbin" > "$tmpdir/dump" 2>/dev/null || return 1
  sed -e 's/\] \[/;/g' -e 's/^\[//' -e 's/\]$//' "$tmpdir/dump" | awk -F';' '
    $1 == "7" && NF >= 8 {
      user = $4; line = $5; host = $6; ip = $7; stamp = $NF
      sub(/[[:space:]]+$/, "", user)
      sub(/[[:space:]]+$/, "", line)
      sub(/[[:space:]]+$/, "", host)
      sub(/[[:space:]]+$/, "", ip)
      # ut_host may be a DNS name. Prefer ut_addr_v6, as last -i does;
      # only use a numeric host when the address field is absent.
      if (ip == "" || ip == "0.0.0.0" || ip == "::" || ip == "0:0:0:0:0:0:0:0") ip = host
      if (user == "" || ip == "0.0.0.0" || ip == "::") next
      if (!((ip ~ /^[0-9.]+$/ && ip ~ /\./) || (ip ~ /^[0-9a-fA-F:.]+$/ && ip ~ /:/))) next
      # The Hub validates and canonicalizes the numeric address and time.
      printf "login|%s|%s|%s|%s\n", user, line, ip, stamp
    }'
  printf 'wtmp_records=%s\n' "$total"
  return 0
}

emit_logins() {
  awk '
    $1 == "reboot" || $1 == "shutdown" || $1 == "crash" ||
    $1 == "wtmp" || $1 == "btmp" { next }
    NF < 8 { next }
    {
      user = $1; tty = $2; ip = $3
      if (ip !~ /[.:]/) next
      # `last -F` prints a 5-field start time ending in the year; the short
      # fallback format ends at $7 and $8 is the session status instead.
      if ($8 ~ /^[0-9]+$/) start = $4 " " $5 " " $6 " " $7 " " $8
      else start = $4 " " $5 " " $6 " " $7
      printf "login|%s|%s|%s|%s\n", user, tty, ip, start
    }
  ' "$1"
}

probe_last() {
  : > "$raw"
  report_stage last
  result=0

  if command -v timeout >/dev/null 2>&1; then
    timeout -k 1 10 last "$@" > "$raw" 2>/dev/null || result=$?
  else
    last "$@" > "$raw" 2>/dev/null || result=$?
  fi
  case "$result" in
    124|137|143) exit "$result" ;;
    0) ;;
    *) return 1 ;;
  esac
  return 0
}

ran=0
if probe_wtmp; then
  ran=2
elif command -v last >/dev/null 2>&1; then
  if [ -n "$SINCE_EPOCH" ]; then
    if probe_last -F -n "$LAST_LINES" -i -s "@$SINCE_EPOCH"; then
      ran=1
    fi
  fi

  if [ "$ran" -eq 0 ] && [ ! -s "$raw" ]; then
    if probe_last -F -n "$LAST_LINES" -i; then
      ran=1
    fi
  fi

  if [ "$ran" -eq 0 ] && [ ! -s "$raw" ]; then
    if probe_last -n "$LAST_LINES" -i; then
      ran=1
    fi
  fi
  if [ "$ran" -eq 0 ] && [ ! -s "$raw" ]; then
    if probe_last -n "$LAST_LINES"; then
      ran=1
    fi
  fi
fi

if [ "$ran" -eq 0 ]; then
  printf 'logins_unavailable=1\n'
  exit 0
fi

if [ "$ran" -eq 1 ]; then
  report_stage parse
  emit_logins "$raw"
fi
printf 'server_epoch=%s\n' "$server_epoch"
report_stage cleanup
exit 0
