package config

import (
	"log"
	"os"
	"strings"
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
			APIKey:  getEnvWithFile("LLM_API_KEY", "LLM_API_KEY_FILE"),
			Model:   getEnv("LLM_MODEL", "deepseek-v3"),
		},
		GitHub: GitHubConfig{
			Token:    getEnvWithFile("GITHUB_TOKEN", "GITHUB_TOKEN_FILE"),
			APIBase:  getEnv("GITHUB_API_URL", "https://api.github.com"),
			CacheTTL: getEnvDuration("GITHUB_CACHE_TTL", 30*time.Minute),
		},
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return defaultVal
}

// getEnvWithFile 密钥类配置: 优先环境变量, 其次从「挂载文件」读取
// (银行环境推荐用 KMS/密钥管理挂载文件, 避免密钥出现在环境变量与进程命令行里)
func getEnvWithFile(valueEnv, fileEnv string) string {
	if v := strings.TrimSpace(os.Getenv(valueEnv)); v != "" {
		return v
	}
	path := strings.TrimSpace(os.Getenv(fileEnv))
	if path == "" {
		return ""
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[config] 读取密钥文件失败 (%s): %v", path, err)
		return ""
	}
	return strings.TrimSpace(string(raw))
}
