package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/anselem-okeke/ai-platform-operator/internal/api/auth"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/handlers"
)

const (
	defaultReadHeaderTimeout = 5 * time.Second
	defaultReadTimeout       = 15 * time.Second
	defaultWriteTimeout      = 30 * time.Second
	defaultIdleTimeout       = 60 * time.Second
)

type ModelServiceStore interface {
	handlers.ModelServiceLister
	handlers.ModelServiceGetter
	handlers.ModelServiceCreator
	handlers.ModelServiceUpdater
	handlers.ModelServiceDeleter
}

type Server struct {
	httpServer *http.Server
}

func NewServer(
	address string,
	logger *slog.Logger,
	verifier auth.Verifier,
	readinessChecker handlers.ReadinessChecker,
	modelServiceStore ModelServiceStore,
	maxModelReplicas int,
	modelServiceDefaults handlers.ModelServiceDefaults,
) *Server {
	readinessHandler := handlers.NewReadinessHandler(
		readinessChecker,
	)

	listModelServicesHandler :=
		handlers.NewListModelServicesHandler(
			logger,
			modelServiceStore,
		)

	getModelServiceHandler :=
		handlers.NewGetModelServiceHandler(
			logger,
			modelServiceStore,
		)

	getModelServiceStatusHandler :=
		handlers.NewGetModelServiceStatusHandler(
			logger,
			modelServiceStore,
		)
	createModelServiceHandler :=
		handlers.NewCreateModelServiceHandler(
			logger,
			modelServiceStore,
			maxModelReplicas,
			modelServiceDefaults,
		)

	updateModelServiceHandler :=
		handlers.NewUpdateModelServiceHandler(
			logger,
			modelServiceStore,
			maxModelReplicas,
			modelServiceDefaults,
		)

	patchModelServiceHandler :=
		handlers.NewPatchModelServiceHandler(
			logger,
			modelServiceStore,
			maxModelReplicas,
			modelServiceDefaults,
		)

	deleteModelServiceHandler :=
		handlers.NewDeleteModelServiceHandler(
			logger,
			modelServiceStore,
		)

	return &Server{
		httpServer: &http.Server{
			Addr: address,
			Handler: newRouter(
				logger,
				verifier,
				readinessHandler,
				listModelServicesHandler,
				getModelServiceHandler,
				getModelServiceStatusHandler,
				createModelServiceHandler,
				updateModelServiceHandler,
				patchModelServiceHandler,
				deleteModelServiceHandler,
			),
			ReadHeaderTimeout: defaultReadHeaderTimeout,
			ReadTimeout:       defaultReadTimeout,
			WriteTimeout:      defaultWriteTimeout,
			IdleTimeout:       defaultIdleTimeout,
		},
	}
}

func (s *Server) HTTPServer() *http.Server {
	return s.httpServer
}
