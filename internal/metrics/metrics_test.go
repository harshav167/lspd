package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesPrometheusMetrics(t *testing.T) {
	t.Parallel()
	registry := New()
	registry.Requests.WithLabelValues("mcp", "getIdeDiagnostics").Inc()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/metrics", nil)
	registry.Handler().ServeHTTP(recorder, request)
	if recorder.Code != 200 {
		t.Fatalf("unexpected status %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "lspd_requests_total") {
		t.Fatalf("missing expected metric output: %s", recorder.Body.String())
	}
}

func TestRegistriesAreIsolated(t *testing.T) {
	t.Parallel()
	first := New()
	second := New()

	first.RecordRequest("socket", "status")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/metrics", nil)
	second.Handler().ServeHTTP(recorder, request)

	if recorder.Code != 200 {
		t.Fatalf("unexpected status %d", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), `surface="socket",method="status"`) {
		t.Fatalf("expected second registry to be isolated from first: %s", recorder.Body.String())
	}
}
