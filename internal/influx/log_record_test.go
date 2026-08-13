package influx

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mosona-manager/internal/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
)

func TestGetLogsByPageUsesFluxParameters(t *testing.T) {
	type queryRequest struct {
		Query  string                 `json:"query"`
		Params map[string]interface{} `json:"params"`
	}

	requests := make([]queryRequest, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var payload queryRequest
		if err = json.Unmarshal(body, &payload); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests = append(requests, payload)
		w.Header().Set("Content-Type", "text/csv")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	previousClient := Client
	previousOrg := config.Conf.InfluxDBOrg
	testClient := influxdb2.NewClient(server.URL, "test-token")
	Client = testClient
	config.Conf.InfluxDBOrg = "test-org"
	t.Cleanup(func() {
		testClient.Close()
		Client = previousClient
		config.Conf.InfluxDBOrg = previousOrg
	})

	message := `") |> yield(name: "injected") //`
	_, _, err := GetLogsByPage(context.Background(), 12, 1, 20, "server", "high", []int64{7, 8}, message)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 {
		t.Fatalf("query request count = %d, want 2", len(requests))
	}
	for _, request := range requests {
		if strings.Contains(request.Query, message) || strings.Contains(request.Query, `== "server"`) || strings.Contains(request.Query, `== "high"`) {
			t.Fatalf("user-controlled value was embedded in Flux source: %s", request.Query)
		}
		for _, fragment := range []string{
			`range(start: -365d)`,
			`r["team_id"] == params.teamID`,
			`r["category"] == params.category`,
			`r["level"] == params.level`,
			`substr: params.message`,
			`r["user_id"] == params.userID0`,
		} {
			if !strings.Contains(request.Query, fragment) {
				t.Fatalf("Flux query missing %q: %s", fragment, request.Query)
			}
		}
		if request.Params["teamID"] != "12" || request.Params["category"] != "server" || request.Params["level"] != "high" || request.Params["message"] != message {
			t.Fatalf("unexpected query params: %#v", request.Params)
		}
	}
}

func TestValidateLogFilters(t *testing.T) {
	if err := ValidateLogFilters("security", "high", strings.Repeat("x", maxLogMessageFilterLength)); err != nil {
		t.Fatalf("valid filters rejected: %v", err)
	}
	for _, test := range []struct {
		name     string
		category string
		level    string
		message  string
	}{
		{name: "category", category: `server") or true`},
		{name: "level", level: "critical"},
		{name: "message length", message: strings.Repeat("x", maxLogMessageFilterLength+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateLogFilters(test.category, test.level, test.message); err == nil {
				t.Fatal("invalid filters were accepted")
			}
		})
	}
}

func TestGetLogsByPageRejectsUnsafePagination(t *testing.T) {
	for _, test := range []struct {
		name     string
		page     int
		pageSize int
	}{
		{name: "page", page: maxLogPage + 1, pageSize: 20},
		{name: "page size", page: 1, pageSize: maxLogPageSize + 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := GetLogsByPage(context.Background(), 1, test.page, test.pageSize, "", "", nil, ""); !errors.Is(err, ErrInvalidLogFilter) {
				t.Fatalf("error = %v, want ErrInvalidLogFilter", err)
			}
		})
	}
}
