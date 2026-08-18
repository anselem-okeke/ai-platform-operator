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

type ModelServiceDeleter interface {
	Delete(
		context.Context,
		*platformv1alpha1.ModelService,
	) error
}

type ModelServiceDeleteStore interface {
	ModelServiceGetter
	ModelServiceDeleter
}

type DeleteModelServiceHandler struct {
	logger *slog.Logger
	store  ModelServiceDeleteStore
}

func NewDeleteModelServiceHandler(
	logger *slog.Logger,
	store ModelServiceDeleteStore,
) *DeleteModelServiceHandler {
	return &DeleteModelServiceHandler{
		logger: logger,
		store:  store,
	}
}

func (h *DeleteModelServiceHandler) ServeHTTP(
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
			"get_modelservice_for_delete_failed",
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
					Message:   messageUnableToLoadModelService,
					RequestID: middleware.RequestIDFromContext(r.Context()),
				},
			},
		)

		return
	}

	if err := h.store.Delete(
		r.Context(),
		modelService,
	); err != nil {
		if apierrors.IsNotFound(err) {
			w.WriteHeader(http.StatusNoContent)

			return
		}

		h.logger.ErrorContext(
			r.Context(),
			"delete_modelservice_failed",
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
					Message:   "unable to delete ModelService",
					RequestID: middleware.RequestIDFromContext(r.Context()),
				},
			},
		)

		return
	}

	h.logger.InfoContext(
		r.Context(),
		"modelservice_delete_requested",
		slog.String(
			"request_id",
			middleware.RequestIDFromContext(r.Context()),
		),
		slog.String(
			"model_service",
			name,
		),
	)

	w.WriteHeader(http.StatusNoContent)
}
