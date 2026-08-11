package middleware

import (
	"log/slog"
	"net/http"

	"github.com/anselem-okeke/ai-platform-operator/internal/api/response"
)

func RequireAnyRole(
	logger *slog.Logger,
	roles ...string,
) func(http.Handler) http.Handler {
	return func(
		next http.Handler,
	) http.Handler {
		return http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				identity, ok :=
					IdentityFromContext(
						r.Context(),
					)

				if !ok {
					logger.ErrorContext(
						r.Context(),
						"authorization_identity_missing",
						slog.String(
							"request_id",
							RequestIDFromContext(
								r.Context(),
							),
						),
					)

					response.WriteJSON(
						w,
						http.StatusUnauthorized,
						response.APIError{
							Error: response.ErrorBody{
								Code:      "UNAUTHORIZED",
								Message:   "authenticated identity is missing",
								RequestID: RequestIDFromContext(r.Context()),
							},
						},
					)

					return
				}

				for _, role := range roles {
					if identity.HasRole(role) {
						next.ServeHTTP(
							w,
							r,
						)
						return
					}
				}

				logger.WarnContext(
					r.Context(),
					"authorization_denied",
					slog.String(
						"request_id",
						RequestIDFromContext(
							r.Context(),
						),
					),
					slog.String(
						"subject",
						identity.Subject,
					),
					slog.String(
						"username",
						identity.PreferredUsername,
					),
					slog.Any(
						"required_roles",
						roles,
					),
				)

				response.WriteJSON(
					w,
					http.StatusForbidden,
					response.APIError{
						Error: response.ErrorBody{
							Code:      "FORBIDDEN",
							Message:   "insufficient role for this operation",
							RequestID: RequestIDFromContext(r.Context()),
						},
					},
				)
			},
		)
	}
}
