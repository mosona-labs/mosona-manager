package db

import (
	"errors"
	"testing"

	"mosona-manager/internal/utils/encrypt"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestHasEncryptedCredentials(t *testing.T) {
	for _, test := range []struct {
		name   string
		exists bool
	}{
		{name: "empty or plaintext-only Agent installation", exists: false},
		{name: "stored credential or Agent envelope", exists: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = database.Close() }()
			oldDB := Db
			Db = sqlx.NewDb(database, "sqlmock")
			t.Cleanup(func() { Db = oldDB })

			mock.ExpectQuery("(?s)SELECT EXISTS.*agents.*convert_to").
				WithArgs(encrypt.EnvelopeMagic()).
				WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(test.exists))
			got, err := HasEncryptedCredentials()
			if err != nil || got != test.exists {
				t.Fatalf("exists = %v, error = %v", got, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestHasEncryptedCredentialsReturnsDatabaseError(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	oldDB := Db
	Db = sqlx.NewDb(database, "sqlmock")
	t.Cleanup(func() { Db = oldDB })

	want := errors.New("database unavailable")
	mock.ExpectQuery("(?s)SELECT EXISTS.*agents.*convert_to").
		WithArgs(encrypt.EnvelopeMagic()).
		WillReturnError(want)
	if _, err := HasEncryptedCredentials(); !errors.Is(err, want) {
		t.Fatalf("error = %v", err)
	}
}
