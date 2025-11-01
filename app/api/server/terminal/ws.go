package aterminal

import (
	"encoding/json"
	"fmt"
	"mosona-manager/db"
	"net/http"
	"strconv"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/ssh"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type xtermMessage struct {
	Type string `json:"type"`
	Data string `json:"data"`
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
}

func ws(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	serverId, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	serverAuth, err := db.GetTerminalInfo(tid, serverId)

	wsConn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	defer func() {
		_ = wsConn.Close()
	}()

	if tid == 0 {
		return wsConn.WriteMessage(websocket.TextMessage, []byte("Invalid team ID.\n"))
	}
	if serverId == 0 {
		return wsConn.WriteMessage(websocket.TextMessage, []byte("Invalid server ID.\n"))
	}
	if err != nil || serverAuth.Address == "" {
		return wsConn.WriteMessage(websocket.TextMessage, []byte("Target server not found or terminal access not enabled.\n"))
	}

	sshClient, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", serverAuth.Address, serverAuth.Port), &ssh.ClientConfig{
		User: serverAuth.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(serverAuth.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
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

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := session.StderrPipe()
	if err != nil {
		return err
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err = session.RequestPty("xterm", 40, 80, modes); err != nil {
		return err
	}
	if err = session.Shell(); err != nil {
		return err
	}

	done := make(chan struct{})

	// Read from stdout (SSH)
	go func() {
		defer close(done)
		buf := make([]byte, 1024)
		for {
			n, err := stdout.Read(buf)
			if err != nil {
				//if err != io.EOF {
				//	c.Logger().Error(err)
				//}
				return
			}
			if n > 0 {
				_ = wsConn.WriteMessage(websocket.TextMessage, buf[:n])
			}
		}
	}()

	// Read from stderr (SSH)
	go func() {
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

	// Read from WebSocket
	go func() {
		for {
			_, msg, err := wsConn.ReadMessage()
			if err != nil {
				_ = session.Close()
				return
			}

			var xtermMsg xtermMessage
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
