package ateam

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mosona-manager/internal/utils"

	"github.com/labstack/echo/v5"
)

func TestCreateTeamRejectsOversizedRequestBeforeParsing(t *testing.T) {
	e := echo.New()
	Router(e.Group(""))
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("x"))
	req.ContentLength = utils.MaxAvatarRequestBytes + 1
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestCreateTeamRejectsUnknownLengthOversizedForm(t *testing.T) {
	e := echo.New()
	Router(e.Group(""))
	body := "name=" + strings.Repeat("x", int(utils.MaxAvatarRequestBytes))
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.ContentLength = -1
	req.TransferEncoding = []string{"chunked"}
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}
