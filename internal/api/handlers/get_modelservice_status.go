package handlers

import (
	"log/slog"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	platformv1alpha1 "github.com/anselem-okeke/ai-platform-operator/api/v1alpha1"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/middleware"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/response"
)

type GetModelServiceStatusHandler struct {
	logger *slog.Logger
	store  ModelServiceGetter
}

func NewGetModelServiceStatusHandler(
	logger *slog.Logger,
	store ModelServiceGetter,
) *GetModelServiceStatusHandler {
	return &GetModelServiceStatusHandler{
		logger: logger,
		store:  store,
	}
}

func (h *GetModelServiceStatusHandler) ServeHTTP(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)

		response.WriteJSON(
			w,
			http.StatusMethodNotAllowed,
			response.APIError{
				Error: response.ErrorBody{
					Code:      "METHOD_NOT_ALLOWED",
					Message:   "method not allowed",
					RequestID: middleware.RequestIDFromContext(r.Context()),
				},
			},
		)

		return
	}

	name := r.PathValue("name")
	if name == "" {
		response.WriteJSON(
			w,
			http.StatusBadRequest,
			response.APIError{
				Error: response.ErrorBody{
					Code:      "INVALID_MODEL_SERVICE_NAME",
					Message:   "ModelService name is required",
					RequestID: middleware.RequestIDFromContext(r.Context()),
				},
			},
		)

		return
	}

	modelService, err := h.store.Get(
		r.Context(),
		name,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			response.WriteJSON(
				w,
				http.StatusNotFound,
				response.APIError{
					Error: response.ErrorBody{
						Code: "MODEL_SERVICE_NOT_FOUND",
						Message: "ModelService \"" +
							name +
							"\" was not found",
						RequestID: middleware.RequestIDFromContext(r.Context()),
					},
				},
			)

			return
		}

		h.logger.ErrorContext(
			r.Context(),
			"get_modelservice_status_failed",
			slog.String(
				"request_id",
				middleware.RequestIDFromContext(r.Context()),
			),
			slog.String(
				"model_service",
				name,
			),
			slog.String(
				"error",
				err.Error(),
			),
		)

		response.WriteJSON(
			w,
			http.StatusServiceUnavailable,
			response.APIError{
				Error: response.ErrorBody{
					Code:      "KUBERNETES_UNAVAILABLE",
					Message:   "unable to get ModelService status",
					RequestID: middleware.RequestIDFromContext(r.Context()),
				},
			},
		)

		return
	}

	response.WriteJSON(
		w,
		http.StatusOK,
		modelServiceToStatusResponse(
			*modelService,
		),
	)
}

func modelServiceToStatusResponse(
	modelService platformv1alpha1.ModelService,
) response.ModelServiceStatusResponse {
	conditions := make(
		[]response.ModelServiceCondition,
		0,
		len(modelService.Status.Conditions),
	)

	for _, condition := range modelService.Status.Conditions {
		conditions = append(
			conditions,
			response.ModelServiceCondition{
				Type:    condition.Type,
				Status:  string(condition.Status),
				Reason:  condition.Reason,
				Message: condition.Message,
			},
		)
	}

	result := response.ModelServiceStatusResponse{
		Name:               modelService.Name,
		State:              modelServiceState(modelService),
		ObservedGeneration: modelService.Status.ObservedGeneration,
		DesiredReplicas:    modelService.Spec.Replicas,
		ReadyReplicas:      modelService.Status.ReadyReplicas,
		Conditions:         conditions,
	}

	if modelService.Spec.Exposure != nil &&
		modelService.Spec.Exposure.Enabled &&
		modelService.Spec.Exposure.Hostname != "" {

		result.Endpoint =
			"https://" +
				modelService.Spec.Exposure.Hostname
	}

	return result
}
