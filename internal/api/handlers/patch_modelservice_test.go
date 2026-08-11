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

type fakeModelServiceUpdateStore struct {
	item      *platformv1alpha1.ModelService
	getErr    error
	updateErr error
	updated   *platformv1alpha1.ModelService
}

func (f *fakeModelServiceUpdateStore) Get(
	context.Context,
	string,
) (*platformv1alpha1.ModelService, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}

	return f.item.DeepCopy(), nil
}

func (f *fakeModelServiceUpdateStore) Update(
	_ context.Context,
	modelService *platformv1alpha1.ModelService,
) error {
	if f.updateErr != nil {
		return f.updateErr
	}

	f.updated = modelService.DeepCopy()

	return nil
}

func testPatchModelService() *platformv1alpha1.ModelService {
	return &platformv1alpha1.ModelService{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "fraud-model",
			Namespace:  "ai-platform",
			Generation: 2,
		},
		Spec: platformv1alpha1.ModelServiceSpec{
			Image:    "example/fraud:v1",
			Replicas: 2,
			Port:     8080,

			Security: &platformv1alpha1.ModelServiceSecurity{
				RunAsNonRoot:           true,
				RunAsUser:              101,
				RunAsGroup:             101,
				FSGroup:                101,
				ReadOnlyRootFilesystem: true,
			},

			Exposure: &platformv1alpha1.ModelServiceExposure{
				Enabled:    false,
				Hostname:   "fraud-model.local",
				PathPrefix: "/",
			},

			Storage: &platformv1alpha1.ModelServiceStorage{
				Enabled:   false,
				Size:      "1Gi",
				MountPath: "/models",
			},
		},
	}
}

func testPatchDefaults() ModelServiceDefaults {
	return ModelServiceDefaults{
		GatewayName:               "shared-gateway",
		GatewayNamespace:          "gateway-system",
		GatewaySectionName:        "fraud-model-https",
		GatewayDataPlaneNamespace: "envoy-gateway-system",
	}
}

func newPatchRequest(
	body string,
) *http.Request {
	request := httptest.NewRequest(
		http.MethodPatch,
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

func TestPatchModelServiceReplicasOnly(
	t *testing.T,
) {
	original := testPatchModelService()

	store := &fakeModelServiceUpdateStore{
		item: original,
	}

	handler :=
		NewPatchModelServiceHandler(
			testLogger(),
			store,
			10,
			testPatchDefaults(),
		)

	request := newPatchRequest(
		`{"replicas":3}`,
	)

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

	if store.updated.Spec.Replicas != 3 {
		t.Fatalf(
			"expected replicas 3, got %d",
			store.updated.Spec.Replicas,
		)
	}

	if store.updated.Spec.Image !=
		original.Spec.Image {
		t.Fatal(
			"expected image to remain unchanged",
		)
	}

	if store.updated.Spec.Port !=
		original.Spec.Port {
		t.Fatal(
			"expected port to remain unchanged",
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

	if store.updated.Spec.Storage == nil {
		t.Fatal(
			"expected storage configuration to be preserved",
		)
	}

	if store.updated.Spec.Storage.MountPath !=
		original.Spec.Storage.MountPath {
		t.Fatal(
			"expected storage configuration to remain unchanged",
		)
	}

	if store.updated.Spec.Exposure == nil {
		t.Fatal(
			"expected exposure configuration to be preserved",
		)
	}

	if store.updated.Spec.Exposure.Hostname !=
		original.Spec.Exposure.Hostname {
		t.Fatal(
			"expected exposure configuration to remain unchanged",
		)
	}

	if !strings.Contains(
		recorder.Body.String(),
		`"replicas":3`,
	) {
		t.Fatalf(
			"expected response to contain replicas 3, got %s",
			recorder.Body.String(),
		)
	}
}

func TestPatchModelServiceEmptyPatch(
	t *testing.T,
) {
	store := &fakeModelServiceUpdateStore{
		item: testPatchModelService(),
	}

	handler :=
		NewPatchModelServiceHandler(
			testLogger(),
			store,
			10,
			testPatchDefaults(),
		)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		newPatchRequest(`{}`),
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
		`"code":"EMPTY_PATCH"`,
	) {
		t.Fatalf(
			"unexpected response: %s",
			recorder.Body.String(),
		)
	}

	if store.updated != nil {
		t.Fatal(
			"expected no Kubernetes update for empty patch",
		)
	}
}

func TestPatchModelServiceInvalidReplicas(
	t *testing.T,
) {
	store := &fakeModelServiceUpdateStore{
		item: testPatchModelService(),
	}

	handler :=
		NewPatchModelServiceHandler(
			testLogger(),
			store,
			10,
			testPatchDefaults(),
		)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		newPatchRequest(
			`{"replicas":99}`,
		),
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
			"expected invalid patch not to update Kubernetes",
		)
	}
}

func TestPatchModelServiceUnknownField(
	t *testing.T,
) {
	store := &fakeModelServiceUpdateStore{
		item: testPatchModelService(),
	}

	handler :=
		NewPatchModelServiceHandler(
			testLogger(),
			store,
			10,
			testPatchDefaults(),
		)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		newPatchRequest(
			`{"privileged":true}`,
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

func TestPatchModelServiceNotFound(
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
		NewPatchModelServiceHandler(
			testLogger(),
			store,
			10,
			testPatchDefaults(),
		)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		newPatchRequest(
			`{"replicas":3}`,
		),
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

func TestPatchModelServiceGetFailure(
	t *testing.T,
) {
	store := &fakeModelServiceUpdateStore{
		getErr: errors.New(
			"Kubernetes unavailable",
		),
	}

	handler :=
		NewPatchModelServiceHandler(
			testLogger(),
			store,
			10,
			testPatchDefaults(),
		)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		newPatchRequest(
			`{"replicas":3}`,
		),
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

func TestPatchModelServiceUpdateFailure(
	t *testing.T,
) {
	store := &fakeModelServiceUpdateStore{
		item: testPatchModelService(),
		updateErr: errors.New(
			"Kubernetes unavailable",
		),
	}

	handler :=
		NewPatchModelServiceHandler(
			testLogger(),
			store,
			10,
			testPatchDefaults(),
		)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		newPatchRequest(
			`{"replicas":3}`,
		),
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

func TestPatchModelServiceUpdateConflict(
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
		NewPatchModelServiceHandler(
			testLogger(),
			store,
			10,
			testPatchDefaults(),
		)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		newPatchRequest(
			`{"replicas":3}`,
		),
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
