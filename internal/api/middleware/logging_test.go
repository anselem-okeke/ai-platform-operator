package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestLogging(
	t *testing.T,
) {
	var output bytes.Buffer

	logger := slog.New(
		slog.NewJSONHandler(
			&output,
			nil,
		),
	)

	handler := Chain(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				w.WriteHeader(
					http.StatusCreated,
				)
			},
		),
		RequestID,
		RequestLogging(logger),
	)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/model-services",
		nil,
	)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		request,
	)

	logOutput := output.String()

	expectedValues := []string{
		`"msg":"http_request"`,
		`"method":"POST"`,
		`"path":"/api/v1/model-services"`,
		`"status":201`,
		`"request_id":`,
	}

	for _, expected := range expectedValues {
		if !strings.Contains(
			logOutput,
			expected,
		) {
			t.Fatalf(
				"expected log to contain %q, got %s",
				expected,
				logOutput,
			)
		}
	}
}
