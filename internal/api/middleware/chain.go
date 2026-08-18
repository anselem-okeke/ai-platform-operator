package middleware

import (
	"net/http"
	"slices"
)

// Middleware wraps an HTTP handler.
type Middleware func(http.Handler) http.Handler

// Chain applies middleware in the order provided.
func Chain(
	handler http.Handler,
	middlewares ...Middleware,
) http.Handler {
	for _, middleware := range slices.Backward(middlewares) {
		handler = middleware(handler)
	}

	return handler
}
