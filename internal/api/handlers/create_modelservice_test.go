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
