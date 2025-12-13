package httpclient

import (
	"encoding/json"
	"fmt"
	"io"
	"mosona-manager/agent/runtime"
	"net/http"
	"net/url"
	"strings"
)

func PostForm(urlStr string, data map[string]interface{}, headers map[string]string, res any) error {
	formData := make(url.Values)
	for k, v := range data {
		formData.Set(k, fmt.Sprint(v))
	}

	client := &http.Client{}
	req, err := http.NewRequest("POST", urlStr, strings.NewReader(formData.Encode()))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	req.Header.Set("User-Agent", "mosona-manager-agent/"+runtime.Version)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if res != nil {
		return json.Unmarshal(body, res)
	}
	return nil
}
