package access

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mosona-manager/internal/db"
	"mosona-manager/internal/redis"
	"time"
)

var ErrTeamAccessRevoked = errors.New("team access revoked")

func ValidateTeamSession(ctx context.Context, userID, teamID int64, sessionID string, allowedRoles ...int) error {
	if userID <= 0 || teamID <= 0 || sessionID == "" {
		return ErrTeamAccessRevoked
	}

	exists, err := redis.SessionExists(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("check session: %w", err)
	}
	if !exists {
		return ErrTeamAccessRevoked
	}

	role, err := db.GetTeamRole(ctx, userID, teamID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTeamAccessRevoked
		}
		return fmt.Errorf("check team membership: %w", err)
	}
	for _, allowed := range allowedRoles {
		if role == allowed {
			return nil
		}
	}
	return ErrTeamAccessRevoked
}

func WatchTeamSession(
	ctx context.Context,
	interval time.Duration,
	userID, teamID int64,
	sessionID string,
	allowedRoles ...int,
) <-chan error {
	result := make(chan error, 1)
	go func() {
		defer close(result)
		if err := ValidateTeamSession(ctx, userID, teamID, sessionID, allowedRoles...); err != nil {
			result <- err
			return
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := ValidateTeamSession(ctx, userID, teamID, sessionID, allowedRoles...); err != nil {
					result <- err
					return
				}
			}
		}
	}()
	return result
}
