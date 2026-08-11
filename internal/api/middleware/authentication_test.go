package middleware

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anselem-okeke/ai-platform-operator/internal/api/auth"
)

type fakeVerifier struct {
	identity auth.Identity
	err      error
}

func (f fakeVerifier) Verify(
	context.Context,
	string,
) (auth.Identity, error) {
	return f.identity, f.err
}

func authTestLogger() *slog.Logger {
	return slog.New(
		slog.NewJSONHandler(
			&bytes.Buffer{},
			nil,
		),
	)
}

func TestAuthenticationMissingToken(
	t *testing.T,
) {
	handler := Authentication(
		authTestLogger(),
		fakeVerifier{},
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

	if !strings.Contains(
		recorder.Body.String(),
		`"code":"UNAUTHORIZED"`,
	) {
		t.Fatalf(
			"unexpected response: %s",
			recorder.Body.String(),
		)
	}
}

func TestAuthenticationInvalidToken(
	t *testing.T,
) {
	handler := Authentication(
		authTestLogger(),
		fakeVerifier{
			err: errors.New(
				"invalid token",
			),
		},
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

	request.Header.Set(
		"Authorization",
		"Bearer invalid-token",
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

func TestAuthenticationValidToken(
	t *testing.T,
) {
	expected := auth.Identity{
		Subject:           "user-123",
		PreferredUsername: "viewer-user",
		Roles: []string{
			"model-viewer",
		},
	}

	handler := Authentication(
		authTestLogger(),
		fakeVerifier{
			identity: expected,
		},
	)(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				identity, ok :=
					IdentityFromContext(
						r.Context(),
					)

				if !ok {
					t.Fatal(
						"identity missing from context",
					)
				}

				if identity.Subject != expected.Subject {
					t.Fatalf(
						"expected subject %q, got %q",
						expected.Subject,
						identity.Subject,
					)
				}

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

	request.Header.Set(
		"Authorization",
		"Bearer valid-token",
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
