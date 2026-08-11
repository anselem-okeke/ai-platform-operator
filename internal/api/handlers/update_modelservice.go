package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	platformv1alpha1 "github.com/anselem-okeke/ai-platform-operator/api/v1alpha1"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/middleware"
	apirequest "github.com/anselem-okeke/ai-platform-operator/internal/api/request"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/response"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/validation"
)

type ModelServiceUpdater interface {
	Update(
		context.Context,
		*platformv1alpha1.ModelService,
	) error
}

type ModelServiceUpdateStore interface {
	ModelServiceGetter
	ModelServiceUpdater
}

type UpdateModelServiceHandler struct {
	logger      *slog.Logger
	store       ModelServiceUpdateStore
	maxReplicas int
	defaults    ModelServiceDefaults
}

func NewUpdateModelServiceHandler(
	logger *slog.Logger,
	store ModelServiceUpdateStore,
	maxReplicas int,
	defaults ModelServiceDefaults,
) *UpdateModelServiceHandler {
	return &UpdateModelServiceHandler{
		logger:      logger,
		store:       store,
		maxReplicas: maxReplicas,
		defaults:    defaults,
	}
}

func (h *UpdateModelServiceHandler) ServeHTTP(
	w http.ResponseWriter,
	r *http.Request,
) {
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

	r.Body = http.MaxBytesReader(
		w,
		r.Body,
		maxRequestBodyBytes,
	)

	var request apirequest.UpdateModelServiceRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&request); err != nil {
		var maxBytesError *http.MaxBytesError

		if errors.As(
			err,
			&maxBytesError,
		) {
			response.WriteJSON(
				w,
				http.StatusRequestEntityTooLarge,
				response.APIError{
					Error: response.ErrorBody{
						Code:      "REQUEST_TOO_LARGE",
						Message:   "request body is too large",
						RequestID: middleware.RequestIDFromContext(r.Context()),
					},
				},
			)

			return
		}

		response.WriteJSON(
			w,
			http.StatusBadRequest,
			response.APIError{
				Error: response.ErrorBody{
					Code:      "INVALID_JSON",
					Message:   "request body contains invalid JSON",
					RequestID: middleware.RequestIDFromContext(r.Context()),
				},
			},
		)

		return
	}

	if err := ensureSingleJSONValue(
		decoder,
	); err != nil {
		response.WriteJSON(
			w,
			http.StatusBadRequest,
			response.APIError{
				Error: response.ErrorBody{
					Code:      "INVALID_JSON",
					Message:   "request body must contain one JSON object",
					RequestID: middleware.RequestIDFromContext(r.Context()),
				},
			},
		)

		return
	}

	details :=
		validation.ValidateUpdateModelService(
			name,
			request,
			h.maxReplicas,
		)

	if len(details) > 0 {
		response.WriteJSON(
			w,
			http.StatusBadRequest,
			response.APIError{
				Error: response.ErrorBody{
					Code:      "VALIDATION_FAILED",
					Message:   "request validation failed",
					RequestID: middleware.RequestIDFromContext(r.Context()),
					Details:   details,
				},
			},
		)

		return
	}

	modelService, err :=
		h.store.Get(
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
			"get_modelservice_for_update_failed",
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
					Message:   "unable to load ModelService",
					RequestID: middleware.RequestIDFromContext(r.Context()),
				},
			},
		)

		return
	}

	applyUpdateRequest(
		modelService,
		request,
		h.defaults,
	)

	if err := h.store.Update(
		r.Context(),
		modelService,
	); err != nil {
		if apierrors.IsConflict(err) {
			response.WriteJSON(
				w,
				http.StatusConflict,
				response.APIError{
					Error: response.ErrorBody{
						Code:      "MODEL_SERVICE_UPDATE_CONFLICT",
						Message:   "ModelService was modified concurrently; retry the request",
						RequestID: middleware.RequestIDFromContext(r.Context()),
					},
				},
			)

			return
		}

		h.logger.ErrorContext(
			r.Context(),
			"update_modelservice_failed",
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
					Message:   "unable to update ModelService",
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

func applyUpdateRequest(
	modelService *platformv1alpha1.ModelService,
	request apirequest.UpdateModelServiceRequest,
	defaults ModelServiceDefaults,
) {
	modelService.Spec.Image =
		request.Image

	modelService.Spec.Replicas =
		request.Replicas

	modelService.Spec.Port =
		request.Port

	if request.Exposure.Enabled {
		pathPrefix :=
			request.Exposure.PathPrefix

		if pathPrefix == "" {
			pathPrefix = "/"
		}

		modelService.Spec.Exposure =
			&platformv1alpha1.ModelServiceExposure{
				Enabled:                   true,
				Hostname:                  request.Exposure.Hostname,
				PathPrefix:                pathPrefix,
				GatewayName:               defaults.GatewayName,
				GatewayNamespace:          defaults.GatewayNamespace,
				GatewaySectionName:        defaults.GatewaySectionName,
				GatewayDataPlaneNamespace: defaults.GatewayDataPlaneNamespace,
			}
	} else {
		modelService.Spec.Exposure =
			&platformv1alpha1.ModelServiceExposure{
				Enabled: false,
			}
	}

	if request.Storage.Enabled {
		modelService.Spec.Storage =
			&platformv1alpha1.ModelServiceStorage{
				Enabled:   true,
				Size:      request.Storage.Size,
				MountPath: request.Storage.MountPath,
			}
	} else {
		modelService.Spec.Storage =
			&platformv1alpha1.ModelServiceStorage{
				Enabled: false,
			}
	}
}
