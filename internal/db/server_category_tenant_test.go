package db

import (
	"errors"
	"testing"

	"mosona-manager/internal/_type"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestEditServerRejectsCategoryOutsideTeamBeforeUpdates(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM categories WHERE team = \$1 AND id = \$2 FOR KEY SHARE`).
		WithArgs(int64(7), int64(22)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()

	err := EditServer(7, 91, 1, &_type.ServerFullType{Category: 22})
	if !errors.Is(err, ErrServerCategoryNotFound) {
		t.Fatalf("EditServer() error = %v, want ErrServerCategoryNotFound", err)
	}
}

func TestEditServerRollsBackCategoryLookupFailure(t *testing.T) {
	mock := setUserConfigMockDB(t)
	want := errors.New("category lookup failed")
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM categories WHERE team = \$1 AND id = \$2 FOR KEY SHARE`).
		WithArgs(int64(7), int64(5)).
		WillReturnError(want)
	mock.ExpectRollback()

	err := EditServer(7, 91, 1, &_type.ServerFullType{Category: 5})
	if !errors.Is(err, want) {
		t.Fatalf("EditServer() error = %v, want %v", err, want)
	}
}
