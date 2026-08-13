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
	case "generic", "generic+http", "generic+https":
		_, err := parseGenericTarget(target)
		return err
	}
	if strings.HasPrefix(scheme, "generic+") {
		return fmt.Errorf("%w: generic webhook must use HTTP or HTTPS", ErrInvalidConfiguration)
	}

	// Shoutrrr is the source of truth for supported services. Keeping a local
	// allowlist caused valid existing targets to stop working after upgrades.
	if _, err := shoutrrr.CreateSender(target); err != nil {
		return fmt.Errorf("%w: malformed or unsupported %s target", ErrInvalidConfiguration, scheme)
	}
	return nil
}
