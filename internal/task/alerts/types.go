package alerttasks

import (
	"mosona-manager/internal/_type"
	"time"
)

type alertInstance struct {
	serverMap     *map[int64]string
	notifications []*_type.TeamNotification
	statuses      []*_type.ServerStatusType
}

type alertRule struct {
	id           int64
	startTime    time.Time
	threshold    int
	forDuration  int
	lastStatus   *bool
	lastNotifyAt *time.Time
}

type alertRuleUpdate struct {
	id           int64
	lastStatus   *bool
	lastNotifyAt *time.Time
}
