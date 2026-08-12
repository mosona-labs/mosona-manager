package ateam

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"mosona-manager/internal/_type"
)

func TestUpdateTeamMembersPreservesRetainedMembership(t *testing.T) {
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()

	mock.ExpectBegin()
	tx, err := sqlx.NewDb(database, "sqlmock").Beginx()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT id FROM teams WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(`SELECT role FROM m_team_user WHERE team_id = \$1 AND user_id = \$2 FOR UPDATE`).
		WithArgs(int64(7), int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow(0))
	mock.ExpectQuery(`SELECT user_id FROM m_team_user WHERE team_id = \$1 ORDER BY user_id FOR UPDATE`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(10).AddRow(20))
	mock.ExpectExec(`(?s)INSERT INTO m_team_user.*ON CONFLICT`).
		WithArgs(int64(7), int64(10), 0).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO m_team_user.*ON CONFLICT`).
		WithArgs(int64(7), int64(30), 2).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM users_config WHERE uid = \$1 AND active_team = \$2`).
		WithArgs(int64(20), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM m_team_user WHERE team_id = \$1 AND user_id = \$2`).
		WithArgs(int64(7), int64(20)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	members := []_type.TeamUsersRole{
		{User: _type.User{ID: 10}, Role: 0},
		{User: _type.User{ID: 30}, Role: 2},
	}
	removed, err := updateTeamMembers(tx, 7, 10, members)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != 20 {
		t.Fatalf("removed users = %v, want [20]", removed)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
