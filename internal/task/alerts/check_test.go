package alerttasks

import (
	"errors"
	"strings"
	"testing"
	"time"

	"mosona-manager/internal/_type"
)

func boolPtr(value bool) *bool {
	return &value
}

func TestStatusNotificationFor(t *testing.T) {
	tests := []struct {
		name          string
		lastStatus    *bool
		currentStatus bool
		want          statusNotification
	}{
		{
			name:          "first check while online does not notify",
			currentStatus: true,
			want:          statusNotificationNone,
		},
		{
			name:          "first check after offline window notifies down",
			currentStatus: false,
			want:          statusNotificationDown,
		},
		{
			name:          "remaining online does not repeat recovery",
			lastStatus:    boolPtr(true),
			currentStatus: true,
			want:          statusNotificationNone,
		},
		{
			name:          "online to offline notifies down",
			lastStatus:    boolPtr(true),
			currentStatus: false,
			want:          statusNotificationDown,
		},
		{
			name:          "remaining offline does not repeat down",
			lastStatus:    boolPtr(false),
			currentStatus: false,
			want:          statusNotificationNone,
		},
		{
			name:          "confirmed offline to online notifies recovery",
			lastStatus:    boolPtr(false),
			currentStatus: true,
			want:          statusNotificationUp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statusNotificationFor(tt.lastStatus, tt.currentStatus); got != tt.want {
				t.Fatalf("statusNotificationFor() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckStatusAlertDoesNotRepeatRecoveryWhileOnline(t *testing.T) {
	now := time.Now()
	lastNotifyAt := now.Add(-2 * time.Hour)
	serverMap := map[int64]string{1: "test-server"}
	alert := &alertInstance{
		serverMap: &serverMap,
		statuses: []*_type.ServerStatusType{
			{Time: now},
		},
	}

	currentStatus, nextNotifyAt := alert.checkStatusAlert(1, &alertRule{
		startTime:    now.Add(-10 * time.Minute),
		lastStatus:   boolPtr(true),
		lastNotifyAt: &lastNotifyAt,
	})

	if currentStatus == nil || !*currentStatus {
		t.Fatal("checkStatusAlert() reported offline, want online")
	}
	if nextNotifyAt != &lastNotifyAt {
		t.Fatal("checkStatusAlert() updated lastNotifyAt while remaining online")
	}
}

func TestCheckStatusAlertNotifiesRecoveryAfterConfirmedOffline(t *testing.T) {
	originalSender := sendAlertShoutrrr
	sendAlertShoutrrr = func(string, string) error { return nil }
	defer func() { sendAlertShoutrrr = originalSender }()

	now := time.Now()
	lastNotifyAt := now.Add(-time.Minute)
	serverMap := map[int64]string{1: "test-server"}
	alert := &alertInstance{
		serverMap:     &serverMap,
		notifications: []*_type.TeamNotification{{Module: "shoutrrr", Target: "generic://test"}},
		statuses: []*_type.ServerStatusType{
			{Time: now},
		},
	}

	currentStatus, nextNotifyAt := alert.checkStatusAlert(1, &alertRule{
		startTime:    now.Add(-10 * time.Minute),
		lastStatus:   boolPtr(false),
		lastNotifyAt: &lastNotifyAt,
	})

	if currentStatus == nil || !*currentStatus {
		t.Fatal("checkStatusAlert() reported offline, want online")
	}
	if nextNotifyAt == nil || !nextNotifyAt.After(lastNotifyAt) {
		t.Fatal("checkStatusAlert() did not record the recovery notification")
	}
}

func TestCheckStatusAlertFailedDeliveryKeepsPendingTransition(t *testing.T) {
	originalSender := sendAlertShoutrrr
	sendAlertShoutrrr = func(string, string) error { return errors.New("send failed") }
	defer func() { sendAlertShoutrrr = originalSender }()

	now := time.Now()
	lastNotifyAt := now.Add(-time.Minute)
	serverMap := map[int64]string{1: "test-server"}
	alert := &alertInstance{
		serverMap:     &serverMap,
		notifications: []*_type.TeamNotification{{Module: "shoutrrr", Target: "generic://test"}},
		statuses:      []*_type.ServerStatusType{{Time: now}},
	}

	lastStatus := boolPtr(false)
	currentStatus, nextNotifyAt := alert.checkStatusAlert(1, &alertRule{
		startTime:    now.Add(-10 * time.Minute),
		lastStatus:   lastStatus,
		lastNotifyAt: &lastNotifyAt,
	})

	if currentStatus != lastStatus {
		t.Fatal("checkStatusAlert() advanced status after failed delivery")
	}
	if nextNotifyAt != &lastNotifyAt {
		t.Fatal("checkStatusAlert() advanced lastNotifyAt after failed delivery")
	}
}

func TestCheckStatusAlertTracksStateWithoutNotificationChannels(t *testing.T) {
	now := time.Now()
	lastNotifyAt := now.Add(-time.Minute)
	serverMap := map[int64]string{1: "test-server"}
	alert := &alertInstance{
		serverMap: &serverMap,
		statuses:  []*_type.ServerStatusType{{Time: now}},
	}

	currentStatus, nextNotifyAt := alert.checkStatusAlert(1, &alertRule{
		startTime:    now.Add(-10 * time.Minute),
		lastStatus:   boolPtr(false),
		lastNotifyAt: &lastNotifyAt,
	})

	if currentStatus == nil || !*currentStatus {
		t.Fatal("checkStatusAlert() did not update state without notification channels")
	}
	if nextNotifyAt != &lastNotifyAt {
		t.Fatal("checkStatusAlert() changed lastNotifyAt without attempting delivery")
	}
}

func TestCheckStatusAlertUsesConfiguredOfflineWindowInMessage(t *testing.T) {
	originalSender := sendAlertShoutrrr
	var sentMessage string
	sendAlertShoutrrr = func(_ string, message string) error {
		sentMessage = message
		return nil
	}
	defer func() { sendAlertShoutrrr = originalSender }()

	now := time.Now()
	serverMap := map[int64]string{1: "test-server"}
	alert := &alertInstance{
		serverMap:     &serverMap,
		notifications: []*_type.TeamNotification{{Module: "shoutrrr", Target: "generic://test"}},
	}

	alert.checkStatusAlert(1, &alertRule{
		startTime:   now.Add(-17 * time.Minute),
		forDuration: 17,
		lastStatus:  boolPtr(true),
	})

	if !strings.Contains(sentMessage, "No response for 17 minutes.") {
		t.Fatalf("notification message %q does not contain configured offline window", sentMessage)
	}
}

func TestCheckMetricAlertFailedDeliveryKeepsPendingAlert(t *testing.T) {
	originalSender := sendAlertShoutrrr
	sendAlertShoutrrr = func(string, string) error { return errors.New("send failed") }
	defer func() { sendAlertShoutrrr = originalSender }()

	now := time.Now()
	lastNotifyAt := now.Add(-time.Minute)
	lastStatus := boolPtr(false)
	serverMap := map[int64]string{1: "test-server"}
	alert := &alertInstance{
		serverMap:     &serverMap,
		notifications: []*_type.TeamNotification{{Module: "shoutrrr", Target: "generic://test"}},
		statuses:      []*_type.ServerStatusType{{Time: now, CPU: 95}},
	}

	currentStatus, nextNotifyAt := alert.checkCPUUsageAlert(1, &alertRule{
		startTime:    now.Add(-10 * time.Minute),
		threshold:    80,
		forDuration:  10,
		lastStatus:   lastStatus,
		lastNotifyAt: &lastNotifyAt,
	})

	if currentStatus != lastStatus {
		t.Fatal("checkCPUUsageAlert() advanced status after failed delivery")
	}
	if nextNotifyAt != &lastNotifyAt {
		t.Fatal("checkCPUUsageAlert() advanced lastNotifyAt after failed delivery")
	}
}

func TestCheckExpiryReminderFailedDeliveryRemainsPending(t *testing.T) {
	originalSender := sendAlertShoutrrr
	sendAlertShoutrrr = func(string, string) error { return errors.New("send failed") }
	defer func() { sendAlertShoutrrr = originalSender }()

	now := time.Now()
	endTime := now.Add(12 * time.Hour)
	lastNotifyAt := now.Add(-time.Minute)
	lastStatus := boolPtr(false)
	serverMap := map[int64]string{1: "test-server"}
	alert := &alertInstance{
		serverMap:     &serverMap,
		expiryMap:     map[int64]serverExpiryInfo{1: {EndTime: &endTime}},
		notifications: []*_type.TeamNotification{{Module: "shoutrrr", Target: "generic://test"}},
	}

	currentStatus, nextNotifyAt := alert.checkExpiryReminderAlert(1, &alertRule{
		threshold:    1,
		lastStatus:   lastStatus,
		lastNotifyAt: &lastNotifyAt,
	})

	if currentStatus != lastStatus {
		t.Fatal("checkExpiryReminderAlert() marked reminder sent after failed delivery")
	}
	if nextNotifyAt != &lastNotifyAt {
		t.Fatal("checkExpiryReminderAlert() advanced lastNotifyAt after failed delivery")
	}
}

func TestCheckExpiryReminderTracksStateWithoutNotificationChannels(t *testing.T) {
	now := time.Now()
	endTime := now.Add(12 * time.Hour)
	lastNotifyAt := now.Add(-time.Minute)
	serverMap := map[int64]string{1: "test-server"}
	alert := &alertInstance{
		serverMap: &serverMap,
		expiryMap: map[int64]serverExpiryInfo{1: {EndTime: &endTime}},
	}

	currentStatus, nextNotifyAt := alert.checkExpiryReminderAlert(1, &alertRule{
		threshold:    1,
		lastStatus:   boolPtr(false),
		lastNotifyAt: &lastNotifyAt,
	})

	if currentStatus == nil || !*currentStatus {
		t.Fatal("checkExpiryReminderAlert() did not update state without notification channels")
	}
	if nextNotifyAt != &lastNotifyAt {
		t.Fatal("checkExpiryReminderAlert() changed lastNotifyAt without attempting delivery")
	}
}
