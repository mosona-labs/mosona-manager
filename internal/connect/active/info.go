package active

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"mosona-manager/internal/connect/callback"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"
	"mosona-manager/pkg/identity"
	"time"
)

type infoResponse struct {
	System    string `json:"system"`
	StartTime int64  `json:"start_time"`
	HostName  string `json:"host_name"`
	CpuName   string `json:"cpu_name"`
	CoreC     int    `json:"core_c"`
	CoreT     int    `json:"core_t"`
	Kernel    string `json:"kernel"`
	Arch      string `json:"arch"`
	Version   string `json:"version"`
}

func (a *auth) getInformation() error {
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return err
	}
	nonce := base64.StdEncoding.EncodeToString(nonceBytes)
	ts := time.Now().Unix()

	signature, err := identity.SignHeaders(*a.privKey, a.agentUID, ts, nonce)
	if err != nil {
		return err
	}

	var info infoResponse
	if err = utils.PostForm(
		fmt.Sprintf("http://%s:%d/api/info", a.host, a.port),
		map[string]interface{}{},
		map[string]string{
			"X-Agent-Id":        a.agentUID,
			"X-Agent-Timestamp": fmt.Sprintf("%d", ts),
			"X-Agent-Nonce":     nonce,
			"X-Agent-Signature": signature,
		},
		&info,
	); err != nil {
		return err
	}

	callback.Information(
		a.serverID, "",
		info.System, time.Unix(info.StartTime, 0), info.HostName, info.CpuName,
		info.CoreC, info.CoreT, info.Kernel, a.host, info.Arch,
	)

	// Update agent info
	if _, err := db.Db.Exec(
		"UPDATE agents SET status = 1, last_ip = $1, last_version = $2, last_seen_at = NOW() WHERE server_id = $3",
		a.host, info.Version, a.serverID,
	); err != nil {
		return err
	}

	return nil
}
