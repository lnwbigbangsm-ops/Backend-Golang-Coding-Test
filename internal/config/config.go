// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all runtime settings.
type Config struct {
	HTTPAddr          string
	GRPCAddr          string
	MongoURI          string
	MongoDatabase     string
	JWTSecret         string
	JWTIssuer         string
	JWTTTL            time.Duration
	BcryptCost        int
	UserCountInterval time.Duration
	ShutdownTimeout   time.Duration
	// IdempotencyRetention is how long Idempotency-Key results are kept; it
	// must outlive the longest client retry window.
	IdempotencyRetention time.Duration
}

// Load reads configuration from the environment, applying defaults. JWT_SECRET
// has no default so the service never runs with a well-known key.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:      getenv("HTTP_ADDR", ":8080"),
		GRPCAddr:      getenv("GRPC_ADDR", ":9090"),
		MongoURI:      getenv("MONGO_URI", "mongodb://localhost:27017"),
		MongoDatabase: getenv("MONGO_DATABASE", "user_management"),
		JWTSecret:     os.Getenv("JWT_SECRET"),
		JWTIssuer:     getenv("JWT_ISSUER", "user-management-api"),
	}

	var errs []error
	var err error
	if cfg.JWTSecret == "" {
		errs = append(errs, errors.New("JWT_SECRET is required"))
	}
	if cfg.JWTTTL, err = durationEnv("JWT_TTL", 24*time.Hour); err != nil {
		errs = append(errs, err)
	}
	if cfg.UserCountInterval, err = durationEnv("USER_COUNT_INTERVAL", 10*time.Second); err != nil {
		errs = append(errs, err)
	}
	if cfg.ShutdownTimeout, err = durationEnv("SHUTDOWN_TIMEOUT", 15*time.Second); err != nil {
		errs = append(errs, err)
	}
	if cfg.IdempotencyRetention, err = durationEnv("IDEMPOTENCY_RETENTION", 24*time.Hour); err != nil {
		errs = append(errs, err)
	}
	if cfg.BcryptCost, err = intEnv("BCRYPT_COST", 12); err != nil {
		errs = append(errs, err)
	}
	return cfg, errors.Join(errs...)
}

func getenv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func durationEnv(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration (e.g. 10s), got %q", key, v)
	}
	return d, nil
}

func intEnv(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer, got %q", key, v)
	}
	return n, nil
}
