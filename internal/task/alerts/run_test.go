package alerttasks

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"mosona-manager/internal/db"
)

func TestAlertStateEqual(t *testing.T) {
	now := time.Now()
	sameInstant := now.In(time.FixedZone("test", 3600))
	trueValue := true
	otherTrue := true
	falseValue := false

	tests := []struct {
		name                     string
		oldStatus, newStatus     *bool
		oldNotifyAt, newNotifyAt *time.Time
		want                     bool
	}{
		{name: "all nil", want: true},
		{name: "same values", oldStatus: &trueValue, newStatus: &otherTrue, oldNotifyAt: &now, newNotifyAt: &sameInstant, want: true},
		{name: "status changed", oldStatus: &trueValue, newStatus: &falseValue, oldNotifyAt: &now, newNotifyAt: &now},
		{name: "status became known", newStatus: &trueValue},
		{name: "notify time changed", oldStatus: &trueValue, newStatus: &trueValue, oldNotifyAt: &now, newNotifyAt: timePointer(now.Add(time.Second))},
		{name: "notify time cleared", oldStatus: &trueValue, newStatus: &trueValue, oldNotifyAt: &now},
		{name: "notify time became known", oldStatus: &trueValue, newStatus: &trueValue, newNotifyAt: &now},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := alertStateEqual(tt.oldStatus, tt.newStatus, tt.oldNotifyAt, tt.newNotifyAt); got != tt.want {
				t.Fatalf("alertStateEqual() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestUpdateRuleStatusSkipsEmptyQueue(t *testing.T) {
	oldDB := db.Db
	db.Db = nil
	t.Cleanup(func() { db.Db = oldDB })
	if err := updateRuleStatus(nil); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateRuleStatusUsesConditionalUpdate(t *testing.T) {
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	oldDB := db.Db
	db.Db = sqlx.NewDb(database, "sqlmock")
	t.Cleanup(func() {
		db.Db = oldDB
		_ = database.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet SQL expectations: %v", err)
		}
	})

	now := time.Now()
	status := true
	mock.ExpectBegin()
	mock.ExpectPrepare(regexp.QuoteMeta(`UPDATE server_alerts
		SET last_status = $1, last_notify_at = $2
		WHERE id = $3
		  AND (last_status IS DISTINCT FROM $1 OR last_notify_at IS DISTINCT FROM $2)`)).
		ExpectExec().
		WithArgs(&status, &now, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := updateRuleStatus([]alertRuleUpdate{{id: 7, lastStatus: &status, lastNotifyAt: &now}}); err != nil {
		t.Fatal(err)
	}
}

func TestSkippedAlertRulesMessageReportsCountsAndLoadHealth(t *testing.T) {
	observations := newAlertObservationSet()
	observations.queryFailures = 2
	observations.invalidDurations = 1
	observations.loadStopped = true
	message := skippedAlertRulesMessage(4, map[string]int{
		alertItemStatus: 1,
		alertItemCPU:    3,
	}, observations)
	for _, fragment := range []string{
		"skipped 4 rules",
		"cpu_usage=3, status=1",
		"query_failures=2",
		"invalid_durations=1",
		"load_stopped=true",
	} {
		if !strings.Contains(message, fragment) {
			t.Fatalf("summary %q missing %q", message, fragment)
		}
	}
}

func TestSetObservationForRuleRequiresLoadedDataAndClearsState(t *testing.T) {
	observations := newAlertObservationSet()
	cpuKey := alertObservationKey{serverID: 7, item: alertItemCPU}
	observations.loaded[cpuKey] = struct{}{}
	observations.values[cpuKey] = alertObservation{present: true, value: 88}
	alert := &alertInstance{observation: alertObservation{present: true, value: 99}}

	if !alert.setObservationForRule(observations, 7, alertItemCPU) {
		t.Fatal("loaded CPU observation was rejected")
	}
	if alert.observation != (alertObservation{present: true, value: 88}) {
		t.Fatalf("CPU observation = %#v", alert.observation)
	}

	if !alert.setObservationForRule(observations, 7, alertItemExpiry) {
		t.Fatal("expiry rule unexpectedly requires an InfluxDB observation")
	}
	if alert.observation != (alertObservation{}) {
		t.Fatalf("expiry retained a previous observation: %#v", alert.observation)
	}

	alert.observation = alertObservation{present: true, value: 77}
	if alert.setObservationForRule(observations, 7, alertItemMemory) {
		t.Fatal("missing memory observation was accepted")
	}
	if alert.observation != (alertObservation{}) {
		t.Fatalf("missing observation retained previous state: %#v", alert.observation)
	}
}

func timePointer(value time.Time) *time.Time {
	return &value
}
