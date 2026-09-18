package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jjbank/skill-market/internal/model"
	sec "github.com/jjbank/skill-market/internal/security"
)

// ============================================================
// 动态沙箱验证客户端
//   把待审技能送进隔离容器 (无外网 / 只读根 / 非 root / cap-drop ALL / 限额)
//   真跑一次, 观测行为, 作为静态查毒之外的第二道验证。
//   env: SKILLHUB_DYNAMIC_SANDBOX=1|0, SKILLHUB_SANDBOX_URL, SKILLHUB_SANDBOX_TIMEOUT
// ============================================================

const (
	sandboxURLEnv     = "SKILLHUB_SANDBOX_URL"
	sandboxEnabledEnv = "SKILLHUB_DYNAMIC_SANDBOX"
	sandboxTimeoutEnv = "SKILLHUB_SANDBOX_TIMEOUT"
	sandboxMaxBytes   = 6 << 20
)

// SandboxClient 沙箱客户端
type SandboxClient struct {
	url     string
	timeout time.Duration
	enabled bool
	http    *http.Client
}

// NewSandboxClient 从环境变量构建
func NewSandboxClient() *SandboxClient {
	enabled := true
	if v := strings.TrimSpace(os.Getenv(sandboxEnabledEnv)); v == "0" || strings.EqualFold(v, "false") {
		enabled = false
	}
	timeout := 60 * time.Second
	if v := strings.TrimSpace(os.Getenv(sandboxTimeoutEnv)); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 5*time.Second {
			timeout = d
		}
	}
	url := strings.TrimSpace(os.Getenv(sandboxURLEnv))
	if url == "" {
		url = "http://localhost:8090"
	}
	return &SandboxClient{
		url:     strings.TrimRight(url, "/"),
		timeout: timeout,
		enabled: enabled,
		http:    &http.Client{Timeout: timeout + 15*time.Second},
	}
}

// Enabled 是否启用
func (c *SandboxClient) Enabled() bool { return c != nil && c.enabled }

// URL 沙箱地址
func (c *SandboxClient) URL() string {
	if c == nil {
		return ""
	}
	return c.url
}

// Health 沙箱健康 + 隔离自检
func (c *SandboxClient) Health(ctx context.Context) map[string]interface{} {
	out := map[string]interface{}{"enabled": c.Enabled(), "url": c.URL(), "reachable": false}
	if !c.Enabled() {
		out["notice"] = "动态沙箱已关闭 (SKILLHUB_DYNAMIC_SANDBOX=0), 仅做静态安全扫描"
		return out
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"/health", nil)
	if err != nil {
		return out
	}
	resp, err := c.http.Do(req)
	if err != nil {
		out["notice"] = "动态沙箱不可达: " + err.Error()
		return out
	}
	defer resp.Body.Close()
	var payload map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	out["reachable"] = resp.StatusCode == http.StatusOK
	out["engine"] = payload["engine"]
	out["isolated"] = payload["isolated"]
	return out
}

// Verify 把技能包送进沙箱执行并返回行为报告; 失败时返回带 notice 的报告 (不阻断主流程)
func (c *SandboxClient) Verify(ctx context.Context, skillKey string, files []model.SkillFile) *model.SandboxReport {
	report := &model.SandboxReport{Engine: sec.SandboxEngine(), Status: "skipped"}
	if !c.Enabled() {
		report.Notice = "动态沙箱未启用"
		return report
	}
	payloadFiles := make([]map[string]string, 0, len(files))
	var total int64
	runnable := false
	for _, f := range files {
		if f.Skipped || len(f.Content) == 0 {
			continue
		}
		total += int64(len(f.Content))
		if total > sandboxMaxBytes {
			break
		}
		payloadFiles = append(payloadFiles, map[string]string{
			"path": f.Path, "content_b64": base64.StdEncoding.EncodeToString(f.Content),
		})
		if sec.IsRunnableEntry(f.Path) {
			runnable = true
		}
	}
	if len(payloadFiles) == 0 || !runnable {
		report.Notice = "包内没有可执行入口 (无 server.py/main.py/app.py/index.js/install.sh), 跳过动态验证"
		return report
	}
	body, _ := json.Marshal(map[string]interface{}{
		"skill_key": skillKey, "files": payloadFiles,
		"timeout": int(c.timeout.Seconds()) - 15,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/verify", bytes.NewReader(body))
	if err != nil {
		report.Status, report.Notice = "error", err.Error()
		return report
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		report.Status = "unreachable"
		report.Notice = "动态沙箱不可达, 未能完成行为验证: " + err.Error()
		return report
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		report.Status, report.Notice = "error", fmt.Sprintf("沙箱返回 %d", resp.StatusCode)
		return report
	}
	var raw model.SandboxReport
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		report.Status, report.Notice = "error", "沙箱报告解析失败: "+err.Error()
		return report
	}
	raw.Status = "ok"
	if raw.VerdictHint == "skipped" {
		raw.Status = "skipped"
	}
	return &raw
}

// SandboxEngineMeta 引擎元信息中的沙箱状态 (供前端/审计)
func SandboxEngineMeta() map[string]interface{} {
	client := NewSandboxClient()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	return client.Health(ctx)
}
