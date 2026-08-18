package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	platformv1alpha1 "github.com/anselem-okeke/ai-platform-operator/api/v1alpha1"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/middleware"
	apirequest "github.com/anselem-okeke/ai-platform-operator/internal/api/request"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/response"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/validation"
)

const maxRequestBodyBytes = 1 << 20

type ModelServiceCreator interface {
	Create(
		context.Context,
		*platformv1alpha1.ModelService,
	) error
}

type CreateModelServiceHandler struct {
	logger      *slog.Logger
	store       ModelServiceCreator
	maxReplicas int
	defaults    ModelServiceDefaults
}

func NewCreateModelServiceHandler(
	logger *slog.Logger,
	store ModelServiceCreator,
	maxReplicas int,
	defaults ModelServiceDefaults,
) *CreateModelServiceHandler {
	return &CreateModelServiceHandler{
		logger:      logger,
		store:       store,
		maxReplicas: maxReplicas,
		defaults:    defaults,
	}
}

func (h *CreateModelServiceHandler) ServeHTTP(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodPost {
		w.Header().Set(
			"Allow",
			http.MethodGet+", "+http.MethodPost,
		)

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

	if contentType := r.Header.Get(
		"Content-Type",
	); contentType != "" &&
		contentType != "application/json" {
		response.WriteJSON(
			w,
			http.StatusUnsupportedMediaType,
			response.APIError{
				Error: response.ErrorBody{
					Code:      "UNSUPPORTED_MEDIA_TYPE",
					Message:   "Content-Type must be application/json",
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

	var request apirequest.CreateModelServiceRequest

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
		validation.ValidateCreateModelService(
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

	modelService :=
		createRequestToModelService(
			request,
			h.defaults,
		)

	if err := h.store.Create(
		r.Context(),
		modelService,
	); err != nil {
		if apierrors.IsAlreadyExists(err) {
			response.WriteJSON(
				w,
				http.StatusConflict,
				response.APIError{
					Error: response.ErrorBody{
						Code: "MODEL_SERVICE_ALREADY_EXISTS",
						Message: "ModelService \"" +
							request.Name +
							"\" already exists",
						RequestID: middleware.RequestIDFromContext(r.Context()),
					},
				},
			)

			return
		}

		h.logger.ErrorContext(
			r.Context(),
			"create_modelservice_failed",
			slog.String(
				"request_id",
				middleware.RequestIDFromContext(r.Context()),
			),
			slog.String(
				"model_service",
				request.Name,
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
					Message:   "unable to create ModelService",
					RequestID: middleware.RequestIDFromContext(r.Context()),
				},
			},
		)

		return
	}

	response.WriteJSON(
		w,
		http.StatusCreated,
		modelServiceToResponse(
			*modelService,
		),
	)
}

func ensureSingleJSONValue(
	decoder *json.Decoder,
) error {
	var extra any

	err := decoder.Decode(&extra)

	if errors.Is(
		err,
		io.EOF,
	) {
		return nil
	}

	if err == nil {
		return errors.New(
			"multiple JSON values",
		)
	}

	return err
}

func createRequestToModelService(
	request apirequest.CreateModelServiceRequest,
	defaults ModelServiceDefaults,
) *platformv1alpha1.ModelService {
	modelService := &platformv1alpha1.ModelService{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "platform.anselem.dev/v1alpha1",
			Kind:       "ModelService",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: request.Name,
		},
		Spec: platformv1alpha1.ModelServiceSpec{
			Image:    request.Image,
			Replicas: request.Replicas,
			Port:     request.Port,
		},
	}

	if request.Exposure.Enabled {
		pathPrefix := request.Exposure.PathPrefix
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
	}

	return modelService
}
