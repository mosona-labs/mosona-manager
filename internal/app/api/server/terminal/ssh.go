package aterminal

import (
	"encoding/json"
	"fmt"
	"mosona-manager/internal/_type"
	connectSSH "mosona-manager/internal/connect/ssh"
	pbTypes "mosona-manager/pkg/types"
	"sync"

	"github.com/gorilla/websocket"
	gossh "golang.org/x/crypto/ssh"
)

func terminalSSH(serverAuth _type.TerminalDetail, wsConn *websocket.Conn) error {
	defer func() {
		_ = wsConn.Close()
	}()

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

	authMethods, err := connectSSH.BuildAuthMethods(password, key, keyPwd)
	if err != nil {
		return err
	}

	sshClient, err := gossh.Dial("tcp", fmt.Sprintf("%s:%d", *serverAuth.Address, *serverAuth.Port), &gossh.ClientConfig{
		User:            *serverAuth.Username,
		Auth:            authMethods,
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		return wsConn.WriteMessage(websocket.TextMessage, []byte("Failed to connect to target server: "+err.Error()+"\n"))
	}
	defer func() {
		_ = sshClient.Close()
	}()

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

	var wg sync.WaitGroup
	wg.Add(2)

	// Read from stdout (SSH)
	go func() {
		defer wg.Done()
		buf := make([]byte, 1024)
		for {
			n, err := stdout.Read(buf)
			if err != nil {
				return
			}
			if n > 0 {
				_ = wsConn.WriteMessage(websocket.TextMessage, buf[:n])
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
				return
			}
			if n > 0 {
				_ = wsConn.WriteMessage(websocket.TextMessage, buf[:n])
			}
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// Read from WebSocket
	go func() {
		for {
			_, msg, err := wsConn.ReadMessage()
			if err != nil {
				_ = session.Close()
				return
			}

			var xtermMsg pbTypes.XTermMessage
			if err := json.Unmarshal(msg, &xtermMsg); err != nil {
				_, _ = stdin.Write(msg)
			} else {
				switch xtermMsg.Type {
				case "input":
					_, _ = stdin.Write([]byte(xtermMsg.Data))
				case "resize":
					_ = session.WindowChange(int(xtermMsg.Rows), int(xtermMsg.Cols))
				}
			}
		}
	}()

	<-done
	return nil
}
