package store

import (
	"sync"
	"time"
)

type TwoFACodeState struct {
	UID    int64
	Expire int64
}

var (
	twoFAStore = struct {
		sync.RWMutex
		data map[string]TwoFACodeState
	}{data: make(map[string]TwoFACodeState)}
)

const cleanupInterval2FA = 5 * 60 // 5 minutes

func init() {
	go cleanupExpired2FAData()
}

func GetTwoFACodeState(state string) (TwoFACodeState, bool) {
	twoFAStore.RLock()
	defer twoFAStore.RUnlock()
	val, ok := twoFAStore.data[state]
	return val, ok
}

func SetTwoFACodeState(state string, uid int64, expire int64) {
	twoFAStore.Lock()
	defer twoFAStore.Unlock()
	twoFAStore.data[state] = TwoFACodeState{
		UID:    uid,
		Expire: expire,
	}
}

func DeleteTwoFACodeState(state string) {
	twoFAStore.Lock()
	defer twoFAStore.Unlock()
	delete(twoFAStore.data, state)
}

func DeleteTwoFACodeByUID(uid int64) {
	twoFAStore.Lock()
	defer twoFAStore.Unlock()
	for state, codeState := range twoFAStore.data {
		if codeState.UID == uid {
			delete(twoFAStore.data, state)
		}
	}
}

func cleanupExpired2FAData() {
	ticker := time.NewTicker(cleanupInterval2FA * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now().Unix()

		twoFAStore.Lock()
		for state, codeState := range twoFAStore.data {
			if now > codeState.Expire {
				delete(twoFAStore.data, state)
			}
		}
		twoFAStore.Unlock()
	}
}
