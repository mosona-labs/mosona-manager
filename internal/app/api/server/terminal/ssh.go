package aterminal

import (
	"context"
	"encoding/json"
	"log"
	"mosona-manager/internal/_type"
	connectSSH "mosona-manager/internal/connect/ssh"
	"mosona-manager/internal/influx"
	pbTypes "mosona-manager/pkg/types"
	"mosona-manager/pkg/wsutil"
	"strconv"
	"sync"

	"github.com/gorilla/websocket"
	gossh "golang.org/x/crypto/ssh"
)

func terminalSSH(
	ctx context.Context,
	serverAuth _type.TerminalDetail,
	wsConn *websocket.Conn,
	teamID, userID, serverID int64,
	ip, userAgent string,
) error {
	defer func() {
		_ = wsConn.Close()
	}()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	password := ""
	if serverAuth.Password != nil {
		password = *serverAuth.Password
	}

	key := ""
	if serverAuth.Key != nil {
		key = *serverAuth.Key
	}

	keyPwd := ""
	if serverAuth.KeyPwd != nil {
		keyPwd = *serverAuth.KeyPwd
	}

	trustedHostKey := ""
	if serverAuth.HostKey != nil {
		trustedHostKey = *serverAuth.HostKey
	}

	sshClient, err := connectSSH.Dial(
		*serverAuth.Address,
		*serverAuth.Port,
		*serverAuth.Username,
		password,
		key,
		keyPwd,
		trustedHostKey,
		serverAuth.TrustLegacyHostKey,
		connectSSH.DefaultDialTimeout,
	)
	if err != nil {
		if connectSSH.IsPermanentHostKeyError(err) {
			influx.LogAdd(
				teamID, userID, "security",
				"Blocked SSH terminal connection due to untrusted host key (server ID "+strconv.FormatInt(serverID, 10)+"): "+err.Error(),
				ip, userAgent, "high",
			)
		}
		return wsConn.WriteMessage(websocket.TextMessage, []byte("Failed to connect to target server: "+err.Error()+"\n"))
	}
	defer func() {
		_ = sshClient.Close()
	}()
	connectSSH.KeepAlive(ctx, sshClient)

	session, err := sshClient.NewSession()
	if err != nil {
		return wsConn.WriteMessage(websocket.TextMessage, []byte("Failed to create session: "+err.Error()+"\n"))
	}
	defer func() {
		_ = session.Close()
	}()

	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}
	defer func() {
		_ = stdin.Close()
	}()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := session.StderrPipe()
	if err != nil {
		return err
	}

	modes := gossh.TerminalModes{
		gossh.ECHO:          1,
		gossh.TTY_OP_ISPEED: 14400,
		gossh.TTY_OP_OSPEED: 14400,
	}

	if err = session.RequestPty("xterm", 40, 80, modes); err != nil {
		return err
	}
	if err = session.Shell(); err != nil {
		return err
	}

	var once sync.Once
	done := make(chan struct{})
	var writeMu sync.Mutex
	cleanup := func() {
		once.Do(func() {
			cancel()
			_ = session.Close()
			_ = sshClient.Close()
			_ = wsConn.Close()
			close(done)
		})
	}
	defer cleanup()

	writeMessage := func(messageType int, data []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return wsConn.WriteMessage(messageType, data)
	}
	wsutil.SetSafePingHandler(wsConn, &writeMu)
	wsutil.StartPing(ctx, wsConn, &writeMu, "ssh terminal websocket ping")

	go func() {
		<-ctx.Done()
		cleanup()
	}()

	var wg sync.WaitGroup
	wg.Add(2)

	// Read from stdout (SSH)
	go func() {
		defer wg.Done()
		buf := make([]byte, 1024)
		for {
			n, err := stdout.Read(buf)
			if err != nil {
				log.Println("ssh terminal stdout:", err)
				cleanup()
				return
			}
			if n > 0 {
				if err := writeMessage(websocket.BinaryMessage, buf[:n]); err != nil {
					log.Println("ssh terminal stdout websocket:", err)
					cleanup()
					return
				}
			}
		}
	}()

	// Read from stderr (SSH)
	go func() {
		defer wg.Done()
		buf := make([]byte, 1024)
		for {
			n, err := stderr.Read(buf)
			if err != nil {
				log.Println("ssh terminal stderr:", err)
				cleanup()
				return
			}
			if n > 0 {
				if err := writeMessage(websocket.BinaryMessage, buf[:n]); err != nil {
					log.Println("ssh terminal stderr websocket:", err)
					cleanup()
					return
				}
			}
		}
	}()

	go func() {
		wg.Wait()
		cleanup()
	}()

	// Read from WebSocket
	go func() {
		for {
			_, msg, err := wsConn.ReadMessage()
			if err != nil {
				log.Println("ssh terminal websocket read:", err)
				cleanup()
				return
			}

			var xtermMsg pbTypes.XTermMessage
			if err := json.Unmarshal(msg, &xtermMsg); err != nil {
				if _, err := stdin.Write(msg); err != nil {
					log.Println("ssh terminal stdin:", err)
					cleanup()
					return
				}
			} else {
				switch xtermMsg.Type {
				case "input":
					if _, err := stdin.Write([]byte(xtermMsg.Data)); err != nil {
						log.Println("ssh terminal stdin:", err)
						cleanup()
						return
					}
				case "resize":
					if err := session.WindowChange(int(xtermMsg.Rows), int(xtermMsg.Cols)); err != nil {
						log.Println("ssh terminal resize:", err)
						cleanup()
						return
					}
				}
			}
		}
	}()

	<-done
	return nil
}
