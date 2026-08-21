package alerttasks

import (
	"mosona-manager/internal/_type"
	"time"
)

type alertInstance struct {
	serverMap     *map[int64]string
	expiryMap     map[int64]serverExpiryInfo
	notifications []*_type.TeamNotification
	observation   alertObservation
}

type alertRule struct {
	id           int64
	threshold    int
	forDuration  int
	lastStatus   *bool
	lastNotifyAt *time.Time
}

type alertObservation struct {
	present    bool
	value      float64
	mountPoint string
}

type alertObservationKey struct {
	serverID int64
	item     string
}

type alertObservationSet struct {
	values map[alertObservationKey]alertObservation
	// loaded distinguishes a successful empty result from a failed query.
	loaded           map[alertObservationKey]struct{}
	queryFailures    int
	invalidDurations int
	loadStopped      bool
}

func newAlertObservationSet() *alertObservationSet {
	return &alertObservationSet{
		values: make(map[alertObservationKey]alertObservation),
		loaded: make(map[alertObservationKey]struct{}),
	}
}

func (s *alertObservationSet) get(serverID int64, item string) (alertObservation, bool) {
	key := alertObservationKey{serverID: serverID, item: item}
	_, loaded := s.loaded[key]
	return s.values[key], loaded
}

type alertRuleUpdate struct {
	id           int64
	lastStatus   *bool
	lastNotifyAt *time.Time
}

type serverExpiryInfo struct {
	EndTime   *time.Time
	AutoRenew bool
}
