package db

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestUpsertServerAlertScopesInsertAndConflictUpdateToTeam(t *testing.T) {
	for _, test := range []struct {
		name     string
		affected int64
		wantErr  error
	}{
		{name: "owned server insert or update", affected: 1},
		{name: "cross team insert or update", affected: 0, wantErr: ErrAlertServerNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			mock := setAlertMockDB(t)
			mock.ExpectExec(`(?s)INSERT INTO server_alerts.*SELECT s.id, \$2, \$3, \$4.*FROM servers AS s.*WHERE s.id = \$1 AND s.team_id = \$5.*ON CONFLICT \(server_id, item\) DO UPDATE`).
				WithArgs(int64(91), "cpu_usage", 80, 10, int64(7)).
				WillReturnResult(sqlmock.NewResult(0, test.affected))

			err := UpsertServerAlert(7, 91, "cpu_usage", 80, 10)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("UpsertServerAlert() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func setAlertMockDB(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	oldDB := Db
	Db = sqlx.NewDb(database, "sqlmock")
	t.Cleanup(func() {
		Db = oldDB
		_ = database.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet SQL expectations: %v", err)
		}
	})
	return mock
}
