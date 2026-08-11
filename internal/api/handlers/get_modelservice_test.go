package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	platformv1alpha1 "github.com/anselem-okeke/ai-platform-operator/api/v1alpha1"
)

type fakeModelServiceGetter struct {
	item *platformv1alpha1.ModelService
	err  error
}

func (f fakeModelServiceGetter) Get(
	context.Context,
	string,
) (*platformv1alpha1.ModelService, error) {
	return f.item, f.err
}

func TestGetModelService(t *testing.T) {
	store := fakeModelServiceGetter{
		item: &platformv1alpha1.ModelService{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "fraud-model",
				Generation: 3,
			},
			Spec: platformv1alpha1.ModelServiceSpec{
				Image:    "example/fraud:v1",
				Replicas: 2,
				Port:     8080,
			},
		},
	}

	handler := NewGetModelServiceHandler(
		testLogger(),
		store,
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/model-services/fraud-model",
		nil,
	)

	request.SetPathValue(
		"name",
		"fraud-model",
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

	expected := []string{
		`"apiVersion":"v1"`,
		`"kind":"ModelService"`,
		`"name":"fraud-model"`,
		`"image":"example/fraud:v1"`,
		`"replicas":2`,
		`"port":8080`,
		`"generation":3`,
	}

	for _, value := range expected {
		if !strings.Contains(body, value) {
			t.Fatalf(
				"expected body to contain %q, got %s",
				value,
				body,
			)
		}
	}
}

func TestGetModelServiceNotFound(
	t *testing.T,
) {
	notFound := apierrors.NewNotFound(
		schema.GroupResource{
			Group:    "platform.anselem.dev",
			Resource: "modelservices",
		},
		"missing-model",
	)

	handler := NewGetModelServiceHandler(
		testLogger(),
		fakeModelServiceGetter{
			err: notFound,
		},
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/model-services/missing-model",
		nil,
	)

	request.SetPathValue(
		"name",
		"missing-model",
	)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		request,
	)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusNotFound,
			recorder.Code,
		)
	}

	if !strings.Contains(
		recorder.Body.String(),
		`"code":"MODEL_SERVICE_NOT_FOUND"`,
	) {
		t.Fatalf(
			"unexpected response: %s",
			recorder.Body.String(),
		)
	}
}

func TestGetModelServiceKubernetesFailure(
	t *testing.T,
) {
	handler := NewGetModelServiceHandler(
		testLogger(),
		fakeModelServiceGetter{
			err: errors.New(
				"Kubernetes unavailable",
			),
		},
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/model-services/fraud-model",
		nil,
	)

	request.SetPathValue(
		"name",
		"fraud-model",
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
