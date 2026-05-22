package shellsession

import (
	"io"
	"os"
	"os/exec"
	"os/user"
	"runtime"
)

type Session struct {
	input  io.WriteCloser
	output io.ReadCloser
	close  func() error
	resize func(rows, cols uint16) error
}

func (s *Session) Read(p []byte) (int, error) {
	return s.output.Read(p)
}

func (s *Session) Write(p []byte) (int, error) {
	return s.input.Write(p)
}

func (s *Session) Close() error {
	if s.close != nil {
		return s.close()
	}
	return nil
}

func (s *Session) Resize(rows, cols uint16) error {
	if s.resize != nil {
		return s.resize(rows, cols)
	}
	return nil
}

func shellCommand() (string, []string) {
	switch runtime.GOOS {
	case "windows":
		if path, err := exec.LookPath("powershell.exe"); err == nil {
			return path, []string{"-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass"}
		}
		return "cmd.exe", nil
	case "darwin", "linux":
		for _, sh := range []string{"bash", "zsh", "ksh", "ash", "dash"} {
			path, err := exec.LookPath(sh)
			if err == nil {
				return path, nil
			}
		}
		return "/bin/sh", nil
	default:
		return "/bin/sh", nil
	}
}

func newCommand() *exec.Cmd {
	shell, args := shellCommand()
	cmd := exec.Command(shell, args...)

	u, err := user.Current()
	if err != nil {
		return cmd
	}

	switch runtime.GOOS {
	case "windows":
		if u.HomeDir != "" {
			cmd.Dir = u.HomeDir
		}
		cmd.Env = append(os.Environ(), []string{
			"USERPROFILE=" + u.HomeDir,
			"USERNAME=" + u.Username,
			"TERM=xterm-256color",
		}...)
	case "darwin", "linux":
		if u.HomeDir != "" {
			cmd.Dir = u.HomeDir
		}
		cmd.Env = append(os.Environ(), []string{
			"HOME=" + u.HomeDir,
			"USER=" + u.Name,
			"LOGNAME=" + u.Name,
			"SHELL=" + shell,
			"TERM=xterm-256color",
		}...)
	}

	return cmd
}
