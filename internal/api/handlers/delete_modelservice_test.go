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

type fakeModelServiceDeleteStore struct {
	item      *platformv1alpha1.ModelService
	getErr    error
	deleteErr error
	deleted   *platformv1alpha1.ModelService
}

func (f *fakeModelServiceDeleteStore) Get(
	context.Context,
	string,
) (*platformv1alpha1.ModelService, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}

	if f.item == nil {
		return nil, nil
	}

	return f.item.DeepCopy(), nil
}

func (f *fakeModelServiceDeleteStore) Delete(
	_ context.Context,
	modelService *platformv1alpha1.ModelService,
) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}

	f.deleted = modelService.DeepCopy()

	return nil
}

func testDeleteModelService() *platformv1alpha1.ModelService {
	return &platformv1alpha1.ModelService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fraud-model",
			Namespace: "ai-platform",
		},
		Spec: platformv1alpha1.ModelServiceSpec{
			Image:    "example/fraud:v1",
			Replicas: 2,
			Port:     8080,
		},
	}
}

func newDeleteRequest(
	name string,
) *http.Request {
	request := httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/model-services/"+name,
		nil,
	)

	request.SetPathValue(
		"name",
		name,
	)

	return request
}

func TestDeleteModelService(
	t *testing.T,
) {
	store := &fakeModelServiceDeleteStore{
		item: testDeleteModelService(),
	}

	handler :=
		NewDeleteModelServiceHandler(
			testLogger(),
			store,
		)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		newDeleteRequest("fraud-model"),
	)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf(
			"expected status %d, got %d: %s",
			http.StatusNoContent,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if store.deleted == nil {
		t.Fatal(
			"expected ModelService to be deleted",
		)
	}

	if store.deleted.Name != "fraud-model" {
		t.Fatalf(
			"expected deleted ModelService fraud-model, got %q",
			store.deleted.Name,
		)
	}

	if store.deleted.Namespace != "ai-platform" {
		t.Fatalf(
			"expected namespace ai-platform, got %q",
			store.deleted.Namespace,
		)
	}

	if recorder.Body.Len() != 0 {
		t.Fatalf(
			"expected empty response body for 204, got %s",
			recorder.Body.String(),
		)
	}
}

func TestDeleteModelServiceNotFound(
	t *testing.T,
) {
	notFound := apierrors.NewNotFound(
		schema.GroupResource{
			Group:    "platform.anselem.dev",
			Resource: "modelservices",
		},
		"missing-model",
	)

	store := &fakeModelServiceDeleteStore{
		getErr: notFound,
	}

	handler :=
		NewDeleteModelServiceHandler(
			testLogger(),
			store,
		)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		newDeleteRequest("missing-model"),
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

	if store.deleted != nil {
		t.Fatal(
			"expected no delete operation for missing ModelService",
		)
	}
}

func TestDeleteModelServiceGetFailure(
	t *testing.T,
) {
	store := &fakeModelServiceDeleteStore{
		getErr: errors.New(
			"Kubernetes unavailable",
		),
	}

	handler :=
		NewDeleteModelServiceHandler(
			testLogger(),
			store,
		)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		newDeleteRequest("fraud-model"),
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

	if store.deleted != nil {
		t.Fatal(
			"expected no delete operation after GET failure",
		)
	}
}

func TestDeleteModelServiceDeleteFailure(
	t *testing.T,
) {
	store := &fakeModelServiceDeleteStore{
		item: testDeleteModelService(),
		deleteErr: errors.New(
			"Kubernetes unavailable",
		),
	}

	handler :=
		NewDeleteModelServiceHandler(
			testLogger(),
			store,
		)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		newDeleteRequest("fraud-model"),
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

	if store.deleted != nil {
		t.Fatal(
			"expected failed delete not to be recorded as successful",
		)
	}
}

func TestDeleteModelServiceAlreadyDeleted(
	t *testing.T,
) {
	notFound := apierrors.NewNotFound(
		schema.GroupResource{
			Group:    "platform.anselem.dev",
			Resource: "modelservices",
		},
		"fraud-model",
	)

	store := &fakeModelServiceDeleteStore{
		item:      testDeleteModelService(),
		deleteErr: notFound,
	}

	handler :=
		NewDeleteModelServiceHandler(
			testLogger(),
			store,
		)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		newDeleteRequest("fraud-model"),
	)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf(
			"expected status %d, got %d: %s",
			http.StatusNoContent,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if recorder.Body.Len() != 0 {
		t.Fatalf(
			"expected empty response body for 204, got %s",
			recorder.Body.String(),
		)
	}
}

func TestDeleteModelServiceMissingName(
	t *testing.T,
) {
	store := &fakeModelServiceDeleteStore{
		item: testDeleteModelService(),
	}

	handler :=
		NewDeleteModelServiceHandler(
			testLogger(),
			store,
		)

	request := httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/model-services/",
		nil,
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
		`"code":"INVALID_MODEL_SERVICE_NAME"`,
	) {
		t.Fatalf(
			"unexpected response: %s",
			recorder.Body.String(),
		)
	}

	if store.deleted != nil {
		t.Fatal(
			"expected no delete operation without a resource name",
		)
	}
}
