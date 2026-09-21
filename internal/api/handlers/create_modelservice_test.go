package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	platformv1alpha1 "github.com/anselem-okeke/ai-platform-operator/api/v1alpha1"
	apirequest "github.com/anselem-okeke/ai-platform-operator/internal/api/request"
)

type fakeModelServiceCreator struct {
	createErr error
	created   *platformv1alpha1.ModelService
}

func (f *fakeModelServiceCreator) Create(
	_ context.Context,
	modelService *platformv1alpha1.ModelService,
) error {
	if f.createErr != nil {
		return f.createErr
	}

	f.created = modelService.DeepCopy()

	return nil
}

func testCreateDefaults() ModelServiceDefaults {
	return ModelServiceDefaults{
		GatewayName:               "shared-gateway",
		GatewayNamespace:          "gateway-system",
		GatewaySectionName:        "fraud-model-https",
		GatewayDataPlaneNamespace: "envoy-gateway-system",
	}
}

func newCreateRequest(
	body string,
) *http.Request {
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/model-services",
		strings.NewReader(body),
	)

	request.Header.Set(
		"Content-Type",
		"application/json",
	)

	return request
}

func TestCreateRequestToKServeModelService(
	t *testing.T,
) {
	request := apirequest.CreateModelServiceRequest{
		Name:     "iris-api",
		Backend:  "KServe",
		Replicas: 1,
		Predictor: &apirequest.PredictorRequest{
			ModelFormat: apirequest.ModelFormatRequest{
				Name:    "sklearn",
				Version: "1",
			},
			Runtime: "kserve-sklearnserver",
			StorageURI: "s3://models/sklearn/" +
				"iris/v1",
			ServiceAccountName: "kserve-model-reader",
		},
	}

	modelService := createRequestToModelService(
		request,
		testCreateDefaults(),
	)

	if modelService.Spec.Backend !=
		platformv1alpha1.ModelServiceBackendKServe {
		t.Fatalf(
			"expected KServe backend, got %q",
			modelService.Spec.Backend,
		)
	}

	if modelService.Spec.Image != "" {
		t.Fatalf(
			"expected empty image, got %q",
			modelService.Spec.Image,
		)
	}

	if modelService.Spec.Predictor == nil {
		t.Fatal(
			"expected KServe predictor",
		)
	}

	predictor := modelService.Spec.Predictor

	if predictor.ModelFormat.Name != "sklearn" {
		t.Fatalf(
			"expected sklearn, got %q",
			predictor.ModelFormat.Name,
		)
	}

	if predictor.ModelFormat.Version != "1" {
		t.Fatalf(
			"expected version 1, got %q",
			predictor.ModelFormat.Version,
		)
	}

	if predictor.Runtime !=
		"kserve-sklearnserver" {
		t.Fatalf(
			"unexpected runtime %q",
			predictor.Runtime,
		)
	}

	if predictor.StorageURI !=
		"s3://models/sklearn/iris/v1" {
		t.Fatalf(
			"unexpected storage URI %q",
			predictor.StorageURI,
		)
	}

	if predictor.ServiceAccountName !=
		"kserve-model-reader" {
		t.Fatalf(
			"unexpected service account %q",
			predictor.ServiceAccountName,
		)
	}
}

func TestCreateRequestToDeploymentModelService(
	t *testing.T,
) {
	request := apirequest.CreateModelServiceRequest{
		Name:     "stateful-model",
		Image:    "example/model:v1",
		Replicas: 1,
		Port:     8080,
		Storage: apirequest.StorageRequest{
			Enabled:   true,
			Size:      "10Gi",
			MountPath: "/models",
		},
	}

	modelService := createRequestToModelService(
		request,
		testCreateDefaults(),
	)

	if modelService.Spec.Backend !=
		platformv1alpha1.ModelServiceBackendDeployment {
		t.Fatalf(
			"expected default Deployment backend, got %q",
			modelService.Spec.Backend,
		)
	}

	if modelService.Spec.Predictor != nil {
		t.Fatal(
			"expected Deployment predictor to be nil",
		)
	}

	if modelService.Spec.Storage == nil {
		t.Fatal(
			"expected persistent storage",
		)
	}

	if !modelService.Spec.Storage.Enabled {
		t.Fatal(
			"expected persistent storage to be enabled",
		)
	}

	if modelService.Spec.Storage.Size != "10Gi" {
		t.Fatalf(
			"expected storage size 10Gi, got %q",
			modelService.Spec.Storage.Size,
		)
	}

	if modelService.Spec.Storage.MountPath != "/models" {
		t.Fatalf(
			"expected mount path /models, got %q",
			modelService.Spec.Storage.MountPath,
		)
	}
}

func TestCreateModelService(
	t *testing.T,
) {
	store := &fakeModelServiceCreator{}

	handler :=
		NewCreateModelServiceHandler(
			testLogger(),
			store,
			10,
			testCreateDefaults(),
		)

	request := newCreateRequest(`
{
  "name": "fraud-model",
  "image": "example/fraud:v1",
  "replicas": 2,
  "port": 8080,
  "exposure": {
    "enabled": false
  },
  "storage": {
    "enabled": false
  }
}
`)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		request,
	)

	if recorder.Code != http.StatusCreated {
		t.Fatalf(
			"expected status %d, got %d: %s",
			http.StatusCreated,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if store.created == nil {
		t.Fatal(
			"expected ModelService to be created",
		)
	}

	if store.created.Name != "fraud-model" {
		t.Fatalf(
			"expected name fraud-model, got %q",
			store.created.Name,
		)
	}

	if store.created.Spec.Image !=
		"example/fraud:v1" {
		t.Fatalf(
			"expected image example/fraud:v1, got %q",
			store.created.Spec.Image,
		)
	}

	if store.created.Spec.Replicas != 2 {
		t.Fatalf(
			"expected replicas 2, got %d",
			store.created.Spec.Replicas,
		)
	}

	if store.created.Spec.Port != 8080 {
		t.Fatalf(
			"expected port 8080, got %d",
			store.created.Spec.Port,
		)
	}

	body := recorder.Body.String()

	expected := []string{
		`"name":"fraud-model"`,
		`"image":"example/fraud:v1"`,
		`"replicas":2`,
		`"port":8080`,
	}

	for _, value := range expected {
		if !strings.Contains(
			body,
			value,
		) {
			t.Fatalf(
				"expected body to contain %q, got %s",
				value,
				body,
			)
		}
	}
}

func TestCreateKServeModelService(
	t *testing.T,
) {
	store := &fakeModelServiceCreator{}

	handler :=
		NewCreateModelServiceHandler(
			testLogger(),
			store,
			10,
			testCreateDefaults(),
		)

	request := newCreateRequest(`
{
  "name": "iris-api",
  "backend": "KServe",
  "replicas": 1,
  "predictor": {
    "modelFormat": {
      "name": "sklearn",
      "version": "1"
    },
    "runtime": "kserve-sklearnserver",
    "storageUri": "s3://models/sklearn/iris/v1",
    "serviceAccountName": "kserve-model-reader"
  }
}
`)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		request,
	)

	if recorder.Code != http.StatusCreated {
		t.Fatalf(
			"expected status %d, got %d: %s",
			http.StatusCreated,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if store.created == nil {
		t.Fatal(
			"expected KServe ModelService to be created",
		)
	}

	if store.created.Spec.Backend !=
		platformv1alpha1.ModelServiceBackendKServe {
		t.Fatalf(
			"expected backend KServe, got %q",
			store.created.Spec.Backend,
		)
	}

	if store.created.Spec.Image != "" {
		t.Fatalf(
			"expected no Deployment image, got %q",
			store.created.Spec.Image,
		)
	}

	if store.created.Spec.Predictor == nil {
		t.Fatal(
			"expected KServe predictor",
		)
	}

	predictor := store.created.Spec.Predictor

	if predictor.ModelFormat.Name != "sklearn" {
		t.Fatalf(
			"expected sklearn format, got %q",
			predictor.ModelFormat.Name,
		)
	}

	if predictor.ModelFormat.Version != "1" {
		t.Fatalf(
			"expected model version 1, got %q",
			predictor.ModelFormat.Version,
		)
	}

	if predictor.Runtime !=
		"kserve-sklearnserver" {
		t.Fatalf(
			"unexpected runtime %q",
			predictor.Runtime,
		)
	}

	if predictor.StorageURI !=
		"s3://models/sklearn/iris/v1" {
		t.Fatalf(
			"unexpected storage URI %q",
			predictor.StorageURI,
		)
	}

	if predictor.ServiceAccountName !=
		"kserve-model-reader" {
		t.Fatalf(
			"unexpected service account %q",
			predictor.ServiceAccountName,
		)
	}

	body := recorder.Body.String()

	expected := []string{
		`"name":"iris-api"`,
		`"backend":"KServe"`,
		`"replicas":1`,
		`"predictor":`,
		`"name":"sklearn"`,
		`"version":"1"`,
		`"runtime":"kserve-sklearnserver"`,
		`"storageUri":"s3://models/sklearn/iris/v1"`,
		`"serviceAccountName":"kserve-model-reader"`,
	}

	for _, value := range expected {
		if !strings.Contains(
			body,
			value,
		) {
			t.Fatalf(
				"expected body to contain %q, got %s",
				value,
				body,
			)
		}
	}

	if strings.Contains(
		body,
		`"image":`,
	) {
		t.Fatalf(
			"expected KServe response to omit image, got %s",
			body,
		)
	}

	if strings.Contains(
		body,
		`"port":`,
	) {
		t.Fatalf(
			"expected KServe response to omit port, got %s",
			body,
		)
	}
}

func TestCreateModelServiceInvalidReplicas(
	t *testing.T,
) {
	store := &fakeModelServiceCreator{}

	handler :=
		NewCreateModelServiceHandler(
			testLogger(),
			store,
			10,
			testCreateDefaults(),
		)

	request := newCreateRequest(`
{
  "name": "fraud-model",
  "image": "example/fraud:v1",
  "replicas": 99,
  "port": 8080,
  "exposure": {
    "enabled": false
  },
  "storage": {
    "enabled": false
  }
}
`)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		request,
	)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusBadRequest,
			recorder.Code,
		)
	}

	body := recorder.Body.String()

	if !strings.Contains(
		body,
		`"code":"VALIDATION_FAILED"`,
	) {
		t.Fatalf(
			"unexpected response: %s",
			body,
		)
	}

	if !strings.Contains(
		body,
		`"field":"replicas"`,
	) {
		t.Fatalf(
			"expected replicas validation detail, got %s",
			body,
		)
	}

	if store.created != nil {
		t.Fatal(
			"expected invalid request not to create ModelService",
		)
	}
}

func TestCreateModelServiceMissingImage(
	t *testing.T,
) {
	store := &fakeModelServiceCreator{}

	handler :=
		NewCreateModelServiceHandler(
			testLogger(),
			store,
			10,
			testCreateDefaults(),
		)

	request := newCreateRequest(`
{
  "name": "fraud-model",
  "image": "",
  "replicas": 2,
  "port": 8080,
  "exposure": {
    "enabled": false
  },
  "storage": {
    "enabled": false
  }
}
`)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		request,
	)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusBadRequest,
			recorder.Code,
		)
	}

	if !strings.Contains(
		recorder.Body.String(),
		`"code":"VALIDATION_FAILED"`,
	) {
		t.Fatalf(
			"unexpected response: %s",
			recorder.Body.String(),
		)
	}

	if store.created != nil {
		t.Fatal(
			"expected invalid request not to create ModelService",
		)
	}
}

func TestCreateModelServiceUnknownField(
	t *testing.T,
) {
	store := &fakeModelServiceCreator{}

	handler :=
		NewCreateModelServiceHandler(
			testLogger(),
			store,
			10,
			testCreateDefaults(),
		)

	request := newCreateRequest(`
{
  "name": "fraud-model",
  "image": "example/fraud:v1",
  "replicas": 2,
  "port": 8080,
  "privileged": true,
  "exposure": {
    "enabled": false
  },
  "storage": {
    "enabled": false
  }
}
`)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		request,
	)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusBadRequest,
			recorder.Code,
		)
	}

	if !strings.Contains(
		recorder.Body.String(),
		`"code":"INVALID_JSON"`,
	) {
		t.Fatalf(
			"unexpected response: %s",
			recorder.Body.String(),
		)
	}

	if store.created != nil {
		t.Fatal(
			"expected unknown field not to create ModelService",
		)
	}
}

func TestCreateModelServiceMalformedJSON(
	t *testing.T,
) {
	store := &fakeModelServiceCreator{}

	handler :=
		NewCreateModelServiceHandler(
			testLogger(),
			store,
			10,
			testCreateDefaults(),
		)

	request := newCreateRequest(
		`{"name":"fraud-model"`,
	)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		request,
	)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusBadRequest,
			recorder.Code,
		)
	}

	if !strings.Contains(
		recorder.Body.String(),
		`"code":"INVALID_JSON"`,
	) {
		t.Fatalf(
			"unexpected response: %s",
			recorder.Body.String(),
		)
	}
}

func TestCreateModelServiceAlreadyExists(
	t *testing.T,
) {
	alreadyExists :=
		apierrors.NewAlreadyExists(
			schema.GroupResource{
				Group:    "platform.anselem.dev",
				Resource: "modelservices",
			},
			"fraud-model",
		)

	store := &fakeModelServiceCreator{
		createErr: alreadyExists,
	}

	handler :=
		NewCreateModelServiceHandler(
			testLogger(),
			store,
			10,
			testCreateDefaults(),
		)

	request := newCreateRequest(`
{
  "name": "fraud-model",
  "image": "example/fraud:v1",
  "replicas": 2,
  "port": 8080,
  "exposure": {
    "enabled": false
  },
  "storage": {
    "enabled": false
  }
}
`)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		request,
	)

	if recorder.Code != http.StatusConflict {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusConflict,
			recorder.Code,
		)
	}

	if !strings.Contains(
		recorder.Body.String(),
		`"code":"MODEL_SERVICE_ALREADY_EXISTS"`,
	) {
		t.Fatalf(
			"unexpected response: %s",
			recorder.Body.String(),
		)
	}
}

func TestCreateModelServiceKubernetesFailure(
	t *testing.T,
) {
	store := &fakeModelServiceCreator{
		createErr: errors.New(
			"Kubernetes unavailable",
		),
	}

	handler :=
		NewCreateModelServiceHandler(
			testLogger(),
			store,
			10,
			testCreateDefaults(),
		)

	request := newCreateRequest(`
{
  "name": "fraud-model",
  "image": "example/fraud:v1",
  "replicas": 2,
  "port": 8080,
  "exposure": {
    "enabled": false
  },
  "storage": {
    "enabled": false
  }
}
`)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		request,
	)

	if recorder.Code !=
		http.StatusServiceUnavailable {
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
