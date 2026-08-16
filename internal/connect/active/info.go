package active

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"mosona-manager/internal/connect/callback"
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

func (a *auth) getInformation(ctx context.Context) error {
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
	if err = utils.PostFormContext(
		ctx,
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

	if err = callback.AgentInformation(
		ctx,
		a.serverID, "",
		info.System, time.Unix(info.StartTime, 0), info.HostName, info.CpuName,
		info.CoreC, info.CoreT, info.Kernel, a.host, info.Arch, info.Version,
	); err != nil {
		return err
	}

	return nil
}
