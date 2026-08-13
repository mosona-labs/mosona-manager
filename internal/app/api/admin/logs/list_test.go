package mlogs

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestListRejectsInvalidLogFilter(t *testing.T) {
	e := echo.New()
	request := httptest.NewRequest(http.MethodGet, "/?level=critical", nil)
	recorder := httptest.NewRecorder()
	c := e.NewContext(request, recorder)

	if err := list(c); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"invalid"`) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestListRejectsExcessivePage(t *testing.T) {
	e := echo.New()
	request := httptest.NewRequest(http.MethodGet, "/?page=100001", nil)
	recorder := httptest.NewRecorder()
	c := e.NewContext(request, recorder)

	if err := list(c); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"invalid"`) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}
