package redis

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"mosona-manager/internal/_type"

	goredis "github.com/redis/go-redis/v9"
)

type userTeamSessionOps struct {
	list   func(context.Context, int64) ([]string, error)
	get    func(context.Context, string) (string, error)
	remove func(context.Context, int64, []string) error
}

func ParseSessionData(id, data string) (*_type.SessionData, error) {
	var session map[interface{}]interface{}
	decoder := gob.NewDecoder(bytes.NewReader([]byte(data)))
	if err := decoder.Decode(&session); err != nil {
		return nil, fmt.Errorf("failed to decode session data: %w", err)
	}
	uid, ok := session["uid"].(int64)
	if !ok {
		return nil, fmt.Errorf("session uid is missing")
	}
	tid, ok := session["tid"].(int64)
	if !ok {
		return nil, fmt.Errorf("session tid is missing")
	}
	userAgent, ok := session["user_agent"].(string)
	if !ok {
		return nil, fmt.Errorf("session user_agent is missing")
	}
	loginTime, ok := session["time"].(int64)
	if !ok {
		return nil, fmt.Errorf("session time is missing")
	}
	clientIP, _ := session["client_ip"].(string)
	return &_type.SessionData{
		ID:        id,
		UID:       uid,
		TID:       tid,
		UserAgent: userAgent,
		ClientIP:  clientIP,
		Time:      loginTime,
	}, nil
}

func AddSessionID(ctx context.Context, uid int64, sessionID string) error {
	key := fmt.Sprintf("user:sessions:%d", uid)
	if err := Client.SAdd(ctx, key, sessionID).Err(); err != nil {
		return fmt.Errorf("failed to add session: %w", err)
	}
	return nil
}

func SessionExists(ctx context.Context, sessionID string) (bool, error) {
	if sessionID == "" {
		return false, nil
	}
	count, err := Client.Exists(ctx, fmt.Sprintf("mosona:session:%s", sessionID)).Result()
	return count > 0, err
}

// RemoveUserTeamSessions revokes indexed sessions whose active team matches
// teamID. All indexed sessions are read and validated before any are deleted.
func RemoveUserTeamSessions(ctx context.Context, uid, teamID int64) error {
	if uid <= 0 || teamID <= 0 {
		return errors.New("invalid user or team ID")
	}
	return removeUserTeamSessions(ctx, uid, teamID, userTeamSessionOps{
		list:   listUserSessionIDs,
		get:    getSessionValue,
		remove: removeUserSessionIDsAtomic,
	})
}

func removeUserTeamSessions(ctx context.Context, uid, teamID int64, ops userTeamSessionOps) error {
	ids, err := ops.list(ctx, uid)
	if err != nil {
		return fmt.Errorf("list sessions for user %d: %w", uid, err)
	}

	matching := make([]string, 0, len(ids))
	for _, sessionID := range ids {
		data, getErr := ops.get(ctx, sessionID)
		if getErr != nil {
			if errors.Is(getErr, goredis.Nil) {
				matching = append(matching, sessionID)
				continue
			}
			return fmt.Errorf("read indexed session %s: %w", sessionID, getErr)
		}
		sessionData, parseErr := ParseSessionData(sessionID, data)
		if parseErr != nil {
			matching = append(matching, sessionID)
			continue
		}
		if sessionData.UID != uid {
			return fmt.Errorf("indexed session %s belongs to user %d, expected %d", sessionID, sessionData.UID, uid)
		}
		if sessionData.TID == teamID {
			matching = append(matching, sessionID)
		}
	}

	if len(matching) == 0 {
		return nil
	}
	if err = ops.remove(ctx, uid, matching); err != nil {
		return fmt.Errorf("revoke team sessions for user %d: %w", uid, err)
	}
	return nil
}

func listUserSessionIDs(ctx context.Context, uid int64) ([]string, error) {
	ids, err := Client.SMembers(ctx, fmt.Sprintf("user:sessions:%d", uid)).Result()
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func getSessionValue(ctx context.Context, sessionID string) (string, error) {
	return Client.Get(ctx, fmt.Sprintf("mosona:session:%s", sessionID)).Result()
}

func removeUserSessionIDsAtomic(ctx context.Context, uid int64, sessionIDs []string) error {
	if len(sessionIDs) == 0 {
		return nil
	}
	values := make([]interface{}, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		values = append(values, sessionID)
	}
	_, err := Client.TxPipelined(ctx, func(pipe goredis.Pipeliner) error {
		for _, sessionID := range sessionIDs {
			pipe.Del(ctx, fmt.Sprintf("mosona:session:%s", sessionID))
		}
		pipe.SRem(ctx, fmt.Sprintf("user:sessions:%d", uid), values...)
		return nil
	})
	if err != nil {
		return fmt.Errorf("delete sessions and index references: %w", err)
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
		if err = RemoveUserSessionIDs(ctx, uid, notExistIDs); err != nil {
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

func RemoveUserSessionIDs(ctx context.Context, uid int64, sessionIDs []string) error {
	if len(sessionIDs) == 0 {
		return nil
	}
	if err := RemoveSessionIDs(ctx, sessionIDs); err != nil {
		return err
	}
	return RemoveUserSessionIDRefs(ctx, uid, sessionIDs)
}

func RemoveUserSessionIDRefs(ctx context.Context, uid int64, sessionIDs []string) error {
	if len(sessionIDs) == 0 {
		return nil
	}
	key := fmt.Sprintf("user:sessions:%d", uid)
	values := make([]interface{}, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		values = append(values, sessionID)
	}
	if err := Client.SRem(ctx, key, values...).Err(); err != nil {
		return fmt.Errorf("failed to remove sessions from user set: %w", err)
	}
	return nil
}
