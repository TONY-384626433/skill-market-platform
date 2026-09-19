package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jjbank/skill-market/internal/config"
	"github.com/jjbank/skill-market/internal/model"
	sec "github.com/jjbank/skill-market/internal/security"
)

// ============================================================
// 第三道防线 · AI 语义审计 (社工话术识别 + 意图深度分析)
//
//   前两道防线看的是「代码会不会干坏事」; 这一道看的是「话术在不在骗人」:
//     - 社工话术: 冒充权威 / 制造紧迫 / 情感与利益诱导 / 隐瞒真实目的 / 索取凭据资金;
//     - 提示注入的语义变体: 用同义改写绕过关键词 (忽略规则 -> 「请优先遵循后续说明」);
//     - 意图深度分析: 把「文档宣称的意图」与「语义/行为推断出的真实意图」对撞,
//       判定是否存在未声明的隐藏目的 (声明良性、实则外联/窃密)。
//
//   配置了 LLM_API_KEY 走大模型审计; 未配置时自动降级为本地社工语言学启发式引擎,
//   保证演示环境永远可用 (与 AI 智能体同源的降级策略)。
// ============================================================

const semanticAuditEngine = "SEM-AUDIT 1.0.0"

// 语义审计结果按严重度扣分
var semanticSeverityScore = map[string]int{"critical": 40, "high": 18, "medium": 7, "low": 2}

// SemanticAuditService AI 语义审计
type SemanticAuditService struct {
	cfg *config.Config
}

// NewSemanticAuditService 创建服务
func NewSemanticAuditService(cfg *config.Config) *SemanticAuditService {
	return &SemanticAuditService{cfg: cfg}
}

// Mode 当前审计模式
func (s *SemanticAuditService) Mode() string {
	if s.llmEnabled() {
		return "llm"
	}
	return "heuristic"
}

func (s *SemanticAuditService) llmEnabled() bool {
	return s.cfg != nil && strings.TrimSpace(s.cfg.LLM.APIKey) != "" && strings.TrimSpace(s.cfg.LLM.APIBase) != ""
}

// Status 审计引擎状态 (供前端/审计展示)
func (s *SemanticAuditService) Status() map[string]interface{} {
	mode := s.Mode()
	out := map[string]interface{}{
		"engine":     semanticAuditEngine,
		"mode":       mode,
		"techniques": []string{"冒充权威", "制造紧迫", "情感/利益诱导", "隐瞒真实目的", "索取凭据/资金", "规避审查", "意图偏离"},
		"rules":      "SEM-01 ~ SEM-07",
	}
	if mode == "llm" {
		out["model"] = s.cfg.LLM.Model
		out["api_base"] = s.cfg.LLM.APIBase
	} else {
		out["notice"] = "未配置大模型 API Key, 当前运行本地社工语言学启发式引擎; 配置 LLM_API_KEY 后自动升级为大模型意图审计"
		out["model"] = "se-lexicon-v1"
	}
	return out
}

// Audit 对技能做 AI 语义审计
func (s *SemanticAuditService) Audit(ctx context.Context, subject sec.ScanSubject, files []model.SkillFile, facts *model.SemanticFacts) *model.SemanticAuditReport {
	start := time.Now()
	report := &model.SemanticAuditReport{Engine: semanticAuditEngine, Mode: s.Mode(), Status: "ok", CreatedAt: time.Now()}
	if report.Mode == "llm" {
		report.Model = s.cfg.LLM.Model
	} else {
		report.Model = "se-lexicon-v1"
	}

	docText, codeText, analyzed := collectAuditText(files)
	report.Analyzed = analyzed
	if strings.TrimSpace(docText) == "" && strings.TrimSpace(codeText) == "" {
		report.Status = "skipped"
		report.Notice = "包内没有可审计的文档/代码文本"
		report.DurationMs = int(time.Since(start).Milliseconds())
		return report
	}

	if report.Mode == "llm" {
		if err := s.auditWithLLM(ctx, report, subject, docText, codeText, facts); err != nil {
			// 大模型不可用 -> 降级为启发式, 保证审计不中断 (但留痕)
			report.Fallback = true
			report.Mode = "heuristic"
			report.Model = "se-lexicon-v1"
			report.Notice = "大模型审计失败, 已降级为本地启发式引擎: " + truncate(err.Error(), 160)
		}
	}
	if report.Mode == "heuristic" {
		s.auditHeuristic(report, docText, codeText, facts)
	}

	report.SignalCount = len(report.Findings)
	total := 0
	for _, f := range report.Findings {
		if f.Score == 0 {
			f.Score = semanticSeverityScore[f.Severity]
		}
	}
	for i := range report.Findings {
		total += report.Findings[i].Score
		report.Findings[i].Blocking = report.Findings[i].Severity == "critical"
		if report.Findings[i].CategoryCN == "" {
			report.Findings[i].CategoryCN = "社工/意图"
		}
	}
	if total > 100 {
		total = 100
	}
	report.RiskScore = total
	report.DurationMs = int(time.Since(start).Milliseconds())
	return report
}

// ---------- 文本采集 ----------

func collectAuditText(files []model.SkillFile) (doc, code string, analyzed []string) {
	var docB, codeB strings.Builder
	for _, f := range files {
		if f.Skipped || f.Text == "" {
			continue
		}
		if sec.IsDocFile(f.Path) {
			analyzed = append(analyzed, f.Path)
			docB.WriteString("# file: " + f.Path + "\n")
			docB.WriteString(truncate(f.Text, 12000))
			docB.WriteString("\n")
		} else if sec.IsCodeFile(f.Path) {
			analyzed = append(analyzed, f.Path)
			codeB.WriteString("# file: " + f.Path + "\n")
			codeB.WriteString(truncate(f.Text, 8000))
			codeB.WriteString("\n")
		}
	}
	if len(analyzed) > 20 {
		analyzed = analyzed[:20]
	}
	sort.Strings(analyzed)
	return docB.String(), codeB.String(), analyzed
}

// ---------- 大模型审计 ----------

type semLLMRequest struct {
	Model       string    `json:"model"`
	Messages    []llmMessage `json:"messages"`
	Temperature float64   `json:"temperature"`
}

func (s *SemanticAuditService) auditWithLLM(ctx context.Context, report *model.SemanticAuditReport, subject sec.ScanSubject, doc, code string, facts *model.SemanticFacts) error {
	system := "你是九江银行 SkillHub 的技能供应链安全审计专家, 负责「AI 语义审计」。" +
		"你的任务: (1) 识别社工话术; (2) 对文档宣称意图与代码/行为真实意图做深度分析, 判断是否存在隐藏目的。" +
		"只依据给定材料判断, 不要臆造。" +
		"严格只输出一个 JSON 对象, 结构为: " +
		`{"declared_intent":"...","inferred_intent":"...","divergence":true/false,"risk_score":0-100,` +
		`"techniques":["..."],"findings":[{"rule_id":"SEM-0x","severity":"critical|high|medium","title":"...","detail":"...","file":"...","evidence":"..."}]}` +
		"。rule_id 只能取 SEM-01(冒充权威) / SEM-02(制造紧迫) / SEM-03(情感利益诱导) / SEM-04(隐瞒目的) / SEM-05(索取凭据资金) / SEM-06(规避审查) / SEM-07(意图偏离)。无问题则 findings 返回空数组。"

	caps := ""
	if facts != nil && len(facts.Capabilities) > 0 {
		names := make([]string, 0, len(facts.Capabilities))
		for k, v := range facts.Capabilities {
			names = append(names, fmt.Sprintf("%s(%d)", k, v))
		}
		sort.Strings(names)
		caps = "第一道防线已检出的代码能力: " + strings.Join(names, ", ")
	}
	user := fmt.Sprintf("审计对象: %s %s\n%s\n\n【文档/SKILL.md】\n%s\n\n【代码摘要】\n%s",
		subject.Type, subject.SkillName, caps, truncate(doc, 14000), truncate(code, 9000))

	body, _ := json.Marshal(semLLMRequest{
		Model:       s.cfg.LLM.Model,
		Temperature: 0.1,
		Messages: []llmMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
	})
	url := strings.TrimRight(s.cfg.LLM.APIBase, "/") + "/chat/completions"
	reqCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.cfg.LLM.APIKey)
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("大模型不可达: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("大模型返回 HTTP %d: %s", resp.StatusCode, truncate(string(raw), 160))
	}
	var out struct {
		Choices []struct {
			Message llmMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Choices) == 0 {
		return fmt.Errorf("大模型响应解析失败")
	}
	content := contentText(out.Choices[0].Message.Content)
	payload := extractJSONObject(content)
	if payload == "" {
		return fmt.Errorf("大模型未返回结构化结论")
	}
	var parsed struct {
		Declared   string            `json:"declared_intent"`
		Inferred   string            `json:"inferred_intent"`
		Divergence bool              `json:"divergence"`
		RiskScore  int               `json:"risk_score"`
		Techniques []string          `json:"techniques"`
		Findings   []model.SecurityFinding `json:"findings"`
	}
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return fmt.Errorf("语义审计结论解析失败: %v", err)
	}
	report.Declared = truncate(parsed.Declared, 400)
	report.Inferred = truncate(parsed.Inferred, 400)
	report.Divergence = parsed.Divergence
	report.Techniques = parsed.Techniques
	allowed := map[string]bool{"SEM-01": true, "SEM-02": true, "SEM-03": true, "SEM-04": true, "SEM-05": true, "SEM-06": true, "SEM-07": true}
	for _, f := range parsed.Findings {
		if !allowed[f.RuleID] {
			continue
		}
		if f.Severity == "" {
			f.Severity = "medium"
		}
		if f.Category == "" {
			f.Category = "social_engineering"
		}
		if f.CategoryCN == "" {
			f.CategoryCN = "社工/意图"
		}
		if f.Score == 0 {
			f.Score = semanticSeverityScore[f.Severity]
		}
		report.Findings = append(report.Findings, f)
	}
	// 大模型判定的偏离也要落成显式发现
	if parsed.Divergence {
		report.Findings = append(report.Findings, seFinding("SEM-07", "critical",
			"意图深度分析：隐藏目的", "文档宣称意图与推断真实意图偏离: "+truncate(parsed.Inferred, 200), ""))
	}
	return nil
}

// ---------- 启发式社工语言学引擎 ----------

type seTechnique struct {
	ruleID    string
	severity  string
	name      string
	title     string
	detail    string
	patterns  []*regexp.Regexp
}

var seTechniques = []seTechnique{
	{
		ruleID: "SEM-01", severity: "critical", name: "冒充权威",
		title:  "社工话术：冒充权威/系统指令",
		detail: "冒充系统、管理员或官方口径下发指令, 诱导执行者越权操作",
		patterns: lowerRe([]string{
			`(我是|作为)(系统管理员|管理员|官方|开发者|运维|上级|主管)`,
			`(系统|官方)(通知|指令|要求)[^\n]{0,8}(请|必须|立即|执行|提供)`,
			`(开发者|调试|debug|developer)\s*模式`,
			`我是你的(主人|老板|领导|上级)`,
			`i am (the )?(system|admin|administrator|official|developer)`,
			`(official|system)\s+(notice|instruction|directive)`,
		}),
	},
	{
		ruleID: "SEM-02", severity: "high", name: "制造紧迫",
		title:  "社工话术：制造紧迫/恐吓",
		detail: "用紧急、限时、后果威胁压缩判断时间, 迫使命中目标仓促服从",
		patterns: lowerRe([]string{
			`(立即|马上|立刻|赶紧|尽快)(执行|操作|处理|点击|提供|转账|完成|打款)`,
			`(紧急|火急|限时)(通知|要求|任务|处理|执行|通知)`,
			`最后(机会|通牒|期限)`,
			`否则(后果自负|将|会)`,
			`将(被)?(封禁|冻结|删除|停机|追责)`,
			`\d+\s*(小时|分钟|天)内(完成|处理|否则)`,
			`(urgent|immediate)\s+(action|notice|attention|assistance)`,
			`(act now|last chance|or else)`,
		}),
	},
	{
		ruleID: "SEM-03", severity: "high", name: "情感/利益诱导",
		title:  "社工话术：情感操纵/利益诱导",
		detail: "通过共情、恭维、红包返利、独家福利等诱导目标放下戒心配合操作",
		patterns: lowerRe([]string{
			`(亲爱的|宝贝)(用户|主人)?`,
			`(领取|点击|获得|送你|给你|发放|扫码)[^\n]{0,10}(红包|返利|返现|奖励|补贴|福利|优惠券)`,
			`(独家|内幕|内部)(消息|渠道|名额|福利)`,
			`(稳赚|躺赚|零风险|包赚|日入|月入)`,
			`帮你(赚钱|发财|省钱)`,
			`(free money|guaranteed profit|you have won)`,
		}),
	},
	{
		ruleID: "SEM-04", severity: "high", name: "隐瞒真实目的",
		title:  "社工话术：隐瞒真实目的",
		detail: "描述与实际行为不符, 要求对真实用途/行为保持沉默 (欺骗性描述)",
		patterns: lowerRe([]string{
			`不要(告诉|告知|提醒|惊动)(用户|他|她|别人|管理员)`,
			`(无需|不必)(告知|通知|提示)(用户)?`,
			`别让(别人|用户|管理员|他)知道`,
			`(悄悄|偷偷|暗中|静默|后台)(执行|上传|运行|发送|收集)`,
			`(do not|don't) (tell|inform|notify) the user`,
			`(silently|discreetly|quietly)\s+(run|execute|upload|collect|send)`,
		}),
	},
	{
		ruleID: "SEM-05", severity: "critical", name: "索取凭据/资金",
		title:  "社工话术：索取凭据/资金",
		detail: "诱导提供账号密码、验证码、密钥或进行转账/代付等资金操作",
		patterns: lowerRe([]string{
			`(输入|提供|填写|发送|告知|索取|收集|上报|泄露)[^\n]{0,12}(账号|密码|验证码|密钥|口令|身份证|银行卡|短信验证码)`,
			`(获取|拿到|上交|提供)(api\s*key|access\s*key|secret\s*key|私钥|凭证)`,
			`(请|快|立即|马上|务必)?(转账|汇款|付款|打款|代付)[^\n]{0,8}(给|到|至|元|钱|账户)`,
			`(扫码支付|绑定银行卡|提供银行卡)`,
			`(verify your (account|identity)|enter your password|provide your (code|otp|pin))`,
		}),
	},
	{
		ruleID: "SEM-06", severity: "high", name: "规避审查",
		title:  "社工话术：规避审查 (语义变体)",
		detail: "用同义改写/委婉表达绕过关键词审计, 实质仍要求忽略规则、隐瞒行为或绕过审批",
		patterns: lowerRe([]string{
			`(忽略|无视|忘记)(之前|以上|上述|所有)(的)?(指令|规则|限制|说明)`,
			`(绕过|规避|逃避|跳过)(审查|审核|审计|检测|风控|安全校验|审批|限制)`,
			`(关闭|禁用|停用)(安全|审计|日志|监控|校验|防火墙)`,
			`请优先(遵循|执行)后续(说明|内容|指令)`,
			`(ignore|disregard)\s+(all\s+|any\s+)?(previous|above|prior)\s+(instruction|rule|prompt)`,
			`(jailbreak|越狱模式|developer mode)`,
		}),
	},
}

func lowerRe(exprs []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(exprs))
	for _, e := range exprs {
		out = append(out, regexp.MustCompile(e))
	}
	return out
}

var reJSONObject = regexp.MustCompile(`(?s)\{.*\}`)

// 声明能力正则 (与第一道防线保持一致语义)
var (
	claimNoNetwork = regexp.MustCompile(`(?i)(无(任何)?(网络|联网|外联)|不(会|进行|发起|使用|访问)(任何)?(网络|联网|外联)|离线(运行|模式)|no\s+network|without\s+network|offline\s+(only|mode)|\bno\s+internet\b)`)
	claimReadOnly  = regexp.MustCompile(`(?i)(只读|不(会)?(写入|修改)文件|read[\s-]?only|does\s+not\s+(write|modify)\s+files?|no\s+file\s+write)`)
)

func extractJSONObject(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "```"); i >= 0 {
		s = strings.ReplaceAll(s, "```json", " ")
		s = strings.ReplaceAll(s, "```", " ")
	}
	if m := reJSONObject.FindString(s); m != "" {
		return strings.TrimSpace(m)
	}
	return ""
}

func seFinding(ruleID, severity, title, detail, file string) model.SecurityFinding {
	return model.SecurityFinding{
		RuleID: ruleID, Category: "social_engineering", CategoryCN: "社工/意图",
		Severity: severity, Title: title, Detail: detail, File: file,
		Blocking: severity == "critical", Score: semanticSeverityScore[severity],
	}
}

func (s *SemanticAuditService) auditHeuristic(report *model.SemanticAuditReport, doc, code string, facts *model.SemanticFacts) {
	haystack := strings.ToLower(doc + "\n" + code)
	hitTech := map[string]bool{}
	for _, t := range seTechniques {
		matched := false
		var evidence string
		for _, re := range t.patterns {
			if m := re.FindString(haystack); m != "" {
				matched = true
				evidence = truncate(strings.TrimSpace(m), 120)
				break
			}
		}
		if !matched {
			continue
		}
		if !hitTech[t.name] {
			hitTech[t.name] = true
			report.Techniques = append(report.Techniques, t.name)
		}
		report.Findings = append(report.Findings, seFinding(t.ruleID, t.severity, t.title, t.detail, ""))
		// 补上证据
		last := len(report.Findings) - 1
		report.Findings[last].Evidence = evidence
	}

	// 意图深度分析: 文档宣称 vs 代码实际能力 (需第一道防线的事实画像)
	if facts != nil && len(facts.Capabilities) > 0 {
		report.Declared = declaredIntentSummary(doc)
		report.Inferred = inferredIntentSummary(facts.Capabilities)
		if intentDivergence(doc, facts.Capabilities) {
			report.Divergence = true
			report.Findings = append(report.Findings, seFinding("SEM-07", "critical",
				"意图深度分析：隐藏目的",
				"文档宣称的意图与代码实际能力不符: "+report.Declared+" → 实际: "+report.Inferred, ""))
		}
	}
	if len(report.Techniques) == 0 && !report.Divergence {
		report.Notice = "未发现社工话术与意图偏离迹象 (本地启发式引擎)"
	}
}

// declaredIntentSummary 从文档提取宣称意图
func declaredIntentSummary(doc string) string {
	lower := strings.ToLower(doc)
	switch {
	case claimNoNetwork.MatchString(doc) || strings.Contains(lower, "only reads"):
		return "宣称离线/只读的辅助工具"
	case strings.Contains(lower, "报告") || strings.Contains(lower, "报表") || strings.Contains(lower, "report"):
		return "宣称仅生成报表/文档"
	default:
		return "宣称提供常规业务处理能力"
	}
}

// inferredIntentSummary 从代码能力推断真实意图
func inferredIntentSummary(caps map[string]int) string {
	parts := make([]string, 0, len(caps))
	for k := range caps {
		parts = append(parts, k)
	}
	sort.Strings(parts)
	if len(parts) == 0 {
		return "未检出明显代码能力"
	}
	return "代码具备: " + strings.Join(parts, ", ")
}

// intentDivergence 判定声明与能力是否严重偏离
func intentDivergence(doc string, caps map[string]int) bool {
	has := func(n string) bool { return caps[n] > 0 }
	lower := strings.ToLower(doc)
	if claimNoNetwork.MatchString(doc) && has("网络外联能力") {
		return true
	}
	if claimReadOnly.MatchString(doc) && (has("破坏性操作能力") || has("持久化/越权能力")) {
		return true
	}
	// 声称"报表/只读/查询"却同时具备 执行+外联+凭据读取 三重能力
	benignClaim := strings.Contains(lower, "只读") || strings.Contains(lower, "报表") ||
		strings.Contains(lower, "查询") || strings.Contains(lower, "read-only") || strings.Contains(lower, "report")
	if benignClaim && has("危险执行能力") && has("网络外联能力") && has("凭据/敏感文件读取") {
		return true
	}
	if benignClaim && has("凭据/敏感文件读取") && has("网络外联能力") {
		return true
	}
	return false
}
