package active

import (
	"encoding/json"
	"mosona-manager/agent/runtime"
	"mosona-manager/agent/telemetry"
	"net/http"
	"os"
	"time"
)

func handleInfo(resp http.ResponseWriter, req *http.Request) {
	data := telemetry.CollectHostInfo(req.Context())
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	response := map[string]interface{}{
		"system":     data.SystemVersion,
		"start_time": time.Now().Unix() - data.Uptime,
		"host_name":  hostname,
		"cpu_name":   data.CpuName,
		"core_c":     data.CpuC,
		"core_t":     data.CpuT,
		"kernel":     data.KernelVersion,
		"arch":       data.Architecture,
		"version":    runtime.Version,
	}

	resp.Header().Set("Content-Type", "application/json")

	responseJson, _ := json.Marshal(response)
	_, _ = resp.Write(responseJson)
}
