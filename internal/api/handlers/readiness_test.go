package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeReadinessChecker struct {
	err error
}

func (c fakeReadinessChecker) Check(
	context.Context,
) error {
	return c.err
}

func TestReadinessReady(t *testing.T) {
	handler := NewReadinessHandler(
		fakeReadinessChecker{},
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/readyz",
		nil,
	)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		request,
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusOK,
			recorder.Code,
		)
	}

	if !strings.Contains(
		recorder.Body.String(),
		`"status":"ready"`,
	) {
		t.Fatalf(
			"unexpected response body: %s",
			recorder.Body.String(),
		)
	}
}

func TestReadinessNotReady(t *testing.T) {
	handler := NewReadinessHandler(
		fakeReadinessChecker{
			err: errors.New(
				"Kubernetes unavailable",
			),
		},
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/readyz",
		nil,
	)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		request,
	)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusServiceUnavailable,
			recorder.Code,
		)
	}

	if !strings.Contains(
		recorder.Body.String(),
		`"status":"not-ready"`,
	) {
		t.Fatalf(
			"unexpected response body: %s",
			recorder.Body.String(),
		)
	}
}

func TestReadinessWithoutChecker(
	t *testing.T,
) {
	handler := NewReadinessHandler(nil)

	request := httptest.NewRequest(
		http.MethodGet,
		"/readyz",
		nil,
	)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		request,
	)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusServiceUnavailable,
			recorder.Code,
		)
	}
}

func TestReadinessRejectsNonGET(
	t *testing.T,
) {
	handler := NewReadinessHandler(
		fakeReadinessChecker{},
	)

	request := httptest.NewRequest(
		http.MethodPost,
		"/readyz",
		nil,
	)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		request,
	)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusMethodNotAllowed,
			recorder.Code,
		)
	}
}
