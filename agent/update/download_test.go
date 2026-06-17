package update

import (
	"errors"
	"testing"
)

func TestIsDownloadError(t *testing.T) {
	if !IsDownloadError(&DownloadError{Err: errors.New("x")}) {
		t.Fatal("expected download error")
	}
	if IsDownloadError(errors.New("restart service: x")) {
		t.Fatal("restart should not be download error")
	}
}
