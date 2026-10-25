package config

import (
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("JWT_SECRET", "secret")
	for _, k := range []string{"HTTP_ADDR", "GRPC_ADDR", "MONGO_URI", "JWT_TTL", "USER_COUNT_INTERVAL", "BCRYPT_COST"} {
		t.Setenv(k, "")
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPAddr != ":8080" || cfg.GRPCAddr != ":9090" || cfg.JWTTTL != 24*time.Hour ||
		cfg.UserCountInterval != 10*time.Second || cfg.BcryptCost != 12 {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
}

func TestLoad_Errors(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	t.Setenv("JWT_TTL", "forever")
	t.Setenv("USER_COUNT_INTERVAL", "-1s")
	t.Setenv("BCRYPT_COST", "high")
	if _, err := Load(); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoad_Overrides(t *testing.T) {
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("HTTP_ADDR", ":1234")
	t.Setenv("USER_COUNT_INTERVAL", "1m")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":1234" || cfg.UserCountInterval != time.Minute {
		t.Errorf("overrides not applied: %+v", cfg)
	}
}
