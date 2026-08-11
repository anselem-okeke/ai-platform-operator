package handlers

import (
	"context"
	"log/slog"
	"net/http"

	platformv1alpha1 "github.com/anselem-okeke/ai-platform-operator/api/v1alpha1"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/middleware"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/response"
)

type ModelServiceLister interface {
	List(
		context.Context,
	) ([]platformv1alpha1.ModelService, error)
}

type ListModelServicesHandler struct {
	logger *slog.Logger
	store  ModelServiceLister
}

func NewListModelServicesHandler(
	logger *slog.Logger,
	store ModelServiceLister,
) *ListModelServicesHandler {
	return &ListModelServicesHandler{
		logger: logger,
		store:  store,
	}
}

func (h *ListModelServicesHandler) ServeHTTP(
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

	modelServices, err := h.store.List(r.Context())
	if err != nil {
		h.logger.ErrorContext(
			r.Context(),
			"list_modelservices_failed",
			slog.String(
				"request_id",
				middleware.RequestIDFromContext(r.Context()),
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
					Message:   "unable to list ModelServices",
					RequestID: middleware.RequestIDFromContext(r.Context()),
				},
			},
		)

		return
	}

	items := make(
		[]response.ModelServiceSummary,
		0,
		len(modelServices),
	)

	for _, modelService := range modelServices {
		items = append(
			items,
			modelServiceToSummary(modelService),
		)
	}

	response.WriteJSON(
		w,
		http.StatusOK,
		response.ModelServiceListResponse{
			Items: items,
			Count: len(items),
		},
	)
}

func modelServiceToSummary(
	modelService platformv1alpha1.ModelService,
) response.ModelServiceSummary {
	summary := response.ModelServiceSummary{
		Name:     modelService.Name,
		Image:    modelService.Spec.Image,
		Replicas: modelService.Spec.Replicas,
		State:    modelServiceState(modelService),
	}

	if modelService.Spec.Exposure != nil &&
		modelService.Spec.Exposure.Enabled {

		summary.Hostname =
			modelService.Spec.Exposure.Hostname
	}

	return summary
}

func modelServiceState(
	modelService platformv1alpha1.ModelService,
) string {
	if modelService.DeletionTimestamp != nil {
		return "Deleting"
	}

	if modelService.Status.Phase != "" {
		return modelService.Status.Phase
	}

	return "Pending"
}
