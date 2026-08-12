package db

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestSetUserActiveTeamUsesAtomicMembershipInsert(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectQuery(`(?s)INSERT INTO users_config \(uid, active_team\).*SELECT \$1, \$2.*FROM m_team_user.*WHERE user_id = \$1 AND team_id = \$2.*ON CONFLICT.*RETURNING active_team`).
		WithArgs(int64(42), int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"active_team"}).AddRow(7))

	if err := SetUserActiveTeam(42, 7); err != nil {
		t.Fatal(err)
	}
}

func TestSetUserActiveTeamRejectsNonMember(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectQuery(`(?s)INSERT INTO users_config.*FROM m_team_user.*RETURNING active_team`).
		WithArgs(int64(42), int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"active_team"}))

	if err := SetUserActiveTeam(42, 7); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("SetUserActiveTeam() error = %v, want sql.ErrNoRows", err)
	}
}

func TestSetUserActiveTeamZeroClearsConfiguration(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectExec(`DELETE FROM users_config WHERE uid = \$1`).
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := SetUserActiveTeam(42, 0); err != nil {
		t.Fatal(err)
	}
}

func TestGetUserActiveTeamRequiresCurrentMembership(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectQuery(`(?s)SELECT uc.active_team.*FROM users_config AS uc.*JOIN m_team_user AS mtu.*mtu.team_id = uc.active_team AND mtu.user_id = uc.uid.*WHERE uc.uid = \$1`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"active_team"}).AddRow(7))

	tid, err := GetUserActiveTeam(42)
	if err != nil {
		t.Fatal(err)
	}
	if tid != 7 {
		t.Fatalf("GetUserActiveTeam() = %d, want 7", tid)
	}
}

func TestGetUserActiveTeamReturnsZeroWithoutCurrentMembership(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectQuery(`(?s)SELECT uc.active_team.*JOIN m_team_user AS mtu.*WHERE uc.uid = \$1`).
		WithArgs(int64(42)).
		WillReturnError(sql.ErrNoRows)

	tid, err := GetUserActiveTeam(42)
	if err != nil {
		t.Fatal(err)
	}
	if tid != 0 {
		t.Fatalf("GetUserActiveTeam() = %d, want 0", tid)
	}
}

func TestClearUserActiveTeamIsConditional(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectExec(`DELETE FROM users_config WHERE uid = \$1 AND active_team = \$2`).
		WithArgs(int64(42), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := ClearUserActiveTeam(42, 7); err != nil {
		t.Fatal(err)
	}
}

func setUserConfigMockDB(t *testing.T) sqlmock.Sqlmock {
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
