package msettings

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mosona-manager/internal/utils"

	"github.com/labstack/echo/v5"
)

func TestUploadFaviconRejectsOversizedRequestBeforeParsing(t *testing.T) {
	e := echo.New()
	Router(e.Group(""))
	req := httptest.NewRequest(http.MethodPost, "/favicon", strings.NewReader("x"))
	req.ContentLength = utils.MaxAvatarRequestBytes + 1
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}
