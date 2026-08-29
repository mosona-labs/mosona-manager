package connection

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type manualSessionTimer struct {
	callback func()
	stopped  atomic.Bool
}

func (t *manualSessionTimer) Stop() bool {
	return !t.stopped.Swap(true)
}

func (t *manualSessionTimer) Fire() {
	// A callback already selected by the runtime may run even if Stop races
	// with it, so deliberately invoke it regardless of the stopped flag.
	t.callback()
}

func TestUserTakeRequiresMatchingServerAndConsumesSession(t *testing.T) {
	const sessionID = "server-bound-session"
	done, remove := UserSet(sessionID, 41, nil)
	t.Cleanup(remove)

	if _, _, ok := UserTake(sessionID, 42); ok {
		t.Fatal("session was claimed by a different server")
	}

	conn, finish, ok := UserTake(sessionID, 41)
	if !ok {
		t.Fatal("session was not claimable by its server")
	}
	if conn != nil {
		t.Fatal("test session unexpectedly has a connection")
	}
	if finish == nil {
		t.Fatal("session did not return a completion function")
	}

	if _, _, ok = UserTake(sessionID, 41); ok {
		t.Fatal("session was claimable more than once")
	}

	var finishers sync.WaitGroup
	for range 32 {
		finishers.Add(1)
		go func() {
			defer finishers.Done()
			finish()
		}()
	}
	finishers.Wait()
	select {
	case <-done:
	default:
		t.Fatal("finishing a claimed session did not notify the browser handler")
	}
}

func TestUserTakeConcurrentSingleWinner(t *testing.T) {
	const (
		sessionID = "concurrent-claim-session"
		serverID  = int64(41)
		claimers  = 64
	)
	done, remove := UserSet(sessionID, serverID, nil)
	t.Cleanup(remove)

	start := make(chan struct{})
	var winners atomic.Int32
	var claimWG sync.WaitGroup
	for range claimers {
		claimWG.Add(1)
		go func() {
			defer claimWG.Done()
			<-start
			_, finish, ok := UserTake(sessionID, serverID)
			if !ok {
				return
			}
			winners.Add(1)
			finish()
		}()
	}
	close(start)
	claimWG.Wait()

	if got := winners.Load(); got != 1 {
		t.Fatalf("successful claims = %d, want 1", got)
	}
	select {
	case <-done:
	default:
		t.Fatal("winning claimant did not finish the session")
	}
}

func TestUserTakeRacesSessionTimeout(t *testing.T) {
	const iterations = 100
	for i := range iterations {
		sessionID := fmt.Sprintf("timeout-race-%d", i)
		var timer *manualSessionTimer
		done, remove := userSet(sessionID, 41, nil, func(_ time.Duration, callback func()) sessionTimer {
			timer = &manualSessionTimer{callback: callback}
			return timer
		})
		t.Cleanup(remove)

		start := make(chan struct{})
		claimResult := make(chan bool, 1)
		timeoutDone := make(chan struct{})
		go func() {
			<-start
			_, finish, ok := UserTake(sessionID, 41)
			if ok {
				finish()
			}
			claimResult <- ok
		}()
		go func() {
			<-start
			timer.Fire()
			close(timeoutDone)
		}()

		close(start)
		claimed := <-claimResult
		<-timeoutDone

		select {
		case <-done:
		default:
			t.Fatalf("iteration %d: neither claim nor timeout finished the session", i)
		}
		if _, _, ok := UserTake(sessionID, 41); ok {
			t.Fatalf("iteration %d: session remained claimable after race (first claim won: %t)", i, claimed)
		}
	}
}

func TestUserSetReplacementClosesOldSessionAndIgnoresOldTimeout(t *testing.T) {
	const sessionID = "replacement-session"
	var oldTimer *manualSessionTimer
	oldDone, oldRemove := userSet(sessionID, 41, nil, func(_ time.Duration, callback func()) sessionTimer {
		oldTimer = &manualSessionTimer{callback: callback}
		return oldTimer
	})

	newDone, newRemove := UserSet(sessionID, 42, nil)
	t.Cleanup(newRemove)
	select {
	case <-oldDone:
	default:
		t.Fatal("replacing a session did not finish the old entry")
	}

	oldTimer.Fire()
	oldRemove()
	_, finish, ok := UserTake(sessionID, 42)
	if !ok {
		t.Fatal("old timeout removed the replacement session")
	}
	finish()
	select {
	case <-newDone:
	default:
		t.Fatal("replacement session was not finished")
	}
}
