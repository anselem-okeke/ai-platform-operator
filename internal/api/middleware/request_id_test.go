package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestIDGeneratesID(t *testing.T) {
	handler := RequestID(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				requestID := RequestIDFromContext(
					r.Context(),
				)

				if requestID == "" {
					t.Fatal(
						"request ID missing from context",
					)
				}

				w.WriteHeader(http.StatusOK)
			},
		),
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/healthz",
		nil,
	)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		request,
	)

	if recorder.Header().Get(
		requestIDHeader,
	) == "" {
		t.Fatal(
			"request ID missing from response",
		)
	}
}

func TestRequestIDPreservesClientID(
	t *testing.T,
) {
	const expected = "client-request-123"

	handler := RequestID(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				actual := RequestIDFromContext(
					r.Context(),
				)

				if actual != expected {
					t.Fatalf(
						"expected request ID %q, got %q",
						expected,
						actual,
					)
				}

				w.WriteHeader(http.StatusOK)
			},
		),
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/healthz",
		nil,
	)

	request.Header.Set(
		requestIDHeader,
		expected,
	)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		request,
	)

	if actual := recorder.Header().Get(
		requestIDHeader,
	); actual != expected {
		t.Fatalf(
			"expected response request ID %q, got %q",
			expected,
			actual,
		)
	}
}
