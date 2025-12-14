package passive

import (
	"encoding/json"
	"fmt"
	"log"
	pbTypes "mosona-manager/pkg/types"
	"os"
	"os/exec"
	"runtime"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

func terminal(sessionID string) {
	client, err := connectHub(fmt.Sprintf("/api/agent/terminal/%s", sessionID))
	if err != nil {
		log.Printf("Failed to connect terminal session %s: %v", sessionID, err)
		return
	}
	defer func(client *WSClient) {
		_ = client.Close()
	}(client)

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
		log.Printf("Failed to start pty for session %s: %v", sessionID, err)
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
				_ = client.SendMessage(websocket.TextMessage, buf[:n])
			}
		}
	}()

	// Handle websocket input -> pty
	go func() {
		for {
			_, msg, err := client.ReadMessage()
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
