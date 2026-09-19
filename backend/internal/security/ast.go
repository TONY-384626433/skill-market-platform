package security

import (
	"encoding/base64"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jjbank/skill-market/internal/model"
)

// ============================================================
// 第一道防线 · 代码级检测 (静态分析 + AST/能力语义分析)
//
//   正则规则 (scanner.go) 只能匹配「字面量」, 会被注释拆行、大小写、
//   字符串拼接、base64/hex/char-code 编码轻易绕过。
//   本引擎做的是「语义」层面的判断:
//     1) 归一化: 去注释 / 合并字符串拼接 / 展开 \\x 转义;
//     2) 解码:   base64 / hex / char-code / fromCharCode 载荷还原后重新扫描;
//     3) 能力推理: 不匹配单个危险词, 而是判断「这段代码具不具备
//        执行 / 外联 / 读凭据 / 破坏 / 持久化 / 反沙箱」这类能力;
//     4) Go 文件走真正的 AST (go/parser) 遍历调用表达式, 而非文本匹配;
//     5) 语义一致性: 声明能力(文档) vs 实际能力(代码) 是否自相矛盾 (防幻觉/隐瞒)。
//
//   结论以 AST-01 ~ AST-08 规则编号输出, 与规则库文件保持一致以便审计。
// ============================================================

const semanticEngineVersion = "SEMAST-ENGINE 1.0.0"

// SemanticEngine 语义分析引擎版本
func SemanticEngine() string { return semanticEngineVersion }

type semCapability struct {
	ruleID   string
	category string
	severity string
	name     string
	title    string
	detail   string
	patterns []*regexp.Regexp
}

func mustList(exprs []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(exprs))
	for _, e := range exprs {
		out = append(out, regexp.MustCompile(e))
	}
	return out
}

var semCapabilities = []semCapability{
	{
		ruleID: "AST-01", category: "execution", severity: "critical", name: "危险执行能力",
		title:  "语义分析：具备系统命令/动态代码执行能力",
		detail: "从代码语义判断, 该文件可执行系统命令或动态求值代码, 是后门/木马/窃密脚本的核心能力",
		patterns: mustList([]string{
			`(?:^|[^.\w])eval\s*\(`,
			`(?:^|[^.\w])exec\s*\(`,
			`(?:^|[^.\w])(execfile|compile)\s*\(`,
			`\b__import__\s*\(`,
			`\bos\s*\.\s*system\s*\(`,
			`\bos\s*\.\s*popen\s*\(`,
			`\bsubprocess\s*\.\s*(popen|run|call|check_output|check_call)`,
			`\bos\s*\.\s*spawn[lvpe]*\s*\(`,
			`\bpty\s*\.\s*spawn`,
			`\bchild_process\s*\.\s*(exec|execsync|spawn|spawnsync|fork)`,
			`(?:^|[^.\w])execsync\s*\(`,
			`(?:^|[^.\w])spawnsync\s*\(`,
			`\bnew\s+function\s*\(`,
			`\bexec\s*\.\s*command\s*\(`,
			`\binvoke-expression\b`,
			`\bstart-process\b`,
			`\bsyscall\s*\.\s*exec`,
			`(?:^|[^.\w])popen\s*\(`,
			`(?:^|[^.\w])system\s*\(`,
		}),
	},
	{
		ruleID: "AST-02", category: "network", severity: "high", name: "网络外联能力",
		title:  "语义分析：具备网络外联能力",
		detail: "代码语义上可向外部发起网络请求 (可能用于回传数据/C2 通信), 需结合文档声明核对",
		patterns: mustList([]string{
			`\brequests\s*\.\s*(get|post|put|delete|patch|request|session)\b`,
			`\burllib\s*\.\s*request`,
			`\burlopen\s*\(`,
			`\bhttpx\s*\.`,
			`\baiohttp\s*\.`,
			`\bsocket\s*\.\s*(socket|create_connection)\s*\(`,
			`\bhttp\s*\.\s*client\b`,
			`\bnet\s*\.\s*(dial|dialtimeout)\b`,
			`\bhttps?\s*\.\s*(get|post|newrequest)\b`,
			`\bfetch\s*\(`,
			`\baxios\s*\.`,
			`\binvoke-webrequest\b`,
			`\binvoke-restmethod\b`,
			`\bwebclient\b`,
			`\bcurl\s+(-[a-z]+\s+)*https?://`,
			`\bwget\s+.*https?://`,
		}),
	},
	{
		ruleID: "AST-03", category: "credential", severity: "critical", name: "凭据/敏感文件读取",
		title:  "语义分析：读取凭据或系统敏感文件",
		detail: "语义上会读取密钥/密码/凭据文件或系统账号文件, 典型的窃密行为特征",
		patterns: mustList([]string{
			`/etc/shadow`,
			`/etc/passwd`,
			`/etc/sudoers`,
			`\.ssh/id_(rsa|dsa|ecdsa|ed25519)`,
			`\bid_rsa\b`,
			`\.aws/credentials`,
			`\.kube/config`,
			`\.git-credentials`,
			`\.npmrc`,
			`\.docker/config\.json`,
			`\bcookies\b`,
			`\blogin data\b`,
			`wallet\.dat`,
			`keystore`,
			`(os\.environ(\.get)?\s*\(\s*['"][^'"]*(secret|token|password|passwd|credential|api[_-]?key|private[_-]?key|access[_-]?key))`,
			`getenv\s*\(\s*['"][^'"]*(secret|password|token|credential|api[_-]?key)`,
		}),
	},
	{
		ruleID: "AST-04", category: "destructive", severity: "high", name: "破坏性操作能力",
		title:  "语义分析：具备文件/数据破坏能力",
		detail: "语义上会删除、清空或格式化数据, 可能造成生产事故或被用于勒索/破坏",
		patterns: mustList([]string{
			`\bshutil\s*\.\s*rmtree\s*\(`,
			`\bos\s*\.\s*(remove|unlink|removedirs|rmdir)\s*\(`,
			`\brm\s+-[rf]`,
			`\brm\s+-rf\s+/`,
			`\bremove-item\b[^\n]*-recurse`,
			`\bdel\s+/[fqs]`,
			`\bformat\s+[a-z]:`,
			`\bmkfs\.`,
			`\bdd\s+if=`,
			`\bdrop\s+(table|database)\b`,
			`\btruncate\s+table\b`,
			`\bfs\s*\.\s*(unlink|rm|rmdir)(sync)?\s*\(`,
		}),
	},
	{
		ruleID: "AST-05", category: "persistence", severity: "high", name: "持久化/越权能力",
		title:  "语义分析：具备持久化或权限提升能力",
		detail: "语义上会写计划任务/开机自启/SSH 公钥/创建用户, 用于长期驻留或提权",
		patterns: mustList([]string{
			`\bcrontab\b`,
			`/etc/cron`,
			`\bsystemctl\s+enable\b`,
			`\brc\.local\b`,
			`\bschtasks\b`,
			`\blaunchctl\b`,
			`\bauthorized_keys\b`,
			`\bnew-service\b`,
			`\buseradd\b`,
			`\bnet\s+user\b[^\n]*/add`,
			`\breg\s+add\b`,
			`hklm\\+software\\+microsoft\\+windows\\+currentversion\\+run`,
			`hkcu\\+software\\+microsoft\\+windows\\+currentversion\\+run`,
			`\bbashrc\b`,
		}),
	},
	{
		ruleID: "AST-06", category: "obfuscation", severity: "critical", name: "编码载荷加载",
		title:  "语义分析：解码后动态执行 (混淆载荷)",
		detail: "在运行时解码 base64/hex/字符码后再求值/执行, 是规避静态审查的典型手法, 高度可疑",
		patterns: mustList([]string{
			`(eval|exec|function|execsync|child_process\s*\.\s*exec)\s*\(\s*(atob|buffer\s*\.\s*from|base64\s*\.\s*b64decode|base64\s*\.\s*decodebytes|bytes\s*\.\s*fromhex|unhexlify|decrypt|zlib\s*\.\s*decompress|gzip\s*\.\s*decompress)`,
			`\bfromcharcode\b`,
			`\bcodecs\s*\.\s*decode\b`,
		}),
	},
	{
		ruleID: "AST-07", category: "evasion", severity: "high", name: "反调试/反沙箱",
		title:  "语义分析：具备反调试/反沙箱检测",
		detail: "代码会检测调试器/虚拟机/沙箱环境或长时间休眠以躲避动态分析, 恶意样本常见特征",
		patterns: mustList([]string{
			`\btracerpid\b`,
			`\bisdebuggerpresent\b`,
			`\bptrace\b`,
			`\.dockerenv`,
			`\bsandboxie\b`,
			`\bvmware\b`,
			`\bvirtualbox\b`,
			`\bqemu\b`,
			`\bwin32_computersystem\b`,
			`\bcheckvm\b`,
			`\bsleep\s*\(\s*[3-9]\d{2,}`,
			`\btime\s*\.\s*sleep\s*\(\s*[3-9]\d{2,}`,
		}),
	},
}

// capabilityName 返回能力名 (供事实画像)
func capabilityName(ruleID string) string {
	for _, c := range semCapabilities {
		if c.ruleID == ruleID {
			return c.name
		}
	}
	return ruleID
}

// severityOrder 用于升级/降级
var severityOrder = map[string]int{"low": 0, "medium": 1, "high": 2, "critical": 3}

func escalateSeverity(sev string) string {
	switch sev {
	case "low":
		return "medium"
	case "medium":
		return "high"
	default:
		return "critical"
	}
}

// semResult 单文件语义分析中间结果
type semResult struct {
	line int
	snip string
}

// AnalyzeSemantics 对技能文件集做静态语义分析 (第一道防线)
// 返回发现的语义风险项 + 能力画像事实
func AnalyzeSemantics(files []model.SkillFile) ([]model.SecurityFinding, *model.SemanticFacts) {
	facts := &model.SemanticFacts{Engine: semanticEngineVersion, Capabilities: map[string]int{}}
	var findings []model.SecurityFinding
	langSet := map[string]bool{}
	var docText strings.Builder

	for _, f := range files {
		if f.Skipped {
			continue
		}
		text := f.Text
		if text == "" {
			continue
		}
		if IsDocFile(f.Path) {
			docText.WriteString(text)
			docText.WriteString("\n")
		}
		if !IsCodeFile(f.Path) && !IsDocFile(f.Path) {
			continue
		}
		if IsDocFile(f.Path) && !IsCodeFile(f.Path) {
			// 文档只做内嵌代码块之外的话术检查, 交由第三道防线; 此处跳过能力推理
			continue
		}
		facts.FilesAnalyzed++

		norm, decoded, decodeCount := normalizeSource(text, commentStyleOf(f.Path))
		langSet[langNameOf(f.Path)] = true
		facts.DecodedPayload += decodeCount

		obfuscatedHit := false
		entry := IsRunnableEntry(f.Path)
		for _, cap := range semCapabilities {
			clean, cleanSnip := cap.match(norm)
			obf, obfSnip := cap.match(strings.Join(decoded, "\n"))
			if !clean && !obf {
				continue
			}
			sev := cap.severity
			evidence := cleanSnip
			detail := cap.detail
			onlyDecoded := !clean && obf
			if onlyDecoded {
				sev = escalateSeverity(sev)
				evidence = obfSnip
				detail = cap.detail + " (命中内容来自 runtime 解码后的载荷, 原始代码中不可见)"
				obfuscatedHit = true
			}
			// 上下文降级: 非技能入口文件(如仓库内的开发/校验脚本)的能力不直接判为阻断级
			if !entry && sev == "critical" {
				sev = "high"
			}
			facts.Capabilities[cap.name]++
			findings = append(findings, model.SecurityFinding{
				RuleID: cap.ruleID, Category: cap.category, CategoryCN: categoryCN[cap.category],
				Severity: sev, Title: cap.title, Detail: detail, File: f.Path,
				Evidence: maskEvidence(evidence), Blocking: sev == "critical",
			})
		}
		if obfuscatedHit {
			// 记录一条「解码后仍具危险能力」的显式结论
			sev := "critical"
			if !entry {
				sev = "high"
			}
			findings = append(findings, model.SecurityFinding{
				RuleID: "AST-06", Category: "obfuscation", CategoryCN: categoryCN["obfuscation"],
				Severity: sev, Title: "语义分析：载荷编码规避",
				Detail: fmt.Sprintf("该文件含 %d 段编码载荷, 解码后命中危险能力, 属主动规避静态审查", decodeCount),
				File:   f.Path, Evidence: fmt.Sprintf("decoded_payloads=%d", decodeCount), Blocking: sev == "critical",
			})
		}

		// Go 源码: 走真正的 AST 遍历 (非文本匹配)
		if strings.EqualFold(path.Ext(f.Path), ".go") {
			for _, g := range analyzeGoAST(f.Path, text) {
				facts.Capabilities[capabilityName(g.RuleID)]++
				findings = append(findings, g)
			}
		}
	}

	if len(langSet) > 0 {
		for l := range langSet {
			if l != "" {
				facts.Languages = append(facts.Languages, l)
			}
		}
		sort.Strings(facts.Languages)
	}
	if facts.FilesAnalyzed == 0 && len(facts.Capabilities) == 0 {
		facts = nil
	} else if facts.FilesAnalyzed > 0 {
		facts.Consistency = consistencyNotes(docText.String(), facts.Capabilities)
	}
	return findings, facts
}

// match 在归一化文本中匹配该能力的任一模式
func (c semCapability) match(text string) (bool, string) {
	if strings.TrimSpace(text) == "" {
		return false, ""
	}
	for _, re := range c.patterns {
		if m := re.FindStringIndex(text); m != nil {
			s := text[m[0]:m[1]]
			if len(s) > 160 {
				s = s[:160]
			}
			return true, strings.TrimSpace(s)
		}
	}
	return false, ""
}

// ---------- 归一化 / 解码 ----------

// commentStyleOf 按扩展名选择注释语法
func commentStyleOf(p string) string {
	switch strings.ToLower(path.Ext(p)) {
	case ".py", ".sh", ".bash", ".zsh", ".rb", ".pl", ".yaml", ".yml", ".toml", ".ini", ".cfg", ".conf", ".env":
		return "hash"
	case ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx", ".go", ".c", ".cpp", ".h", ".cs", ".java", ".kt", ".rs", ".php":
		return "slash"
	default:
		return ""
	}
}

func langNameOf(p string) string {
	switch strings.ToLower(path.Ext(p)) {
	case ".py":
		return "python"
	case ".js", ".mjs", ".cjs", ".jsx":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".go":
		return "go"
	case ".sh", ".bash", ".zsh":
		return "shell"
	case ".ps1", ".psm1":
		return "powershell"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".java":
		return "java"
	default:
		return "generic"
	}
}

var (
	reBlockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	reLineComment  = regexp.MustCompile(`(?m)(^|[^:])//[^\n]*`)
	reHashComment  = regexp.MustCompile(`(?m)#[^\n]*`)
	reStrConcat    = regexp.MustCompile(`(['"])\s*\+\s*(['"])`)
	reDotSpace    = regexp.MustCompile(`\s*\.\s*`)
	reHexEscape    = regexp.MustCompile(`(?:\\x[0-9a-fA-F]{2}){6,}`)
	reB64Token     = regexp.MustCompile(`[A-Za-z0-9+/]{24,}={0,2}`)
	reChrRun       = regexp.MustCompile(`(?:chr\s*\(\s*\d{1,3}\s*\)\s*\+\s*){4,}chr\s*\(\s*\d{1,3}\s*\)`)
	reFromCharCode = regexp.MustCompile(`fromcharcode\s*\(\s*((?:\d{1,3}\s*,\s*){4,}\d{1,3})\s*\)`)
	reByteArray    = regexp.MustCompile(`\[\s*((?:\d{1,3}\s*,\s*){8,}\d{1,3})\s*\]`)
	reIntInParen   = regexp.MustCompile(`\d{1,3}`)
)

// normalizeSource 去注释 + 合并字符串拼接, 返回小写归一化文本与解码载荷
func normalizeSource(src, style string) (string, []string, int) {
	s := src
	if style == "hash" {
		s = reHashComment.ReplaceAllString(s, "")
	} else if style == "slash" {
		s = replaceKeepLines(reBlockComment, s)
		s = reLineComment.ReplaceAllString(s, "$1")
		s = reHashComment.ReplaceAllString(s, "") // php 也支持 #
	} else {
		s = replaceKeepLines(reBlockComment, s)
	}
	s = reHTMLComment.ReplaceAllString(s, " ")
	// 合并相邻/相加的字符串字面量: "ev"+"al" -> "eval" ; "ev" "al" -> "eval"
	for i := 0; i < 4; i++ {
		prev := s
		s = reStrConcat.ReplaceAllString(s, "$1$2")
		if s == prev {
			break
		}
	}
	// 归一化属性访问中的空白/换行: "os.\nsystem" -> "os.system" (跨越换行/注释的规避写法)
	s = reDotSpace.ReplaceAllString(s, ".")
	lower := strings.ToLower(s)

	decoded, count := decodePayloads(lower, s)
	for i := range decoded {
		decoded[i] = strings.ToLower(decoded[i])
	}
	return lower, decoded, count
}

// replaceKeepLines 用等长空白替换块注释 (保留换行, 便于行号对齐)
func replaceKeepLines(re *regexp.Regexp, s string) string {
	return re.ReplaceAllStringFunc(s, func(m string) string {
		b := []byte(m)
		for i := range b {
			if b[i] != '\n' {
				b[i] = ' '
			}
		}
		return string(b)
	})
}

// decodePayloads 解码 hex 转义 / char-code / base64 载荷
//   - lower: 小写归一化文本 (用于 hex / 字符码 / 数组这类大小写无关的模式)
//   - orig:  原始文本 (base64 大小写敏感, 必须用原文解码)
func decodePayloads(lower, orig string) ([]string, int) {
	out := []string{}
	count := 0

	for _, m := range reHexEscape.FindAllString(lower, 6) {
		if d, ok := decodeHexEscape(m); ok && isMostlyPrintable(d) {
			out = append(out, strings.ToLower(d))
			count++
		}
	}
	for _, m := range reChrRun.FindAllString(lower, 6) {
		nums := reIntInParen.FindAllString(m, -1)
		if d, ok := decodeCodes(nums); ok {
			out = append(out, strings.ToLower(d))
			count++
		}
	}
	for _, m := range reFromCharCode.FindAllStringSubmatch(lower, 6) {
		if d, ok := decodeCodes(strings.Split(m[1], ",")); ok {
			out = append(out, strings.ToLower(d))
			count++
		}
	}
	for _, m := range reByteArray.FindAllStringSubmatch(lower, 8) {
		if d, ok := decodeCodes(strings.Split(m[1], ",")); ok && isMostlyPrintable(d) {
			out = append(out, strings.ToLower(d))
			count++
		}
	}
	for _, m := range reB64Token.FindAllString(orig, 24) {
		if len(m) > 200 {
			continue
		}
		if d, ok := decodeB64(m); ok {
			out = append(out, strings.ToLower(d))
			count++
		}
	}
	if len(out) > 20 {
		out = out[:20]
	}
	return out, count
}

func decodeHexEscape(s string) (string, bool) {
	var b strings.Builder
	for i := 0; i+3 < len(s); i += 4 {
		v, err := strconv.ParseUint(s[i+2:i+4], 16, 8)
		if err != nil {
			return "", false
		}
		b.WriteByte(byte(v))
	}
	return b.String(), b.Len() > 0
}

func decodeCodes(parts []string) (string, bool) {
	if len(parts) < 4 {
		return "", false
	}
	var b strings.Builder
	okAny := false
	for _, p := range parts {
		v, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || v < 0 || v > 255 {
			continue
		}
		b.WriteByte(byte(v))
		okAny = true
	}
	return b.String(), okAny && b.Len() >= 4
}

func base64Decode(s string) ([]byte, error) {
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}
	return base64.StdEncoding.DecodeString(s)
}

func decodeB64(s string) (string, bool) {
	if !strings.ContainsAny(s, "+/=") && len(s) < 32 {
		return "", false
	}
	raw, err := base64Decode(s)
	if err != nil || len(raw) < 8 {
		return "", false
	}
	str := string(raw)
	if !isMostlyPrintable(str) {
		return "", false
	}
	// 只保留看起来像代码/文本的解码结果
	return str, strings.ContainsAny(str, "abcdefghijklmnopqrstuvwxyz")
}

func isMostlyPrintable(s string) bool {
	if len(s) == 0 {
		return false
	}
	good := 0
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' || (r >= 32 && r < 127) {
			good++
		}
	}
	return good*100/len([]rune(s)) >= 90
}

// ---------- 声明一致性 ----------

// consistencyNotes 声明能力 vs 实际能力 (语义一致性)
func consistencyNotes(doc string, caps map[string]int) []string {
	if strings.TrimSpace(doc) == "" || len(caps) == 0 {
		return nil
	}
	lower := strings.ToLower(doc)
	notes := []string{}
	has := func(name string) bool { return caps[name] > 0 }
	if claimNoNetwork.MatchString(doc) && has("网络外联能力") {
		notes = append(notes, "文档声明无网络, 但代码具备网络外联能力")
	}
	if claimReadOnly.MatchString(doc) && (has("破坏性操作能力") || has("持久化/越权能力")) {
		notes = append(notes, "文档声明只读, 但代码具备写/删除/持久化能力")
	}
	if (strings.Contains(lower, "只读") || strings.Contains(lower, "read-only")) && has("凭据/敏感文件读取") {
		notes = append(notes, "文档声明仅只读查询, 但代码会读取凭据/敏感文件")
	}
	return notes
}

// ---------- Go AST 遍历 ----------

var goDangerousCalls = map[string]struct {
	ruleID   string
	category string
	severity string
	title    string
}{
	"exec.Command":        {"AST-01", "execution", "critical", "语义分析(AST)：调用 exec.Command 执行外部命令"},
	"exec.CommandContext": {"AST-01", "execution", "critical", "语义分析(AST)：调用 exec.CommandContext 执行外部命令"},
	"os.RemoveAll":        {"AST-04", "destructive", "high", "语义分析(AST)：调用 os.RemoveAll 递归删除"},
	"os.Remove":           {"AST-04", "destructive", "medium", "语义分析(AST)：调用 os.Remove 删除文件"},
	"net.Dial":            {"AST-02", "network", "high", "语义分析(AST)：调用 net.Dial 建立网络连接"},
	"net.DialTimeout":     {"AST-02", "network", "high", "语义分析(AST)：调用 net.DialTimeout 建立网络连接"},
	"http.Get":            {"AST-02", "network", "high", "语义分析(AST)：调用 http.Get 发起外联"},
	"http.Post":           {"AST-02", "network", "high", "语义分析(AST)：调用 http.Post 发起外联"},
	"syscall.Exec":        {"AST-01", "execution", "critical", "语义分析(AST)：调用 syscall.Exec 替换进程映像"},
	"plugin.Open":         {"AST-06", "obfuscation", "high", "语义分析(AST)：调用 plugin.Open 动态加载插件"},
	"os.Setenv":           {"AST-05", "persistence", "low", "语义分析(AST)：调用 os.Setenv 修改进程环境变量"},
}

// analyzeGoAST 用 go/parser 解析 Go 源码并遍历调用表达式 (真正的 AST, 非文本匹配)
func analyzeGoAST(filePath, src string) []model.SecurityFinding {
	fset := token.NewFileSet()
	// 容错解析: 即使有语法错误也尽量取出可用的 AST
	f, err := parser.ParseFile(fset, filePath, src, parser.SkipObjectResolution)
	if f == nil {
		_ = err
		return nil
	}
	var out []model.SecurityFinding
	seen := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := callName(call.Fun)
		if name == "" {
			return true
		}
		spec, ok := goDangerousCalls[name]
		if !ok || seen[name] {
			return true
		}
		seen[name] = true
		pos := fset.Position(call.Pos())
		out = append(out, model.SecurityFinding{
			RuleID: spec.ruleID, Category: spec.category, CategoryCN: categoryCN[spec.category],
			Severity: spec.severity, Title: spec.title,
			Detail: "AST 遍历命中危险调用 (非文本匹配, 不受注释/换行/拼接影响)", File: filePath,
			Line: pos.Line, Blocking: spec.severity == "critical",
			Evidence: name + "(...)",
		})
		return true
	})
	return out
}

// callName 还原调用表达式的可读名 (pkg.Func / Func)
func callName(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		if x, ok := v.X.(*ast.Ident); ok {
			return x.Name + "." + v.Sel.Name
		}
		return v.Sel.Name
	}
	return ""
}
