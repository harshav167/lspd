package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry wraps exported metrics.
type Registry struct {
	registry    *prometheus.Registry
	Requests    *prometheus.CounterVec
	Diagnostics *prometheus.CounterVec
	Restarts    *prometheus.CounterVec
	OpenDocs    *prometheus.GaugeVec
}

// New creates the metrics registry.
func New() *Registry {
	registry := prometheus.NewRegistry()
	factory := promauto.With(registry)
	return &Registry{
		registry:    registry,
		Requests:    factory.NewCounterVec(prometheus.CounterOpts{Name: "lspd_requests_total", Help: "MCP and socket requests"}, []string{"surface", "method"}),
		Diagnostics: factory.NewCounterVec(prometheus.CounterOpts{Name: "lspd_diagnostics_total", Help: "Published diagnostics"}, []string{"language"}),
		Restarts:    factory.NewCounterVec(prometheus.CounterOpts{Name: "lspd_restarts_total", Help: "Language server restarts"}, []string{"language"}),
		OpenDocs:    factory.NewGaugeVec(prometheus.GaugeOpts{Name: "lspd_open_documents", Help: "Tracked open documents"}, []string{"language"}),
	}
}

// Handler returns the Prometheus HTTP handler.
func (r *Registry) Handler() http.Handler {
	if r == nil || r.registry == nil {
		return promhttp.Handler()
	}
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{})
}

// RecordRequest increments a request counter.
func (r *Registry) RecordRequest(surface, method string) {
	if r == nil {
		return
	}
	r.Requests.WithLabelValues(surface, method).Inc()
}

// RecordDiagnostics records a published diagnostic count.
func (r *Registry) RecordDiagnostics(language string, count int) {
	if r == nil || count <= 0 {
		return
	}
	r.Diagnostics.WithLabelValues(language).Add(float64(count))
}

// RecordRestart increments the restart counter for a language.
func (r *Registry) RecordRestart(language string) {
	if r == nil {
		return
	}
	r.Restarts.WithLabelValues(language).Inc()
}

// SetOpenDocuments sets the tracked open-document gauge.
func (r *Registry) SetOpenDocuments(language string, count int) {
	if r == nil {
		return
	}
	r.OpenDocs.WithLabelValues(language).Set(float64(count))
}
