package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/anselem-okeke/ai-platform-operator/internal/api"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/auth"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/config"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/handlers"
	apikubernetes "github.com/anselem-okeke/ai-platform-operator/internal/api/kubernetes"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/logging"
)

const shutdownTimeout = 10 * time.Second

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error(
			"configuration error",
			slog.String("error", err.Error()),
		)

		os.Exit(1)
	}

	logger, err := logging.New(cfg.LogLevel)
	if err != nil {
		slog.Error(
			"logger initialization failed",
			slog.String("error", err.Error()),
		)

		os.Exit(1)
	}

	slog.SetDefault(logger)

	oidcVerifier, err := auth.NewOIDCVerifier(
		context.Background(),
		cfg.OIDCIssuer,
		cfg.OIDCAudience,
		cfg.OIDCCAFile,
	)
	if err != nil {
		logger.Error(
			"oidc_verifier_initialization_failed",
			slog.String(
				"issuer",
				cfg.OIDCIssuer,
			),
			slog.String(
				"error",
				err.Error(),
			),
		)

		os.Exit(1)
	}

	logger.Info(
		"oidc_verifier_initialized",
		slog.String(
			"issuer",
			cfg.OIDCIssuer,
		),
		slog.String(
			"audience",
			cfg.OIDCAudience,
		),
		slog.String(
			"ca_file",
			cfg.OIDCCAFile,
		),
	)

	kubernetesClients, err := apikubernetes.NewClient()
	if err != nil {
		logger.Error(
			"kubernetes_client_initialization_failed",
			slog.String(
				"error",
				err.Error(),
			),
		)

		os.Exit(1)
	}

	logger.Info(
		"kubernetes_client_initialized",
		slog.String(
			"model_service_namespace",
			cfg.ModelServiceNamespace,
		),
	)

	readinessChecker :=
		apikubernetes.NewReadinessChecker(
			kubernetesClients.Client,
			cfg.ModelServiceNamespace,
		)

	modelServiceStore :=
		apikubernetes.NewModelServiceStore(
			kubernetesClients.Client,
			cfg.ModelServiceNamespace,
		)

	modelServiceDefaults :=
		handlers.ModelServiceDefaults{
			GatewayName: cfg.ModelGatewayName,

			GatewayNamespace: cfg.ModelGatewayNamespace,

			GatewaySectionName: cfg.ModelGatewaySectionName,

			GatewayDataPlaneNamespace: cfg.ModelGatewayDataPlaneNamespace,
		}

	server := api.NewServer(
		cfg.HTTPAddress,
		logger,
		oidcVerifier,
		readinessChecker,
		modelServiceStore,
		cfg.MaxModelReplicas,
		modelServiceDefaults,
	)

	errCh := make(chan error, 1)

	go func() {
		logger.Info(
			"api_server_starting",
			slog.String(
				"address",
				cfg.HTTPAddress,
			),
			slog.String(
				"model_service_namespace",
				cfg.ModelServiceNamespace,
			),
			slog.Int(
				"max_model_replicas",
				cfg.MaxModelReplicas,
			),
			slog.String(
				"oidc_issuer",
				cfg.OIDCIssuer,
			),
			slog.String(
				"oidc_audience",
				cfg.OIDCAudience,
			),
		)

		if err := server.HTTPServer().ListenAndServe(); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	signalContext, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	select {
	case <-signalContext.Done():
		logger.Info(
			"shutdown_signal_received",
		)

	case err := <-errCh:
		logger.Error(
			"http_server_failed",
			slog.String(
				"error",
				err.Error(),
			),
		)

		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		shutdownTimeout,
	)
	defer cancel()

	if err := server.HTTPServer().Shutdown(ctx); err != nil {
		logger.Error(
			"graceful_shutdown_failed",
			slog.String(
				"error",
				err.Error(),
			),
		)

		if closeErr := server.HTTPServer().Close(); closeErr != nil {
			logger.Error(
				"forced_shutdown_failed",
				slog.String(
					"error",
					closeErr.Error(),
				),
			)
		}
	}

	logger.Info(
		"api_server_stopped",
	)
}
