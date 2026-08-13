package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mosona-manager/internal/utils"

	"github.com/labstack/echo/v5"
)

func TestAvatarUploadLimitRejectsOversizedRequest(t *testing.T) {
	e := echo.New()
	e.POST("/upload", func(c *echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	}, AvatarUploadLimit)

	req := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader("x"))
	req.ContentLength = utils.MaxAvatarRequestBytes + 1
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestAvatarUploadLimitRejectsUnknownLengthBody(t *testing.T) {
	e := echo.New()
	e.POST("/upload", func(c *echo.Context) error {
		_, err := io.Copy(io.Discard, c.Request().Body)
		return err
	}, AvatarUploadLimit)

	req := httptest.NewRequest(http.MethodPost, "/upload", io.LimitReader(zeroReader{}, utils.MaxAvatarRequestBytes+1))
	req.ContentLength = -1
	req.TransferEncoding = []string{"chunked"}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}
