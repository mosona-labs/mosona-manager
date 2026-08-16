package callback

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"mosona-manager/internal/utils"
)

func TestUpdateInformationWritesChangedValues(t *testing.T) {
	tx, mock, cleanup := beginInformationTestTransaction(t)
	defer cleanup()

	mock.ExpectExec("UPDATE server_info").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE server_info_adv").WillReturnResult(sqlmock.NewResult(0, 1))

	if err := updateTestInformation(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateInformationAcceptsUnchangedExistingRows(t *testing.T) {
	tx, mock, cleanup := beginInformationTestTransaction(t)
	defer cleanup()

	mock.ExpectExec("UPDATE server_info").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS (SELECT 1 FROM server_info WHERE sid = $1)")).
		WithArgs(int64(91)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("UPDATE server_info_adv").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS (SELECT 1 FROM server_info_adv WHERE sid = $1)")).
		WithArgs(int64(91)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	if err := updateTestInformation(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateInformationRejectsMissingRows(t *testing.T) {
	tx, mock, cleanup := beginInformationTestTransaction(t)
	defer cleanup()

	mock.ExpectExec("UPDATE server_info").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS (SELECT 1 FROM server_info WHERE sid = $1)")).
		WithArgs(int64(91)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	if err := updateTestInformation(context.Background(), tx); err == nil {
		t.Fatal("expected a missing server_info row to fail")
	}
}

func TestUpdateInformationFailsWhenRowCountIsUnknown(t *testing.T) {
	tx, mock, cleanup := beginInformationTestTransaction(t)
	defer cleanup()

	mock.ExpectExec("UPDATE server_info").
		WillReturnResult(sqlmock.NewErrorResult(errors.New("rows affected unsupported")))

	if err := updateTestInformation(context.Background(), tx); err == nil {
		t.Fatal("expected an unknown affected row count to fail")
	}
}

func TestUpdateInformationRejectsMultipleAffectedRows(t *testing.T) {
	tx, mock, cleanup := beginInformationTestTransaction(t)
	defer cleanup()

	mock.ExpectExec("UPDATE server_info").WillReturnResult(sqlmock.NewResult(0, 2))

	if err := updateTestInformation(context.Background(), tx); err == nil {
		t.Fatal("expected an update affecting more than one row to fail")
	}
}

func beginInformationTestTransaction(t *testing.T) (*sqlx.Tx, sqlmock.Sqlmock, func()) {
	t.Helper()
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	tx, err := sqlx.NewDb(database, "sqlmock").Beginx()
	if err != nil {
		t.Fatal(err)
	}
	return tx, mock, func() {
		mock.ExpectRollback()
		_ = tx.Rollback()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet SQL expectations: %v", err)
		}
		_ = database.Close()
	}
}

func updateTestInformation(ctx context.Context, tx *sqlx.Tx) error {
	return updateInformation(
		ctx,
		tx,
		91,
		"Linux",
		time.Unix(1_700_000_000, 0),
		"host-1",
		"cpu-1",
		4,
		8,
		"6.12",
		"192.0.2.10",
		"amd64",
		utils.IPGeoResponse{CountryCode: "US", Country: "United States"},
	)
}
