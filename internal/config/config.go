package config

import (
	"os"
	"strconv"
	"time"
)


type Config struct {
	Port        string
	DatabaseURL string

	MigrateTimeout time.Duration
	MigrationsTable string

	JWTSecret  string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

func Load() *Config {
	cfg := &Config{
		Port:        getEnv("HTTP_PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", ""),
		
		MigrateTimeout:  getDuration("MIGRATE_TIMEOUT", 30*time.Second),
		MigrationsTable: getEnv("MIGRATIONS_TABLE", "schema_migrations"),

		JWTSecret:  getEnv("JWT_SECRET", ""),
		AccessTTL:  getDuration("ACCESS_TTL", 15*time.Minute),
		RefreshTTL: getDuration("REFRESH_TTL", 30*24*time.Hour),
	}

	return cfg
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getDuration(k string, def time.Duration) time.Duration {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

var _ = strconv.Atoi