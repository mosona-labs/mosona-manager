package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestLeaveTeamRejectsNonMember(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT owner_id FROM teams WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"owner_id"}).AddRow(99))
	mock.ExpectQuery(`SELECT user_id FROM m_team_user WHERE team_id = \$1 ORDER BY user_id FOR UPDATE`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(99))
	mock.ExpectRollback()

	if err := LeaveTeam(context.Background(), 42, 7); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("LeaveTeam() error = %v, want sql.ErrNoRows", err)
	}
}

func TestLeaveTeamTransfersOwnerAndRemovesMembershipAtomically(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT owner_id FROM teams WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"owner_id"}).AddRow(42))
	mock.ExpectQuery(`SELECT user_id FROM m_team_user WHERE team_id = \$1 ORDER BY user_id FOR UPDATE`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(42).AddRow(50))
	mock.ExpectExec(`UPDATE teams SET owner_id = \$1 WHERE id = \$2`).
		WithArgs(int64(50), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM users_config WHERE uid = \$1 AND active_team = \$2`).
		WithArgs(int64(42), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM m_team_user WHERE user_id = \$1 AND team_id = \$2`).
		WithArgs(int64(42), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := LeaveTeam(context.Background(), 42, 7); err != nil {
		t.Fatal(err)
	}
}
