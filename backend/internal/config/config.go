package config

import (
	"os"
	"time"
)

// Config 应用配置
type Config struct {
	ServerPort string
	DB         DBConfig
	Redis      RedisConfig
	ES         ESConfig
	JWT        JWTConfig
	LLM        LLMConfig
	GitHub     GitHubConfig
}

type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
}

func (d DBConfig) DSN() string {
	return "host=" + d.Host + " port=" + d.Port +
		" user=" + d.User + " password=" + d.Password +
		" dbname=" + d.Name + " sslmode=disable"
}

type RedisConfig struct {
	Host     string
	Port     string
	Password string
}

type ESConfig struct {
	Host string
	Port string
}

type JWTConfig struct {
	Secret     string
	ExpireHour int
}

type LLMConfig struct {
	APIBase string
	APIKey  string
	Model   string
}

type GitHubConfig struct {
	Token    string
	APIBase  string
	CacheTTL time.Duration
}

// Load 加载配置 (从环境变量)
func Load() *Config {
	return &Config{
		ServerPort: getEnv("SERVER_PORT", "8080"),
		DB: DBConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "skillhub"),
			Password: getEnv("DB_PASSWORD", "skillhub_dev_2026"),
			Name:     getEnv("DB_NAME", "skillhub"),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnv("REDIS_PORT", "6379"),
			Password: getEnv("REDIS_PASSWORD", "skillhub_redis_2026"),
		},
		ES: ESConfig{
			Host: getEnv("ES_HOST", "localhost"),
			Port: getEnv("ES_PORT", "9200"),
		},
		JWT: JWTConfig{
			Secret:     getEnv("JWT_SECRET", "skillhub-jwt-secret-dev"),
			ExpireHour: 24,
		},
		LLM: LLMConfig{
			APIBase: getEnv("LLM_API_BASE", "http://localhost:8080/v1"),
			APIKey:  getEnv("LLM_API_KEY", ""),
			Model:   getEnv("LLM_MODEL", "deepseek-v3"),
		},
		GitHub: GitHubConfig{
			Token:    getEnv("GITHUB_TOKEN", ""),
			APIBase:  getEnv("GITHUB_API_URL", "https://api.github.com"),
			CacheTTL: 5 * time.Minute,
		},
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
