package security

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jjbank/skill-market/internal/model"
)

// ============================================================
// 技能安全扫描引擎 (静态分析)
//   定位: 银行内网可落地 —— 任何技能在「进入」平台前、发布前、安装前
//   都要过一遍这层静态安检, 阻断病毒/木马/后门/提示注入/供应链投毒。
// ============================================================

const engineVersion = "SEC-ENGINE 2.0.0"

// ScanSubject 扫描对象元信息
type ScanSubject struct {
	Type       string // skill / import / package
	SkillID    string
	SkillKey   string
	SkillName  string
	Version    string
	Target     string
	Trigger    string
	TriggerBy  string
	TriggerNam string
	OwnerID    string
}

// Scope 规则适用的文件类型
const (
	scopeCode = "code"     // 可执行/脚本/配置
	scopeDoc  = "markdown" // 说明文档 (SKILL.md 等)
	scopePath = "path"     // 文件路径本身
	scopePkg  = "package"  // 包级规则 (由服务层调用)
)

type rule struct {
	id        string
	category  string
	severity  string
	title     string
	detail    string
	scope     string
	pattern   *regexp.Regexp
	signature bool // 是否来自编码病毒特征库
}

func mustRe(expr string) *regexp.Regexp {
	re, err := regexp.Compile(expr)
	if err != nil {
		panic("security rule regexp error: " + expr + ": " + err.Error())
	}
	return re
}

// ---------- 规则库 (RE2 语法, 不支持 lookbehind/lookahead) ----------

// rules 静态规则集会由 LoadRules 从外部规则库文件装载 (见 rules.go)。
// 设计原因: 病毒特征串/危险模式若直接编入二进制, 引擎自身会被杀毒软件误报隔离,
// 且规则库需要能独立更新与审计, 因此规则以数据文件形式交付。
var rules []rule

// zeroWidthChars 不可见字符集合
var zeroWidthChars = []rune{'\u200b', '\u200c', '\u200d', '\u2060', '\ufeff', '\u202a', '\u202b', '\u202c', '\u202d', '\u202e', '\u2066', '\u2067', '\u2068', '\u2069'}

var (
	codeExt      = map[string]bool{".py": true, ".js": true, ".mjs": true, ".cjs": true, ".ts": true, ".tsx": true, ".jsx": true, ".sh": true, ".bash": true, ".zsh": true, ".ps1": true, ".psm1": true, ".bat": true, ".cmd": true, ".vbs": true, ".go": true, ".rb": true, ".pl": true, ".php": true, ".java": true, ".kt": true, ".rs": true, ".c": true, ".cpp": true, ".h": true, ".cs": true, ".yaml": true, ".yml": true, ".json": true, ".toml": true, ".ini": true, ".cfg": true, ".conf": true, ".env": true, ".sql": true, ".tf": true}
	docExt       = map[string]bool{".md": true, ".markdown": true, ".mdx": true, ".txt": true, ".rst": true}
	execExt      = map[string]bool{".exe": true, ".dll": true, ".so": true, ".dylib": true, ".msi": true, ".scr": true, ".jar": true, ".bin": true, ".elf": true, ".dmg": true}
	scriptExt    = map[string]bool{".bat": true, ".cmd": true, ".vbs": true, ".ps1": true, ".sh": true}
	maxTextBytes = 2 << 20 // 单文件最多读取 2MB 做静态分析
	maxLineScan  = 20000
)

// severityScore 单条发现的扣分
var severityScore = map[string]int{"critical": 40, "high": 18, "medium": 7, "low": 2}

var severityCN = map[string]string{"critical": "严重", "high": "高", "medium": "中", "low": "低"}

var categoryCN = map[string]string{
	"execution": "危险执行", "network": "网络外联", "credential": "凭据窃取", "obfuscation": "代码混淆",
	"persistence": "持久化/提权", "destructive": "破坏性操作", "injection": "提示注入", "supply_chain": "供应链风险",
	"malware": "恶意软件", "integrity": "完整性", "compliance": "合规一致性",
}

// Rules 返回规则说明表 (供前端展示)
func Rules() []model.SecurityRuleDoc {
	out := make([]model.SecurityRuleDoc, 0, len(rules))
	for _, r := range rules {
		out = append(out, model.SecurityRuleDoc{
			RuleID: r.id, Category: r.category, CategoryCN: categoryCN[r.category],
			Severity: r.severity, Title: r.title, Detail: r.detail, Scope: r.scope,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RuleID < out[j].RuleID })
	return out
}

// RuleCount 规则数量
func RuleCount() int { return len(rules) }

// EngineVersion 引擎版本
func EngineVersion() string { return engineVersion }

// ScanFiles 对一组技能文件执行完整静态安全扫描
func ScanFiles(subject ScanSubject, files []model.SkillFile) *model.SecurityScan {
	start := time.Now()
	scan := &model.SecurityScan{
		ID:            newScanID(),
		SubjectType:   fallback(subject.Type, "skill"),
		SkillID:       subject.SkillID,
		SkillKey:      subject.SkillKey,
		SkillName:     subject.SkillName,
		Version:       subject.Version,
		Target:        subject.Target,
		TriggerType:   fallback(subject.Trigger, "manual"),
		TriggeredBy:   subject.TriggerBy,
		TriggeredName: subject.TriggerNam,
		EngineVersion: engineVersion,
		CreatedAt:     time.Now(),
		Findings:      []model.SecurityFinding{},
	}

	seen := map[string]int{} // ruleID|file -> 计数, 同一规则同一文件最多计 3 次
	addFinding := func(f model.SecurityFinding) {
		key := f.RuleID + "|" + f.File
		if seen[key] >= 3 {
			return
		}
		seen[key]++
		f.CategoryCN = categoryCN[f.Category]
		f.Blocking = f.Severity == "critical"
		f.Score = severityScore[f.Severity]
		scan.Findings = append(scan.Findings, f)
	}

	// 文件级扫描
	for _, f := range files {
		scan.FilesScanned++
		// 路径穿越
		if !safeRelPath(f.Path) {
			addFinding(model.SecurityFinding{RuleID: "FILE-01", Category: "integrity", Severity: "critical",
				Title: ruleTitle("FILE-01"), Detail: ruleDetail("FILE-01"), File: f.Path})
		}
		ext := strings.ToLower(path.Ext(f.Path))
		name := strings.ToLower(path.Base(f.Path))
		if execExt[ext] {
			addFinding(model.SecurityFinding{RuleID: "MAL-05", Category: "malware", Severity: "high",
				Title: ruleTitle("MAL-05"), Detail: ruleDetail("MAL-05"), File: f.Path,
				Evidence: fmt.Sprintf("可执行文件: %s (%s)", f.Path, humanBytes(f.Size))})
		}
		if len(f.Content) == 0 {
			continue
		}

		// 二进制载荷: 只做病毒特征匹配 (查毒不能因文件类型/编码而失效)
		if f.Skipped || LooksBinary(f.Content) {
			scan.BytesScanned += int64(len(f.Content))
			raw := string(f.Content)
			for _, r := range rules {
				if r.category != "malware" || r.pattern == nil {
					continue
				}
				if lineNo, snippet, ok := firstMatch(r.pattern, raw); ok {
					addFinding(model.SecurityFinding{
						RuleID: r.id, Category: r.category, Severity: r.severity,
						Title: r.title, Detail: r.detail, File: f.Path, Line: lineNo,
						Evidence: maskEvidence(snippet),
					})
				}
			}
			continue
		}
		text := f.Text
		if text == "" {
			text = string(f.Content)
		}
		scope := scopeCode
		if docExt[ext] || name == "skill.md" {
			scope = scopeDoc
		}
		scan.BytesScanned += int64(len(f.Content))

		for _, r := range rules {
			if r.scope != scope || r.pattern == nil {
				continue
			}
			lineNo, snippet, ok := firstMatch(r.pattern, text)
			if !ok {
				continue
			}
			addFinding(model.SecurityFinding{
				RuleID: r.id, Category: r.category, Severity: r.severity,
				Title: r.title, Detail: r.detail, File: f.Path, Line: lineNo,
				Evidence: maskEvidence(snippet),
			})
		}

		// 特殊检测: 超长行 / 零宽字符 / 依赖未固定 / 声明一致性
		if scope == scopeCode {
			if ln, snip, ok := longLine(text, 1200); ok {
				addFinding(model.SecurityFinding{RuleID: "OBF-03", Category: "obfuscation", Severity: "medium",
					Title: ruleTitle("OBF-03"), Detail: ruleDetail("OBF-03"), File: f.Path, Line: ln,
					Evidence: maskEvidence(truncate(snip, 120))})
			}
			if line, dep, ok := unpinnedDependency(text); ok {
				addFinding(model.SecurityFinding{RuleID: "SUP-01", Category: "supply_chain", Severity: "medium",
					Title: ruleTitle("SUP-01"), Detail: ruleDetail("SUP-01"), File: f.Path, Line: line,
					Evidence: maskEvidence(dep)})
			}
			if scriptExt[ext] {
				addFinding(model.SecurityFinding{RuleID: "MAL-05", Category: "malware", Severity: "medium",
					Title: "脚本载荷需人工确认", Detail: "技能包含可直接执行脚本, 需确认来源与用途", File: f.Path,
					Evidence: path.Base(f.Path)})
			}
		}
		if scope == scopeDoc {
			if n, sample := countZeroWidth(text); n > 3 {
				addFinding(model.SecurityFinding{RuleID: "INJ-02", Category: "injection", Severity: "high",
					Title: ruleTitle("INJ-02"), Detail: ruleDetail("INJ-02"), File: f.Path,
					Evidence: fmt.Sprintf("检出 %d 个不可见字符, 示例 U+%04X", n, sample)})
			}
			// 元数据完整性仅检查技能定义文件 (SKILL.md), 不对仓库内其他说明文档扣分
			if name == "skill.md" {
				if ok, why := metadataMissing(text); ok {
					addFinding(model.SecurityFinding{RuleID: "HALL-02", Category: "compliance", Severity: "medium",
						Title: ruleTitle("HALL-02"), Detail: ruleDetail("HALL-02") + " (" + why + ")", File: f.Path})
				}
			}
		}
	}

	// 外部查毒引擎 (ClamAV / YARA): 可用时合并结论, 不可用则降级 (状态随扫描记录留痕)
	avFindings, avStatus := ScanWithExternalAV(files)
	for _, st := range avStatus {
		scan.AVEngines = append(scan.AVEngines, model.AVEngine{Engine: st.Engine, Available: st.Available,
			Binary: st.Binary, Rules: st.Rules, Version: st.Version, Detail: st.Detail})
	}
	for _, f := range avFindings {
		addFinding(f)
	}
	scan.KeyID = KeyID()

	// 跨文件一致性: 声明"无网络/只读"但代码有网络/写操作
	if ok, why := declaredVsActual(files); ok {
		addFinding(model.SecurityFinding{RuleID: "HALL-01", Category: "compliance", Severity: "high",
			Title: ruleTitle("HALL-01"), Detail: ruleDetail("HALL-01") + ": " + why})
	}

	finalize(scan, time.Since(start))
	return scan
}

// finalize 汇总打分与结论
func finalize(scan *model.SecurityScan, elapsed time.Duration) {
	scan.DurationMs = int(elapsed.Milliseconds())
	rescore(scan)
}

// MergeFindings 合并额外发现 (如动态沙箱行为结论) 并重算结论
func MergeFindings(scan *model.SecurityScan, extra []model.SecurityFinding) {
	for _, f := range extra {
		if f.CategoryCN == "" {
			f.CategoryCN = categoryCN[f.Category]
		}
		if f.Score == 0 {
			f.Score = severityScore[f.Severity]
		}
		if f.Severity == "critical" {
			f.Blocking = true
		}
		scan.Findings = append(scan.Findings, f)
	}
	rescore(scan)
}

// rescore 重算计数 / 风险分 / 结论 / 等级 / 摘要
func rescore(scan *model.SecurityScan) {
	sort.SliceStable(scan.Findings, func(i, j int) bool {
		return severityRank(scan.Findings[i].Severity) < severityRank(scan.Findings[j].Severity)
	})
	total := 0
	scan.CriticalCount, scan.HighCount = 0, 0
	for _, f := range scan.Findings {
		total += f.Score
		switch f.Severity {
		case "critical":
			scan.CriticalCount++
		case "high":
			scan.HighCount++
		}
	}
	if total > 100 {
		total = 100
	}
	scan.RiskScore = total
	scan.FindingCount = len(scan.Findings)

	switch {
	case scan.CriticalCount > 0:
		scan.Verdict, scan.VerdictCN = "malicious", "高危拦截"
	case total >= 45 || scan.HighCount >= 2:
		scan.Verdict, scan.VerdictCN = "suspicious", "可疑待核"
	default:
		scan.Verdict, scan.VerdictCN = "safe", "安全"
	}
	switch {
	case total >= 60:
		scan.Grade = "D"
	case total >= 30:
		scan.Grade = "C"
	case total >= 10:
		scan.Grade = "B"
	default:
		scan.Grade = "A"
	}
	scan.Summary = buildSummary(scan)
}

func buildSummary(scan *model.SecurityScan) string {
	if scan.FindingCount == 0 {
		return fmt.Sprintf("未发现风险项, 共扫描 %d 个文件 / %s, 风险分 %d, 结论: 安全",
			scan.FilesScanned, humanBytes(scan.BytesScanned), scan.RiskScore)
	}
	top := scan.Findings[0]
	return fmt.Sprintf("共发现 %d 项风险 (严重 %d / 高 %d), 风险分 %d, 结论: %s; 首要问题: %s (%s)",
		scan.FindingCount, scan.CriticalCount, scan.HighCount, scan.RiskScore, scan.VerdictCN, top.Title, top.File)
}

func severityRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	default:
		return 3
	}
}

func ruleTitle(id string) string {
	for _, r := range rules {
		if r.id == id {
			return r.title
		}
	}
	return id
}

func ruleDetail(id string) string {
	for _, r := range rules {
		if r.id == id {
			return r.detail
		}
	}
	return ""
}

func fallback(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
