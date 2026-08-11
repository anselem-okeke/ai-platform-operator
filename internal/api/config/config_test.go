package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("HTTP_ADDRESS", "")
	t.Setenv("MODEL_SERVICE_NAMESPACE", "")
	t.Setenv("MAX_MODEL_REPLICAS", "")
	t.Setenv("OIDC_ISSUER", "")
	t.Setenv("OIDC_AUDIENCE", "")
	t.Setenv("OIDC_CA_FILE", "")
	t.Setenv("LOG_LEVEL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.HTTPAddress != ":8080" {
		t.Fatalf(
			"expected HTTPAddress :8080, got %q",
			cfg.HTTPAddress,
		)
	}

	if cfg.ModelServiceNamespace != "ai-platform" {
		t.Fatalf(
			"expected namespace ai-platform, got %q",
			cfg.ModelServiceNamespace,
		)
	}

	if cfg.MaxModelReplicas != 10 {
		t.Fatalf(
			"expected max replicas 10, got %d",
			cfg.MaxModelReplicas,
		)
	}

	if cfg.OIDCIssuer != "https://auth.ai-platform.local/realms/ai-platform" {
		t.Fatalf(
			"unexpected issuer %q",
			cfg.OIDCIssuer,
		)
	}

	if cfg.OIDCCAFile != ".local/keycloak/auth-ai-platform-root-ca.crt" {
		t.Fatalf(
			"unexpected OIDC CA file %q",
			cfg.OIDCCAFile,
		)
	}

	if cfg.OIDCAudience != "ai-platform-gateway" {
		t.Fatalf(
			"unexpected audience %q",
			cfg.OIDCAudience,
		)
	}

	if cfg.LogLevel != "info" {
		t.Fatalf(
			"expected log level info, got %q",
			cfg.LogLevel,
		)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("HTTP_ADDRESS", ":9090")
	t.Setenv("MODEL_SERVICE_NAMESPACE", "models")
	t.Setenv("MAX_MODEL_REPLICAS", "25")
	t.Setenv(
		"OIDC_ISSUER",
		"https://identity.example.test/realms/platform",
	)
	t.Setenv("OIDC_AUDIENCE", "platform-api")
	t.Setenv(
		"OIDC_CA_FILE",
		"/etc/ai-platform/pki/keycloak-ca.crt",
	)
	t.Setenv("LOG_LEVEL", "DEBUG")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.HTTPAddress != ":9090" {
		t.Fatalf(
			"expected HTTPAddress :9090, got %q",
			cfg.HTTPAddress,
		)
	}

	if cfg.ModelServiceNamespace != "models" {
		t.Fatalf(
			"expected namespace models, got %q",
			cfg.ModelServiceNamespace,
		)
	}

	if cfg.MaxModelReplicas != 25 {
		t.Fatalf(
			"expected max replicas 25, got %d",
			cfg.MaxModelReplicas,
		)
	}

	if cfg.OIDCAudience != "platform-api" {
		t.Fatalf(
			"unexpected audience %q",
			cfg.OIDCAudience,
		)
	}

	if cfg.OIDCCAFile != "/etc/ai-platform/pki/keycloak-ca.crt" {
		t.Fatalf(
			"unexpected OIDC CA file %q",
			cfg.OIDCCAFile,
		)
	}

	if cfg.LogLevel != "debug" {
		t.Fatalf(
			"expected log level debug, got %q",
			cfg.LogLevel,
		)
	}
}

func TestLoadRejectsInvalidReplicaLimit(t *testing.T) {
	t.Setenv("MAX_MODEL_REPLICAS", "not-a-number")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid replica limit")
	}
}

func TestLoadRejectsZeroReplicaLimit(t *testing.T) {
	t.Setenv("MAX_MODEL_REPLICAS", "0")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for zero replica limit")
	}
}

func TestLoadRejectsInvalidLogLevel(t *testing.T) {
	t.Setenv("LOG_LEVEL", "verbose")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid log level")
	}
}
