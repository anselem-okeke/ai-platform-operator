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

func TestGitOpsOwnershipMutations(t *testing.T) {
	for _, marker := range []string{"annotation", "empty-annotation", "legacy-label", "untracked", "ordinary-instance"} {
		for _, method := range []string{http.MethodPut, http.MethodPatch, http.MethodDelete} {
			t.Run(marker+"/"+method, func(t *testing.T) {
				model := testPatchModelService()
				switch marker {
				case "annotation":
					model.Annotations = map[string]string{"argocd.argoproj.io/tracking-id": "ai-platform-modelservices:platform.anselem.dev/ModelService:ai-platform/fraud-model"}
				case "empty-annotation":
					model.Annotations = map[string]string{"argocd.argoproj.io/tracking-id": ""}
				case "legacy-label":
					model.Labels = map[string]string{"argocd.argoproj.io/instance": "ai-platform-modelservices"}
				case "ordinary-instance":
					model.Labels = map[string]string{"app.kubernetes.io/instance": "fraud-model"}
				}
				updateStore := &fakeModelServiceUpdateStore{item: model}
				deleteStore := &fakeModelServiceDeleteStore{item: model}
				var handler http.Handler
				var request *http.Request
				want := http.StatusOK
				switch method {
				case http.MethodPut:
					handler = NewUpdateModelServiceHandler(testLogger(), updateStore, 10, testPatchDefaults())
					request = newUpdateRequest(`{"image":"example/fraud:v2","replicas":2,"port":8080}`)
				case http.MethodPatch:
					handler = NewPatchModelServiceHandler(testLogger(), updateStore, 10, testPatchDefaults())
					request = newPatchRequest(`{"replicas":3}`)
				case http.MethodDelete:
					handler = NewDeleteModelServiceHandler(testLogger(), deleteStore)
					request = newDeleteRequest(model.Name)
					want = http.StatusNoContent
				}
				blocked := marker != "untracked" && marker != "ordinary-instance"
				if blocked {
					want = http.StatusConflict
				}
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, request)
				if recorder.Code != want {
					t.Fatalf("status=%d want=%d body=%s", recorder.Code, want, recorder.Body.String())
				}
				if blocked && (updateStore.updated != nil || deleteStore.deleted != nil) {
					t.Fatal("GitOps resource was mutated")
				}
				if blocked && !strings.Contains(recorder.Body.String(), "MODEL_SERVICE_OWNERSHIP_CONFLICT") {
					t.Fatal("missing ownership conflict code")
				}
			})
		}
	}
}

func TestGitOpsOwnershipReadAccess(t *testing.T) {
	model := testPatchModelService()
	model.Annotations = map[string]string{"argocd.argoproj.io/tracking-id": "ai-platform-modelservices:platform.anselem.dev/ModelService:ai-platform/fraud-model"}
	store := &fakeModelServiceUpdateStore{item: model}
	for _, handler := range []http.Handler{
		NewGetModelServiceHandler(testLogger(), store),
		NewGetModelServiceStatusHandler(testLogger(), store),
	} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/model-services/fraud-model", nil)
		request.SetPathValue("name", model.Name)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("read status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	}
}

func TestOwnershipMetadataCannotBePatched(t *testing.T) {
	store := &fakeModelServiceUpdateStore{item: testPatchModelService()}
	handler := NewPatchModelServiceHandler(testLogger(), store, 10, testPatchDefaults())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, newPatchRequest(`{"replicas":3,"metadata":{"annotations":{"argocd.argoproj.io/tracking-id":null}}}`))
	if recorder.Code != http.StatusBadRequest || store.updated != nil {
		t.Fatalf("metadata patch was not rejected: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestConcurrentDeleteReturnsConflict(t *testing.T) {
	store := &fakeModelServiceDeleteStore{
		item:      testDeleteModelService(),
		deleteErr: apierrors.NewConflict(schema.GroupResource{Group: "platform.anselem.dev", Resource: "modelservices"}, "fraud-model", errors.New("resource version changed")),
	}
	recorder := httptest.NewRecorder()
	NewDeleteModelServiceHandler(testLogger(), store).ServeHTTP(recorder, newDeleteRequest("fraud-model"))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "MODEL_SERVICE_DELETE_CONFLICT") {
		t.Fatalf("expected delete conflict: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
