package security

import (
	"os"
	"strings"
	"testing"
)

// withCleanEnv 隔离并复原密钥相关环境变量 (密钥有全局缓存, 必须显式复位)
func withCleanEnv(t *testing.T) {
	t.Helper()
	keys := []string{"SKILLHUB_SIGNING_KEY", "SKILLHUB_SIGNING_KEY_FILE", "SKILLHUB_ENV"}
	saved := map[string]string{}
	for _, k := range keys {
		saved[k] = os.Getenv(k)
		_ = os.Unsetenv(k)
	}
	ResetKeyCache()
	t.Cleanup(func() {
		for k, v := range saved {
			if v == "" {
				_ = os.Unsetenv(k)
			} else {
				_ = os.Setenv(k, v)
			}
		}
		ResetKeyCache()
	})
}

func TestKeyMaterialAndProductionGuard(t *testing.T) {
	withCleanEnv(t)

	// 1) 默认: 内置演示密钥可用, 但非生产环境不阻断
	if !UsesDefaultKey() {
		t.Fatal("默认应识别为内置演示密钥")
	}
	if err := ValidateKeyMaterial(); err != nil {
		t.Fatalf("非生产环境不应因密钥报错: %v", err)
	}
	kidDefault := KeyID()
	if !strings.HasPrefix(kidDefault, "kid_") {
		t.Fatalf("key_id 格式错误: %s", kidDefault)
	}

	// 2) 生产环境仍用演示密钥 -> 拒绝启动
	_ = os.Setenv("SKILLHUB_ENV", "production")
	if err := ValidateKeyMaterial(); err == nil {
		t.Fatal("生产环境使用内置演示密钥必须拒绝启动 (fail-closed)")
	}

	// 3) 注入受控密钥 (模拟 KMS 注入) -> 通过, 且指纹变化
	_ = os.Setenv("SKILLHUB_SIGNING_KEY", "unit-test-controlled-key-0123456789abcdef")
	ResetKeyCache()
	if UsesDefaultKey() {
		t.Fatal("注入密钥后不应再判定为演示密钥")
	}
	if err := ValidateKeyMaterial(); err != nil {
		t.Fatalf("注入受控密钥后生产环境应通过: %v", err)
	}
	if KeySource() != "env" {
		t.Fatalf("密钥来源应为 env, 实际 %s", KeySource())
	}
	if KeyID() == kidDefault {
		t.Fatal("不同密钥必须产生不同 key_id")
	}

	// 4) 密钥文件优先级高于环境变量 (模拟 KMS 挂载文件)
	dir := t.TempDir()
	keyFile := dir + string(os.PathSeparator) + "signing.key"
	if err := os.WriteFile(keyFile, []byte("kms-mounted-key-abcdef0123456789\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Setenv("SKILLHUB_SIGNING_KEY_FILE", keyFile)
	ResetKeyCache()
	if KeySource() != "file" {
		t.Fatalf("挂载文件应优先, 实际来源 %s", KeySource())
	}
	if err := ValidateKeyMaterial(); err != nil {
		t.Fatalf("挂载密钥后不应报错: %v", err)
	}

	// 5) 密钥文件不可读 -> 明确报错 (不静默降级)
	_ = os.Setenv("SKILLHUB_SIGNING_KEY_FILE", dir+string(os.PathSeparator)+"missing.key")
	ResetKeyCache()
	if err := ValidateKeyMaterial(); err == nil {
		t.Fatal("密钥文件不可读时必须报错, 不允许静默回退")
	}
}

func TestExternalAVStatusShape(t *testing.T) {
	status := ExternalAVStatus()
	if len(status) != 2 {
		t.Fatalf("期望 ClamAV + YARA 两个适配器, 实际 %d", len(status))
	}
	names := map[string]bool{}
	for _, s := range status {
		names[s.Engine] = true
		if strings.TrimSpace(s.Detail) == "" {
			t.Errorf("%s 缺少状态说明", s.Engine)
		}
		if s.Available && s.Binary == "" {
			t.Errorf("%s 标记可用但未给出可执行文件路径", s.Engine)
		}
	}
	if !names["ClamAV"] || !names["YARA"] {
		t.Fatalf("适配器缺失: %v", names)
	}
	if summary := AVSummary(status); !strings.Contains(summary, "ClamAV") {
		t.Fatalf("摘要异常: %s", summary)
	}
	// 无外部引擎时不得凭空产生结论 (降级由内置特征库兜底)
	findings, _ := ScanWithExternalAV(safeSkill())
	if len(findings) != 0 {
		t.Fatalf("安全内容不应产生外部引擎结论: %v", findings)
	}
}

func TestManifestCarriesKeyID(t *testing.T) {
	withCleanEnv(t)
	files := safeSkill()
	m := BuildManifest("demo-skill", "demo", "1.0.0", "u-1", "internal", "WM-1", "DL-1",
		"2026-09-18T00:00:00Z", "market", "safe", 0, files)
	if m.KeyID == "" || m.KeyID != KeyID() {
		t.Fatalf("清单应记录签发密钥指纹: %s vs %s", m.KeyID, KeyID())
	}
	if ok, msg := VerifyManifest(m, files, SigningKey()); !ok {
		t.Fatalf("验签失败: %s", msg)
	}
	// 篡改 key_id 应导致验签失败 (key_id 参与签名载荷)
	swapped := m
	swapped.KeyID = "kid_deadbeef"
	if ok, _ := VerifyManifest(swapped, files, SigningKey()); ok {
		t.Fatal("篡改 key_id 后仍验签通过")
	}
	// 换密钥后旧签名不可信 (密钥轮换语义)
	if ok, _ := VerifyManifest(m, files, "rotated-key"); ok {
		t.Fatal("密钥轮换后旧签名不应通过")
	}
}

func TestAVSanitizeNameBlocksTraversal(t *testing.T) {
	got := sanitizeAVName("../../etc/passwd")
	if strings.Contains(got, "..") || strings.HasPrefix(got, "/") || strings.Contains(got, ":") {
		t.Fatalf("落盘文件名未阻断路径穿越: %s", got)
	}
	if sanitizeAVName("") != "unnamed" {
		t.Fatal("空路径应回退为 unnamed")
	}
}
