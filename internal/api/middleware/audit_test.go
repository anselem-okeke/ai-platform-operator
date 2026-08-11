package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anselem-okeke/ai-platform-operator/internal/api/auth"
)

func TestAuditLoggingRecordsSuccessfulMutation(
	t *testing.T,
) {
	var output bytes.Buffer

	logger := slog.New(
		slog.NewJSONHandler(
			&output,
			nil,
		),
	)

	mux := http.NewServeMux()

	mux.HandleFunc(
		"/api/v1/model-services/{name}",
		func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			w.WriteHeader(
				http.StatusNoContent,
			)
		},
	)

	handler := AuditLogging(
		logger,
	)(
		mux,
	)

	request := httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/model-services/test-model",
		nil,
	)

	request = requestWithIdentity(
		request,
		auth.Identity{
			Subject:           "admin-123",
			PreferredUsername: "admin-user",
			Roles: []string{
				"platform-admin",
			},
		},
	)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		request,
	)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf(
			"expected %d, got %d",
			http.StatusNoContent,
			recorder.Code,
		)
	}

	logOutput := output.String()

	expectedValues := []string{
		`"msg":"api_audit"`,
		`"event":"api_audit"`,
		`"method":"DELETE"`,
		`"route":"/api/v1/model-services/{name}"`,
		`"resource_name":"test-model"`,
		`"status":204`,
		`"outcome":"success"`,
		`"subject":"admin-123"`,
		`"username":"admin-user"`,
		`"platform-admin"`,
	}

	for _, expected := range expectedValues {
		if !strings.Contains(
			logOutput,
			expected,
		) {
			t.Fatalf(
				"expected audit log to contain %q, got %s",
				expected,
				logOutput,
			)
		}
	}
}

func TestAuditLoggingRecordsDeniedMutation(
	t *testing.T,
) {
	var output bytes.Buffer

	logger := slog.New(
		slog.NewJSONHandler(
			&output,
			nil,
		),
	)

	handler := AuditLogging(
		logger,
	)(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				w.WriteHeader(
					http.StatusForbidden,
				)
			},
		),
	)

	request := httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/model-services/fraud-model",
		nil,
	)

	request = requestWithIdentity(
		request,
		auth.Identity{
			Subject:           "deployer-123",
			PreferredUsername: "deployer-user",
			Roles: []string{
				"model-deployer",
			},
		},
	)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		request,
	)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf(
			"expected %d, got %d",
			http.StatusForbidden,
			recorder.Code,
		)
	}

	logOutput := output.String()

	expectedValues := []string{
		`"msg":"api_audit"`,
		`"method":"DELETE"`,
		`"status":403`,
		`"outcome":"denied"`,
		`"subject":"deployer-123"`,
		`"username":"deployer-user"`,
	}

	for _, expected := range expectedValues {
		if !strings.Contains(
			logOutput,
			expected,
		) {
			t.Fatalf(
				"expected audit log to contain %q, got %s",
				expected,
				logOutput,
			)
		}
	}
}

func TestAuditLoggingSkipsReadRequests(
	t *testing.T,
) {
	var output bytes.Buffer

	logger := slog.New(
		slog.NewJSONHandler(
			&output,
			nil,
		),
	)

	handler := AuditLogging(
		logger,
	)(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				w.WriteHeader(
					http.StatusOK,
				)
			},
		),
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/model-services",
		nil,
	)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		request,
	)

	if strings.Contains(
		output.String(),
		`"msg":"api_audit"`,
	) {
		t.Fatalf(
			"GET request unexpectedly produced audit event: %s",
			output.String(),
		)
	}
}
