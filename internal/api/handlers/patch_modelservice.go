package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	platformv1alpha1 "github.com/anselem-okeke/ai-platform-operator/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/anselem-okeke/ai-platform-operator/internal/api/middleware"
	apirequest "github.com/anselem-okeke/ai-platform-operator/internal/api/request"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/response"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/validation"
)

type PatchModelServiceHandler struct {
	logger      *slog.Logger
	store       ModelServiceUpdateStore
	maxReplicas int
	defaults    ModelServiceDefaults
}

func NewPatchModelServiceHandler(
	logger *slog.Logger,
	store ModelServiceUpdateStore,
	maxReplicas int,
	defaults ModelServiceDefaults,
) *PatchModelServiceHandler {
	return &PatchModelServiceHandler{
		logger:      logger,
		store:       store,
		maxReplicas: maxReplicas,
		defaults:    defaults,
	}
}

func (h *PatchModelServiceHandler) ServeHTTP(
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
					Code:      codeInvalidModelServiceName,
					Message:   messageModelServiceNameRequired,
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

	var request apirequest.PatchModelServiceRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&request); err != nil {
		var maxBytesError *http.MaxBytesError

		if errors.As(err, &maxBytesError) {
			response.WriteJSON(
				w,
				http.StatusRequestEntityTooLarge,
				response.APIError{
					Error: response.ErrorBody{
						Code:      codeRequestTooLarge,
						Message:   messageRequestBodyTooLarge,
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
					Code:      codeInvalidJSON,
					Message:   messageRequestBodyInvalidJSON,
					RequestID: middleware.RequestIDFromContext(r.Context()),
				},
			},
		)

		return
	}

	if err := ensureSingleJSONValue(decoder); err != nil {
		response.WriteJSON(
			w,
			http.StatusBadRequest,
			response.APIError{
				Error: response.ErrorBody{
					Code:      codeInvalidJSON,
					Message:   messageRequestBodySingleJSONObject,
					RequestID: middleware.RequestIDFromContext(r.Context()),
				},
			},
		)

		return
	}

	if !hasPatchFields(request) {
		response.WriteJSON(
			w,
			http.StatusBadRequest,
			response.APIError{
				Error: response.ErrorBody{
					Code:      "EMPTY_PATCH",
					Message:   "request must contain at least one field to update",
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
			"get_modelservice_for_patch_failed",
			slog.String(
				"request_id",
				middleware.RequestIDFromContext(r.Context()),
			),
			slog.String("model_service", name),
			slog.String("error", err.Error()),
		)

		response.WriteJSON(
			w,
			http.StatusServiceUnavailable,
			response.APIError{
				Error: response.ErrorBody{
					Code:      codeKubernetesUnavailable,
					Message:   messageUnableToLoadModelService,
					RequestID: middleware.RequestIDFromContext(r.Context()),
				},
			},
		)

		return
	}

	candidate := modelService.DeepCopy()

	applyPatchRequest(
		candidate,
		request,
		h.defaults,
	)

	updateRequest :=
		modelServiceToUpdateRequest(candidate)

	details :=
		validation.ValidateUpdateModelService(
			name,
			updateRequest,
			h.maxReplicas,
		)

	if len(details) > 0 {
		response.WriteJSON(
			w,
			http.StatusBadRequest,
			response.APIError{
				Error: response.ErrorBody{
					Code:      codeValidationFailed,
					Message:   messageRequestValidationFailed,
					RequestID: middleware.RequestIDFromContext(r.Context()),
					Details:   details,
				},
			},
		)

		return
	}

	if err := h.store.Update(
		r.Context(),
		candidate,
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
			"patch_modelservice_failed",
			slog.String(
				"request_id",
				middleware.RequestIDFromContext(r.Context()),
			),
			slog.String("model_service", name),
			slog.String("error", err.Error()),
		)

		response.WriteJSON(
			w,
			http.StatusServiceUnavailable,
			response.APIError{
				Error: response.ErrorBody{
					Code:      codeKubernetesUnavailable,
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
		modelServiceToResponse(*candidate),
	)
}

func hasPatchFields(
	request apirequest.PatchModelServiceRequest,
) bool {
	return request.Image != nil ||
		request.Replicas != nil ||
		request.Port != nil ||
		request.Exposure != nil ||
		request.Storage != nil
}

func applyPatchRequest(
	modelService *platformv1alpha1.ModelService,
	request apirequest.PatchModelServiceRequest,
	defaults ModelServiceDefaults,
) {
	if request.Image != nil {
		modelService.Spec.Image =
			*request.Image
	}

	if request.Replicas != nil {
		modelService.Spec.Replicas =
			*request.Replicas
	}

	if request.Port != nil {
		modelService.Spec.Port =
			*request.Port
	}

	if request.Exposure != nil {
		applyExposurePatch(
			modelService,
			*request.Exposure,
			defaults,
		)
	}

	if request.Storage != nil {
		applyStoragePatch(
			modelService,
			*request.Storage,
		)
	}
}

func applyExposurePatch(
	modelService *platformv1alpha1.ModelService,
	request apirequest.PatchExposureRequest,
	defaults ModelServiceDefaults,
) {
	if modelService.Spec.Exposure == nil {
		modelService.Spec.Exposure =
			&platformv1alpha1.ModelServiceExposure{
				Enabled:                   false,
				GatewayName:               defaults.GatewayName,
				GatewayNamespace:          defaults.GatewayNamespace,
				GatewaySectionName:        defaults.GatewaySectionName,
				GatewayDataPlaneNamespace: defaults.GatewayDataPlaneNamespace,
			}
	}

	exposure := modelService.Spec.Exposure

	if request.Enabled != nil {
		exposure.Enabled =
			*request.Enabled
	}

	if request.Hostname != nil {
		exposure.Hostname =
			*request.Hostname
	}

	if request.PathPrefix != nil {
		exposure.PathPrefix =
			*request.PathPrefix
	}

	if exposure.Enabled &&
		exposure.PathPrefix == "" {
		exposure.PathPrefix = "/"
	}

	if exposure.GatewayName == "" {
		exposure.GatewayName =
			defaults.GatewayName
	}

	if exposure.GatewayNamespace == "" {
		exposure.GatewayNamespace =
			defaults.GatewayNamespace
	}

	if exposure.GatewaySectionName == "" {
		exposure.GatewaySectionName =
			defaults.GatewaySectionName
	}

	if exposure.GatewayDataPlaneNamespace == "" {
		exposure.GatewayDataPlaneNamespace =
			defaults.GatewayDataPlaneNamespace
	}
}

func applyStoragePatch(
	modelService *platformv1alpha1.ModelService,
	request apirequest.PatchStorageRequest,
) {
	if modelService.Spec.Storage == nil {
		modelService.Spec.Storage =
			&platformv1alpha1.ModelServiceStorage{
				Enabled: false,
			}
	}

	storage := modelService.Spec.Storage

	if request.Enabled != nil {
		storage.Enabled =
			*request.Enabled
	}

	if request.Size != nil {
		storage.Size =
			*request.Size
	}

	if request.MountPath != nil {
		storage.MountPath =
			*request.MountPath
	}
}

func modelServiceToUpdateRequest(
	modelService *platformv1alpha1.ModelService,
) apirequest.UpdateModelServiceRequest {
	request :=
		apirequest.UpdateModelServiceRequest{
			Image:    modelService.Spec.Image,
			Replicas: modelService.Spec.Replicas,
			Port:     modelService.Spec.Port,
		}

	if modelService.Spec.Exposure != nil {
		request.Exposure =
			apirequest.ExposureRequest{
				Enabled:    modelService.Spec.Exposure.Enabled,
				Hostname:   modelService.Spec.Exposure.Hostname,
				PathPrefix: modelService.Spec.Exposure.PathPrefix,
			}
	}

	if modelService.Spec.Storage != nil {
		request.Storage =
			apirequest.StorageRequest{
				Enabled:   modelService.Spec.Storage.Enabled,
				Size:      modelService.Spec.Storage.Size,
				MountPath: modelService.Spec.Storage.MountPath,
			}
	}

	return request
}
