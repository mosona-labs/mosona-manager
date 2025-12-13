package redis

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"mosona-manager/internal/_type"
)

func ParseSessionData(id, data string) (*_type.SessionData, error) {
	var session map[interface{}]interface{}
	decoder := gob.NewDecoder(bytes.NewReader([]byte(data)))
	if err := decoder.Decode(&session); err != nil {
		return nil, fmt.Errorf("failed to decode session data: %w", err)
	}
	return &_type.SessionData{
		ID:        id,
		UID:       session["uid"].(int64),
		TID:       session["tid"].(int64),
		UserAgent: session["user_agent"].(string),
		Time:      session["time"].(int64),
	}, nil
}

func AddSessionID(ctx context.Context, uid int64, sessionID string) error {
	key := fmt.Sprintf("user:sessions:%d", uid)
	if err := Client.SAdd(ctx, key, sessionID).Err(); err != nil {
		return fmt.Errorf("failed to add session: %w", err)
	}
	return nil
}

func GetUserSessions(ctx context.Context, uid int64) ([]*_type.SessionData, error) {
	key := fmt.Sprintf("user:sessions:%d", uid)
	list, err := Client.SMembers(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get sessions: %w", err)
	}
	var notExistIDs []string
	var sessions = make([]*_type.SessionData, 0)
	for _, sessionID := range list {
		if data, err := Client.Get(ctx, fmt.Sprintf("mosona:session:%s", sessionID)).Result(); err != nil || data == "" {
			notExistIDs = append(notExistIDs, sessionID)
		} else {
			config, err := ParseSessionData(sessionID, data)
			if err != nil {
				notExistIDs = append(notExistIDs, sessionID)
				continue
			}
			sessions = append(sessions, config)
		}
	}

	// Clean up non-existing sessions
	if len(notExistIDs) > 0 {
		if err = RemoveSessionIDs(ctx, notExistIDs); err != nil {
			return nil, fmt.Errorf("failed to clean up sessions: %w", err)
		}
	}

	return sessions, nil
}

func CheckSessionOwnership(ctx context.Context, uid int64, sessionID string) (bool, error) {
	key := fmt.Sprintf("user:sessions:%d", uid)
	exists, err := Client.SIsMember(ctx, key, sessionID).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check session ownership: %w", err)
	}
	return exists, nil
}

func RemoveSessionIDs(ctx context.Context, sessionIDs []string) error {
	for _, sessionID := range sessionIDs {
		id := fmt.Sprintf("mosona:session:%s", sessionID)
		if err := Client.Del(ctx, id).Err(); err != nil {
			return fmt.Errorf("failed to delete session %s: %w", sessionID, err)
		}
	}

	return nil
}
