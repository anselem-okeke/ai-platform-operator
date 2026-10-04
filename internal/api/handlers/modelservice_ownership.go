package handlers

import (
	"net/http"

	platformv1alpha1 "github.com/anselem-okeke/ai-platform-operator/api/v1alpha1"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/middleware"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/response"
)

// Existing GitOps resources carry Argo CD tracking annotations. Treat even an
// empty tracking marker conservatively; API clients cannot supply metadata.
// Do not use app.kubernetes.io/instance: it is also used for ordinary model labels.
func rejectGitOpsMutation(w http.ResponseWriter, r *http.Request, model *platformv1alpha1.ModelService) bool {
	_, tracked := model.Annotations["argocd.argoproj.io/tracking-id"]
	_, legacyTracked := model.Labels["argocd.argoproj.io/instance"]
	if !tracked && !legacyTracked {
		return false
	}

	response.WriteJSON(w, http.StatusConflict, response.APIError{
		Error: response.ErrorBody{
			Code:      "MODEL_SERVICE_OWNERSHIP_CONFLICT",
			Message:   "ModelService is managed by GitOps; change or delete it through its Git repository",
			RequestID: middleware.RequestIDFromContext(r.Context()),
		},
	})
	return true
}
