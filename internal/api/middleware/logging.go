package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *responseRecorder) WriteHeader(
	statusCode int,
) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *responseRecorder) Write(
	body []byte,
) (int, error) {
	if r.statusCode == 0 {
		r.WriteHeader(http.StatusOK)
	}

	return r.ResponseWriter.Write(body)
}

func RequestLogging(
	logger *slog.Logger,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
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

				logger.InfoContext(
					r.Context(),
					"http_request",
					slog.String(
						"request_id",
						RequestIDFromContext(r.Context()),
					),
					slog.String(
						"method",
						r.Method,
					),
					slog.String(
						"path",
						r.URL.Path,
					),
					slog.Int(
						"status",
						statusCode,
					),
					slog.Int64(
						"duration_ms",
						time.Since(start).Milliseconds(),
					),
					slog.String(
						"remote_addr",
						r.RemoteAddr,
					),
					slog.String(
						"user_agent",
						r.UserAgent(),
					),
				)
			},
		)
	}
}
