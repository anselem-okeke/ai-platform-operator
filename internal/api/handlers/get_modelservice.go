package handlers

import (
	"context"
	"log/slog"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	platformv1alpha1 "github.com/anselem-okeke/ai-platform-operator/api/v1alpha1"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/middleware"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/response"
)

type ModelServiceGetter interface {
	Get(
		context.Context,
		string,
	) (*platformv1alpha1.ModelService, error)
}

type GetModelServiceHandler struct {
	logger *slog.Logger
	store  ModelServiceGetter
}

func NewGetModelServiceHandler(
	logger *slog.Logger,
	store ModelServiceGetter,
) *GetModelServiceHandler {
	return &GetModelServiceHandler{
		logger: logger,
		store:  store,
	}
}

func (h *GetModelServiceHandler) ServeHTTP(
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
			"get_modelservice_failed",
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
					Message:   "unable to get ModelService",
					RequestID: middleware.RequestIDFromContext(r.Context()),
				},
			},
		)

		return
	}

	response.WriteJSON(
		w,
		http.StatusOK,
		modelServiceToResponse(
			*modelService,
		),
	)
}

func modelServiceToResponse(
	modelService platformv1alpha1.ModelService,
) response.ModelServiceResponse {
	result := response.ModelServiceResponse{
		APIVersion: "v1",
		Kind:       "ModelService",
		Name:       modelService.Name,
		Image:      modelService.Spec.Image,
		Replicas:   modelService.Spec.Replicas,
		Port:       modelService.Spec.Port,
		State:      modelServiceState(modelService),
		Generation: modelService.Generation,
	}

	if modelService.Spec.Exposure != nil {
		result.Exposure.Enabled =
			modelService.Spec.Exposure.Enabled

		if modelService.Spec.Exposure.Enabled {
			result.Exposure.Hostname =
				modelService.Spec.Exposure.Hostname

			result.Exposure.PathPrefix =
				modelService.Spec.Exposure.PathPrefix
		}
	}

	if modelService.Spec.Storage != nil {
		result.Storage.Enabled =
			modelService.Spec.Storage.Enabled

		if modelService.Spec.Storage.Enabled {
			result.Storage.Size =
				modelService.Spec.Storage.Size

			result.Storage.MountPath =
				modelService.Spec.Storage.MountPath
		}
	}

	return result
}
