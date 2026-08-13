package config

import (
	"sync"
	"testing"
)

func TestDynamicConfigSnapshotsArePublishedAtomically(t *testing.T) {
	previous := ReadDynamicConf()
	t.Cleanup(func() { ReplaceDynamicConf(previous) })

	a := DynamicConfigType{
		Title:        "snapshot-a",
		Domain:       "https://a.example.com",
		Token:        "token-a",
		SMTPHost:     "smtp-a.example.com",
		SMTPPort:     1025,
		SMTPUsername: "user-a",
		SMTPPassword: "password-a",
	}
	b := DynamicConfigType{
		Title:        "snapshot-b",
		Domain:       "https://b.example.com",
		Token:        "token-b",
		SMTPHost:     "smtp-b.example.com",
		SMTPPort:     2025,
		SMTPUsername: "user-b",
		SMTPPassword: "password-b",
	}
	ReplaceDynamicConf(a)

	const iterations = 10_000
	start := make(chan struct{})
	invalid := make(chan DynamicConfigType, 1)
	var readers sync.WaitGroup
	for range 8 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			<-start
			for range iterations {
				got := ReadDynamicConf()
				if got != a && got != b {
					select {
					case invalid <- got:
					default:
					}
					return
				}
			}
		}()
	}

	close(start)
	for range iterations {
		ReplaceDynamicConf(b)
		ReplaceDynamicConf(a)
	}
	readers.Wait()

	select {
	case got := <-invalid:
		t.Fatalf("reader observed partial snapshot: %+v", got)
	default:
	}
}
