package security

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
)

// ============================================================
// 签名/水印密钥材料管理
//   生产环境要求: 密钥必须由 KMS / 密钥管理 / 受控挂载文件注入,
//   不允许使用内置演示密钥 (SKILLHUB_ENV=production 时直接拒绝启动)。
//   对外只公开「密钥指纹」(key_id), 不泄漏密钥本体。
// ============================================================

const (
	signingKeyFileEnv = "SKILLHUB_SIGNING_KEY_FILE"
	strictEnvVar      = "SKILLHUB_ENV"
)

var (
	keyOnce     sync.Once
	keyValue    string
	keyID       string
	keyFromFile bool
	keyLoadErr  error
)

// resolveSigningKey 密钥解析顺序: 挂载文件 > 环境变量 > 内置演示密钥
func resolveSigningKey() (string, string, bool, error) {
	if path := strings.TrimSpace(os.Getenv(signingKeyFileEnv)); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", "", false, fmt.Errorf("读取签名密钥文件失败 (%s): %w", path, err)
		}
		key := strings.TrimSpace(string(raw))
		if key == "" {
			return "", "", false, fmt.Errorf("签名密钥文件为空: %s", path)
		}
		return key, path, true, nil
	}
	if key := strings.TrimSpace(os.Getenv(signingKeyEnv)); key != "" {
		return key, "", false, nil
	}
	return defaultKey, "", false, nil
}

func loadKeyOnce() {
	keyValue, keyFromFile, keyLoadErr = func() (string, bool, error) {
		key, _, fromFile, err := resolveSigningKey()
		return key, fromFile, err
	}()
	if keyValue == "" {
		keyValue = defaultKey
	}
	sum := sha256.Sum256([]byte(keyValue))
	keyID = "kid_" + hex.EncodeToString(sum[:4])
}

// ResetKeyCache 重新加载密钥 (测试用; 生产不调用)
func ResetKeyCache() { keyOnce = sync.Once{} }

// KeyID 密钥指纹 (可公开, 用于审计: 证明签发所用密钥版本)
func KeyID() string {
	keyOnce.Do(loadKeyOnce)
	return keyID
}

// UsesDefaultKey 是否仍在使用内置演示密钥
func UsesDefaultKey() bool {
	keyOnce.Do(loadKeyOnce)
	return keyValue == defaultKey
}

// KeySource 密钥来源
func KeySource() string {
	keyOnce.Do(loadKeyOnce)
	if keyFromFile {
		return "file"
	}
	if keyValue != defaultKey {
		return "env"
	}
	return "builtin-demo"
}

// ValidateKeyMaterial 启动自检: 生产环境不允许内置演示密钥
func ValidateKeyMaterial() error {
	keyOnce.Do(loadKeyOnce)
	if keyLoadErr != nil {
		return keyLoadErr
	}
	env := strings.ToLower(strings.TrimSpace(os.Getenv(strictEnvVar)))
	strict := env == "production" || env == "prod" || env == "bank"
	if !strict {
		return nil
	}
	if UsesDefaultKey() {
		return fmt.Errorf("检测到生产环境(%s=%s)仍在使用内置演示签名密钥, 拒绝启动; "+
			"请通过 %s (推荐, 对接 KMS 挂载) 或 %s 注入受控密钥", strictEnvVar, env, signingKeyFileEnv, signingKeyEnv)
	}
	return nil
}

// KeyMeta 密钥元信息 (供前端/审计展示)
func KeyMeta() map[string]interface{} {
	keyOnce.Do(loadKeyOnce)
	return map[string]interface{}{
		"key_id":     KeyID(),
		"source":     KeySource(),
		"algorithm":  Algorithm,
		"is_default": UsesDefaultKey(),
		"strict_env": strings.TrimSpace(os.Getenv(strictEnvVar)),
	}
}
