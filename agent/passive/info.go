package passive

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"mosona-manager/agent/config"
	"mosona-manager/agent/httpclient"
	"mosona-manager/agent/identity"
	"mosona-manager/agent/telemetry"
	"os"
	"time"
)

func reportInfo() error {
	data := telemetry.CollectHostInfo(context.Background())
	hostname, _ := os.Hostname()

	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return err
	}
	nonce := base64.StdEncoding.EncodeToString(nonceBytes)
	ts := time.Now().Unix()

	signature, err := identity.SignHeaders(config.PrivateKey, config.Current.UUID, ts, nonce)
	if err != nil {
		return err
	}

	return httpclient.PostForm(config.Current.Hub+"/api/agent/info", map[string]interface{}{
		"system":     data.SystemVersion,
		"start_time": time.Now().Unix() - data.Uptime,
		"host_name":  hostname,
		"cpu_name":   data.CpuName,
		"core_c":     data.CpuC,
		"core_t":     data.CpuT,
		"kernel":     data.KernelVersion,
		"arch":       data.Architecture,
	}, map[string]string{
		"X-Agent-Id":        config.Current.UUID,
		"X-Agent-Timestamp": fmt.Sprintf("%d", ts),
		"X-Agent-Nonce":     nonce,
		"X-Agent-Signature": signature,
	}, nil)
}
