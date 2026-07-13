package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mosona-manager/agent/runtime"
	"mosona-manager/pkg/netutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func PostForm(urlStr string, data map[string]interface{}, headers map[string]string, res any, ipPreference string) error {
	client, err := newClient(ipPreference)
	if err != nil {
		return err
	}
	return postForm(client, urlStr, data, headers, res)
}

func newClient(ipPreference string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if ipPreference != "" {
		network, err := netutil.NetworkForIPPreference(ipPreference)
		if err != nil {
			return nil, err
		}
		netDialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
		transport.DialContext = func(ctx context.Context, networkAddr, addr string) (net.Conn, error) {
			return netDialer.DialContext(ctx, network, addr)
		}
	}
	return &http.Client{Timeout: 30 * time.Second, Transport: transport}, nil
}

func postForm(client *http.Client, urlStr string, data map[string]interface{}, headers map[string]string, res any) error {
	formData := make(url.Values)
	for k, v := range data {
		formData.Set(k, fmt.Sprint(v))
	}

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
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if res != nil {
		return json.Unmarshal(body, res)
	}
	return nil
}
