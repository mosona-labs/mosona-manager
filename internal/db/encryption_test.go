package db

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestHasEncryptedCredentials(t *testing.T) {
	for _, test := range []struct {
		name   string
		exists bool
	}{
		{name: "empty installation", exists: false},
		{name: "stored credentials", exists: true},
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

			mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(test.exists))
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
	mock.ExpectQuery("SELECT EXISTS").WillReturnError(want)
	if _, err := HasEncryptedCredentials(); !errors.Is(err, want) {
		t.Fatalf("error = %v", err)
	}
}
