package config

import (
	"os"
)

type MySQLConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
}

var DB MySQLConfig

func Init() {
	DB = MySQLConfig{
		Host:     envOrDefault("DB_HOST", "124.221.220.89"),
		Port:     23306,
		User:     envOrDefault("DB_USER", "root"),
		Password: envOrDefault("DB_PASS", "glt01054717@"),
		Database: envOrDefault("DB_NAME", "ai_os"),
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
