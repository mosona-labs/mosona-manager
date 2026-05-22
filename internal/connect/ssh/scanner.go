package ssh

import (
	"bufio"
	"io"
)

const maxSSHLineSize = 1024 * 1024

func newLineScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxSSHLineSize)
	return scanner
}
