package active

import (
	"bytes"
	"errors"
	"log"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"mosona-manager/internal/db"
	"mosona-manager/pkg/identity"
)

func setupIdentityTest(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	oldDB := db.Db
	db.Db = sqlx.NewDb(database, "sqlmock")
	t.Cleanup(func() {
		db.Db = oldDB
		_ = database.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
	})
	return mock
}

func testPublicKeyPEM(t *testing.T) string {
	t.Helper()
	_, publicKey, err := identity.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	return publicKey
}

func TestPinActiveAgentPublicKeyFirstUse(t *testing.T) {
	mock := setupIdentityTest(t)
	candidate := testPublicKeyPEM(t)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE agents SET public_key = $1, protocol_version = 2 WHERE server_id = $2 AND public_key = '' AND EXISTS (SELECT 1 FROM servers WHERE id = $2 AND type = 1)")).
		WithArgs(candidate, int64(91)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT team_id FROM servers WHERE id = $1")).
		WithArgs(int64(91)).WillReturnRows(sqlmock.NewRows([]string{"team_id"}).AddRow(42))

	got, err := pinActiveAgentPublicKey(91, candidate)
	if err != nil || got != candidate {
		t.Fatalf("pin = %q, %v", got, err)
	}
}

func TestPinActiveAgentPublicKeyIsIdempotent(t *testing.T) {
	mock := setupIdentityTest(t)
	candidate := testPublicKeyPEM(t)
	mock.ExpectExec("UPDATE agents SET public_key").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT public_key FROM agents WHERE server_id = $1")).
		WithArgs(int64(91)).WillReturnRows(sqlmock.NewRows([]string{"public_key"}).AddRow(candidate))

	if got, err := pinActiveAgentPublicKey(91, candidate); err != nil || got != candidate {
		t.Fatalf("idempotent pin = %q, %v", got, err)
	}
}

func TestPinActiveAgentPublicKeyRejectsDifferentWinner(t *testing.T) {
	mock := setupIdentityTest(t)
	candidate := testPublicKeyPEM(t)
	mock.ExpectExec("UPDATE agents SET public_key").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT public_key FROM agents WHERE server_id = $1")).
		WithArgs(int64(91)).WillReturnRows(sqlmock.NewRows([]string{"public_key"}).AddRow(testPublicKeyPEM(t)))

	if _, err := pinActiveAgentPublicKey(91, candidate); !errors.Is(err, ErrAgentIdentityMismatch) {
		t.Fatalf("pin error = %v, want identity mismatch", err)
	}
}

func TestRecordActiveIdentityEventLogsPreparationFailure(t *testing.T) {
	var output bytes.Buffer
	oldWriter := log.Writer()
	oldFlags := log.Flags()
	log.SetOutput(&output)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(oldWriter)
		log.SetFlags(oldFlags)
	})

	recordActiveIdentityEvent(91, "invalid key", "event", "high")
	if got := output.String(); !bytes.Contains([]byte(got), []byte("Failed to record Active Agent identity event for Server (ID91)")) {
		t.Fatalf("local log = %q, want audit preparation failure", got)
	}
}
