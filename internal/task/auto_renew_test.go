package task

import (
	"errors"
	"mosona-manager/internal/db"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestNextRenewalEndTimeCatchesUpInOneRun(t *testing.T) {
	location := time.FixedZone("UTC+8", 8*60*60)
	tests := map[string]struct {
		endTime time.Time
		now     time.Time
		cycle   int
		want    time.Time
	}{
		"monthly month-end": {
			endTime: time.Date(2025, time.January, 31, 12, 0, 0, 0, location),
			now:     time.Date(2025, time.May, 1, 12, 0, 0, 0, location),
			cycle:   1,
			want:    time.Date(2025, time.May, 31, 12, 0, 0, 0, location),
		},
		"monthly non-month-end anchor": {
			endTime: time.Date(2025, time.January, 30, 12, 0, 0, 0, location),
			now:     time.Date(2025, time.March, 1, 12, 0, 0, 0, location),
			cycle:   1,
			want:    time.Date(2025, time.March, 30, 12, 0, 0, 0, location),
		},
		"quarterly skips elapsed renewal": {
			endTime: time.Date(2025, time.January, 31, 12, 0, 0, 0, location),
			now:     time.Date(2025, time.May, 1, 12, 0, 0, 0, location),
			cycle:   2,
			want:    time.Date(2025, time.July, 31, 12, 0, 0, 0, location),
		},
		"semiannual skips elapsed renewal": {
			endTime: time.Date(2024, time.August, 31, 12, 0, 0, 0, location),
			now:     time.Date(2025, time.September, 1, 12, 0, 0, 0, location),
			cycle:   3,
			want:    time.Date(2026, time.February, 28, 12, 0, 0, 0, location),
		},
		"annual leap-day anchor": {
			endTime: time.Date(2020, time.February, 29, 8, 30, 0, 0, location),
			now:     time.Date(2023, time.March, 1, 8, 30, 0, 0, location),
			cycle:   4,
			want:    time.Date(2024, time.February, 29, 8, 30, 0, 0, location),
		},
		"exact renewal boundary": {
			endTime: time.Date(2025, time.January, 15, 9, 0, 0, 0, location),
			now:     time.Date(2025, time.April, 15, 9, 0, 0, 0, location),
			cycle:   1,
			want:    time.Date(2025, time.April, 15, 9, 0, 0, 0, location),
		},
		"past renewal boundary": {
			endTime: time.Date(2025, time.January, 15, 9, 0, 0, 0, location),
			now:     time.Date(2025, time.April, 15, 9, 0, 0, 1, location),
			cycle:   1,
			want:    time.Date(2025, time.May, 15, 9, 0, 0, 0, location),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := nextRenewalEndTime(test.endTime, test.now, test.cycle)
			if !ok {
				t.Fatal("valid cycle was rejected")
			}
			if !got.Equal(test.want) {
				t.Fatalf("next renewal = %s, want %s", got, test.want)
			}
			if got.Before(test.now) {
				t.Fatalf("next renewal %s is before now %s", got, test.now)
			}
		})
	}
}

func TestNextRenewalEndTimePreservesWallClockAcrossDST(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	endTime := time.Date(2025, time.January, 31, 12, 30, 0, 0, location)
	now := time.Date(2025, time.April, 1, 12, 30, 0, 0, location)
	want := time.Date(2025, time.April, 30, 12, 30, 0, 0, location)

	got, ok := nextRenewalEndTime(endTime, now, 1)
	if !ok {
		t.Fatal("valid cycle was rejected")
	}
	if !got.Equal(want) {
		t.Fatalf("next renewal = %s, want %s", got, want)
	}
	_, originalOffset := endTime.Zone()
	_, renewedOffset := got.Zone()
	if originalOffset == renewedOffset {
		t.Fatalf("test did not cross a DST offset change: %s -> %s", endTime, got)
	}
}

func TestNextRenewalEndTimeRejectsInvalidCycle(t *testing.T) {
	if _, ok := nextRenewalEndTime(time.Now(), time.Now(), 5); ok {
		t.Fatal("invalid cycle was accepted")
	}
}

func TestAutoRenewAtLocksAndCatchesUpExpiredRows(t *testing.T) {
	mock := setAutoRenewMockDB(t)
	now := time.Date(2025, time.May, 1, 12, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, time.January, 31, 12, 0, 0, 0, time.UTC)
	wantEndTime := time.Date(2025, time.May, 31, 12, 0, 0, 0, time.UTC)
	secondEndTime := time.Date(2024, time.August, 31, 12, 0, 0, 0, time.UTC)
	secondWantEndTime := time.Date(2025, time.August, 31, 12, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(autoRenewSelectQuery).
		WithArgs(now).
		WillReturnRows(sqlmock.NewRows([]string{"sid", "end_time", "cycle"}).
			AddRow(7, endTime, 1).
			AddRow(8, secondEndTime, 3))
	mock.ExpectExec("UPDATE server_info SET end_time=$1 WHERE sid=$2").
		WithArgs(wantEndTime, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE server_info SET end_time=$1 WHERE sid=$2").
		WithArgs(secondWantEndTime, int64(8)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := autoRenewAt(now); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAutoRenewAtRollsBackOnUpdateFailure(t *testing.T) {
	mock := setAutoRenewMockDB(t)
	now := time.Date(2025, time.May, 1, 12, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, time.January, 31, 12, 0, 0, 0, time.UTC)
	wantErr := errors.New("update failed")

	mock.ExpectBegin()
	mock.ExpectQuery(autoRenewSelectQuery).
		WithArgs(now).
		WillReturnRows(sqlmock.NewRows([]string{"sid", "end_time", "cycle"}).AddRow(7, endTime, 1))
	mock.ExpectExec("UPDATE server_info SET end_time=$1 WHERE sid=$2").
		WithArgs(time.Date(2025, time.May, 31, 12, 0, 0, 0, time.UTC), int64(7)).
		WillReturnError(wantErr)
	mock.ExpectRollback()

	if err := autoRenewAt(now); !errors.Is(err, wantErr) {
		t.Fatalf("autoRenewAt error = %v, want %v", err, wantErr)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

const autoRenewSelectQuery = `
SELECT sid, end_time, cycle
FROM server_info
WHERE end_time IS NOT NULL
  AND end_time < $1
  AND auto_renew
  AND cycle BETWEEN 1 AND 4
FOR UPDATE`

func setAutoRenewMockDB(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatal(err)
	}

	previous := db.Db
	db.Db = sqlx.NewDb(database, "sqlmock")
	t.Cleanup(func() {
		_ = db.Db.Close()
		db.Db = previous
	})
	return mock
}
