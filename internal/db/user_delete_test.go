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
	expectLockedAdmins(mock, 1)
	mock.ExpectQuery(`SELECT username, is_admin FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"username", "is_admin"}).AddRow("target", false))
	mock.ExpectQuery(`SELECT id, name FROM teams WHERE owner_id = \$1 ORDER BY id`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).
			AddRow(7, "Production").
			AddRow(9, "Operations"))
	mock.ExpectRollback()

	_, err := DeleteUser(context.Background(), 1, 42, "target", "actor-hash")
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
	expectLockedAdmins(mock, 1)
	mock.ExpectQuery(`SELECT username, is_admin FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"username", "is_admin"}).AddRow("target", false))
	mock.ExpectQuery(`SELECT id, name FROM teams WHERE owner_id = \$1 ORDER BY id`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}))
	mock.ExpectExec(`DELETE FROM users WHERE id = \$1`).
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	username, err := DeleteUser(context.Background(), 1, 42, "target", "actor-hash")
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
	expectLockedAdmins(mock, 1)
	mock.ExpectQuery(`SELECT username, is_admin FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(42)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	if _, err := DeleteUser(context.Background(), 1, 42, "target", "actor-hash"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("DeleteUser() error = %v, want sql.ErrNoRows", err)
	}
}

func TestDeleteUserRequiresMatchingUsernameConfirmation(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectBegin()
	expectLockedAdmins(mock, 1)
	mock.ExpectQuery(`SELECT username, is_admin FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"username", "is_admin"}).AddRow("target", false))
	mock.ExpectRollback()

	if _, err := DeleteUser(context.Background(), 1, 42, "different", "actor-hash"); !errors.Is(err, ErrDeleteUserConfirmationMismatch) {
		t.Fatalf("DeleteUser() error = %v, want confirmation mismatch", err)
	}
}

func TestDeleteUserOwnedTeamQueryFailureRollsBack(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectBegin()
	expectLockedAdmins(mock, 1)
	mock.ExpectQuery(`SELECT username, is_admin FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"username", "is_admin"}).AddRow("target", false))
	mock.ExpectQuery(`SELECT id, name FROM teams WHERE owner_id = \$1 ORDER BY id`).
		WithArgs(int64(42)).
		WillReturnError(errors.New("query failed"))
	mock.ExpectRollback()

	if _, err := DeleteUser(context.Background(), 1, 42, "target", "actor-hash"); err == nil || err.Error() != "query failed" {
		t.Fatalf("DeleteUser() error = %v, want query failure", err)
	}
}

func TestDeleteUserDeleteFailureRollsBack(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectBegin()
	expectLockedAdmins(mock, 1)
	mock.ExpectQuery(`SELECT username, is_admin FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"username", "is_admin"}).AddRow("target", false))
	mock.ExpectQuery(`SELECT id, name FROM teams WHERE owner_id = \$1 ORDER BY id`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}))
	mock.ExpectExec(`DELETE FROM users WHERE id = \$1`).
		WithArgs(int64(42)).
		WillReturnError(errors.New("delete failed"))
	mock.ExpectRollback()

	if _, err := DeleteUser(context.Background(), 1, 42, "target", "actor-hash"); err == nil || err.Error() != "delete failed" {
		t.Fatalf("DeleteUser() error = %v, want delete failure", err)
	}
}

func TestDeleteUserRejectsSelfBeforeStartingTransaction(t *testing.T) {
	setUserConfigMockDB(t)

	if _, err := DeleteUser(context.Background(), 42, 42, "target", "actor-hash"); !errors.Is(err, ErrCannotModifySelf) {
		t.Fatalf("DeleteUser() error = %v, want ErrCannotModifySelf", err)
	}
}

func TestDeleteUserRequiresReauthenticationBeforeStartingTransaction(t *testing.T) {
	setUserConfigMockDB(t)

	if _, err := DeleteUser(context.Background(), 1, 42, "target", ""); !errors.Is(err, ErrAdminReauthenticationRequired) {
		t.Fatalf("DeleteUser() error = %v, want ErrAdminReauthenticationRequired", err)
	}
}

func TestDeleteUserRejectsStaleReauthentication(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectBegin()
	expectLockedAdmins(mock, 1)
	mock.ExpectRollback()

	if _, err := DeleteUser(context.Background(), 1, 42, "target", "previous-hash"); !errors.Is(err, ErrAdminReauthenticationRequired) {
		t.Fatalf("DeleteUser() error = %v, want ErrAdminReauthenticationRequired", err)
	}
}

func TestDeleteUserRejectsActorWhoIsNoLongerAdmin(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectBegin()
	expectLockedAdmins(mock, 7)
	mock.ExpectRollback()

	if _, err := DeleteUser(context.Background(), 1, 42, "target", "actor-hash"); !errors.Is(err, ErrActorNotAdmin) {
		t.Fatalf("DeleteUser() error = %v, want ErrActorNotAdmin", err)
	}
}

func TestDeleteUserCanRemoveAdminWhenAnotherRemains(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectBegin()
	expectLockedAdmins(mock, 1, 42)
	mock.ExpectQuery(`SELECT username, is_admin FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"username", "is_admin"}).AddRow("target", true))
	mock.ExpectQuery(`SELECT id, name FROM teams WHERE owner_id = \$1 ORDER BY id`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}))
	mock.ExpectExec(`DELETE FROM users WHERE id = \$1`).
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if _, err := DeleteUser(context.Background(), 1, 42, "target", "actor-hash"); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteUserCommitFailureIsReturned(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectBegin()
	expectLockedAdmins(mock, 1)
	mock.ExpectQuery(`SELECT username, is_admin FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"username", "is_admin"}).AddRow("target", false))
	mock.ExpectQuery(`SELECT id, name FROM teams WHERE owner_id = \$1 ORDER BY id`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}))
	mock.ExpectExec(`DELETE FROM users WHERE id = \$1`).
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit().WillReturnError(errors.New("commit failed"))

	if _, err := DeleteUser(context.Background(), 1, 42, "target", "actor-hash"); err == nil || err.Error() != "commit failed" {
		t.Fatalf("DeleteUser() error = %v, want commit failure", err)
	}
}

func expectLockedAdmins(mock sqlmock.Sqlmock, ids ...int64) {
	rows := sqlmock.NewRows([]string{"id", "password"})
	for _, id := range ids {
		password := "target-hash"
		if id == 1 {
			password = "actor-hash"
		}
		rows.AddRow(id, password)
	}
	mock.ExpectQuery(`SELECT id, password FROM users WHERE is_admin = true ORDER BY id FOR UPDATE`).
		WillReturnRows(rows)
}
