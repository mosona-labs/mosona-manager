package ssh

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

func stalledSSHServer(t *testing.T, stage string) (string, int, string, <-chan struct{}) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	signer := newTestSigner(t)
	reached := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	t.Cleanup(func() {
		close(release)
		_ = listener.Close()
		<-done
	})
	go func() {
		defer close(done)
		raw, err := listener.Accept()
		if err != nil {
			return
		}
		defer raw.Close()
		if stage == "authentication" {
			cfg := &gossh.ServerConfig{
				PasswordCallback: func(gossh.ConnMetadata, []byte) (*gossh.Permissions, error) {
					close(reached)
					<-release
					return nil, errors.New("stalled authentication")
				},
			}
			cfg.AddHostKey(signer)
			_, _, _, _ = gossh.NewServerConn(raw, cfg)
			return
		}
		if _, err := bufio.NewReader(raw).ReadString('\n'); err != nil {
			return
		}
		if stage == "key exchange" {
			_, _ = fmt.Fprint(raw, "SSH-2.0-stalled-test\r\n")
		}
		close(reached)
		<-release
	}()
	host, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portText)
	return host, port, string(gossh.MarshalAuthorizedKey(signer.PublicKey())), reached
}

func TestDialContextBoundsEveryHandshakeStage(t *testing.T) {
	for _, stage := range []string{"banner", "key exchange", "authentication"} {
		t.Run(stage, func(t *testing.T) {
			host, port, pin, reached := stalledSSHServer(t, stage)
			done := make(chan error, 1)
			go func() {
				client, err := DialContext(context.Background(), host, port, "test", "password", "", "", pin, false, 300*time.Millisecond)
				if client != nil {
					_ = client.Close()
				}
				done <- err
			}()
			select {
			case err := <-done:
				var netErr net.Error
				if !errors.Is(err, context.DeadlineExceeded) && !(errors.As(err, &netErr) && netErr.Timeout()) {
					t.Fatalf("DialContext() = %v, want timeout", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("SSH handshake outlived dial timeout")
			}
			select {
			case <-reached:
			default:
				t.Fatalf("never reached %s", stage)
			}
		})
	}
}

func TestDialContextCancellationInterruptsHandshake(t *testing.T) {
	host, port, pin, reached := stalledSSHServer(t, "authentication")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		client, err := DialContext(ctx, host, port, "test", "password", "", "", pin, false, time.Minute)
		if client != nil {
			_ = client.Close()
		}
		done <- err
	}()
	select {
	case <-reached:
	case <-time.After(3 * time.Second):
		t.Fatal("authentication never started")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("DialContext() = %v, want canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not interrupt handshake")
	}
}

func TestDialContextClearsHandshakeDeadlineAndChecksHostKey(t *testing.T) {
	for _, trusted := range []bool{true, false} {
		t.Run(fmt.Sprint(trusted), func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			signer := newTestSigner(t)
			done := make(chan struct{})
			go func() {
				defer close(done)
				raw, err := listener.Accept()
				if err != nil {
					return
				}
				defer raw.Close()
				cfg := &gossh.ServerConfig{NoClientAuth: true}
				cfg.AddHostKey(signer)
				conn, channels, requests, err := gossh.NewServerConn(raw, cfg)
				if err != nil {
					return
				}
				defer conn.Close()
				go func() {
					for channel := range channels {
						_ = channel.Reject(gossh.Prohibited, "no sessions")
					}
				}()
				for req := range requests {
					_ = req.Reply(true, nil)
				}
			}()
			pin := signer.PublicKey()
			if !trusted {
				pin = newTestSigner(t).PublicKey()
			}
			host, portText, _ := net.SplitHostPort(listener.Addr().String())
			port, _ := strconv.Atoi(portText)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client, err := DialContext(ctx, host, port, "test", "", "", "", string(gossh.MarshalAuthorizedKey(pin)), false, 300*time.Millisecond)
			cancel()
			if !trusted {
				if !errors.Is(err, ErrHostKeyMismatch) {
					t.Fatalf("DialContext() = %v, want host key mismatch", err)
				}
				<-done
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			// Neither expiration of the dial budget nor cancellation of the dial
			// context may close a successfully returned, long-lived connection.
			time.Sleep(400 * time.Millisecond)
			replied := make(chan error, 1)
			go func() {
				ok, _, err := client.SendRequest("test", true, nil)
				if err == nil && !ok {
					err = errors.New("request rejected")
				}
				replied <- err
			}()
			select {
			case err := <-replied:
				if err != nil {
					t.Fatalf("established connection retained dial deadline: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("established connection stopped responding")
			}
			_ = client.Close()
			<-done
		})
	}
}
