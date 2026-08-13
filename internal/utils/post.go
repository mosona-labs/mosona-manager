package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mosona-manager/internal/runtime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func PostForm(urlStr string, data map[string]interface{}, headers map[string]string, res any) error {
	return PostFormContext(context.Background(), urlStr, data, headers, res)
}

func PostFormContext(ctx context.Context, urlStr string, data map[string]interface{}, headers map[string]string, res any) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	formData := make(url.Values)
	for k, v := range data {
		formData.Set(k, fmt.Sprint(v))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, strings.NewReader(formData.Encode()))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	req.Header.Set("User-Agent", "mosona-manager-hub/"+runtime.Version)

	resp, err := http.DefaultClient.Do(req)
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
