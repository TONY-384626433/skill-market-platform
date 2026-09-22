package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

// ============================================================
// 多厂商大模型接入层
// ------------------------------------------------------------
// 支持两类协议:
//   1) openai 兼容  — /chat/completions (DeepSeek / Kimi / OpenAI / 火山方舟Seedance / 通义 / 智谱 ...)
//   2) anthropic    — /messages (Claude, 协议不同, 本层做双向适配)
// 每家通过环境变量配置: <PREFIX>_API_KEY (必), <PREFIX>_API_BASE / <PREFIX>_MODEL (可选覆盖)
// 选择默认厂商: LLM_PROVIDER=deepseek|kimi|openai|claude|ark|qwen|zhipu|custom
// ============================================================

const anthropicVersion = "2023-06-01"

type llmProvider struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	Kind       string `json:"kind"` // openai | anthropic
	APIBase    string `json:"api_base"`
	Model      string `json:"model"`
	EnvPrefix  string `json:"-"`
	KeyEnv     string `json:"-"`
	APIKey     string `json:"-"`
	Configured bool   `json:"configured"`
}

func providerPresets() []llmProvider {
	return []llmProvider{
		{Key: "deepseek", Label: "DeepSeek", Kind: "openai", APIBase: "https://api.deepseek.com/v1", Model: "deepseek-chat", EnvPrefix: "DEEPSEEK"},
		{Key: "kimi", Label: "Kimi (Moonshot)", Kind: "openai", APIBase: "https://api.moonshot.cn/v1", Model: "moonshot-v1-8k", EnvPrefix: "MOONSHOT"},
		{Key: "openai", Label: "OpenAI (ChatGPT)", Kind: "openai", APIBase: "https://api.openai.com/v1", Model: "gpt-4o-mini", EnvPrefix: "OPENAI"},
		{Key: "claude", Label: "Claude (Anthropic)", Kind: "anthropic", APIBase: "https://api.anthropic.com/v1", Model: "claude-3-5-sonnet-latest", EnvPrefix: "ANTHROPIC"},
		{Key: "deepseek-anthropic", Label: "DeepSeek · Anthropic 端点", Kind: "anthropic", APIBase: "https://api.deepseek.com/anthropic", Model: "deepseek-chat", EnvPrefix: "DEEPSEEK_ANTHROPIC"},
		{Key: "ark", Label: "火山方舟 (豆包/Seedance)", Kind: "openai", APIBase: "https://ark.cn-beijing.volces.com/api/v3", Model: "doubao-pro-32k", EnvPrefix: "ARK"},
		{Key: "qwen", Label: "通义千问 (DashScope)", Kind: "openai", APIBase: "https://dashscope.aliyuncs.com/compatible-mode/v1", Model: "qwen-plus", EnvPrefix: "DASHSCOPE"},
		{Key: "zhipu", Label: "智谱 GLM", Kind: "openai", APIBase: "https://open.bigmodel.cn/api/paas/v4", Model: "glm-4-plus", EnvPrefix: "ZHIPU"},
	}
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

// resolveProvider 把某厂商的预置值 + 环境变量解析成可用的 provider (含 Key/覆盖)
func resolveProvider(p llmProvider) llmProvider {
	if p.EnvPrefix != "" {
		p.APIKey = firstEnv(p.EnvPrefix + "_API_KEY")
		if v := firstEnv(p.EnvPrefix + "_API_BASE"); v != "" {
			p.APIBase = v
		}
		if v := firstEnv(p.EnvPrefix + "_MODEL"); v != "" {
			p.Model = v
		}
	}
	// 别名 Key 兼容 (如 Kimi)
	if p.APIKey == "" && p.KeyEnv != "" {
		p.APIKey = firstEnv(p.KeyEnv)
	}
	p.Configured = p.APIKey != "" && p.APIBase != ""
	return p
}

// ListProviders 列出全部厂商及配置状态 (custom 来自 LLM_API_BASE/LLM_API_KEY/LLM_MODEL)
func (s *AgentService) ListProviders() []llmProvider {
	out := make([]llmProvider, 0, len(providerPresets())+1)

	custom := llmProvider{
		Key: "custom", Label: "自定义 (OpenAI 兼容)", Kind: "openai",
		APIBase: s.cfg.LLM.APIBase, Model: s.cfg.LLM.Model,
		APIKey: s.cfg.LLM.APIKey, EnvPrefix: "", KeyEnv: "LLM_API_KEY",
	}
	custom.Configured = custom.APIKey != "" && custom.APIBase != ""
	// custom 若非用户显式配置(base 仍是默认本地地址且无 key), 也一并列出但标记未配置
	out = append(out, custom)

	for _, p := range providerPresets() {
		// Kimi 额外兼容 KIMI_API_KEY
		if p.Key == "kimi" {
			p.KeyEnv = "KIMI_API_KEY"
		}
		rp := resolveProvider(p)
		out = append(out, rp)
	}
	return out
}

func (s *AgentService) ProviderSummary() map[string]interface{} {
	list := s.ListProviders()
	items := make([]map[string]interface{}, 0, len(list))
	configuredCount := 0
	for _, p := range list {
		if p.Configured {
			configuredCount++
		}
		items = append(items, map[string]interface{}{
			"key": p.Key, "label": p.Label, "kind": p.Kind,
			"model": p.Model, "api_base": p.APIBase, "configured": p.Configured,
		})
	}
	active, ok := s.activeProvider("")
	summary := map[string]interface{}{
		"providers": items,
		"count":     len(items),
		"configured_count": configuredCount,
		"active":    nil,
	}
	if ok {
		summary["active"] = map[string]interface{}{
			"key": active.Key, "label": active.Label, "model": active.Model, "kind": active.Kind,
		}
	}
	return summary
}

// activeProvider 依据请求指定 / LLM_PROVIDER / 已配置项 选出当前厂商
func (s *AgentService) activeProvider(requested string) (llmProvider, bool) {
	requested = strings.TrimSpace(requested)
	list := s.ListProviders()

	byKey := func(key string) (llmProvider, bool) {
		for _, p := range list {
			if p.Key == key {
				return p, true
			}
		}
		return llmProvider{}, false
	}

	if requested != "" {
		if p, ok := byKey(requested); ok && p.Configured {
			return p, true
		}
	}
	if envp := strings.TrimSpace(os.Getenv("LLM_PROVIDER")); envp != "" {
		if p, ok := byKey(envp); ok && p.Configured {
			return p, true
		}
	}
	// 旧的单通道配置优先(custom)
	if p, ok := byKey("custom"); ok && p.Configured {
		return p, true
	}
	// 再取第一个已配置的预置厂商
	presets := providerPresets()
	sort.SliceStable(presets, func(i, j int) bool { return presets[i].Key < presets[j].Key })
	for _, p := range presets {
		rp := resolveProvider(p)
		if rp.Configured {
			return rp, true
		}
	}
	return llmProvider{}, false
}

// providerChain 返回候选厂商顺序链（用于自动择优 / 故障切换）：
// 指定厂商 -> LLM_PROVIDER -> custom -> 其余已配置预置厂商（稳定排序）
func (s *AgentService) providerChain(requested string) []llmProvider {
	list := s.ListProviders()
	byKey := map[string]llmProvider{}
	for _, p := range list {
		byKey[p.Key] = p
	}
	seen := map[string]bool{}
	chain := make([]llmProvider, 0, len(list))
	add := func(key string) {
		key = strings.TrimSpace(key)
		if key == "" || seen[key] {
			return
		}
		p, ok := byKey[key]
		if !ok || !p.Configured {
			return
		}
		seen[key] = true
		chain = append(chain, p)
	}

	if requested != "" && requested != "auto" {
		add(requested)
	}
	add(os.Getenv("LLM_PROVIDER"))
	add("custom")

	presets := providerPresets()
	sort.SliceStable(presets, func(i, j int) bool { return presets[i].Key < presets[j].Key })
	for _, p := range presets {
		add(p.Key)
	}
	return chain
}

// ============================================================
// 厂商调用统计 (可观测性)
// ============================================================

type providerStat struct {
	Calls      int    `json:"calls"`
	Success    int    `json:"success"`
	Failures   int    `json:"failures"`
	TotalMs    int64  `json:"total_ms"`
	LastMs     int64  `json:"last_ms"`
	LastError  string `json:"last_error,omitempty"`
	LastUsedAt int64  `json:"last_used_at"`
}

func (s *AgentService) recordProviderAttempt(key string, ok bool, ms int64, errMsg string) {
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.providerStats == nil {
		s.providerStats = map[string]*providerStat{}
	}
	st := s.providerStats[key]
	if st == nil {
		st = &providerStat{}
		s.providerStats[key] = st
	}
	st.Calls++
	st.TotalMs += ms
	st.LastMs = ms
	st.LastUsedAt = time.Now().Unix()
	if ok {
		st.Success++
		st.LastError = ""
	} else {
		st.Failures++
		st.LastError = errMsg
	}
}

// providerStatsSnapshot 返回按调用次数倒序的统计快照
func (s *AgentService) providerStatsSnapshot() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]map[string]interface{}, 0, len(s.providerStats))
	var totalCalls, totalFails int
	for key, st := range s.providerStats {
		avg := int64(0)
		if st.Calls > 0 {
			avg = st.TotalMs / int64(st.Calls)
		}
		rate := 0.0
		if st.Calls > 0 {
			rate = float64(st.Success) / float64(st.Calls) * 100
		}
		totalCalls += st.Calls
		totalFails += st.Failures
		items = append(items, map[string]interface{}{
			"provider": key, "calls": st.Calls, "success": st.Success, "failures": st.Failures,
			"success_rate": math.Round(rate*10) / 10, "avg_ms": avg, "last_ms": st.LastMs,
			"last_error": st.LastError, "last_used_at": st.LastUsedAt,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i]["calls"].(int) > items[j]["calls"].(int)
	})
	return map[string]interface{}{
		"total_calls":    totalCalls,
		"total_failures": totalFails,
		"providers":      items,
	}
}

// callProvider 按协议分发
func (s *AgentService) callProvider(ctx context.Context, p llmProvider, req llmRequest) (*llmResponse, error) {
	if p.Kind == "anthropic" {
		return s.callAnthropic(ctx, p, req)
	}
	return s.callOpenAICompatible(ctx, p, req)
}

// ---- OpenAI 兼容 ----
func (s *AgentService) callOpenAICompatible(ctx context.Context, p llmProvider, req llmRequest) (*llmResponse, error) {
	req.Model = p.Model
	body, _ := json.Marshal(req)
	url := strings.TrimRight(p.APIBase, "/") + "/chat/completions"
	raw, status, err := httpPostJSON(ctx, url, body, map[string]string{
		"Authorization": "Bearer " + p.APIKey,
	}, 60*time.Second)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("%s 返回 HTTP %d: %s", p.Label, status, truncate(string(raw), 200))
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

// ---- Anthropic (Claude) 适配 ----
func (s *AgentService) callAnthropic(ctx context.Context, p llmProvider, req llmRequest) (*llmResponse, error) {
	system, msgs := toAnthropicMessages(req.Messages)
	payload := map[string]interface{}{
		"model":      p.Model,
		"max_tokens": 2048,
		"messages":   msgs,
	}
	if system != "" {
		payload["system"] = system
	}
	if len(req.Tools) > 0 {
		payload["tools"] = toAnthropicTools(req.Tools)
	}
	body, _ := json.Marshal(payload)
	url := strings.TrimRight(p.APIBase, "/") + "/messages"
	raw, status, err := httpPostJSON(ctx, url, body, map[string]string{
		"x-api-key":         p.APIKey,
		"anthropic-version": anthropicVersion,
	}, 60*time.Second)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("%s 返回 HTTP %d: %s", p.Label, status, truncate(string(raw), 200))
	}

	var ar struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Error      *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &ar); err != nil {
		return nil, fmt.Errorf("Claude 响应解析失败: %v", err)
	}
	if ar.Error != nil {
		return nil, fmt.Errorf("Claude 错误: %s", ar.Error.Message)
	}

	// 转回 OpenAI 风格 llmResponse
	var text strings.Builder
	var calls []llmToolCall
	for _, c := range ar.Content {
		switch c.Type {
		case "text":
			text.WriteString(c.Text)
		case "tool_use":
			args := "{}"
			if len(c.Input) > 0 {
				args = string(c.Input)
			}
			var tc llmToolCall
			tc.ID = c.ID
			tc.Type = "function"
			tc.Function.Name = c.Name
			tc.Function.Arguments = args
			calls = append(calls, tc)
		}
	}
	out := &llmResponse{}
	out.Choices = make([]struct {
		Message      llmMessage `json:"message"`
		FinishReason string     `json:"finish_reason"`
	}, 1)
	out.Choices[0].Message = llmMessage{Role: "assistant", Content: text.String(), ToolCalls: calls}
	out.Choices[0].FinishReason = ar.StopReason
	return out, nil
}

func toAnthropicTools(tools []interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(tools))
	for _, t := range tools {
		m, ok := t.(map[string]interface{})
		if !ok {
			continue
		}
		fn, _ := m["function"].(map[string]interface{})
		if fn == nil {
			continue
		}
		params := fn["parameters"]
		if params == nil {
			params = map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
		}
		out = append(out, map[string]interface{}{
			"name":         fn["name"],
			"description":  fn["description"],
			"input_schema": params,
		})
	}
	return out
}

// toAnthropicMessages: OpenAI 风格 messages -> (system, anthropic messages)
func toAnthropicMessages(msgs []llmMessage) (string, []map[string]interface{}) {
	var system string
	out := make([]map[string]interface{}, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case "system":
			if s := contentText(m.Content); s != "" {
				if system != "" {
					system += "\n"
				}
				system += s
			}
		case "assistant":
			var blocks []map[string]interface{}
			if txt := contentText(m.Content); txt != "" {
				blocks = append(blocks, map[string]interface{}{"type": "text", "text": txt})
			}
			for _, tc := range m.ToolCalls {
				var input interface{}
				if strings.TrimSpace(tc.Function.Arguments) != "" {
					_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
				}
				if input == nil {
					input = map[string]interface{}{}
				}
				blocks = append(blocks, map[string]interface{}{
					"type": "tool_use", "id": tc.ID, "name": tc.Function.Name, "input": input,
				})
			}
			if len(blocks) == 0 {
				blocks = append(blocks, map[string]interface{}{"type": "text", "text": ""})
			}
			out = append(out, map[string]interface{}{"role": "assistant", "content": blocks})
		case "tool":
			out = append(out, map[string]interface{}{
				"role": "user",
				"content": []map[string]interface{}{{
					"type":        "tool_result",
					"tool_use_id": m.ToolCallID,
					"content":     contentText(m.Content),
				}},
			})
		default: // user
			out = append(out, map[string]interface{}{"role": "user", "content": contentText(m.Content)})
		}
	}
	return system, out
}

// httpPostJSON 统一 POST 帮助函数
func httpPostJSON(ctx context.Context, url string, body []byte, headers map[string]string, timeout time.Duration) ([]byte, int, error) {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("大模型服务不可达: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return raw, resp.StatusCode, nil
}
