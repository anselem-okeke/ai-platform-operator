package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	defaultHTTPAddress             = ":8080"
	defaultModelServiceNamespace   = "ai-platform"
	defaultMaxModelReplicas        = 10
	defaultOIDCIssuer              = "https://auth.ai-platform.local/realms/ai-platform"
	defaultOIDCAudience            = "ai-platform-gateway"
	defaultOIDCCAFile              = ".local/keycloak/auth-ai-platform-root-ca.crt"
	defaultLogLevel                = "info"
	defaultModelGatewayName        = "shared-gateway"
	defaultModelGatewayNamespace   = "gateway-system"
	defaultModelGatewaySectionName = "fraud-model-https"
	defaultModelGatewayDataPlaneNS = "envoy-gateway-system"
)

type Config struct {
	HTTPAddress                    string
	ModelServiceNamespace          string
	MaxModelReplicas               int
	OIDCIssuer                     string
	OIDCAudience                   string
	OIDCCAFile                     string
	LogLevel                       string
	ModelGatewayName               string
	ModelGatewayNamespace          string
	ModelGatewaySectionName        string
	ModelGatewayDataPlaneNamespace string
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddress:           envOrDefault("HTTP_ADDRESS", defaultHTTPAddress),
		ModelServiceNamespace: envOrDefault("MODEL_SERVICE_NAMESPACE", defaultModelServiceNamespace),

		ModelGatewayName: envOrDefault(
			"MODEL_GATEWAY_NAME",
			defaultModelGatewayName,
		),

		ModelGatewayNamespace: envOrDefault(
			"MODEL_GATEWAY_NAMESPACE",
			defaultModelGatewayNamespace,
		),

		ModelGatewaySectionName: envOrDefault(
			"MODEL_GATEWAY_SECTION_NAME",
			defaultModelGatewaySectionName,
		),

		ModelGatewayDataPlaneNamespace: envOrDefault(
			"MODEL_GATEWAY_DATAPLANE_NAMESPACE",
			defaultModelGatewayDataPlaneNS,
		),

		OIDCIssuer: envOrDefault(
			"OIDC_ISSUER",
			defaultOIDCIssuer,
		),

		OIDCAudience: envOrDefault(
			"OIDC_AUDIENCE",
			defaultOIDCAudience,
		),

		OIDCCAFile: envOrDefault(
			"OIDC_CA_FILE",
			defaultOIDCCAFile,
		),

		LogLevel: strings.ToLower(
			envOrDefault(
				"LOG_LEVEL",
				defaultLogLevel,
			),
		),
	}

	maxReplicasRaw := envOrDefault(
		"MAX_MODEL_REPLICAS",
		strconv.Itoa(defaultMaxModelReplicas),
	)

	maxReplicas, err := strconv.Atoi(maxReplicasRaw)
	if err != nil {
		return Config{}, fmt.Errorf(
			"MAX_MODEL_REPLICAS must be an integer: %w",
			err,
		)
	}

	cfg.MaxModelReplicas = maxReplicas

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.HTTPAddress) == "" {
		return fmt.Errorf("HTTP_ADDRESS must not be empty")
	}

	if strings.TrimSpace(c.ModelServiceNamespace) == "" {
		return fmt.Errorf(
			"MODEL_SERVICE_NAMESPACE must not be empty",
		)
	}

	if c.MaxModelReplicas < 1 {
		return fmt.Errorf(
			"MAX_MODEL_REPLICAS must be at least 1",
		)
	}

	if strings.TrimSpace(c.OIDCIssuer) == "" {
		return fmt.Errorf("OIDC_ISSUER must not be empty")
	}

	if strings.TrimSpace(c.OIDCAudience) == "" {
		return fmt.Errorf("OIDC_AUDIENCE must not be empty")
	}

	if strings.TrimSpace(c.OIDCCAFile) == "" {
		return fmt.Errorf(
			"OIDC_CA_FILE must not be empty",
		)
	}

	if strings.TrimSpace(c.ModelGatewayName) == "" {
		return fmt.Errorf(
			"MODEL_GATEWAY_NAME must not be empty",
		)
	}

	if strings.TrimSpace(c.ModelGatewayNamespace) == "" {
		return fmt.Errorf(
			"MODEL_GATEWAY_NAMESPACE must not be empty",
		)
	}

	if strings.TrimSpace(c.ModelGatewaySectionName) == "" {
		return fmt.Errorf(
			"MODEL_GATEWAY_SECTION_NAME must not be empty",
		)
	}

	if strings.TrimSpace(c.ModelGatewayDataPlaneNamespace) == "" {
		return fmt.Errorf(
			"MODEL_GATEWAY_DATAPLANE_NAMESPACE must not be empty",
		)
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf(
			"LOG_LEVEL must be one of debug, info, warn, error",
		)
	}

	return nil
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}
