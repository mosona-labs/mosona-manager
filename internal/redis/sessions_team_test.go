package redis

import (
	"bytes"
	"context"
	"encoding/gob"
	"reflect"
	"testing"

	goredis "github.com/redis/go-redis/v9"
)

func TestRemoveUserTeamSessionsOnlyRevokesTargetTeam(t *testing.T) {
	values := map[string]string{
		"target": encodeSessionForTest(t, 42, 7),
		"other":  encodeSessionForTest(t, 42, 8),
	}
	var removed []string
	err := removeUserTeamSessions(context.Background(), 42, 7, userTeamSessionOps{
		list: func(context.Context, int64) ([]string, error) {
			return []string{"target", "other", "expired"}, nil
		},
		get: func(_ context.Context, id string) (string, error) {
			value, ok := values[id]
			if !ok {
				return "", goredis.Nil
			}
			return value, nil
		},
		remove: func(_ context.Context, uid int64, ids []string) error {
			if uid != 42 {
				t.Fatalf("remove uid = %d, want 42", uid)
			}
			removed = append(removed, ids...)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(removed, []string{"target", "expired"}) {
		t.Fatalf("removed sessions = %v, want [target expired]", removed)
	}
}

func encodeSessionForTest(t *testing.T, uid, tid int64) string {
	t.Helper()
	var data bytes.Buffer
	values := map[interface{}]interface{}{
		"uid": uid, "tid": tid, "user_agent": "test", "time": int64(1),
	}
	if err := gob.NewEncoder(&data).Encode(values); err != nil {
		t.Fatal(err)
	}
	return data.String()
}
