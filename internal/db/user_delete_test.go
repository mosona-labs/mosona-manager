package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestDeleteUserRejectsOwnedTeams(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT username FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("target"))
	mock.ExpectQuery(`SELECT id, name FROM teams WHERE owner_id = \$1 ORDER BY id`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).
			AddRow(7, "Production").
			AddRow(9, "Operations"))
	mock.ExpectRollback()

	_, err := DeleteUser(context.Background(), 42, "target")
	var ownsTeams *UserOwnsTeamsError
	if !errors.As(err, &ownsTeams) {
		t.Fatalf("DeleteUser() error = %v, want UserOwnsTeamsError", err)
	}
	if len(ownsTeams.Teams) != 2 || ownsTeams.Teams[0].ID != 7 || ownsTeams.Teams[1].Name != "Operations" {
		t.Fatalf("owned teams = %#v", ownsTeams.Teams)
	}
}

func TestDeleteUserWithoutOwnedTeamsCommits(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT username FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("target"))
	mock.ExpectQuery(`SELECT id, name FROM teams WHERE owner_id = \$1 ORDER BY id`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}))
	mock.ExpectExec(`DELETE FROM users WHERE id = \$1`).
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	username, err := DeleteUser(context.Background(), 42, "target")
	if err != nil {
		t.Fatal(err)
	}
	if username != "target" {
		t.Fatalf("DeleteUser() username = %q, want target", username)
	}
}

func TestDeleteUserMissingUserRollsBack(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT username FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(42)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	if _, err := DeleteUser(context.Background(), 42, "target"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("DeleteUser() error = %v, want sql.ErrNoRows", err)
	}
}

func TestDeleteUserRequiresMatchingUsernameConfirmation(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT username FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("target"))
	mock.ExpectRollback()

	if _, err := DeleteUser(context.Background(), 42, "different"); !errors.Is(err, ErrDeleteUserConfirmationMismatch) {
		t.Fatalf("DeleteUser() error = %v, want confirmation mismatch", err)
	}
}

func TestDeleteUserOwnedTeamQueryFailureRollsBack(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT username FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("target"))
	mock.ExpectQuery(`SELECT id, name FROM teams WHERE owner_id = \$1 ORDER BY id`).
		WithArgs(int64(42)).
		WillReturnError(errors.New("query failed"))
	mock.ExpectRollback()

	if _, err := DeleteUser(context.Background(), 42, "target"); err == nil || err.Error() != "query failed" {
		t.Fatalf("DeleteUser() error = %v, want query failure", err)
	}
}

func TestDeleteUserDeleteFailureRollsBack(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT username FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("target"))
	mock.ExpectQuery(`SELECT id, name FROM teams WHERE owner_id = \$1 ORDER BY id`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}))
	mock.ExpectExec(`DELETE FROM users WHERE id = \$1`).
		WithArgs(int64(42)).
		WillReturnError(errors.New("delete failed"))
	mock.ExpectRollback()

	if _, err := DeleteUser(context.Background(), 42, "target"); err == nil || err.Error() != "delete failed" {
		t.Fatalf("DeleteUser() error = %v, want delete failure", err)
	}
}
