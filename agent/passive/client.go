package passive

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"mosona-manager/agent/config"
	"mosona-manager/agent/runtime"
	"mosona-manager/pkg/identity"
	"mosona-manager/pkg/ws"
	"strings"
	"time"
)

func connectHub(path string) (*ws.Client, error) {
	client := ws.NewClient()
	if err := setAuthHeaders(client); err != nil {
		return nil, err
	}
	client.SetHeader("User-Agent", "mosona-manager-agent/"+runtime.Version)

	client.SetReconnectConfig(-1, 10*time.Second)
	client.OnReconnect(func() {
		_ = setAuthHeaders(client)
	})

	url := strings.Replace(
		strings.Replace(
			config.Current.Hub,
			"https://",
			"wss://",
			1,
		),
		"http://",
		"ws://",
		1,
	)

	if err := client.Connect(context.Background(), url+path); err != nil {
		return nil, err
	}

	return client, nil
}

func setAuthHeaders(client *ws.Client) error {
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

	client.SetHeader("X-Agent-Id", config.Current.UUID)
	client.SetHeader("X-Agent-Timestamp", fmt.Sprintf("%d", ts))
	client.SetHeader("X-Agent-Nonce", nonce)
	client.SetHeader("X-Agent-Signature", signature)
	return nil
}
