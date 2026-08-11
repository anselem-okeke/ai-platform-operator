package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func newUpdateRequest(
	body string,
) *http.Request {
	request := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/model-services/fraud-model",
		strings.NewReader(body),
	)

	request.Header.Set(
		"Content-Type",
		"application/json",
	)

	request.SetPathValue(
		"name",
		"fraud-model",
	)

	return request
}

func TestUpdateModelService(
	t *testing.T,
) {
	original := testPatchModelService()

	store := &fakeModelServiceUpdateStore{
		item: original,
	}

	handler :=
		NewUpdateModelServiceHandler(
			testLogger(),
			store,
			10,
			testPatchDefaults(),
		)

	request := newUpdateRequest(`
{
  "image": "example/fraud:v2",
  "replicas": 4,
  "port": 9090,
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

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected status %d, got %d: %s",
			http.StatusOK,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if store.updated == nil {
		t.Fatal(
			"expected ModelService to be updated",
		)
	}

	if store.updated.Spec.Image !=
		"example/fraud:v2" {
		t.Fatalf(
			"expected image example/fraud:v2, got %q",
			store.updated.Spec.Image,
		)
	}

	if store.updated.Spec.Replicas != 4 {
		t.Fatalf(
			"expected replicas 4, got %d",
			store.updated.Spec.Replicas,
		)
	}

	if store.updated.Spec.Port != 9090 {
		t.Fatalf(
			"expected port 9090, got %d",
			store.updated.Spec.Port,
		)
	}

	if store.updated.Spec.Security == nil {
		t.Fatal(
			"expected security configuration to be preserved",
		)
	}

	if store.updated.Spec.Security.RunAsUser !=
		original.Spec.Security.RunAsUser {
		t.Fatal(
			"expected security configuration to remain unchanged",
		)
	}

	if store.updated.Spec.Exposure == nil {
		t.Fatal(
			"expected exposure configuration to exist",
		)
	}

	if store.updated.Spec.Exposure.Enabled {
		t.Fatal(
			"expected exposure to be disabled",
		)
	}

	if store.updated.Spec.Storage == nil {
		t.Fatal(
			"expected storage configuration to exist",
		)
	}

	if store.updated.Spec.Storage.Enabled {
		t.Fatal(
			"expected storage to be disabled",
		)
	}

	body := recorder.Body.String()

	expected := []string{
		`"name":"fraud-model"`,
		`"image":"example/fraud:v2"`,
		`"replicas":4`,
		`"port":9090`,
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

func TestUpdateModelServiceInvalidReplicas(
	t *testing.T,
) {
	store := &fakeModelServiceUpdateStore{
		item: testPatchModelService(),
	}

	handler :=
		NewUpdateModelServiceHandler(
			testLogger(),
			store,
			10,
			testPatchDefaults(),
		)

	request := newUpdateRequest(`
{
  "image": "example/fraud:v2",
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

	if store.updated != nil {
		t.Fatal(
			"expected invalid request not to update ModelService",
		)
	}
}

func TestUpdateModelServiceUnknownField(
	t *testing.T,
) {
	store := &fakeModelServiceUpdateStore{
		item: testPatchModelService(),
	}

	handler :=
		NewUpdateModelServiceHandler(
			testLogger(),
			store,
			10,
			testPatchDefaults(),
		)

	request := newUpdateRequest(`
{
  "image": "example/fraud:v2",
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

	if store.updated != nil {
		t.Fatal(
			"expected unknown field not to update ModelService",
		)
	}
}

func TestUpdateModelServiceMalformedJSON(
	t *testing.T,
) {
	store := &fakeModelServiceUpdateStore{
		item: testPatchModelService(),
	}

	handler :=
		NewUpdateModelServiceHandler(
			testLogger(),
			store,
			10,
			testPatchDefaults(),
		)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		newUpdateRequest(
			`{"image":"example/fraud:v2"`,
		),
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

func TestUpdateModelServiceNotFound(
	t *testing.T,
) {
	notFound := apierrors.NewNotFound(
		schema.GroupResource{
			Group:    "platform.anselem.dev",
			Resource: "modelservices",
		},
		"fraud-model",
	)

	store := &fakeModelServiceUpdateStore{
		getErr: notFound,
	}

	handler :=
		NewUpdateModelServiceHandler(
			testLogger(),
			store,
			10,
			testPatchDefaults(),
		)

	request := newUpdateRequest(`
{
  "image": "example/fraud:v2",
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

func TestUpdateModelServiceGetFailure(
	t *testing.T,
) {
	store := &fakeModelServiceUpdateStore{
		getErr: errors.New(
			"Kubernetes unavailable",
		),
	}

	handler :=
		NewUpdateModelServiceHandler(
			testLogger(),
			store,
			10,
			testPatchDefaults(),
		)

	request := newUpdateRequest(`
{
  "image": "example/fraud:v2",
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
}

func TestUpdateModelServiceUpdateFailure(
	t *testing.T,
) {
	store := &fakeModelServiceUpdateStore{
		item: testPatchModelService(),
		updateErr: errors.New(
			"Kubernetes unavailable",
		),
	}

	handler :=
		NewUpdateModelServiceHandler(
			testLogger(),
			store,
			10,
			testPatchDefaults(),
		)

	request := newUpdateRequest(`
{
  "image": "example/fraud:v2",
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
}

func TestUpdateModelServiceConflict(
	t *testing.T,
) {
	conflict := apierrors.NewConflict(
		schema.GroupResource{
			Group:    "platform.anselem.dev",
			Resource: "modelservices",
		},
		"fraud-model",
		errors.New(
			"resource version conflict",
		),
	)

	store := &fakeModelServiceUpdateStore{
		item:      testPatchModelService(),
		updateErr: conflict,
	}

	handler :=
		NewUpdateModelServiceHandler(
			testLogger(),
			store,
			10,
			testPatchDefaults(),
		)

	request := newUpdateRequest(`
{
  "image": "example/fraud:v2",
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
		`"code":"MODEL_SERVICE_UPDATE_CONFLICT"`,
	) {
		t.Fatalf(
			"unexpected response: %s",
			recorder.Body.String(),
		)
	}
}
