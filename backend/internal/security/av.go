package security

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jjbank/skill-market/internal/model"
)

// ============================================================
// 外部查毒引擎适配层 (ClamAV / YARA)
//   设计: 有则调用并合并结论, 无则降级为「内置特征库」并在引擎元信息里
//   明确标注 unavailable —— 银行环境可挂载内网 ClamAV / YARA 规则集。
// ============================================================

const (
	clamavBinEnv  = "SKILLHUB_CLAMAV_BIN"
	yaraBinEnv    = "SKILLHUB_YARA_BIN"
	yaraRulesEnv  = "SKILLHUB_YARA_RULES"
	avTimeoutEnv  = "SKILLHUB_AV_TIMEOUT"
	defaultAVTime = 90 * time.Second
)

// AVStatus 外部引擎状态 (供前端与审计展示)
type AVStatus struct {
	Engine    string `json:"engine"`
	Available bool   `json:"available"`
	Binary    string `json:"binary,omitempty"`
	Rules     string `json:"rules,omitempty"`
	Version   string `json:"version,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

func firstExisting(candidates []string) string {
	for _, c := range candidates {
		if strings.TrimSpace(c) == "" {
			continue
		}
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

// ExternalAVStatus 探测外部查毒引擎可用性
func ExternalAVStatus() []AVStatus {
	out := []AVStatus{}

	clam := firstExisting([]string{os.Getenv(clamavBinEnv), "clamscan", "clamdscan"})
	s := AVStatus{Engine: "ClamAV"}
	if clam == "" {
		s.Detail = "未检测到 clamscan/clamdscan, 已降级使用内置病毒特征库 (可用 SKILLHUB_CLAMAV_BIN 指定)"
	} else {
		s.Available = true
		s.Binary = clam
		if out, err := exec.Command(clam, "--version").Output(); err == nil {
			s.Version = strings.TrimSpace(string(bytes.SplitN(out, []byte("\n"), 2)[0]))
		}
		s.Detail = "已启用 ClamAV 实时病毒库扫描"
	}
	out = append(out, s)

	yara := firstExisting([]string{os.Getenv(yaraBinEnv), "yara", "yara64"})
	rules := strings.TrimSpace(os.Getenv(yaraRulesEnv))
	y := AVStatus{Engine: "YARA", Rules: rules}
	switch {
	case yara == "":
		y.Detail = "未检测到 yara, 已降级使用内置规则库 (可用 SKILLHUB_YARA_BIN 指定)"
	case rules == "":
		y.Binary = yara
		y.Detail = "已检测到 yara, 但未配置规则集 (SKILLHUB_YARA_RULES)"
	default:
		if st, err := os.Stat(rules); err != nil || st.IsDir() {
			y.Binary = yara
			y.Detail = "YARA 规则路径无效: " + rules
			break
		}
		y.Available = true
		y.Binary = yara
		y.Detail = "已启用 YARA 规则集扫描"
	}
	out = append(out, y)
	return out
}

func avTimeout() time.Duration {
	if v := strings.TrimSpace(os.Getenv(avTimeoutEnv)); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return defaultAVTime
}

// ScanWithExternalAV 用外部引擎扫描技能包文件, 返回命中的风险项
//
//	文件会写入隔离临时目录后交由外部引擎扫描, 扫描结束立即清理。
func ScanWithExternalAV(files []model.SkillFile) ([]model.SecurityFinding, []AVStatus) {
	status := ExternalAVStatus()
	findings := []model.SecurityFinding{}

	needClam := status[0].Available
	needYara := status[1].Available
	if !needClam && !needYara {
		return findings, status
	}

	dir, err := os.MkdirTemp("", "skillhub-av-")
	if err != nil {
		return findings, status
	}
	defer os.RemoveAll(dir)
	for _, f := range files {
		target := filepath.Join(dir, sanitizeAVName(f.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			continue
		}
		_ = os.WriteFile(target, f.Content, 0o600)
	}

	run := func(bin string, args ...string) (string, int) {
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		done := make(chan error, 1)
		if err := cmd.Start(); err != nil {
			return "", -1
		}
		go func() { done <- cmd.Wait() }()
		select {
		case <-time.After(avTimeout()):
			_ = cmd.Process.Kill()
			return buf.String(), -2
		case <-done:
			return buf.String(), cmd.ProcessState.ExitCode()
		}
	}

	if needClam {
		out, code := run(status[0].Binary, "--no-summary", "--infected", "-r", dir)
		if code == -2 {
			findings = append(findings, model.SecurityFinding{RuleID: "MAL-06", Category: "malware", Severity: "high",
				Title: "ClamAV 扫描超时", Detail: "外部查毒引擎未在超时时间内完成, 结论不完整, 建议人工复核", Blocking: false})
		}
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasSuffix(line, "FOUND") {
				continue
			}
			parts := strings.SplitN(line, ":", 2)
			sig := strings.TrimSpace(parts[len(parts)-1])
			sig = strings.TrimSuffix(sig, "FOUND")
			rel := ""
			if len(parts) == 2 {
				rel = strings.TrimPrefix(strings.TrimSpace(parts[0]), dir)
				rel = strings.TrimPrefix(filepath.ToSlash(rel), "/")
			}
			findings = append(findings, model.SecurityFinding{RuleID: "MAL-06", Category: "malware", Severity: "critical",
				Title: "ClamAV 病毒库命中", Detail: "外部查毒引擎(ClamAV)判定为恶意文件, 依据银行安全基线必须阻断",
				File: rel, Evidence: strings.TrimSpace(sig), Blocking: true})
		}
	}

	if needYara {
		out, code := run(status[1].Binary, "-r", "-w", status[1].Rules, dir)
		if code == -2 {
			findings = append(findings, model.SecurityFinding{RuleID: "MAL-07", Category: "malware", Severity: "high",
				Title: "YARA 扫描超时", Detail: "YARA 规则集扫描未在超时时间内完成, 建议人工复核", Blocking: false})
		}
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "error") {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}
			findings = append(findings, model.SecurityFinding{RuleID: "MAL-07", Category: "malware", Severity: "critical",
				Title: "YARA 规则命中", Detail: "命中内网 YARA 恶意特征规则, 依据银行安全基线必须阻断",
				File: strings.TrimPrefix(strings.TrimSpace(parts[len(parts)-1]), dir), Evidence: parts[0], Blocking: true})
		}
	}
	return findings, status
}

// sanitizeAVName 生成安全的落盘文件名 (防目录穿越)
func sanitizeAVName(p string) string {
	clean := strings.ReplaceAll(p, "\\", "/")
	clean = strings.TrimPrefix(clean, "/")
	parts := strings.Split(clean, "/")
	kept := make([]string, 0, len(parts))
	for _, seg := range parts {
		if seg == "" || seg == "." || seg == ".." {
			continue
		}
		kept = append(kept, seg)
	}
	if len(kept) == 0 {
		return "unnamed"
	}
	return filepath.Join(kept...)
}

// AVSummary 供前端展示的一行摘要
func AVSummary(status []AVStatus) string {
	parts := make([]string, 0, len(status))
	for _, s := range status {
		if s.Available {
			parts = append(parts, fmt.Sprintf("%s(已启用)", s.Engine))
		} else {
			parts = append(parts, fmt.Sprintf("%s(降级)", s.Engine))
		}
	}
	return strings.Join(parts, " · ")
}
