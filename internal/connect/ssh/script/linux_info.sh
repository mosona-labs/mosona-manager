system_version=$([ -f /etc/os-release ] && . /etc/os-release && distro_name="$NAME" || { [ -f /etc/redhat-release ] && distro_name="$(cat /etc/redhat-release)" || distro_name="Linux"; }; echo "$distro_name")
uptime=$(awk '{print int($1)}' /proc/uptime)
hostname=$(hostname)
cpu_name=$(cat /proc/cpuinfo | grep 'model name' | head -1 | awk -F': ' '{print $2}')
if [ -z "$cpu_name" ]; then
  cpu_name=$(cat /proc/device-tree/model 2>/dev/null || cat /sys/firmware/devicetree/base/model 2>/dev/null | tr -d '\0')
fi
if [ -z "$cpu_name" ]; then
  cpu_name=$(lscpu 2>/dev/null | sed -nr '/Model name/ s/.*:\s*(.*)/\1/p')
fi
cpu_cores=$(awk '/^processor/ {nproc++} /^physical id/ {phy=$NF} /^core id/ {pair=phy ":" $NF; if (!seen[pair]++) total++} END { for (p in seen) { split(p,a,":"); sockets[a[1]]=1 } sockets_count=0; for (s in sockets) sockets_count++; if (sockets_count==0) sockets_count=1; cores_per_socket = total ? int(total / sockets_count) : 0; print cores_per_socket, nproc }' /proc/cpuinfo)
kernel_version=$(uname -r)

if command -v curl >/dev/null 2>&1; then
  ip_address=$( (curl -4 -sS -A 'Mozilla' --connect-timeout 4 --max-time 10 --fail https://api.ip.sb/ip || curl -4 -sS -A 'Mozilla' --connect-timeout 4 --max-time 10 --fail https://cdid.c-ctrip.com/model-poc2/h || curl -6 -sS -A 'Mozilla' --connect-timeout 4 --max-time 10 --fail https://api.ip.sb/ip || curl -6 -sS -A 'Mozilla' --connect-timeout 4 --max-time 10 --fail https://cdid.c-ctrip.com/model-poc2/h) 2>/dev/null )
else
  ip_address="unknown"
fi
ip_address=$(echo "$ip_address" | tr -d '\r\n' | awk '{$1=$1;print}')
arch=$(uname -m)

system_version=$(echo "$system_version" | awk '{print $1}')
cpu_c_core=$(echo "$cpu_cores" | awk '{print $1}')
cpu_t_core=$(echo "$cpu_cores" | awk '{print $2}')

printf '{"system_version": "%s", "uptime": "%s", "hostname": "%s", "cpu_name": "%s", "cpu_c": %d, "cpu_t": %d, "kernel_version": "%s", "ip_address": "%s", "architecture": "%s"}\n' \
  "$system_version" "$uptime" "$hostname" "$cpu_name" "$cpu_c_core" "$cpu_t_core" "$kernel_version" "$ip_address" "$arch"
