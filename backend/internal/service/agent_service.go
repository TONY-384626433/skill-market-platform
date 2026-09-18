package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jjbank/skill-market/internal/config"
	"github.com/jjbank/skill-market/internal/model"
)

// ============================================================
// AI 智能体 (Agent) — 自然语言 → 技能编排 → 真实调用
// ============================================================
//
// 设计要点:
//   1. 工具清单不是硬编码的, 而是从「已通过可用性审核」的技能上真实抓取 tools/list 得到;
//   2. 配置了 LLM API Key 时走大模型 function calling 多轮编排;
//      未配置时自动降级为本地意图引擎 (关键词路由 + 参数抽取), 保证演示环境永远可用;
//   3. 每一次技能调用都复用网关的安全规则 (敏感输入拦截 + 脱敏白名单) 并写入审计日志。

const (
	agentEngineVersion = "1.0.0"
	agentMaxRounds     = 4
	agentToolCacheTTL  = 60 * time.Second
)

// AgentService 智能体服务
type AgentService struct {
	db       *sql.DB
	cfg      *config.Config
	skillSvc *SkillService

	mu         sync.Mutex
	toolCache  []model.AgentTool
	toolCacheT time.Time
}

// NewAgentService 创建智能体服务
func NewAgentService(db *sql.DB, cfg *config.Config, skillSvc *SkillService) *AgentService {
	return &AgentService{db: db, cfg: cfg, skillSvc: skillSvc}
}

// ============================================================
// 工具发现
// ============================================================

type agentSkillRow struct {
	ID        string
	SkillKey  string
	Name      string
	Category  string
	SkillType string
}

// publishedAuditedSkills 只把「已发布 + 已通过可用性审核」的技能暴露给智能体
func (s *AgentService) publishedAuditedSkills() ([]agentSkillRow, error) {
	rows, err := s.db.Query(`
		SELECT id, skill_key, name, category, skill_type
		FROM skills
		WHERE status='published' AND COALESCE(audit_status,'pending')='passed'
		ORDER BY category, name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]agentSkillRow, 0)
	for rows.Next() {
		var r agentSkillRow
		if err := rows.Scan(&r.ID, &r.SkillKey, &r.Name, &r.Category, &r.SkillType); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListTools 返回智能体可用的工具清单 (带 60s 缓存)
func (s *AgentService) ListTools(ctx context.Context) ([]model.AgentTool, error) {
	s.mu.Lock()
	if len(s.toolCache) > 0 && time.Since(s.toolCacheT) < agentToolCacheTTL {
		cached := s.toolCache
		s.mu.Unlock()
		return cached, nil
	}
	s.mu.Unlock()

	skills, err := s.publishedAuditedSkills()
	if err != nil {
		return nil, err
	}
	tools := make([]model.AgentTool, 0, 8)
	for _, sk := range skills {
		if sk.SkillType != "mcp" {
			continue
		}
		resp, _, err := s.callRunner(ctx, sk.SkillKey, "tools/list", nil)
		if err != nil || resp.Result == nil {
			continue
		}
		for _, t := range resp.Result.Tools {
			tools = append(tools, model.AgentTool{
				Name:        agentToolName(sk.SkillKey, t.Name),
				SkillID:     sk.ID,
				SkillKey:    sk.SkillKey,
				SkillName:   sk.Name,
				Category:    sk.Category,
				Tool:        t.Name,
				Description: t.Description,
				InputSchema: t.InputSchema,
			})
		}
	}
	s.mu.Lock()
	s.toolCache = tools
	s.toolCacheT = time.Now()
	s.mu.Unlock()
	return tools, nil
}

func agentToolName(skillKey, tool string) string {
	clean := func(v string) string {
		var b strings.Builder
		for _, r := range v {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
				b.WriteRune(r)
			} else {
				b.WriteRune('_')
			}
		}
		return b.String()
	}
	return clean(skillKey) + "__" + clean(tool)
}

// ============================================================
// 对话入口
// ============================================================

// Chat 处理一次智能体对话
func (s *AgentService) Chat(ctx context.Context, userID, message string, history []model.AgentMessage) (*model.AgentAnswer, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil, fmt.Errorf("请输入内容")
	}
	start := time.Now()

	tools, err := s.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	if len(tools) == 0 {
		return nil, fmt.Errorf("当前没有可用技能 (需已发布且通过可用性审核)")
	}

	answer := &model.AgentAnswer{
		SessionID: fmt.Sprintf("ag_%x", time.Now().UnixNano()),
		Question:  message,
		Provider:  "local-intent",
		Model:     "intent-router-v1",
		Steps:     []model.AgentStep{},
		Engine:    agentEngineVersion,
	}
	if s.llmEnabled() {
		answer.Provider = "llm"
		answer.Model = s.cfg.LLM.Model
	}

	var steps []model.AgentStep
	var text string
	if s.llmEnabled() {
		text, steps, err = s.chatWithLLM(ctx, userID, message, history, tools)
		if err != nil {
			// 大模型不可用时降级, 保证演示不中断
			answer.Fallback = true
			answer.Provider = "local-intent"
			answer.Model = "intent-router-v1"
			text, steps = s.chatWithIntentEngine(ctx, userID, message, tools)
			text = fmt.Sprintf("> ⚠ 大模型调用失败已降级为本地意图引擎 (%s)\n\n%s", truncate(err.Error(), 160), text)
		}
	} else {
		answer.Fallback = true
		text, steps = s.chatWithIntentEngine(ctx, userID, message, tools)
	}

	answer.Answer = text
	if steps == nil {
		steps = []model.AgentStep{}
	}
	answer.Steps = steps
	answer.ToolCalls = len(steps)
	answer.DurationMs = int(time.Since(start).Milliseconds())
	return answer, nil
}

func (s *AgentService) llmEnabled() bool {
	return strings.TrimSpace(s.cfg.LLM.APIKey) != "" && strings.TrimSpace(s.cfg.LLM.APIBase) != ""
}

// Status 智能体运行状态
func (s *AgentService) Status(ctx context.Context) map[string]interface{} {
	tools, _ := s.ListTools(ctx)
	skills := map[string]bool{}
	for _, t := range tools {
		skills[t.SkillKey] = true
	}
	mode := "local-intent"
	model := "intent-router-v1"
	if s.llmEnabled() {
		mode = "llm"
		model = s.cfg.LLM.Model
	}
	return map[string]interface{}{
		"mode":           mode,
		"model":          model,
		"api_base":       s.cfg.LLM.APIBase,
		"tool_count":     len(tools),
		"skill_count":    len(skills),
		"engine_version": agentEngineVersion,
		"max_rounds":     agentMaxRounds,
	}
}

// ============================================================
// 大模型编排 (OpenAI 兼容 function calling)
// ============================================================

type llmToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type llmMessage struct {
	Role       string        `json:"role"`
	Content    interface{}   `json:"content,omitempty"`
	ToolCalls  []llmToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	Name       string        `json:"name,omitempty"`
}

type llmRequest struct {
	Model       string        `json:"model"`
	Messages    []llmMessage  `json:"messages"`
	Tools       []interface{} `json:"tools,omitempty"`
	ToolChoice  string        `json:"tool_choice,omitempty"`
	Temperature float64       `json:"temperature"`
}

type llmResponse struct {
	Choices []struct {
		Message      llmMessage `json:"message"`
		FinishReason string     `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (s *AgentService) chatWithLLM(ctx context.Context, userID, message string, history []model.AgentMessage, tools []model.AgentTool) (string, []model.AgentStep, error) {
	specs := make([]interface{}, 0, len(tools))
	for _, t := range tools {
		params := t.InputSchema
		if params == nil {
			params = map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
		}
		specs = append(specs, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        t.Name,
				"description": fmt.Sprintf("[%s] %s", t.SkillName, t.Description),
				"parameters":  params,
			},
		})
	}

	msgs := []llmMessage{{
		Role:    "system",
		Content: "你是九江银行 SkillHub 的技能编排智能体。你可以调用平台上已通过可用性审核的技能工具来回答用户问题。" + "请优先调用真实技能获取数据, 再用简洁的中文总结结果; 不要编造技能返回之外的数字。",
	}}
	for _, h := range history {
		role := h.Role
		if role != "assistant" {
			role = "user"
		}
		msgs = append(msgs, llmMessage{Role: role, Content: h.Content})
	}
	msgs = append(msgs, llmMessage{Role: "user", Content: message})

	steps := make([]model.AgentStep, 0)
	for round := 0; round < agentMaxRounds; round++ {
		resp, err := s.callLLM(ctx, llmRequest{Model: s.cfg.LLM.Model, Messages: msgs, Tools: specs, ToolChoice: "auto", Temperature: 0.2})
		if err != nil {
			return "", steps, err
		}
		if len(resp.Choices) == 0 {
			return "", steps, fmt.Errorf("大模型未返回内容")
		}
		choice := resp.Choices[0].Message

		if len(choice.ToolCalls) == 0 {
			return contentText(choice.Content), steps, nil
		}

		msgs = append(msgs, llmMessage{Role: "assistant", Content: choice.Content, ToolCalls: choice.ToolCalls})
		for _, tc := range choice.ToolCalls {
			tool := findTool(tools, tc.Function.Name)
			args := map[string]interface{}{}
			if strings.TrimSpace(tc.Function.Arguments) != "" {
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					msgs = append(msgs, llmMessage{Role: "tool", ToolCallID: tc.ID, Name: tc.Function.Name, Content: "参数解析失败: " + err.Error()})
					continue
				}
			}
			var step model.AgentStep
			var resultText string
			if tool == nil {
				resultText = "未知工具: " + tc.Function.Name
				step = model.AgentStep{Step: len(steps) + 1, SkillKey: "-", ToolName: tc.Function.Name, Args: args, Status: "failed", Result: resultText}
			} else {
				step, resultText = s.executeTool(ctx, userID, *tool, args)
			}
			steps = append(steps, step)
			msgs = append(msgs, llmMessage{Role: "tool", ToolCallID: tc.ID, Name: tc.Function.Name, Content: truncate(resultText, 6000)})
		}
	}
	return "已达到最大工具调用轮次, 以下是已获取的信息。", steps, nil
}

func (s *AgentService) callLLM(ctx context.Context, req llmRequest) (*llmResponse, error) {
	body, _ := json.Marshal(req)
	url := strings.TrimRight(s.cfg.LLM.APIBase, "/") + "/chat/completions"
	reqCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.cfg.LLM.APIKey)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("大模型服务不可达: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("大模型返回 HTTP %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var out llmResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("大模型响应解析失败: %v", err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("大模型错误: %s", out.Error.Message)
	}
	return &out, nil
}

func contentText(c interface{}) string {
	switch v := c.(type) {
	case string:
		return v
	case []interface{}:
		var b strings.Builder
		for _, part := range v {
			if m, ok := part.(map[string]interface{}); ok {
				if s, ok := m["text"].(string); ok {
					b.WriteString(s)
				}
			}
		}
		return b.String()
	}
	return ""
}

// ============================================================
// 本地意图引擎 (无 LLM Key 时的降级实现)
// ============================================================

type intentRule struct {
	Keywords []string
	SkillKey string
	Tool     string
	ArgFunc  func(message string) map[string]interface{}
}

var intentRules = []intentRule{
	{
		Keywords: []string{"巡检", "数据库", "健康", "db", "mysql", "oracle", "容量", "cpu", "内存", "磁盘"},
		SkillKey: "db-inspection", Tool: "inspect_database",
		ArgFunc: func(m string) map[string]interface{} {
			db := "core-banking-db-01"
			for _, cand := range []string{"payment-db-02", "risk-control-db-01", "core-banking-db-01"} {
				if strings.Contains(m, cand) {
					db = cand
					break
				}
			}
			scope := "full"
			for _, s := range []string{"performance", "security", "capacity"} {
				if strings.Contains(m, s) {
					scope = s
				}
			}
			return map[string]interface{}{"target_db": db, "check_scope": scope}
		},
	},
	{
		Keywords: []string{"慢查询", "slow", "慢sql"},
		SkillKey: "db-inspection", Tool: "get_slow_queries",
		ArgFunc: func(m string) map[string]interface{} {
			return map[string]interface{}{"target_db": "payment-db-02", "limit": 5}
		},
	},
	{
		Keywords: []string{"脱敏", "日志", "敏感", "身份证", "手机号", "pii", "隐私", "掩码"},
		SkillKey: "log-desensitization", Tool: "desensitize",
		ArgFunc: func(m string) map[string]interface{} {
			return map[string]interface{}{"log_content": m}
		},
	},
	{
		Keywords: []string{"告警", "收敛", "根因", "alert", "噪声", "风暴"},
		SkillKey: "alert-convergence", Tool: "analyze_alerts",
		ArgFunc: func(m string) map[string]interface{} {
			host := "payment"
			for _, cand := range []string{"core", "risk", "payment", "gateway"} {
				if strings.Contains(m, cand) {
					host = cand
					break
				}
			}
			return map[string]interface{}{"host": host, "time_range_minutes": 60}
		},
	},
	{
		Keywords: []string{"需求", "prd", "业务规则", "立项", "功能点"},
		SkillKey: "requirement-analysis", Tool: "analyze_requirement",
		ArgFunc: func(m string) map[string]interface{} {
			return map[string]interface{}{"title": truncate(m, 30), "description": m}
		},
	},
}

func (s *AgentService) chatWithIntentEngine(ctx context.Context, userID, message string, tools []model.AgentTool) (string, []model.AgentStep) {
	lower := strings.ToLower(message)
	// 取命中关键词最多的规则 (命中最多的意图优先, 而不是简单首条匹配)
	bestScore, bestIdx := 0, -1
	for i, rule := range intentRules {
		score := 0
		for _, kw := range rule.Keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				score++
			}
		}
		if score > bestScore {
			bestScore, bestIdx = score, i
		}
	}
	if bestIdx >= 0 {
		rule := intentRules[bestIdx]
		tool := findToolBySkillTool(tools, rule.SkillKey, rule.Tool)
		if tool != nil {
			args := rule.ArgFunc(message)
			step, resultText := s.executeTool(ctx, userID, *tool, args)
			header := fmt.Sprintf("已识别意图 → **%s · %s**\n\n", tool.SkillName, tool.Tool)
			note := "\n\n---\n*当前运行在**本地意图引擎**（未配置大模型 API Key）。配置 `LLM_API_KEY` 后可启用大模型自主编排。*"
			return header + resultText + note, []model.AgentStep{step}
		}
	}
	avail := make([]string, 0, len(tools))
	for _, t := range tools {
		avail = append(avail, fmt.Sprintf("- %s（%s）", t.SkillName, t.Tool))
	}
	sort.Strings(avail)
	return "本地意图引擎没有匹配到可执行技能。当前可自动调度以下能力：\n\n" + strings.Join(avail, "\n") +
		"\n\n可以试试：「巡检一下核心银行数据库」「帮我把这段日志脱敏」「分析支付节点最近的告警」「把需求整理成 PRD 要点」。", []model.AgentStep{}
}

func findTool(tools []model.AgentTool, name string) *model.AgentTool {
	for i := range tools {
		if tools[i].Name == name {
			return &tools[i]
		}
	}
	return nil
}

func findToolBySkillTool(tools []model.AgentTool, skillKey, tool string) *model.AgentTool {
	for i := range tools {
		if tools[i].SkillKey == skillKey && tools[i].Tool == tool {
			return &tools[i]
		}
	}
	return nil
}

// ============================================================
// 工具执行 (复用网关安全规则 + 写审计)
// ============================================================

func (s *AgentService) executeTool(ctx context.Context, userID string, tool model.AgentTool, args map[string]interface{}) (model.AgentStep, string) {
	step := model.AgentStep{
		Step:      0, // 由调用方补齐序号
		SkillKey:  tool.SkillKey,
		SkillName: tool.SkillName,
		ToolName:  tool.Tool,
		Args:      args,
		Status:    "success",
	}
	traceID := fmt.Sprintf("tr_agent_%x", time.Now().UnixNano()%1e12)
	step.TraceID = traceID

	// 安全层: 敏感输入拦截 (脱敏类技能白名单放行)
	if !isDesensitizeSkillKey(tool.SkillKey) && hasSensitiveProbe(args) {
		step.Status = "blocked"
		step.Result = "已拦截: 检测到敏感信息参数, 请先脱敏后再调用该技能。"
		_ = s.skillSvc.RecordAudit(&model.AuditLog{TraceID: traceID, SkillID: tool.SkillKey, UserID: userID,
			Method: tool.Tool, ResponseStatus: "blocked", DurationMs: 0, SourceIP: "agent", PIIDetected: true})
		return step, step.Result
	}

	start := time.Now()
	resp, _, err := s.callRunner(ctx, tool.SkillKey, "tools/call", map[string]interface{}{
		"name":      tool.Tool,
		"arguments": args,
	})
	step.DurationMs = int(time.Since(start).Milliseconds())

	if err != nil {
		step.Status = "failed"
		step.Result = "调用失败: " + err.Error()
		_ = s.skillSvc.RecordAudit(&model.AuditLog{TraceID: traceID, SkillID: tool.SkillKey, UserID: userID,
			Method: tool.Tool, ResponseStatus: "error", DurationMs: step.DurationMs, SourceIP: "agent"})
		return step, step.Result
	}
	if resp.Error != nil {
		step.Status = "failed"
		step.Result = "技能返回错误: " + resp.Error.Message
		return step, step.Result
	}

	text := ""
	isError := false
	if resp.Result != nil {
		for _, part := range resp.Result.Content {
			text += part.Text
		}
		isError = resp.Result.IsError
	}
	if isError {
		step.Status = "failed"
	}
	step.Result = text

	_ = s.skillSvc.RecordAudit(&model.AuditLog{TraceID: traceID, SkillID: tool.SkillKey, UserID: userID,
		Method: tool.Tool, ResponseStatus: map[bool]string{true: "error", false: "success"}[isError],
		DurationMs: step.DurationMs, SourceIP: "agent", PIIDetected: false})
	_ = s.skillSvc.BumpCallCount(tool.SkillKey)

	return step, text
}

// ============================================================
// 技能运行服务调用 (与审核引擎同源)
// ============================================================

func (s *AgentService) runnerBase() string {
	return (&AuditService{}).runnerBase()
}

func (s *AgentService) callRunner(ctx context.Context, skillKey, method string, params map[string]interface{}) (*mcpResponse, int, error) {
	a := &AuditService{cfg: s.cfg}
	return a.callRunner(ctx, skillKey, method, params)
}

// 防止未使用告警 (regexp 供后续参数抽取扩展使用)
var _ = regexp.MustCompile
