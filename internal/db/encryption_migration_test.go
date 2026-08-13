package db

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql/driver"
	"errors"
	"io"
	"regexp"
	"testing"

	"mosona-manager/internal/utils/encrypt"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestMigrateEncryptedCredentialsCommitsLegacyRows(t *testing.T) {
	mock := setEncryptionMigrationMockDB(t)
	key := bytes.Repeat([]byte{0x42}, 32)
	previousKey := encrypt.Key
	encrypt.Key = key
	t.Cleanup(func() { encrypt.Key = previousKey })

	legacyContent := legacyCiphertextForMigrationTest(t, []byte("private-key"), key)
	legacySSHPassword := legacyCiphertextForMigrationTest(t, []byte("ssh-password"), key)
	currentPassword, err := encrypt.Encrypt([]byte("key-password"), key, encrypt.KeyPasswordContext(7))
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, content, password FROM keys ORDER BY id FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "content", "password"}).
			AddRow(int64(7), legacyContent, currentPassword))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE keys SET content = $1, password = $2 WHERE id = $3")).
		WithArgs(authenticatedCiphertextMatcher{
			plaintext: []byte("private-key"),
			context:   encrypt.KeyContentContext(7),
		}, currentPassword, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT server_id, password FROM ssh ORDER BY server_id FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"server_id", "password"}).
			AddRow(int64(9), legacySSHPassword))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE ssh SET password = $1 WHERE server_id = $2")).
		WithArgs(authenticatedCiphertextMatcher{
			plaintext: []byte("ssh-password"),
			context:   encrypt.SSHPasswordContext(9),
		}, int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := MigrateEncryptedCredentials(); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateEncryptedCredentialsSkipsCurrentAndEmptyRows(t *testing.T) {
	mock := setEncryptionMigrationMockDB(t)
	key := bytes.Repeat([]byte{0x42}, 32)
	previousKey := encrypt.Key
	encrypt.Key = key
	t.Cleanup(func() { encrypt.Key = previousKey })

	current, err := encrypt.Encrypt([]byte("private-key"), key, encrypt.KeyContentContext(7))
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, content, password FROM keys ORDER BY id FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "content", "password"}).
			AddRow(int64(7), current, nil))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT server_id, password FROM ssh ORDER BY server_id FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"server_id", "password"}).
			AddRow(int64(9), []byte{}))
	mock.ExpectCommit()

	if err := MigrateEncryptedCredentials(); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateEncryptedCredentialsRollsBackOnInvalidCiphertext(t *testing.T) {
	mock := setEncryptionMigrationMockDB(t)
	previousKey := encrypt.Key
	encrypt.Key = bytes.Repeat([]byte{0x42}, 32)
	t.Cleanup(func() { encrypt.Key = previousKey })

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, content, password FROM keys ORDER BY id FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "content", "password"}).
			AddRow(int64(7), []byte("invalid"), nil))
	mock.ExpectRollback()

	if err := MigrateEncryptedCredentials(); err == nil {
		t.Fatal("migration accepted invalid ciphertext")
	}
}

func TestMigrateCiphertextValidatesCurrentEnvelopeContext(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	previousKey := encrypt.Key
	encrypt.Key = key
	t.Cleanup(func() { encrypt.Key = previousKey })

	ciphertext, err := encrypt.Encrypt([]byte("secret"), key, encrypt.SSHPasswordContext(9))
	if err != nil {
		t.Fatal(err)
	}
	got, changed, err := migrateCiphertext(ciphertext, encrypt.SSHPasswordContext(9))
	if err != nil {
		t.Fatal(err)
	}
	if changed || !bytes.Equal(got, ciphertext) {
		t.Fatal("current valid ciphertext should be authenticated but not rewritten")
	}
	if _, _, err := migrateCiphertext(ciphertext, encrypt.SSHPasswordContext(10)); err == nil {
		t.Fatal("migration accepted ciphertext bound to a different record")
	}
}

func TestMigrateEncryptedCredentialsRollsBackOnDatabaseError(t *testing.T) {
	mock := setEncryptionMigrationMockDB(t)
	want := errors.New("database unavailable")
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, content, password FROM keys ORDER BY id FOR UPDATE")).
		WillReturnError(want)
	mock.ExpectRollback()

	if err := MigrateEncryptedCredentials(); !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestMigrateEncryptedCredentialsRollsBackAfterEarlierUpdate(t *testing.T) {
	mock := setEncryptionMigrationMockDB(t)
	key := bytes.Repeat([]byte{0x42}, 32)
	previousKey := encrypt.Key
	encrypt.Key = key
	t.Cleanup(func() { encrypt.Key = previousKey })

	legacyContent := legacyCiphertextForMigrationTest(t, []byte("private-key"), key)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, content, password FROM keys ORDER BY id FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "content", "password"}).
			AddRow(int64(7), legacyContent, nil))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE keys SET content = $1, password = $2 WHERE id = $3")).
		WithArgs(sqlmock.AnyArg(), nil, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT server_id, password FROM ssh ORDER BY server_id FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"server_id", "password"}).
			AddRow(int64(9), []byte("invalid")))
	mock.ExpectRollback()

	if err := MigrateEncryptedCredentials(); err == nil {
		t.Fatal("migration accepted invalid SSH ciphertext after updating a key")
	}
}

func setEncryptionMigrationMockDB(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	previous := Db
	Db = sqlx.NewDb(database, "sqlmock")
	t.Cleanup(func() {
		Db = previous
		_ = database.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet SQL expectations: %v", err)
		}
	})
	return mock
}

func legacyCiphertextForMigrationTest(t *testing.T, plaintext, key []byte) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	padding := aes.BlockSize - len(plaintext)%aes.BlockSize
	padded := append(append([]byte(nil), plaintext...), bytes.Repeat([]byte{byte(padding)}, padding)...)
	ciphertext := make([]byte, aes.BlockSize+len(padded))
	if _, err := io.ReadFull(rand.Reader, ciphertext[:aes.BlockSize]); err != nil {
		t.Fatal(err)
	}
	cipher.NewCBCEncrypter(block, ciphertext[:aes.BlockSize]).CryptBlocks(ciphertext[aes.BlockSize:], padded)
	return ciphertext
}

type authenticatedCiphertextMatcher struct {
	plaintext []byte
	context   string
}

func (matcher authenticatedCiphertextMatcher) Match(value driver.Value) bool {
	ciphertext, ok := value.([]byte)
	if !ok || !encrypt.IsCurrentCiphertext(ciphertext) {
		return false
	}
	plaintext, err := encrypt.Decrypt(ciphertext, encrypt.Key, matcher.context)
	return err == nil && bytes.Equal(plaintext, matcher.plaintext)
}
