package gui

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterHandlersTelemetryDisabled(t *testing.T) {
	originalMux := http.DefaultServeMux
	http.DefaultServeMux = http.NewServeMux()
	t.Cleanup(func() { http.DefaultServeMux = originalMux })

	// Disabled routes must not access configuration or telemetry, even for
	// requests that would have been accepted by their former implementations.
	registerHandlers(nil, nil)

	routes := []struct {
		path string
		body string
	}{
		{"/api/telemetry/push", "telemetry import is unavailable from the dashboard\n"},
		{"/api/telemetry/pull", "telemetry export is unavailable from the dashboard\n"},
		{"/api/telemetry/pull?since_id=0", "telemetry export is unavailable from the dashboard\n"},
		{"/api/telemetry/pull?since_id=invalid", "telemetry export is unavailable from the dashboard\n"},
		{"/api/telemetry/sync", "telemetry sync is unavailable from the dashboard\n"},
	}
	methods := []string{
		http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodHead, http.MethodOptions,
	}
	for _, route := range routes {
		for _, method := range methods {
			t.Run(method+" "+route.path, func(t *testing.T) {
				req := httptest.NewRequest(method, route.path, nil)
				req.Body = unreadTelemetryBody{t: t}
				recorder := httptest.NewRecorder()
				http.DefaultServeMux.ServeHTTP(recorder, req)

				if recorder.Code != http.StatusForbidden {
					t.Errorf("status = %d, want %d", recorder.Code, http.StatusForbidden)
				}
				if got := recorder.Body.String(); got != route.body {
					t.Errorf("body = %q, want %q", got, route.body)
				}
				if got := recorder.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
					t.Errorf("Content-Type = %q, want text/plain; charset=utf-8", got)
				}
				if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
					t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
				}
			})
		}
	}
}

type unreadTelemetryBody struct {
	t *testing.T
}

func (body unreadTelemetryBody) Read([]byte) (int, error) {
	body.t.Fatal("disabled telemetry route read the request body")
	return 0, nil
}

func (unreadTelemetryBody) Close() error { return nil }
