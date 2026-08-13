package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func TestUpdateAdminUserEditsRegularUserWithoutReauthentication(t *testing.T) {
	mock := setUserConfigMockDB(t)
	expectAdminUserUpdateTarget(mock, []int64{1}, 42, false)
	mock.ExpectExec(`UPDATE users SET username = \$1, email = \$2, verified = \$3, is_admin = \$4 WHERE id = \$5`).
		WithArgs("target", "target@example.com", true, false, int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := UpdateAdminUser(context.Background(), 1, 42, AdminUserUpdate{
		Username: "target", Email: "target@example.com", Verified: true,
	}, "")
	if err != nil {
		t.Fatal(err)
	}
}

func TestUpdateAdminUserRequiresReauthenticationForDemotion(t *testing.T) {
	mock := setUserConfigMockDB(t)
	expectAdminUserUpdateTarget(mock, []int64{1, 42}, 42, true)
	mock.ExpectRollback()

	err := UpdateAdminUser(context.Background(), 1, 42, AdminUserUpdate{}, "")
	if !errors.Is(err, ErrAdminReauthenticationRequired) {
		t.Fatalf("UpdateAdminUser() error = %v, want ErrAdminReauthenticationRequired", err)
	}
}

func TestUpdateAdminUserDemotesAdminWhenAnotherRemains(t *testing.T) {
	mock := setUserConfigMockDB(t)
	expectAdminUserUpdateTarget(mock, []int64{1, 42}, 42, true)
	mock.ExpectExec(`UPDATE users SET username = \$1, email = \$2, verified = \$3, is_admin = \$4 WHERE id = \$5`).
		WithArgs("target", "target@example.com", true, false, int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := UpdateAdminUser(context.Background(), 1, 42, AdminUserUpdate{
		Username: "target", Email: "target@example.com", Verified: true,
	}, "actor-hash")
	if err != nil {
		t.Fatal(err)
	}
}

func TestUpdateAdminUserRejectsLastAdminDemotion(t *testing.T) {
	mock := setUserConfigMockDB(t)
	expectAdminUserUpdateTarget(mock, []int64{1}, 42, true)
	mock.ExpectRollback()

	err := UpdateAdminUser(context.Background(), 1, 42, AdminUserUpdate{}, "actor-hash")
	if !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("UpdateAdminUser() error = %v, want ErrLastAdmin", err)
	}
}

func TestUpdateAdminUserRejectsSelfDemotionBeforeStartingTransaction(t *testing.T) {
	setUserConfigMockDB(t)

	err := UpdateAdminUser(context.Background(), 42, 42, AdminUserUpdate{}, "actor-hash")
	if !errors.Is(err, ErrCannotModifySelf) {
		t.Fatalf("UpdateAdminUser() error = %v, want ErrCannotModifySelf", err)
	}
}

func TestUpdateAdminUserRequiresReauthenticationForPromotion(t *testing.T) {
	mock := setUserConfigMockDB(t)
	expectAdminUserUpdateTarget(mock, []int64{1}, 42, false)
	mock.ExpectRollback()

	err := UpdateAdminUser(context.Background(), 1, 42, AdminUserUpdate{IsAdmin: true}, "")
	if !errors.Is(err, ErrAdminReauthenticationRequired) {
		t.Fatalf("UpdateAdminUser() error = %v, want ErrAdminReauthenticationRequired", err)
	}
}

func TestUpdateAdminUserRequiresReauthenticationForPasswordReset(t *testing.T) {
	mock := setUserConfigMockDB(t)
	expectAdminUserUpdateTarget(mock, []int64{1}, 42, false)
	mock.ExpectRollback()

	err := UpdateAdminUser(context.Background(), 1, 42, AdminUserUpdate{PasswordHash: "hash"}, "")
	if !errors.Is(err, ErrAdminReauthenticationRequired) {
		t.Fatalf("UpdateAdminUser() error = %v, want ErrAdminReauthenticationRequired", err)
	}
}

func TestUpdateAdminUserRejectsStaleReauthentication(t *testing.T) {
	mock := setUserConfigMockDB(t)
	expectAdminUserUpdateTarget(mock, []int64{1, 42}, 42, true)
	mock.ExpectRollback()

	err := UpdateAdminUser(context.Background(), 1, 42, AdminUserUpdate{}, "previous-hash")
	if !errors.Is(err, ErrAdminReauthenticationRequired) {
		t.Fatalf("UpdateAdminUser() error = %v, want ErrAdminReauthenticationRequired", err)
	}
}

func TestUpdateAdminUserMapsEmailConstraintViolation(t *testing.T) {
	mock := setUserConfigMockDB(t)
	expectAdminUserUpdateTarget(mock, []int64{1}, 42, false)
	mock.ExpectExec(`UPDATE users SET username = \$1, email = \$2, verified = \$3, is_admin = \$4 WHERE id = \$5`).
		WithArgs("target", "taken@example.com", false, false, int64(42)).
		WillReturnError(&pq.Error{Code: "23505", Constraint: "users_email_unique"})
	mock.ExpectRollback()

	err := UpdateAdminUser(context.Background(), 1, 42, AdminUserUpdate{
		Username: "target", Email: "taken@example.com",
	}, "")
	if !errors.Is(err, ErrUserEmailExists) {
		t.Fatalf("UpdateAdminUser() error = %v, want ErrUserEmailExists", err)
	}
}

func TestUpdateAdminUserChangesPasswordAndTimestamp(t *testing.T) {
	mock := setUserConfigMockDB(t)
	expectAdminUserUpdateTarget(mock, []int64{1}, 42, false)
	mock.ExpectExec(`UPDATE users SET username = \$1, email = \$2, verified = \$3, is_admin = \$4, password = \$5, salt = \$6, pwd_at = now\(\) WHERE id = \$7`).
		WithArgs("target", "target@example.com", true, false, "hash", "salt", int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := UpdateAdminUser(context.Background(), 1, 42, AdminUserUpdate{
		Username: "target", Email: "target@example.com", Verified: true,
		PasswordHash: "hash", PasswordSalt: "salt",
	}, "actor-hash")
	if err != nil {
		t.Fatal(err)
	}
}

func TestUpdateAdminUserZeroRowsRollsBack(t *testing.T) {
	mock := setUserConfigMockDB(t)
	expectAdminUserUpdateTarget(mock, []int64{1}, 42, false)
	mock.ExpectExec(`UPDATE users SET username = \$1, email = \$2, verified = \$3, is_admin = \$4 WHERE id = \$5`).
		WithArgs("target", "target@example.com", false, false, int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	err := UpdateAdminUser(context.Background(), 1, 42, AdminUserUpdate{
		Username: "target", Email: "target@example.com",
	}, "")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("UpdateAdminUser() error = %v, want sql.ErrNoRows", err)
	}
}

func TestUpdateAdminUserCommitFailureIsReturned(t *testing.T) {
	mock := setUserConfigMockDB(t)
	expectAdminUserUpdateTarget(mock, []int64{1}, 42, false)
	mock.ExpectExec(`UPDATE users SET username = \$1, email = \$2, verified = \$3, is_admin = \$4 WHERE id = \$5`).
		WithArgs("target", "target@example.com", false, false, int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit().WillReturnError(errors.New("commit failed"))

	err := UpdateAdminUser(context.Background(), 1, 42, AdminUserUpdate{
		Username: "target", Email: "target@example.com",
	}, "")
	if err == nil || err.Error() != "commit failed" {
		t.Fatalf("UpdateAdminUser() error = %v, want commit failure", err)
	}
}

func TestUpdateAdminUserMissingTargetRollsBack(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectBegin()
	expectLockedAdmins(mock, 1)
	mock.ExpectQuery(`SELECT is_admin FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(42)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	err := UpdateAdminUser(context.Background(), 1, 42, AdminUserUpdate{}, "")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("UpdateAdminUser() error = %v, want sql.ErrNoRows", err)
	}
}

func TestUpdateAdminUserRejectsActorWhoIsNoLongerAdmin(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectBegin()
	expectLockedAdmins(mock, 7)
	mock.ExpectRollback()

	err := UpdateAdminUser(context.Background(), 1, 42, AdminUserUpdate{}, "")
	if !errors.Is(err, ErrActorNotAdmin) {
		t.Fatalf("UpdateAdminUser() error = %v, want ErrActorNotAdmin", err)
	}
}

func expectAdminUserUpdateTarget(mock sqlmock.Sqlmock, adminIDs []int64, userID int64, isAdmin bool) {
	mock.ExpectBegin()
	expectLockedAdmins(mock, adminIDs...)
	mock.ExpectQuery(`SELECT is_admin FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"is_admin"}).AddRow(isAdmin))
}
