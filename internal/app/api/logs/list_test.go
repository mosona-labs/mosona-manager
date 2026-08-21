package alogs

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
)

func TestListRejectsInvalidLogFilter(t *testing.T) {
	e := echo.New()
	request := httptest.NewRequest(http.MethodGet, "/?category=server%22%29+or+true", nil)
	recorder := httptest.NewRecorder()
	c := e.NewContext(request, recorder)
	c.Set("tid", int64(1))

	if err := list(c); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"invalid"`) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestListRejectsLegacyOffsetPage(t *testing.T) {
	e := echo.New()
	request := httptest.NewRequest(http.MethodGet, "/?page=2", nil)
	recorder := httptest.NewRecorder()
	c := e.NewContext(request, recorder)
	c.Set("tid", int64(1))

	if err := list(c); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"msg":"Offset pagination is no longer supported"`) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestListRejectsCursorOutsideTimeRange(t *testing.T) {
	start := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	values := url.Values{
		"start":  {start.Format(time.RFC3339)},
		"end":    {end.Format(time.RFC3339)},
		"cursor": {base64.RawURLEncoding.EncodeToString([]byte(start.Add(-time.Nanosecond).Format(time.RFC3339Nano)))},
	}

	e := echo.New()
	request := httptest.NewRequest(http.MethodGet, "/?"+values.Encode(), nil)
	recorder := httptest.NewRecorder()
	c := e.NewContext(request, recorder)
	c.Set("tid", int64(1))

	if err := list(c); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"msg":"Invalid log query"`) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestListReturnsEmptyPageForCursorAtStart(t *testing.T) {
	start := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	values := url.Values{
		"start":  {start.Format(time.RFC3339)},
		"end":    {end.Format(time.RFC3339)},
		"cursor": {base64.RawURLEncoding.EncodeToString([]byte(start.Format(time.RFC3339Nano)))},
	}

	e := echo.New()
	request := httptest.NewRequest(http.MethodGet, "/?"+values.Encode(), nil)
	recorder := httptest.NewRecorder()
	c := e.NewContext(request, recorder)
	c.Set("tid", int64(1))

	if err := list(c); err != nil {
		t.Fatal(err)
	}
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, `"logs":[]`) ||
		!strings.Contains(body, `"next_cursor":""`) || !strings.Contains(body, `"has_more":false`) {
		t.Fatalf("status = %d, body = %s", recorder.Code, body)
	}
}
