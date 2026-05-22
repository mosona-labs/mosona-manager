//go:build windows

package shellsession

import (
	"io"
	"sync"
)

func Start() (*Session, error) {
	cmd := newCommand()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}

	outputReader, outputWriter := io.Pipe()
	var copyWG sync.WaitGroup
	copyWG.Add(2)

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = outputReader.Close()
		_ = outputWriter.Close()
		return nil, err
	}

	go func() {
		defer copyWG.Done()
		_, _ = io.Copy(outputWriter, stdout)
	}()
	go func() {
		defer copyWG.Done()
		_, _ = io.Copy(outputWriter, stderr)
	}()
	go func() {
		copyWG.Wait()
		_ = outputWriter.Close()
	}()

	return &Session{
		input:  stdin,
		output: outputReader,
		close: func() error {
			_ = stdin.Close()
			_ = outputReader.Close()
			_ = outputWriter.Close()
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			_ = cmd.Wait()
			return nil
		},
		resize: func(_, _ uint16) error {
			return nil
		},
	}, nil
}
