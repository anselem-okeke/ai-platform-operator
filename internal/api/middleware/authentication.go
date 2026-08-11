package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/anselem-okeke/ai-platform-operator/internal/api/auth"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/response"
)

type identityContextKey struct{}

func Authentication(
	logger *slog.Logger,
	verifier auth.Verifier,
) func(http.Handler) http.Handler {
	return func(
		next http.Handler,
	) http.Handler {
		return http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				rawToken, ok :=
					bearerToken(
						r.Header.Get(
							"Authorization",
						),
					)

				if !ok {
					writeUnauthorized(
						w,
						r,
						"Bearer access token is required",
					)
					return
				}

				identity, err :=
					verifier.Verify(
						r.Context(),
						rawToken,
					)
				if err != nil {
					logger.WarnContext(
						r.Context(),
						"jwt_authentication_failed",
						slog.String(
							"request_id",
							RequestIDFromContext(
								r.Context(),
							),
						),
						slog.String(
							"error",
							err.Error(),
						),
					)

					writeUnauthorized(
						w,
						r,
						"access token is invalid",
					)

					return
				}

				ctx := context.WithValue(
					r.Context(),
					identityContextKey{},
					identity,
				)

				next.ServeHTTP(
					w,
					r.WithContext(ctx),
				)
			},
		)
	}
}

func IdentityFromContext(
	ctx context.Context,
) (auth.Identity, bool) {
	identity, ok := ctx.Value(
		identityContextKey{},
	).(auth.Identity)

	return identity, ok
}

func bearerToken(
	authorization string,
) (string, bool) {
	parts := strings.Fields(
		authorization,
	)

	if len(parts) != 2 {
		return "", false
	}

	if !strings.EqualFold(
		parts[0],
		"Bearer",
	) {
		return "", false
	}

	if parts[1] == "" {
		return "", false
	}

	return parts[1], true
}

func writeUnauthorized(
	w http.ResponseWriter,
	r *http.Request,
	message string,
) {
	w.Header().Set(
		"WWW-Authenticate",
		"Bearer",
	)

	response.WriteJSON(
		w,
		http.StatusUnauthorized,
		response.APIError{
			Error: response.ErrorBody{
				Code:      "UNAUTHORIZED",
				Message:   message,
				RequestID: RequestIDFromContext(r.Context()),
			},
		},
	)
}
