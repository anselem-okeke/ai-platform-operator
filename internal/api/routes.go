package api

import (
	"log/slog"
	"net/http"

	"github.com/anselem-okeke/ai-platform-operator/internal/api/auth"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/handlers"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func newRouter(
	logger *slog.Logger,
	verifier auth.Verifier,
	readinessHandler *handlers.ReadinessHandler,
	listModelServicesHandler *handlers.ListModelServicesHandler,
	getModelServiceHandler *handlers.GetModelServiceHandler,
	getModelServiceStatusHandler *handlers.GetModelServiceStatusHandler,
	createModelServiceHandler *handlers.CreateModelServiceHandler,
	updateModelServiceHandler *handlers.UpdateModelServiceHandler,
	patchModelServiceHandler *handlers.PatchModelServiceHandler,
	deleteModelServiceHandler *handlers.DeleteModelServiceHandler,
) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc(
		"/healthz",
		handlers.Health,
	)

	mux.Handle(
		"/readyz",
		readinessHandler,
	)

	mux.Handle(
		"/metrics",
		promhttp.Handler(),
	)

	protected := http.NewServeMux()

	viewerOnly := middleware.RequireAnyRole(
		logger,
		auth.RoleModelViewer,
	)

	deployerOnly := middleware.RequireAnyRole(
		logger,
		auth.RoleModelDeployer,
	)

	adminOnly := middleware.RequireAnyRole(
		logger,
		auth.RolePlatformAdmin,
	)

	collectionHandler :=
		handlers.NewModelServiceCollectionHandler(
			viewerOnly(
				listModelServicesHandler,
			),
			deployerOnly(
				createModelServiceHandler,
			),
		)

	protected.Handle(
		"/api/v1/model-services",
		collectionHandler,
	)

	protected.Handle(
		"/api/v1/model-services/{name}/status",
		viewerOnly(
			getModelServiceStatusHandler,
		),
	)

	resourceHandler :=
		handlers.NewModelServiceResourceHandler(
			viewerOnly(
				getModelServiceHandler,
			),
			deployerOnly(
				updateModelServiceHandler,
			),
			deployerOnly(
				patchModelServiceHandler,
			),
			adminOnly(
				deleteModelServiceHandler,
			),
		)

	protected.Handle(
		"/api/v1/model-services/{name}",
		resourceHandler,
	)

	mux.Handle(
		"/api/v1/",
		middleware.Authentication(
			logger,
			verifier,
		)(
			middleware.AuditLogging(
				logger,
			)(
				protected,
			),
		),
	)

	return middleware.Chain(
		mux,
		middleware.RequestID,
		middleware.RequestLogging(logger),
		middleware.RequestMetrics,
	)
}
