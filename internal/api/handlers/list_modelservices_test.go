package handlers

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	platformv1alpha1 "github.com/anselem-okeke/ai-platform-operator/api/v1alpha1"
)

type fakeModelServiceLister struct {
	items []platformv1alpha1.ModelService
	err   error
}

func (f fakeModelServiceLister) List(
	context.Context,
) ([]platformv1alpha1.ModelService, error) {
	return f.items, f.err
}

func testLogger() *slog.Logger {
	return slog.New(
		slog.NewJSONHandler(
			&bytes.Buffer{},
			nil,
		),
	)
}

func TestListModelServices(t *testing.T) {
	store := fakeModelServiceLister{
		items: []platformv1alpha1.ModelService{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "fraud-model",
				},
				Spec: platformv1alpha1.ModelServiceSpec{
					Image:    "example/fraud:v1",
					Replicas: 2,
				},
			},
		},
	}

	handler := NewListModelServicesHandler(
		testLogger(),
		store,
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

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusOK,
			recorder.Code,
		)
	}

	body := recorder.Body.String()

	expectedValues := []string{
		`"name":"fraud-model"`,
		`"image":"example/fraud:v1"`,
		`"replicas":2`,
		`"count":1`,
	}

	for _, expected := range expectedValues {
		if !strings.Contains(body, expected) {
			t.Fatalf(
				"expected body to contain %q, got %s",
				expected,
				body,
			)
		}
	}
}

func TestListModelServicesFailure(
	t *testing.T,
) {
	store := fakeModelServiceLister{
		err: errors.New(
			"Kubernetes unavailable",
		),
	}

	handler := NewListModelServicesHandler(
		testLogger(),
		store,
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

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusServiceUnavailable,
			recorder.Code,
		)
	}

	if !strings.Contains(
		recorder.Body.String(),
		`"code":"KUBERNETES_UNAVAILABLE"`,
	) {
		t.Fatalf(
			"unexpected response: %s",
			recorder.Body.String(),
		)
	}
}
