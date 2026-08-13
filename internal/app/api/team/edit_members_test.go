package ateam

import (
	"errors"
	"os"
	"path/filepath"
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
	mock.ExpectQuery(`SELECT owner_id, image FROM teams WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"owner_id", "image"}).AddRow(10, "existing.avif"))
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
	mock.ExpectExec(`UPDATE teams SET name = \$1, description = \$2, color = \$3, image = \$4 WHERE id = \$5`).
		WithArgs("Team", "Description", "blue", "existing.avif", int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	members := []_type.TeamUsersRole{
		{User: _type.User{ID: 10}, Role: 0},
		{User: _type.User{ID: 30}, Role: 2},
	}
	removed, oldAvatar, err := updateTeam(tx, 7, 10, "Team", "Description", "blue", nil, members)
	if err != nil {
		t.Fatal(err)
	}
	if oldAvatar != "existing.avif" {
		t.Fatalf("old avatar = %q, want existing.avif", oldAvatar)
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

func TestUpdateTeamRejectsOwnerRemovalBeforeMemberWrites(t *testing.T) {
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
	mock.ExpectQuery(`SELECT owner_id, image FROM teams WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"owner_id", "image"}).AddRow(10, "existing.avif"))
	mock.ExpectQuery(`SELECT role FROM m_team_user WHERE team_id = \$1 AND user_id = \$2 FOR UPDATE`).
		WithArgs(int64(7), int64(20)).
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow(0))
	mock.ExpectRollback()

	if _, _, err = updateTeam(tx, 7, 20, "Team", "Description", "blue", nil, []_type.TeamUsersRole{
		{User: _type.User{ID: 20}, Role: 0},
	}); !errors.Is(err, errInvalidTeamMembers) {
		t.Fatalf("updateTeam() error = %v, want errInvalidTeamMembers", err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateTeamUsesNewAvatarAndRollsBackOnUpdateError(t *testing.T) {
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
	mock.ExpectQuery(`SELECT owner_id, image FROM teams WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"owner_id", "image"}).AddRow(10, "existing.avif"))
	mock.ExpectQuery(`SELECT role FROM m_team_user WHERE team_id = \$1 AND user_id = \$2 FOR UPDATE`).
		WithArgs(int64(7), int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow(0))
	mock.ExpectQuery(`SELECT user_id FROM m_team_user WHERE team_id = \$1 ORDER BY user_id FOR UPDATE`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(10))
	mock.ExpectExec(`(?s)INSERT INTO m_team_user.*ON CONFLICT`).
		WithArgs(int64(7), int64(10), 0).
		WillReturnResult(sqlmock.NewResult(0, 1))
	want := errors.New("update failed")
	mock.ExpectExec(`UPDATE teams SET name = \$1, description = \$2, color = \$3, image = \$4 WHERE id = \$5`).
		WithArgs("Team", "Description", "blue", "replacement.avif", int64(7)).
		WillReturnError(want)
	mock.ExpectRollback()

	newAvatar := "replacement.avif"
	if _, _, err = updateTeam(tx, 7, 10, "Team", "Description", "blue", &newAvatar, []_type.TeamUsersRole{
		{User: _type.User{ID: 10}, Role: 0},
	}); !errors.Is(err, want) {
		t.Fatalf("updateTeam() error = %v, want %v", err, want)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateTeamMembers(t *testing.T) {
	tests := []struct {
		name    string
		ownerID int64
		actorID int64
		members []_type.TeamUsersRole
		wantErr bool
	}{
		{
			name:    "owner remains administrator",
			ownerID: 10,
			actorID: 10,
			members: []_type.TeamUsersRole{{User: _type.User{ID: 10}, Role: 0}},
		},
		{
			name:    "owner removed",
			ownerID: 10,
			actorID: 20,
			members: []_type.TeamUsersRole{{User: _type.User{ID: 20}, Role: 0}},
			wantErr: true,
		},
		{
			name:    "owner downgraded",
			ownerID: 10,
			actorID: 20,
			members: []_type.TeamUsersRole{{User: _type.User{ID: 10}, Role: 2}, {User: _type.User{ID: 20}, Role: 0}},
			wantErr: true,
		},
		{
			name:    "actor removed",
			ownerID: 10,
			actorID: 20,
			members: []_type.TeamUsersRole{{User: _type.User{ID: 10}, Role: 0}, {User: _type.User{ID: 30}, Role: 0}},
			wantErr: true,
		},
		{
			name:    "actor downgraded",
			ownerID: 10,
			actorID: 20,
			members: []_type.TeamUsersRole{{User: _type.User{ID: 10}, Role: 0}, {User: _type.User{ID: 20}, Role: 1}},
			wantErr: true,
		},
		{
			name:    "invalid role",
			ownerID: 10,
			actorID: 10,
			members: []_type.TeamUsersRole{{User: _type.User{ID: 10}, Role: 0}, {User: _type.User{ID: 20}, Role: 3}},
			wantErr: true,
		},
		{
			name:    "duplicate member",
			ownerID: 10,
			actorID: 10,
			members: []_type.TeamUsersRole{{User: _type.User{ID: 10}, Role: 0}, {User: _type.User{ID: 10}, Role: 0}},
			wantErr: true,
		},
		{
			name:    "invalid user id",
			ownerID: 10,
			actorID: 10,
			members: []_type.TeamUsersRole{{User: _type.User{ID: 10}, Role: 0}, {Role: 2}},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTeamMembers(test.ownerID, test.actorID, test.members)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateTeamMembers() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestRemoveTeamAvatarFile(t *testing.T) {
	directory := t.TempDir()
	oldPath := filepath.Join(directory, "old.avif")
	if err := os.WriteFile(oldPath, []byte("avatar"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeTeamAvatarFile(directory, "old.avif", "new.avif"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old avatar still exists or stat failed unexpectedly: %v", err)
	}

	outside := filepath.Join(filepath.Dir(directory), "outside.avif")
	if err := os.WriteFile(outside, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) })
	if err := removeTeamAvatarFile(directory, "../outside.avif", "new.avif"); err == nil {
		t.Fatal("expected path traversal filename to be rejected")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file was modified: %v", err)
	}
}
