package main

import (
	"log"
	"os"
)

// ──────────────────────────────────────────────
// Application Configuration
// ──────────────────────────────────────────────
// All configuration is loaded from environment variables.
// See .env.example for the full list of available variables.

// Config holds all application configuration.
type Config struct {
	// Server
	ListenAddr  string
	FrontendDir string

	// MLflow backend
	MLflowURL       string
	MLflowAdminUser string
	MLflowAdminPass string

	// Authentication
	JWTSecret  string
	CookieName string
	AuthDBURI  string

	// OIDC Single Sign-On
	OIDCIssuerURL    string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCRedirectURL  string
	OIDCAdminUser    string
}

// cfg is the global configuration instance, populated by loadConfig().
var cfg *Config

// Package-level convenience variables used throughout the codebase.
// These are populated by loadConfig() for backward compatibility.
var (
	mlflowURL  string
	jwtSecret  []byte
	listenAddr string
	cookieName string
	authDBURI  string
)

// loadConfig reads all configuration from environment variables,
// validates required values, and populates package-level variables.
// Must be called at the start of main().
func loadConfig() {
	cfg = &Config{
		// Server
		ListenAddr:  getEnv("LISTEN_ADDR", ":8080"),
		FrontendDir: getEnv("FRONTEND_DIR", "./frontend/dist"),

		// MLflow backend
		MLflowURL:       getEnv("MLFLOW_URL", "http://mlflow:5000"),
		MLflowAdminUser: getEnv("MLFLOW_AUTH_ADMIN_USERNAME", ""),
		MLflowAdminPass: getEnv("MLFLOW_AUTH_ADMIN_PASSWORD", ""),

		// Authentication
		JWTSecret:  getEnv("JWT_SECRET", ""),
		CookieName: "mlflow_session",
		AuthDBURI:  getEnv("AUTH_DB_URI", ""),

		// OIDC
		OIDCIssuerURL:    getEnv("OIDC_ISSUER_URL", ""),
		OIDCClientID:     getEnv("OIDC_CLIENT_ID", ""),
		OIDCClientSecret: getEnv("OIDC_CLIENT_SECRET", ""),
		OIDCRedirectURL:  getEnv("OIDC_REDIRECT_URL", ""),
		OIDCAdminUser:    getEnv("OIDC_ADMIN_USER", ""),
	}

	// ── Validate required configuration ──
	var missing []string
	if cfg.JWTSecret == "" {
		missing = append(missing, "JWT_SECRET")
	}
	if cfg.AuthDBURI == "" {
		missing = append(missing, "AUTH_DB_URI")
	}
	if len(missing) > 0 {
		log.Fatalf("FATAL: Missing required environment variables: %v", missing)
	}

	// ── Populate package-level vars for backward compatibility ──
	mlflowURL = cfg.MLflowURL
	jwtSecret = []byte(cfg.JWTSecret)
	listenAddr = cfg.ListenAddr
	cookieName = cfg.CookieName
	authDBURI = cfg.AuthDBURI

	// ── Log loaded configuration (redact secrets) ──
	log.Println("── Configuration Loaded ──")
	log.Printf("  LISTEN_ADDR:  %s", cfg.ListenAddr)
	log.Printf("  MLFLOW_URL:   %s", cfg.MLflowURL)
	log.Printf("  FRONTEND_DIR: %s", cfg.FrontendDir)
	log.Printf("  AUTH_DB_URI:  %s", redact(cfg.AuthDBURI))
	log.Printf("  OIDC_ENABLED: %v", cfg.OIDCEnabled())
	if cfg.OIDCEnabled() {
		log.Printf("  OIDC_ISSUER:  %s", cfg.OIDCIssuerURL)
	}
}

// OIDCEnabled returns true if all required OIDC configuration is present.
func (c *Config) OIDCEnabled() bool {
	return c.OIDCIssuerURL != "" && c.OIDCClientID != "" && c.OIDCClientSecret != ""
}

// getEnv reads an environment variable or returns the fallback value.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// redact masks sensitive values for safe logging.
func redact(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:8] + "****"
}
