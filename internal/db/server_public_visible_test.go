package db

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"mosona-manager/internal/_type"
)

// TestGetServerInfoScansPublicVisible proves the info endpoint's read path
// surfaces public_visible as a concrete value for both true and false rows.
func TestGetServerInfoScansPublicVisible(t *testing.T) {
	for _, test := range []struct {
		name string
		want bool
	}{
		{name: "visible", want: true},
		{name: "hidden", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			database, mock, err := sqlmock.New()
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

			mock.ExpectQuery("SELECT.*s\\.public_visible.*FROM servers s").
				WithArgs(int64(7), int64(91)).
				WillReturnRows(sqlmock.NewRows([]string{
					"id", "name", "type", "allow_monitor", "allow_terminal", "public_visible",
					"weight", "category", "note", "provider", "cycle", "start_time", "end_time",
					"amount", "auto_renew", "bandwidth", "traffic", "traffic_type", "note_public",
				}).AddRow(
					int64(91), "srv", int16(0), true, true, test.want,
					0, int64(5), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
				))
			mock.ExpectQuery("SELECT address, port, username, key_id FROM ssh").
				WithArgs(int64(91)).
				WillReturnRows(sqlmock.NewRows([]string{"address", "port", "username", "key_id"}).AddRow("s.example", 22, "root", 0))

			data, err := GetServerInfo(7, 91)
			if err != nil {
				t.Fatal(err)
			}
			if data.PublicVisible == nil || *data.PublicVisible != test.want {
				t.Fatalf("public_visible = %#v, want %t", data.PublicVisible, test.want)
			}
		})
	}
}

// TestEditServerOmitsPublicVisibleWhenAbsent proves a PUT payload that does
// not carry public_visible (older bundled UIs, still-open dashboards, or
// third-party clients) leaves the stored column untouched, so servers stay
// visible on the public page instead of being hidden by a zero-value bool.
func TestEditServerOmitsPublicVisibleWhenAbsent(t *testing.T) {
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
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

	data := _type.ServerFullType{Category: 5, HostKey: "ssh-ed25519 AAAANEW"}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM categories WHERE team = $1 AND id = $2 FOR KEY SHARE").
		WithArgs(int64(7), int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(5))
	mock.ExpectExec("UPDATE servers SET name = $1, allow_monitor = $2, allow_terminal = $3, weight = $4, category = $5 WHERE id = $6 AND team_id = $7").
		WithArgs("", false, false, 0, int64(5), int64(91), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE server_info SET note = $1, provider = $2, cycle = $3, start_time = $4, end_time = $5, amount = $6, auto_renew = $7, bandwidth = $8, traffic = $9, traffic_type = $10, note_public = $11 WHERE sid = $12").
		WithArgs(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, int64(91)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE ssh SET address = $1, port = $2, username = $3, host_key = $4, trust_legacy_host_key = $5 WHERE server_id = $6 AND host_key IS NULL").
		WithArgs("", 0, "", data.HostKey, false, int64(91)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err = EditServer(7, 91, 0, &data); err != nil {
		t.Fatal(err)
	}
}

// TestEditServerSetsPublicVisibleWhenProvided proves an explicit value is
// written when the client sends the field.
func TestEditServerSetsPublicVisibleWhenProvided(t *testing.T) {
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
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

	hidden := false
	data := _type.ServerFullType{Category: 5, HostKey: "ssh-ed25519 AAAANEW", PublicVisible: &hidden}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM categories WHERE team = $1 AND id = $2 FOR KEY SHARE").
		WithArgs(int64(7), int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(5))
	mock.ExpectExec("UPDATE servers SET name = $1, allow_monitor = $2, allow_terminal = $3, weight = $4, category = $5, public_visible = $6 WHERE id = $7 AND team_id = $8").
		WithArgs("", false, false, 0, int64(5), false, int64(91), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE server_info SET note = $1, provider = $2, cycle = $3, start_time = $4, end_time = $5, amount = $6, auto_renew = $7, bandwidth = $8, traffic = $9, traffic_type = $10, note_public = $11 WHERE sid = $12").
		WithArgs(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, int64(91)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE ssh SET address = $1, port = $2, username = $3, host_key = $4, trust_legacy_host_key = $5 WHERE server_id = $6 AND host_key IS NULL").
		WithArgs("", 0, "", data.HostKey, false, int64(91)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err = EditServer(7, 91, 0, &data); err != nil {
		t.Fatal(err)
	}
}
