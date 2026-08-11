package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anselem-okeke/ai-platform-operator/internal/api/auth"
)

func requestWithIdentity(
	request *http.Request,
	identity auth.Identity,
) *http.Request {
	ctx := context.WithValue(
		request.Context(),
		identityContextKey{},
		identity,
	)

	return request.WithContext(ctx)
}

func TestRequireAnyRoleAllowsViewer(
	t *testing.T,
) {
	handler := RequireAnyRole(
		authTestLogger(),
		"model-viewer",
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

	request = requestWithIdentity(
		request,
		auth.Identity{
			Subject:           "viewer-1",
			PreferredUsername: "viewer-user",
			Roles: []string{
				"model-viewer",
			},
		},
	)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		request,
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected %d, got %d",
			http.StatusOK,
			recorder.Code,
		)
	}
}

func TestRequireAnyRoleRejectsMissingRole(
	t *testing.T,
) {
	handler := RequireAnyRole(
		authTestLogger(),
		"model-deployer",
	)(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				t.Fatal(
					"protected handler must not be called",
				)
			},
		),
	)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/model-services",
		nil,
	)

	request = requestWithIdentity(
		request,
		auth.Identity{
			Subject:           "viewer-1",
			PreferredUsername: "viewer-user",
			Roles: []string{
				"model-viewer",
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

	if !strings.Contains(
		recorder.Body.String(),
		`"code":"FORBIDDEN"`,
	) {
		t.Fatalf(
			"unexpected response: %s",
			recorder.Body.String(),
		)
	}
}

func TestRequireAnyRoleRejectsMissingIdentity(
	t *testing.T,
) {
	handler := RequireAnyRole(
		authTestLogger(),
		"model-viewer",
	)(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				t.Fatal(
					"protected handler must not be called",
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

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(
			"expected %d, got %d",
			http.StatusUnauthorized,
			recorder.Code,
		)
	}
}
