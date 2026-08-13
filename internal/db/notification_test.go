package db

import (
	"context"
	"errors"
	"testing"

	"mosona-manager/internal/_type"
	"mosona-manager/internal/notification"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestUpdateNotificationsRejectsInvalidTargetBeforeTransaction(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	oldDB := Db
	Db = sqlx.NewDb(database, "sqlmock")
	t.Cleanup(func() { Db = oldDB })

	err = UpdateNotificationsByTeamId(context.Background(), 7, []_type.TeamNotification{{
		Module: "shoutrrr", Target: "unknown://example.com/hook",
	}})
	if !errors.Is(err, notification.ErrInvalidConfiguration) {
		t.Fatalf("error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
