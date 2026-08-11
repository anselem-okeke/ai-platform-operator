package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "ai_platform",
			Subsystem: "api",
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests handled by the AI Platform API.",
		},
		[]string{
			"method",
			"route",
			"status",
		},
	)

	HTTPRequestDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "ai_platform",
			Subsystem: "api",
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds for the AI Platform API.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{
			"method",
			"route",
			"status",
		},
	)

	HTTPRequestsInFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "ai_platform",
			Subsystem: "api",
			Name:      "http_requests_in_flight",
			Help:      "Current number of HTTP requests being handled by the AI Platform API.",
		},
	)
)

func init() {
	prometheus.MustRegister(
		HTTPRequestsTotal,
		HTTPRequestDurationSeconds,
		HTTPRequestsInFlight,
	)
}
