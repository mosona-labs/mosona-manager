package db

import (
	"bytes"
	"errors"
	"regexp"
	"testing"

	"mosona-manager/internal/utils/encrypt"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestAddKeyEncryptsWithGeneratedRecordID(t *testing.T) {
	mock := setEncryptionMigrationMockDB(t)
	previousKey := encrypt.Key
	encrypt.Key = bytes.Repeat([]byte{0x42}, 32)
	t.Cleanup(func() { encrypt.Key = previousKey })

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO keys (team_id, name, content, password) VALUES ($1, $2, $3, NULL) RETURNING id")).
		WithArgs(int64(3), "production", []byte{}).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(7)))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE keys SET content = $1, password = $2 WHERE id = $3")).
		WithArgs(
			authenticatedCiphertextMatcher{plaintext: []byte("private-key"), context: encrypt.KeyContentContext(7)},
			authenticatedCiphertextMatcher{plaintext: []byte("key-password"), context: encrypt.KeyPasswordContext(7)},
			int64(7),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	id, err := AddKey(3, "production", "private-key", "key-password")
	if err != nil {
		t.Fatal(err)
	}
	if id != 7 {
		t.Fatalf("id = %d, want 7", id)
	}
}

func TestAddKeyRollsBackWhenEncryptedUpdateFails(t *testing.T) {
	mock := setEncryptionMigrationMockDB(t)
	previousKey := encrypt.Key
	encrypt.Key = bytes.Repeat([]byte{0x42}, 32)
	t.Cleanup(func() { encrypt.Key = previousKey })
	want := errors.New("update failed")

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO keys (team_id, name, content, password) VALUES ($1, $2, $3, NULL) RETURNING id")).
		WithArgs(int64(3), "production", []byte{}).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(7)))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE keys SET content = $1, password = $2 WHERE id = $3")).
		WithArgs(sqlmock.AnyArg(), nil, int64(7)).
		WillReturnError(want)
	mock.ExpectRollback()

	if _, err := AddKey(3, "production", "private-key", ""); !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
