package middleware

import "net/http"

// Middleware wraps an HTTP handler.
type Middleware func(http.Handler) http.Handler

// Chain applies middleware in the order provided.
func Chain(
	handler http.Handler,
	middlewares ...Middleware,
) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}

	return handler
}
