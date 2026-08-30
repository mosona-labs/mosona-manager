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
	expectNoAgentPrivateKeys(mock)
	mock.ExpectCommit()

	report, err := MigrateEncryptedCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) != 0 {
		t.Fatalf("unexpected failures: %#v", report.Failures)
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
	expectNoAgentPrivateKeys(mock)
	mock.ExpectCommit()

	report, err := MigrateEncryptedCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) != 0 {
		t.Fatalf("unexpected failures: %#v", report.Failures)
	}
}

func TestMigrateEncryptedCredentialsReportsInvalidCiphertextAndCommits(t *testing.T) {
	mock := setEncryptionMigrationMockDB(t)
	previousKey := encrypt.Key
	encrypt.Key = bytes.Repeat([]byte{0x42}, 32)
	t.Cleanup(func() { encrypt.Key = previousKey })

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, content, password FROM keys ORDER BY id FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "content", "password"}).
			AddRow(int64(7), []byte("invalid"), nil))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT server_id, password FROM ssh ORDER BY server_id FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"server_id", "password"}))
	expectNoAgentPrivateKeys(mock)
	mock.ExpectCommit()

	report, err := MigrateEncryptedCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) != 1 {
		t.Fatalf("failures = %#v, want one", report.Failures)
	}
	failure := report.Failures[0]
	if failure.Table != "keys" || failure.RecordID != 7 || failure.Field != "content" || failure.Err == nil {
		t.Fatalf("failure = %#v", failure)
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

	_, err := MigrateEncryptedCredentials()
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestMigrateEncryptedCredentialsRollsBackOnUpdateError(t *testing.T) {
	mock := setEncryptionMigrationMockDB(t)
	key := bytes.Repeat([]byte{0x42}, 32)
	previousKey := encrypt.Key
	encrypt.Key = key
	t.Cleanup(func() { encrypt.Key = previousKey })
	want := errors.New("update failed")
	legacyContent := legacyCiphertextForMigrationTest(t, []byte("private-key"), key)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, content, password FROM keys ORDER BY id FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "content", "password"}).
			AddRow(int64(7), legacyContent, nil))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE keys SET content = $1, password = $2 WHERE id = $3")).
		WithArgs(sqlmock.AnyArg(), nil, int64(7)).
		WillReturnError(want)
	mock.ExpectRollback()

	_, err := MigrateEncryptedCredentials()
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestMigrateEncryptedCredentialsReturnsCommitError(t *testing.T) {
	mock := setEncryptionMigrationMockDB(t)
	want := errors.New("commit failed")
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, content, password FROM keys ORDER BY id FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "content", "password"}))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT server_id, password FROM ssh ORDER BY server_id FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"server_id", "password"}))
	expectNoAgentPrivateKeys(mock)
	mock.ExpectCommit().WillReturnError(want)

	_, err := MigrateEncryptedCredentials()
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestMigrateEncryptedCredentialsCommitsEarlierUpdateAndReportsBadSSHRow(t *testing.T) {
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
	expectNoAgentPrivateKeys(mock)
	mock.ExpectCommit()

	report, err := MigrateEncryptedCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) != 1 || report.Failures[0].Table != "ssh" || report.Failures[0].RecordID != 9 {
		t.Fatalf("failures = %#v", report.Failures)
	}
}

func TestMigrateEncryptedCredentialsMigratesHealthyFieldBesideBadField(t *testing.T) {
	mock := setEncryptionMigrationMockDB(t)
	key := bytes.Repeat([]byte{0x42}, 32)
	previousKey := encrypt.Key
	encrypt.Key = key
	t.Cleanup(func() { encrypt.Key = previousKey })
	legacyPassword := legacyCiphertextForMigrationTest(t, []byte("key-password"), key)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, content, password FROM keys ORDER BY id FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "content", "password"}).
			AddRow(int64(7), []byte("invalid"), legacyPassword))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE keys SET content = $1, password = $2 WHERE id = $3")).
		WithArgs([]byte("invalid"), authenticatedCiphertextMatcher{
			plaintext: []byte("key-password"),
			context:   encrypt.KeyPasswordContext(7),
		}, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT server_id, password FROM ssh ORDER BY server_id FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"server_id", "password"}))
	expectNoAgentPrivateKeys(mock)
	mock.ExpectCommit()

	report, err := MigrateEncryptedCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) != 1 || report.Failures[0].Field != "content" {
		t.Fatalf("failures = %#v", report.Failures)
	}
}

func TestMigrateEncryptedCredentialsEncryptsPlaintextAgentKeys(t *testing.T) {
	mock := setEncryptionMigrationMockDB(t)
	key := bytes.Repeat([]byte{0x42}, 32)
	previousKey := encrypt.Key
	encrypt.Key = key
	t.Cleanup(func() { encrypt.Key = previousKey })

	current, err := encrypt.Encrypt([]byte("current-key"), key, encrypt.AgentPrivateKeyContext(8))
	if err != nil {
		t.Fatal(err)
	}
	wrongContext, err := encrypt.Encrypt([]byte("swapped-key"), key, encrypt.AgentPrivateKeyContext(99))
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, content, password FROM keys ORDER BY id FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "content", "password"}))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT server_id, password FROM ssh ORDER BY server_id FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"server_id", "password"}))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT server_id, private_key FROM agents ORDER BY server_id FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"server_id", "private_key"}).
			AddRow(int64(7), []byte("plaintext-pem")).
			AddRow(int64(8), current).
			AddRow(int64(9), []byte{}).
			AddRow(int64(10), wrongContext))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE agents SET private_key = $1 WHERE server_id = $2")).
		WithArgs(authenticatedCiphertextMatcher{
			plaintext: []byte("plaintext-pem"),
			context:   encrypt.AgentPrivateKeyContext(7),
		}, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	report, err := MigrateEncryptedCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) != 1 {
		t.Fatalf("failures = %#v, want one", report.Failures)
	}
	failure := report.Failures[0]
	if failure.Table != "agents" || failure.RecordID != 10 || failure.Field != "private_key" {
		t.Fatalf("failure = %#v", failure)
	}
}

func TestMigrateEncryptedCredentialsRollsBackOnAgentUpdateError(t *testing.T) {
	mock := setEncryptionMigrationMockDB(t)
	previousKey := encrypt.Key
	encrypt.Key = bytes.Repeat([]byte{0x42}, 32)
	t.Cleanup(func() { encrypt.Key = previousKey })
	want := errors.New("update Agent key failed")

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, content, password FROM keys ORDER BY id FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "content", "password"}))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT server_id, password FROM ssh ORDER BY server_id FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"server_id", "password"}))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT server_id, private_key FROM agents ORDER BY server_id FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"server_id", "private_key"}).AddRow(int64(7), []byte("plaintext-pem")))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE agents SET private_key = $1 WHERE server_id = $2")).
		WithArgs(sqlmock.AnyArg(), int64(7)).WillReturnError(want)
	mock.ExpectRollback()

	_, err := MigrateEncryptedCredentials()
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
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

func expectNoAgentPrivateKeys(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT server_id, private_key FROM agents ORDER BY server_id FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"server_id", "private_key"}))
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
