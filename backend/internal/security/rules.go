package security

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jjbank/skill-market/internal/model"
)

// ============================================================
// 规则库装载 (签名库外置)
//   规则以 JSON 数据文件交付: 可独立更新、可审计、可回滚, 且避免
//   特征串被编入可执行文件导致引擎自身被杀毒软件误报。
// ============================================================

const (
	rulesFileEnv     = "SKILLHUB_SECURITY_RULES"
	defaultRulesFile = "security-rules.json"
)

type ruleSpec struct {
	ID               string `json:"id"`
	Category         string `json:"category"`
	Severity         string `json:"severity"`
	Title            string `json:"title"`
	Detail           string `json:"detail"`
	Scope            string `json:"scope"`
	Pattern          string `json:"pattern"`
	SignatureEncoded bool   `json:"signature_encoded"`
}

type ruleFile struct {
	Schema      string     `json:"schema"`
	Engine      string     `json:"engine"`
	UpdatedAt   string     `json:"updated_at"`
	Description string     `json:"description"`
	Rules       []ruleSpec `json:"rules"`
}

var (
	rulesLoadedAt   string
	rulesSourcePath string
	rulesEngine     string
	rulesCount      int
)

// ResolveRulesPath 解析规则库文件路径 (env > 可执行文件同级 rules/ > 当前目录 rules/)
func ResolveRulesPath() string {
	if p := strings.TrimSpace(os.Getenv(rulesFileEnv)); p != "" {
		return p
	}
	candidates := []string{}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(dir, "rules", defaultRulesFile), filepath.Join(dir, defaultRulesFile))
	}
	candidates = append(candidates,
		filepath.Join("rules", defaultRulesFile),
		filepath.Join("..", "rules", defaultRulesFile),
		filepath.Join("backend", "rules", defaultRulesFile),
	)
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return filepath.Join("rules", defaultRulesFile)
}

// LoadRules 装载规则库; 失败即返回错误 (调用方必须视作致命, 保证 fail-closed)
func LoadRules(path string) error {
	if strings.TrimSpace(path) == "" {
		path = ResolveRulesPath()
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("规则库文件不可读 (%s): %w", path, err)
	}
	var doc ruleFile
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("规则库解析失败 (%s): %w", path, err)
	}
	if len(doc.Rules) == 0 {
		return fmt.Errorf("规则库为空 (%s), 安全扫描将失效, 拒绝启动", path)
	}
	loaded := make([]rule, 0, len(doc.Rules))
	for _, spec := range doc.Rules {
		r := rule{id: spec.ID, category: spec.Category, severity: spec.Severity,
			title: spec.Title, detail: spec.Detail, scope: spec.Scope}
		switch {
		case spec.SignatureEncoded || spec.Pattern == "EICAR":
			r.signature = true
			r.pattern = regexp.MustCompile(regexp.QuoteMeta(MalwareSignatureEICAR()))
		case strings.TrimSpace(spec.Pattern) != "":
			re, err := regexp.Compile(spec.Pattern)
			if err != nil {
				return fmt.Errorf("规则 %s 正则非法: %w", spec.ID, err)
			}
			r.pattern = re
		}
		loaded = append(loaded, r)
	}
	rules = loaded
	rulesCount = len(loaded)
	rulesLoadedAt = doc.UpdatedAt
	rulesSourcePath = path
	rulesEngine = doc.Engine
	return nil
}

// RulesLoaded 规则库是否已装载
func RulesLoaded() bool { return len(rules) > 0 }

// RulesMeta 规则库元信息
func RulesMeta() map[string]interface{} {
	return map[string]interface{}{
		"loaded":         RulesLoaded(),
		"count":          rulesCount,
		"source":         rulesSourcePath,
		"updated_at":     rulesLoadedAt,
		"engine":         rulesEngine,
		"signatures":     signatureCount(),
		"signature_mode": "encoded-in-rulepack",
		"av_engines":     ExternalAVStatus(),
		"av_summary":     AVSummary(ExternalAVStatus()),
		"key":            KeyMeta(),
	}
}

// MarshalRulesFile 导出当前规则库 (审计/备份用)
func MarshalRulesFile() ([]byte, error) {
	specs := make([]ruleSpec, 0, len(rules))
	for _, r := range rules {
		pat := ""
		if r.pattern != nil {
			pat = r.pattern.String()
		}
		specs = append(specs, ruleSpec{ID: r.id, Category: r.category, Severity: r.severity,
			Title: r.title, Detail: r.detail, Scope: r.scope, Pattern: pat, SignatureEncoded: r.signature})
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].ID < specs[j].ID })
	doc := ruleFile{Schema: "skillhub.security-rules/v1", Engine: engineVersion,
		UpdatedAt: "2026-09-18", Description: "技能安全静态扫描规则库", Rules: specs}
	return json.MarshalIndent(doc, "", "  ")
}

// rulesByID 辅助查找
func rulesByID(id string) *rule {
	for i := range rules {
		if rules[i].id == id {
			return &rules[i]
		}
	}
	return nil
}

// RuleDocsFor 用于前端展示的规则说明 (含装载状态)
func RuleDocsFor() []model.SecurityRuleDoc { return Rules() }
