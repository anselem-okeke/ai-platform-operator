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
					Code:      codeMethodNotAllowed,
					Message:   messageMethodNotAllowed,
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
					Code:      codeInvalidModelServiceName,
					Message:   messageModelServiceNameRequired,
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
						Code: codeModelServiceNotFound,
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
					Code:      codeKubernetesUnavailable,
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
	backend := normalizedModelServiceBackend(
		modelService.Spec.Backend,
	)

	result := response.ModelServiceResponse{
		APIVersion: "v1",
		Kind:       "ModelService",
		Name:       modelService.Name,
		Backend:    backend,
		Replicas:   modelService.Spec.Replicas,
		State:      modelServiceState(modelService),
		Generation: modelService.Generation,
	}

	if backend == string(
		platformv1alpha1.ModelServiceBackendDeployment,
	) {
		result.Image = modelService.Spec.Image
		result.Port = modelService.Spec.Port
	}

	if modelService.Spec.Predictor != nil {
		result.Predictor =
			&response.ModelServicePredictor{
				ModelFormat: response.ModelServiceModelFormat{
					Name: modelService.Spec.Predictor.
						ModelFormat.Name,
					Version: modelService.Spec.Predictor.
						ModelFormat.Version,
				},
				Runtime: modelService.Spec.Predictor.Runtime,
				StorageURI: modelService.Spec.Predictor.
					StorageURI,
				ServiceAccountName: modelService.Spec.Predictor.
					ServiceAccountName,
			}
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

func normalizedModelServiceBackend(
	backend platformv1alpha1.ModelServiceBackend,
) string {
	if backend == "" {
		return string(
			platformv1alpha1.
				ModelServiceBackendDeployment,
		)
	}

	return string(backend)
}
