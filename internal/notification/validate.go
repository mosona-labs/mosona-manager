package notification

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"mosona-manager/internal/_type"

	"github.com/nicholas-fedor/shoutrrr"
	"github.com/nicholas-fedor/shoutrrr/pkg/services/chat/slack"
	"github.com/nicholas-fedor/shoutrrr/pkg/services/chat/teams"
)

const (
	maxTargetLength      = 8192
	maxNotificationCount = 100
)

var ErrInvalidConfiguration = errors.New("invalid notification configuration")

func NormalizeEntries(ctx context.Context, entries []_type.TeamNotification) ([]_type.TeamNotification, error) {
	if len(entries) > maxNotificationCount {
		return nil, fmt.Errorf("%w: too many notification targets", ErrInvalidConfiguration)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	normalized := make([]_type.TeamNotification, 0, len(entries))
	for _, entry := range entries {
		if strings.TrimSpace(entry.Module) == "" && strings.TrimSpace(entry.Target) == "" {
			continue
		}
		item, err := NormalizeEntry(ctx, entry)
		if err != nil {
			return nil, err
		}
		normalized = append(normalized, item)
	}
	return normalized, nil
}

func NormalizeEntry(ctx context.Context, entry _type.TeamNotification) (_type.TeamNotification, error) {
	entry.Module = strings.ToLower(strings.TrimSpace(entry.Module))
	entry.Target = strings.TrimSpace(entry.Target)
	if entry.Target == "" || len(entry.Target) > maxTargetLength {
		return _type.TeamNotification{}, fmt.Errorf("%w: target is empty or too long", ErrInvalidConfiguration)
	}

	switch entry.Module {
	case "email":
		address, err := mail.ParseAddress(entry.Target)
		if err != nil || address.Address != entry.Target {
			return _type.TeamNotification{}, fmt.Errorf("%w: invalid email address", ErrInvalidConfiguration)
		}
	case "shoutrrr":
		if err := ValidateTarget(ctx, entry.Target); err != nil {
			return _type.TeamNotification{}, err
		}
	default:
		return _type.TeamNotification{}, fmt.Errorf("%w: unsupported module", ErrInvalidConfiguration)
	}

	return entry, nil
}

func ValidateTarget(ctx context.Context, target string) error {
	target = strings.TrimSpace(target)
	if target == "" || len(target) > maxTargetLength {
		return fmt.Errorf("%w: target is empty or too long", ErrInvalidConfiguration)
	}

	parsed, err := url.ParseRequestURI(target)
	if err != nil || parsed.Scheme == "" || parsed.Opaque != "" {
		return fmt.Errorf("%w: malformed target URL", ErrInvalidConfiguration)
	}

	scheme := strings.ToLower(parsed.Scheme)
	switch scheme {
	case "generic", "generic+https":
		config, err := parseGenericTarget(target)
		if err != nil {
			return err
		}
		lookupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if err := validatePublicDestination(lookupCtx, config.webhookURL, true); err != nil {
			return fmt.Errorf("%w: generic webhook destination is not allowed", ErrInvalidConfiguration)
		}
		return nil
	case "telegram", "discord":
		if _, err := shoutrrr.CreateSender(target); err != nil {
			return fmt.Errorf("%w: malformed %s target", ErrInvalidConfiguration, scheme)
		}
		return nil
	case "slack":
		var service slack.Service
		if err := service.Initialize(parsed, nil); err != nil || service.Config == nil {
			return fmt.Errorf("%w: malformed slack target", ErrInvalidConfiguration)
		}
		if service.Config.Token.IsAPIToken() {
			return fmt.Errorf("%w: slack API tokens are not supported; use a webhook URL", ErrInvalidConfiguration)
		}
		return nil
	case "teams":
		var service teams.Service
		if err := service.Initialize(parsed, nil); err != nil || service.Config == nil {
			return fmt.Errorf("%w: malformed teams target", ErrInvalidConfiguration)
		}
		if err := teams.ValidateWebhookURL(service.Config.Host); err != nil {
			return fmt.Errorf("%w: teams webhook host is not allowed", ErrInvalidConfiguration)
		}
		return nil
	default:
		return fmt.Errorf("%w: unsupported notification scheme", ErrInvalidConfiguration)
	}
}
