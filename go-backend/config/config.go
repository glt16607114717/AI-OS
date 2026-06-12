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

// Embedding 配置
type EmbeddingConfig struct {
	APIKey     string
	APIURL     string
	Model      string
	Dimensions int
}

var Embedding EmbeddingConfig

func Init() {
	DB = MySQLConfig{
		Host:     envOrDefault("DB_HOST", "124.221.220.89"),
		Port:     23306,
		User:     envOrDefault("DB_USER", "root"),
		Password: envOrDefault("DB_PASS", "glt01054717@"),
		Database: envOrDefault("DB_NAME", "ai_os"),
	}
	Embedding = EmbeddingConfig{
		APIKey:     envOrDefault("EMBEDDING_API_KEY", "5170eaa827e042d8ba259cff4393e24e.b1ouPVCQDqxOvDS7"),
		APIURL:     "https://open.bigmodel.cn/api/paas/v4/embeddings",
		Model:      "embedding-3",
		Dimensions: 2048,
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
