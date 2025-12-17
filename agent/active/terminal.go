package active

import (
	"crypto/ecdh"
	"encoding/json"
	"log"
	secureWS "mosona-manager/pkg/securews"
	pbTypes "mosona-manager/pkg/types"
	"os"
	"os/exec"
	"os/user"
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
		shell = "powershell.exe"
	case "darwin", "linux":
		for _, sh := range []string{
			"bash", "zsh", "ksh", "ash", "dash",
		} {
			path, err := exec.LookPath("/bin/" + sh)
			if err == nil {
				shell = "/bin/" + path
				break
			}
		}
		if shell == "" {
			shell = "/bin/sh"
		}
	default:
		shell = "/bin/sh"
	}

	cmd := exec.Command(shell, args...)

	u, err := user.Current()
	if err == nil {
		switch runtime.GOOS {
		case "windows":
			cmd.Dir = u.HomeDir
			cmd.Env = append(os.Environ(), []string{
				"USERPROFILE=" + u.HomeDir,
				"USERNAME=" + u.Username,
				"TERM=xterm-256color",
			}...)
		case "darwin", "linux":
			cmd.Dir = u.HomeDir
			cmd.Env = append(os.Environ(), []string{
				"HOME=" + u.HomeDir,
				"USER=" + u.Name,
				"LOGNAME=" + u.Name,
				"SHELL=" + shell,
				"TERM=xterm-256color",
			}...)
		}
	}

	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Printf("Failed to start pty for session: %v", err)
		return
	}
	defer func() {
		_ = ptmx.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	done := make(chan struct{})

	// Handle pty output -> websocket
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := ptmx.Read(buf)
			if err != nil {
				done <- struct{}{}
				return
			}

			if n > 0 {
				data, err := sc.Encrypt(buf[:n])
				if err != nil {
					done <- struct{}{}
					return
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
					done <- struct{}{}
					return
				}
			}
		}
	}()

	// Handle websocket input -> pty
	go func() {
		for {
			_, originMsg, err := conn.ReadMessage()
			if err != nil {
				done <- struct{}{}
				return
			}
			msg, err := sc.Decrypt(originMsg)
			if err != nil {
				done <- struct{}{}
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
