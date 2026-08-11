package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealth(t *testing.T) {
	request := httptest.NewRequest(
		http.MethodGet,
		"/healthz",
		nil,
	)

	recorder := httptest.NewRecorder()

	Health(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusOK,
			recorder.Code,
		)
	}

	if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf(
			"expected application/json, got %q",
			contentType,
		)
	}

	if !strings.Contains(
		recorder.Body.String(),
		`"status":"ok"`,
	) {
		t.Fatalf(
			"unexpected response body: %s",
			recorder.Body.String(),
		)
	}
}

func TestHealthRejectsNonGET(t *testing.T) {
	request := httptest.NewRequest(
		http.MethodPost,
		"/healthz",
		nil,
	)

	recorder := httptest.NewRecorder()

	Health(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusMethodNotAllowed,
			recorder.Code,
		)
	}
}
