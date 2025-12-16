package active

import (
	"crypto/ecdh"
	"encoding/json"
	"log"
	secureWS "mosona-manager/pkg/securews"
	pbTypes "mosona-manager/pkg/types"
	"os"
	"os/exec"
	"runtime"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

func handleTerminalWebSocket(
	conn *websocket.Conn,
	xHubPubKey *ecdh.PublicKey,
	xAgentPrivKey *ecdh.PrivateKey,
	hubNonce string,
	agentNonce string,
) {
	defer func() {
		_ = conn.Close()
	}()

	sc, err := secureWS.NewSessionCrypto(secureWS.RoleAgent, xHubPubKey, xAgentPrivKey, hubNonce, agentNonce)
	if err != nil {
		_ = conn.WriteMessage(websocket.CloseMessage, []byte("crypto init failed"))
		return
	}

	var shell string
	var args []string
	switch runtime.GOOS {
	case "windows":
		shell = "cmd.exe"
	case "darwin", "linux":
		shell = os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		args = []string{"-l"}
	default:
		shell = "/bin/sh"
	}

	cmd := exec.Command(shell, args...)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Printf("Failed to start pty for session: %v", err)
		return
	}
	defer func() {
		_ = ptmx.Close()
		_ = cmd.Process.Kill()
	}()

	done := make(chan struct{})

	// Handle pty output -> websocket
	go func() {
		defer close(done)
		buf := make([]byte, 1024)
		for {
			n, err := ptmx.Read(buf)
			if err != nil {
				//if err != io.EOF {
				//	log.Printf("Error reading from pty: %v", err)
				//}
				return
			}

			if n > 0 {
				data, err := sc.Encrypt(buf[:n])
				if err != nil {
					return
				}
				_ = conn.WriteMessage(websocket.BinaryMessage, data)
			}
		}
	}()

	// Handle websocket input -> pty
	go func() {
		for {
			_, originMsg, err := conn.ReadMessage()
			if err != nil {
				_ = ptmx.Close()
				return
			}
			msg, err := sc.Decrypt(originMsg)
			if err != nil {
				_ = ptmx.Close()
				return
			}

			var xtermMsg pbTypes.XTermMessage
			if err := json.Unmarshal(msg, &xtermMsg); err != nil {
				_, _ = ptmx.Write(msg)
			} else {
				switch xtermMsg.Type {
				case "input":
					_, _ = ptmx.Write([]byte(xtermMsg.Data))
				case "resize":
					_ = pty.Setsize(ptmx, &pty.Winsize{
						Rows: xtermMsg.Rows,
						Cols: xtermMsg.Cols,
					})
				}
			}
		}
	}()

	<-done
}
