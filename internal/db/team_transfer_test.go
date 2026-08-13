package db

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestTransferTeamOwnershipPromotesNewOwnerAtomically(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT 1 FROM teams WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(1))
	mock.ExpectExec(`(?s)INSERT INTO m_team_user.*ON CONFLICT.*DO UPDATE SET role = 0.*WHERE m_team_user.role <> 0`).
		WithArgs(int64(7), int64(50)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE teams SET owner_id = \$1 WHERE id = \$2`).
		WithArgs(int64(50), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := TransferTeamOwnership(7, 50); err != nil {
		t.Fatal(err)
	}
}

func TestTransferTeamOwnershipRollsBackPromotionWhenOwnerUpdateFails(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT 1 FROM teams WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(1))
	mock.ExpectExec(`(?s)INSERT INTO m_team_user.*ON CONFLICT.*DO UPDATE SET role = 0.*WHERE m_team_user.role <> 0`).
		WithArgs(int64(7), int64(50)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	wantErr := errors.New("update owner failed")
	mock.ExpectExec(`UPDATE teams SET owner_id = \$1 WHERE id = \$2`).
		WithArgs(int64(50), int64(7)).
		WillReturnError(wantErr)
	mock.ExpectRollback()

	if err := TransferTeamOwnership(7, 50); !errors.Is(err, wantErr) {
		t.Fatalf("TransferTeamOwnership() error = %v, want %v", err, wantErr)
	}
}

func TestTransferTeamOwnershipRollsBackWhenPromotionFails(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT 1 FROM teams WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(1))
	wantErr := errors.New("promote owner failed")
	mock.ExpectExec(`(?s)INSERT INTO m_team_user.*ON CONFLICT.*DO UPDATE SET role = 0.*WHERE m_team_user.role <> 0`).
		WithArgs(int64(7), int64(50)).
		WillReturnError(wantErr)
	mock.ExpectRollback()

	if err := TransferTeamOwnership(7, 50); !errors.Is(err, wantErr) {
		t.Fatalf("TransferTeamOwnership() error = %v, want %v", err, wantErr)
	}
}
