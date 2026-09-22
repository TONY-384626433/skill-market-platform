package service

import (
	"os"
	"testing"

	"github.com/jjbank/skill-market/internal/config"
)

func TestProviderPresetsCoverage(t *testing.T) {
	presets := providerPresets()
	want := map[string]bool{"deepseek": false, "kimi": false, "openai": false, "claude": false, "ark": false, "qwen": false, "zhipu": false}
	for _, p := range presets {
		if _, ok := want[p.Key]; ok {
			want[p.Key] = true
		}
	}
	for k, seen := range want {
		if !seen {
			t.Fatalf("preset missing: %s", k)
		}
	}
	// claude 必须走 anthropic 协议
	for _, p := range presets {
		if p.Key == "claude" && p.Kind != "anthropic" {
			t.Fatalf("claude kind = %s, want anthropic", p.Kind)
		}
	}
}

func TestResolveProviderReadsEnv(t *testing.T) {
	os.Setenv("DEEPSEEK_API_KEY", "sk-test")
	os.Setenv("DEEPSEEK_MODEL", "deepseek-reasoner")
	defer os.Unsetenv("DEEPSEEK_API_KEY")
	defer os.Unsetenv("DEEPSEEK_MODEL")

	for _, p := range providerPresets() {
		if p.Key != "deepseek" {
			continue
		}
		rp := resolveProvider(p)
		if !rp.Configured {
			t.Fatal("deepseek should be configured when DEEPSEEK_API_KEY set")
		}
		if rp.APIKey != "sk-test" {
			t.Fatalf("key = %s", rp.APIKey)
		}
		if rp.Model != "deepseek-reasoner" {
			t.Fatalf("model override failed: %s", rp.Model)
		}
		return
	}
	t.Fatal("deepseek preset not found")
}

func TestActiveProviderSelection(t *testing.T) {
	os.Setenv("OPENAI_API_KEY", "sk-openai")
	defer os.Unsetenv("OPENAI_API_KEY")
	os.Unsetenv("LLM_PROVIDER")

	svc := &AgentService{cfg: &config.Config{LLM: config.LLMConfig{APIBase: "", Model: "", APIKey: ""}}}

	// 显式指定
	got, ok := svc.activeProvider("openai")
	if !ok || got.Key != "openai" {
		t.Fatalf("explicit selection failed: %v %v", got.Key, ok)
	}
	// 自动挑选（无 LLM_PROVIDER、无 custom）→ 命中唯一已配置项
	got2, ok2 := svc.activeProvider("")
	if !ok2 || got2.Key != "openai" {
		t.Fatalf("auto selection failed: %v %v", got2.Key, ok2)
	}
	// 指定未配置的厂商应回退（不返回该项）
	if p, ok3 := svc.activeProvider("claude"); ok3 && p.Key == "claude" {
		t.Fatal("claude 未配置却返回了 claude")
	}
}

func TestProviderStatsRecording(t *testing.T) {
	svc := &AgentService{}
	svc.recordProviderAttempt("deepseek", true, 120, "")
	svc.recordProviderAttempt("deepseek", false, 80, "HTTP 429")
	svc.recordProviderAttempt("claude", true, 200, "")

	snap := svc.providerStatsSnapshot()
	if snap["total_calls"].(int) != 3 {
		t.Fatalf("total_calls = %v", snap["total_calls"])
	}
	if snap["total_failures"].(int) != 1 {
		t.Fatalf("total_failures = %v", snap["total_failures"])
	}
	items := snap["providers"].([]map[string]interface{})
	if len(items) != 2 {
		t.Fatalf("providers len = %d", len(items))
	}
	// deepseek 调用 2 次, 排最前
	first := items[0]
	if first["provider"] != "deepseek" || first["calls"].(int) != 2 {
		t.Fatalf("first = %#v", first)
	}
	if first["success"].(int) != 1 || first["failures"].(int) != 1 {
		t.Fatalf("deepseek success/fail wrong: %#v", first)
	}
	if first["success_rate"].(float64) != 50 {
		t.Fatalf("success_rate = %v, want 50", first["success_rate"])
	}
	if first["avg_ms"].(int64) != 100 {
		t.Fatalf("avg_ms = %v, want 100", first["avg_ms"])
	}
	if first["last_error"] != "HTTP 429" {
		t.Fatalf("last_error = %v", first["last_error"])
	}
}

func TestProviderChainOrder(t *testing.T) {
	os.Setenv("DEEPSEEK_API_KEY", "sk-a")
	os.Setenv("OPENAI_API_KEY", "sk-b")
	os.Setenv("LLM_PROVIDER", "openai")
	defer os.Unsetenv("DEEPSEEK_API_KEY")
	defer os.Unsetenv("OPENAI_API_KEY")
	defer os.Unsetenv("LLM_PROVIDER")

	svc := &AgentService{cfg: &config.Config{LLM: config.LLMConfig{}}}
	// 指定 deepseek → deepseek 应在最前, 然后默认 openai, 其余已配置的跟上
	chain := svc.providerChain("deepseek")
	if len(chain) < 2 {
		t.Fatalf("chain too short: %d", len(chain))
	}
	if chain[0].Key != "deepseek" {
		t.Fatalf("chain[0] = %s, want deepseek", chain[0].Key)
	}
	if chain[1].Key != "openai" {
		t.Fatalf("chain[1] = %s, want openai", chain[1].Key)
	}
	// 未指定 → 默认(LLM_PROVIDER=openai) 在最前
	chain2 := svc.providerChain("")
	if len(chain2) == 0 || chain2[0].Key != "openai" {
		t.Fatalf("chain2[0] = %v, want openai", chain2)
	}
	// 未配置的厂商不应进入链
	for _, p := range chain {
		if p.Key == "claude" {
			t.Fatal("claude 未配置却进入候选链")
		}
	}
}

func TestToAnthropicMessages(t *testing.T) {
	var tc llmToolCall
	tc.ID = "call_1"
	tc.Type = "function"
	tc.Function.Name = "skill__tool"
	tc.Function.Arguments = `{"x":1}`

	msgs := []llmMessage{
		{Role: "system", Content: "SYS"},
		{Role: "user", Content: "你好"},
		{Role: "assistant", ToolCalls: []llmToolCall{tc}},
		{Role: "tool", ToolCallID: "call_1", Content: "结果"},
	}
	system, out := toAnthropicMessages(msgs)
	if system != "SYS" {
		t.Fatalf("system = %q", system)
	}
	if len(out) != 3 {
		t.Fatalf("messages len = %d, want 3", len(out))
	}
	// assistant 的第二条应含 tool_use
	if out[1]["role"] != "assistant" {
		t.Fatalf("out[1].role = %v", out[1]["role"])
	}
	blocks, _ := out[1]["content"].([]map[string]interface{})
	if len(blocks) != 1 || blocks[0]["type"] != "tool_use" || blocks[0]["name"] != "skill__tool" {
		t.Fatalf("tool_use block wrong: %#v", out[1]["content"])
	}
	// tool 结果应转成 user + tool_result
	if out[2]["role"] != "user" {
		t.Fatalf("out[2].role = %v", out[2]["role"])
	}
	tr, _ := out[2]["content"].([]map[string]interface{})
	if len(tr) != 1 || tr[0]["type"] != "tool_result" || tr[0]["tool_use_id"] != "call_1" {
		t.Fatalf("tool_result block wrong: %#v", out[2]["content"])
	}
}

func TestToAnthropicTools(t *testing.T) {
	tools := []interface{}{
		map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "skill__db",
				"description": "巡检",
				"parameters":  map[string]interface{}{"type": "object"},
			},
		},
	}
	out := toAnthropicTools(tools)
	if len(out) != 1 || out[0]["name"] != "skill__db" {
		t.Fatalf("tools conversion failed: %#v", out)
	}
	if _, ok := out[0]["input_schema"]; !ok {
		t.Fatal("input_schema missing")
	}
}
