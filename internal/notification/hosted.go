package notification

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/nicholas-fedor/shoutrrr/pkg/router"
	"github.com/nicholas-fedor/shoutrrr/pkg/services/chat/discord"
	"github.com/nicholas-fedor/shoutrrr/pkg/services/chat/teams"
	"github.com/nicholas-fedor/shoutrrr/pkg/types"
	"github.com/nicholas-fedor/shoutrrr/pkg/util/jsonclient"
)

var hostedHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		ExpectContinueTimeout: time.Second,
	},
	Timeout: 12 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func init() {
	// Telegram and Slack API delivery use Shoutrrr's shared JSON client.
	jsonclient.DefaultClient = jsonclient.NewWithHTTPClient(hostedHTTPClient)
}

type contextHTTPClient struct {
	ctx context.Context
}

func (c contextHTTPClient) Do(request *http.Request) (*http.Response, error) {
	response, err := hostedHTTPClient.Do(request.Clone(c.ctx))
	if err != nil || response == nil || response.Body == nil {
		return response, err
	}
	response.Body = struct {
		io.Reader
		io.Closer
	}{Reader: io.LimitReader(response.Body, maxGenericResponseBytes+1), Closer: response.Body}
	return response, nil
}

type noWaitSleeper struct{}

func (noWaitSleeper) Sleep(time.Duration) {}

func sendHosted(ctx context.Context, target, message string) error {
	serviceRouter := router.ServiceRouter{}
	service, err := serviceRouter.Locate(target)
	if err != nil {
		return ErrInvalidConfiguration
	}
	client := contextHTTPClient{ctx: ctx}
	switch item := service.(type) {
	case *discord.Service:
		item.HTTPClient = client
		item.Sleeper = noWaitSleeper{}
	case *teams.Service:
		item.SetHTTPClient(client)
	}
	if err := service.Send(message, &types.Params{}); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return context.DeadlineExceeded
		}
		return errors.New("hosted notification delivery failed")
	}
	return nil
}
