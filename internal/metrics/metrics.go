package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry wraps exported metrics.
type Registry struct {
	Requests    *prometheus.CounterVec
	Diagnostics *prometheus.CounterVec
	Restarts    *prometheus.CounterVec
	OpenDocs    *prometheus.GaugeVec
}

// New creates the metrics registry.
func New() *Registry {
	return &Registry{
		Requests:    promauto.NewCounterVec(prometheus.CounterOpts{Name: "lspd_requests_total", Help: "MCP and socket requests"}, []string{"surface", "method"}),
		Diagnostics: promauto.NewCounterVec(prometheus.CounterOpts{Name: "lspd_diagnostics_total", Help: "Published diagnostics"}, []string{"language"}),
		Restarts:    promauto.NewCounterVec(prometheus.CounterOpts{Name: "lspd_restarts_total", Help: "Language server restarts"}, []string{"language"}),
		OpenDocs:    promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "lspd_open_documents", Help: "Tracked open documents"}, []string{"language"}),
	}
}

// Handler returns the Prometheus HTTP handler.
func (r *Registry) Handler() http.Handler { return promhttp.Handler() }
