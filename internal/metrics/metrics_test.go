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
