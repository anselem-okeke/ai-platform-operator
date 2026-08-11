package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	apimetrics "github.com/anselem-okeke/ai-platform-operator/internal/api/metrics"
)

func RequestMetrics(
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(
		func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			// Do not include Prometheus scrape traffic in the API
			// request metrics themselves.
			if r.URL.Path == "/metrics" {
				next.ServeHTTP(w, r)
				return
			}

			apimetrics.HTTPRequestsInFlight.Inc()
			defer apimetrics.HTTPRequestsInFlight.Dec()

			start := time.Now()

			recorder := &responseRecorder{
				ResponseWriter: w,
			}

			next.ServeHTTP(
				recorder,
				r,
			)

			statusCode := recorder.statusCode
			if statusCode == 0 {
				statusCode = http.StatusOK
			}

			route := metricRoute(r)

			status := strconv.Itoa(statusCode)

			apimetrics.HTTPRequestsTotal.
				WithLabelValues(
					r.Method,
					route,
					status,
				).
				Inc()

			apimetrics.HTTPRequestDurationSeconds.
				WithLabelValues(
					r.Method,
					route,
					status,
				).
				Observe(
					time.Since(start).Seconds(),
				)
		},
	)
}

func metricRoute(
	r *http.Request,
) string {
	switch {
	case r.URL.Path == "/healthz":
		return "/healthz"

	case r.URL.Path == "/readyz":
		return "/readyz"

	case r.URL.Path == "/metrics":
		return "/metrics"

	case r.URL.Path == "/api/v1/model-services":
		return "/api/v1/model-services"

	case strings.HasPrefix(
		r.URL.Path,
		"/api/v1/model-services/",
	):
		if strings.HasSuffix(
			r.URL.Path,
			"/status",
		) {
			return "/api/v1/model-services/{name}/status"
		}

		return "/api/v1/model-services/{name}"

	default:
		return "unmatched"
	}
}
