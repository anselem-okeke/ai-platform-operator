package middleware

import (
	"log/slog"
	"net/http"
	"strings"
)

func AuditLogging(
	logger *slog.Logger,
) func(http.Handler) http.Handler {
	return func(
		next http.Handler,
	) http.Handler {
		return http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				if !isAuditedMutation(r.Method) {
					next.ServeHTTP(w, r)
					return
				}

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

				identity, authenticated :=
					IdentityFromContext(
						r.Context(),
					)

				attributes := []any{
					slog.String(
						"event",
						"api_audit",
					),
					slog.String(
						"request_id",
						RequestIDFromContext(
							r.Context(),
						),
					),
					slog.String(
						"method",
						r.Method,
					),
					slog.String(
						"route",
						metricRoute(r),
					),
					slog.String(
						"resource_type",
						"ModelService",
					),
					slog.String(
						"resource_name",
						auditResourceName(r),
					),
					slog.Int(
						"status",
						statusCode,
					),
					slog.String(
						"outcome",
						auditOutcome(statusCode),
					),
				}

				if authenticated {
					attributes = append(
						attributes,
						slog.String(
							"subject",
							identity.Subject,
						),
						slog.String(
							"username",
							identity.PreferredUsername,
						),
						slog.Any(
							"roles",
							identity.Roles,
						),
					)
				}

				logger.InfoContext(
					r.Context(),
					"api_audit",
					attributes...,
				)
			},
		)
	}
}

func isAuditedMutation(
	method string,
) bool {
	switch method {
	case http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete:
		return true

	default:
		return false
	}
}

func auditResourceName(
	r *http.Request,
) string {
	name := strings.TrimSpace(
		r.PathValue("name"),
	)

	if name == "" {
		return ""
	}

	return name
}

func auditOutcome(
	statusCode int,
) string {
	switch {
	case statusCode >= 200 &&
		statusCode < 300:
		return "success"

	case statusCode == http.StatusUnauthorized:
		return "unauthorized"

	case statusCode == http.StatusForbidden:
		return "denied"

	case statusCode >= 400 &&
		statusCode < 500:
		return "rejected"

	default:
		return "error"
	}
}
