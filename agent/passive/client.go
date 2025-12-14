package passive

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"mosona-manager/agent/config"
	"mosona-manager/agent/identity"
	"strings"
	"time"
)

func connectHub(path string) (*WSClient, error) {
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, err
	}
	nonce := base64.StdEncoding.EncodeToString(nonceBytes)

	ts := time.Now().Unix()

	client := NewWSClient()
	client.SetHeader("X-Agent-Id", config.Current.UUID)
	client.SetHeader("X-Agent-Timestamp", fmt.Sprintf("%d", ts))
	client.SetHeader("X-Agent-Nonce", nonce)
	signature, err := identity.SignHeaders(config.PrivateKey, config.Current.UUID, ts, nonce)
	if err != nil {
		return nil, err
	}

	client.SetHeader("X-Agent-Signature", signature)

	client.SetReconnectConfig(-1, 10*time.Second)

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
