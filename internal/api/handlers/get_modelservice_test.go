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
		`"backend":"Deployment"`,
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

func TestModelServiceToResponseKServe(
	t *testing.T,
) {
	modelService := platformv1alpha1.ModelService{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "iris-api",
			Generation: 4,
		},
		Spec: platformv1alpha1.ModelServiceSpec{
			Backend: platformv1alpha1.
				ModelServiceBackendKServe,
			Image:    "ignored-deployment-image",
			Replicas: 1,
			Port:     8080,
			Predictor: &platformv1alpha1.
				ModelServicePredictor{
				ModelFormat: platformv1alpha1.
					ModelServiceModelFormat{
					Name:    "sklearn",
					Version: "1",
				},
				Runtime: "kserve-sklearnserver",
				StorageURI: "s3://models/sklearn/" +
					"iris/v1",
				ServiceAccountName: "kserve-model-reader",
			},
		},
	}

	result := modelServiceToResponse(modelService)

	if result.Backend != "KServe" {
		t.Fatalf(
			"expected backend KServe, got %q",
			result.Backend,
		)
	}

	if result.Image != "" {
		t.Fatalf(
			"expected KServe image to be hidden, got %q",
			result.Image,
		)
	}

	if result.Port != 0 {
		t.Fatalf(
			"expected KServe port to be hidden, got %d",
			result.Port,
		)
	}

	if result.Predictor == nil {
		t.Fatal(
			"expected predictor in response",
		)
	}

	if result.Predictor.ModelFormat.Name !=
		"sklearn" {
		t.Fatalf(
			"expected sklearn model format, got %q",
			result.Predictor.ModelFormat.Name,
		)
	}

	if result.Predictor.ModelFormat.Version != "1" {
		t.Fatalf(
			"expected model version 1, got %q",
			result.Predictor.ModelFormat.Version,
		)
	}

	if result.Predictor.Runtime !=
		"kserve-sklearnserver" {
		t.Fatalf(
			"unexpected runtime %q",
			result.Predictor.Runtime,
		)
	}

	if result.Predictor.StorageURI !=
		"s3://models/sklearn/iris/v1" {
		t.Fatalf(
			"unexpected storage URI %q",
			result.Predictor.StorageURI,
		)
	}

	if result.Predictor.ServiceAccountName !=
		"kserve-model-reader" {
		t.Fatalf(
			"unexpected service account %q",
			result.Predictor.ServiceAccountName,
		)
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
