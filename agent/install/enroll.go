package install

import (
	"fmt"
	"mosona-manager/agent/httpclient"
	"mosona-manager/agent/runtime"
)

type EnrollResponse struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
	Data string `json:"data,omitempty"`
}

func EnrollPassive(hub, token, publicKey, ipPreference string) (string, error) {
	var data EnrollResponse
	if err := httpclient.PostForm(
		fmt.Sprintf(
			"%s/api/agent/enroll",
			hub,
		),
		map[string]interface{}{
			"token":      token,
			"public_key": publicKey,
			"version":    runtime.Version,
		},
		map[string]string{},
		&data,
		ipPreference,
	); err != nil {
		return "", err
	}

	if data.Code != "ok" {
		return "", fmt.Errorf("enroll failed: %s", data.Msg)
	}

	return data.Data, nil
}
