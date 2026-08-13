package db

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestDeleteCategoryRollsBackOnQueryError(t *testing.T) {
	mock := setAlertMockDB(t)
	want := errors.New("query failed")
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM categories WHERE team = \$1 ORDER BY id LIMIT 1`).
		WithArgs(int64(7)).
		WillReturnError(want)
	mock.ExpectRollback()

	if err := DeleteCategory(7, 9); !errors.Is(err, want) {
		t.Fatalf("DeleteCategory() error = %v, want %v", err, want)
	}
}

func TestDeleteCategoryRollsBackDefaultCategoryEarlyReturn(t *testing.T) {
	mock := setAlertMockDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM categories WHERE team = \$1 ORDER BY id LIMIT 1`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(9))
	mock.ExpectRollback()

	if err := DeleteCategory(7, 9); !errors.Is(err, ErrCanNotDeleteDefaultCategory) {
		t.Fatalf("DeleteCategory() error = %v, want %v", err, ErrCanNotDeleteDefaultCategory)
	}
}

func TestDeleteCategoryUsesSingleTransaction(t *testing.T) {
	mock := setAlertMockDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM categories WHERE team = \$1 ORDER BY id LIMIT 1`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(8))
	mock.ExpectExec(`UPDATE servers SET category = \$1 WHERE category = \$2 AND team_id = \$3`).
		WithArgs(int64(8), int64(9), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`DELETE FROM categories WHERE id = \$1 AND team = \$2`).
		WithArgs(int64(9), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := DeleteCategory(7, 9); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteCategoryRejectsCategoryOutsideTeam(t *testing.T) {
	mock := setAlertMockDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM categories WHERE team = \$1 ORDER BY id LIMIT 1`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(8))
	mock.ExpectExec(`UPDATE servers SET category = \$1 WHERE category = \$2 AND team_id = \$3`).
		WithArgs(int64(8), int64(22), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DELETE FROM categories WHERE id = \$1 AND team = \$2`).
		WithArgs(int64(22), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	if err := DeleteCategory(7, 22); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("DeleteCategory() error = %v, want sql.ErrNoRows", err)
	}
}

func TestDeleteTeamAlertRollsBackOnQueryError(t *testing.T) {
	mock := setAlertMockDB(t)
	want := errors.New("query failed")
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT threshold, for_duration FROM team_alerts WHERE team_id = \$1 AND item = \$2`).
		WithArgs(int64(7), "cpu_usage").
		WillReturnError(want)
	mock.ExpectRollback()

	if _, err := DeleteTeamAlert(7, "cpu_usage", false); !errors.Is(err, want) {
		t.Fatalf("DeleteTeamAlert() error = %v, want %v", err, want)
	}
}
