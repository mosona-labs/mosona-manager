package db

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"mosona-manager/internal/_type"
)

func TestEditServerRejectsStaleSSHHostKeyState(t *testing.T) {
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
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

	previousHostKey := "ssh-ed25519 AAAAOLD"
	data := _type.ServerFullType{
		Category:        5,
		HostKey:         "ssh-ed25519 AAAANEW",
		PreviousHostKey: &previousHostKey,
	}

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM categories WHERE team = $1 AND id = $2 FOR KEY SHARE").
		WithArgs(int64(7), int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(5))
	mock.ExpectExec("UPDATE servers SET name = $1, allow_monitor = $2, allow_terminal = $3, weight = $4, category = $5 WHERE id = $6 AND team_id = $7").
		WithArgs("", false, false, 0, int64(5), int64(91), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE server_info SET note = $1, provider = $2, cycle = $3, start_time = $4, end_time = $5, amount = $6, auto_renew = $7, bandwidth = $8, traffic = $9, traffic_type = $10, note_public = $11 WHERE sid = $12").
		WithArgs(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, int64(91)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE ssh SET address = $1, port = $2, username = $3, host_key = $4 WHERE server_id = $5 AND host_key = $6").
		WithArgs("", 0, "", data.HostKey, int64(91), previousHostKey).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	err = EditServer(7, 91, 0, &data)
	if !errors.Is(err, ErrSSHHostKeyStateChanged) {
		t.Fatalf("EditServer() error = %v", err)
	}
}
