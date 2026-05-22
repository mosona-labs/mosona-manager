//go:build !windows

package shellsession

import (
	"github.com/creack/pty"
)

func Start() (*Session, error) {
	cmd := newCommand()
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}

	return &Session{
		input:  ptmx,
		output: ptmx,
		close: func() error {
			_ = ptmx.Close()
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			_ = cmd.Wait()
			return nil
		},
		resize: func(rows, cols uint16) error {
			return pty.Setsize(ptmx, &pty.Winsize{
				Rows: rows,
				Cols: cols,
			})
		},
	}, nil
}
