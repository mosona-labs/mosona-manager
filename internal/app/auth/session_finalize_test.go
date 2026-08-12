package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gorilla/sessions"
	contribsession "github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
)

func TestFinalizeAuthenticatedSessionCleansSavedSessionWhenIndexingFails(t *testing.T) {
	e := echo.New()
	store := &assignIDStore{id: "new-session"}
	e.Use(contribsession.Middleware(store))
	var removedSessions, removedRefs []string
	e.GET("/", func(c *echo.Context) error {
		_, err := finalizeAuthenticatedSessionWithDeps(c, 42, 60, authenticatedSessionDeps{
			getActiveTeam: func(int64) (int64, error) { return 0, nil },
			addSessionID: func(context.Context, int64, string) error {
				return errors.New("redis index unavailable")
			},
			removeSessionIDs: func(_ context.Context, ids []string) error {
				removedSessions = append(removedSessions, ids...)
				return nil
			},
			removeSessionRefs: func(_ context.Context, _ int64, ids []string) error {
				removedRefs = append(removedRefs, ids...)
				return nil
			},
		})
		return err
	})

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if !reflect.DeepEqual(removedSessions, []string{"new-session"}) ||
		!reflect.DeepEqual(removedRefs, []string{"new-session"}) {
		t.Fatalf("cleanup = sessions %v refs %v", removedSessions, removedRefs)
	}
}

type assignIDStore struct{ id string }

func (s *assignIDStore) Get(r *http.Request, name string) (*sessions.Session, error) {
	return sessions.GetRegistry(r).Get(s, name)
}

func (s *assignIDStore) New(_ *http.Request, name string) (*sessions.Session, error) {
	return sessions.NewSession(s, name), nil
}

func (s *assignIDStore) Save(_ *http.Request, _ http.ResponseWriter, sess *sessions.Session) error {
	sess.ID = s.id
	return nil
}
