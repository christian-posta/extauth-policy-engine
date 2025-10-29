package main

import (
	"fmt"
	"os"
	"time"
)

// OpenFGAConfig holds the configuration for OpenFGA integration
type OpenFGAConfig struct {
	// APIURL is the base URL of the OpenFGA server (e.g., "http://localhost:8080")
	APIURL string

	// StoreID is the OpenFGA store identifier
	StoreID string

	// AuthorizationModelID is optional - if empty, uses the latest model
	AuthorizationModelID string

	// Relation is the relationship to check (e.g., "can_access", "viewer")
	Relation string

	// Timeout for OpenFGA API calls
	Timeout time.Duration

	// RetryAttempts for failed requests
	RetryAttempts int
}

// Config holds the application configuration
type Config struct {
	// Port is the gRPC server port
	Port int

	// OpenFGA configuration
	OpenFGA OpenFGAConfig
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	cfg := &Config{
		Port: 7070, // default port
		OpenFGA: OpenFGAConfig{
			APIURL:               getEnv("OPENFGA_API_URL", "http://localhost:8080"),
			StoreID:              getEnv("OPENFGA_STORE_ID", ""),
			AuthorizationModelID: getEnv("OPENFGA_MODEL_ID", ""),
			Relation:             getEnv("OPENFGA_RELATION", "can_access"),
			Timeout:              2 * time.Second,
			RetryAttempts:        2,
		},
	}

	// Validate required fields
	if cfg.OpenFGA.StoreID == "" {
		return nil, fmt.Errorf("OPENFGA_STORE_ID environment variable is required")
	}

	return cfg, nil
}

// getEnv gets an environment variable with a default fallback
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
