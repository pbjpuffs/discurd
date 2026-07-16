package obs

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds every Discurd collector (docs/ARCHITECTURE.md §11) plus its
// registry. Collectors unused by a service simply never emit samples.
type Metrics struct {
	Registry *prometheus.Registry

	HTTPRequests    *prometheus.CounterVec // {service,method,route,status}
	HTTPDuration    *prometheus.HistogramVec
	WSConnections   prometheus.Gauge       // pre-curried with {service}
	WSDispatched    *prometheus.CounterVec // {type}, pre-curried with {service}
	MessagesCreated prometheus.Counter
}

// NewMetrics registers all Discurd collectors and the default Go runtime
// collectors on a fresh registry.
func NewMetrics(service string) *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	m := &Metrics{Registry: reg}

	m.HTTPRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "discurd_http_requests_total",
		Help: "HTTP requests handled, by route and status.",
	}, []string{"service", "method", "route", "status"})

	m.HTTPDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "discurd_http_request_duration_seconds",
		Help:    "HTTP request latency in seconds, by route.",
		Buckets: prometheus.DefBuckets,
	}, []string{"service", "route"})

	wsConns := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "discurd_ws_connections",
		Help: "Currently open WebSocket connections.",
	}, []string{"service"})
	m.WSConnections = wsConns.WithLabelValues(service)

	wsDispatched := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "discurd_ws_events_dispatched_total",
		Help: "Events dispatched to WebSocket sessions, by event type.",
	}, []string{"service", "type"})
	m.WSDispatched = wsDispatched.MustCurryWith(prometheus.Labels{"service": service})

	m.MessagesCreated = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "discurd_messages_created_total",
		Help: "Messages created via the REST API.",
	})

	reg.MustRegister(m.HTTPRequests, m.HTTPDuration, wsConns, wsDispatched, m.MessagesCreated)
	return m
}

// Handler serves the registry in Prometheus exposition format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
}
