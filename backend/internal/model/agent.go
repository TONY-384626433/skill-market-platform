package model

// ============================================================
// AI 智能体 (Agent)
// ============================================================

// AgentTool 智能体可调用的工具 (由真实技能 tools/list 派生)
type AgentTool struct {
	Name        string                 `json:"name"` // 全局唯一工具名: skill__tool
	SkillID     string                 `json:"skill_id"`
	SkillKey    string                 `json:"skill_key"`
	SkillName   string                 `json:"skill_name"`
	Category    string                 `json:"category"`
	Tool        string                 `json:"tool"` // 技能内原始工具名
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema,omitempty"`
}

// AgentStep 智能体的一次工具调用轨迹
type AgentStep struct {
	Step       int                    `json:"step"`
	SkillKey   string                 `json:"skill_key"`
	SkillName  string                 `json:"skill_name,omitempty"`
	ToolName   string                 `json:"tool_name"`
	Args       map[string]interface{} `json:"args,omitempty"`
	Status     string                 `json:"status"` // success / failed / blocked
	Result     string                 `json:"result,omitempty"`
	DurationMs int                    `json:"duration_ms"`
	TraceID    string                 `json:"trace_id,omitempty"`
}

// AgentMessage 对话历史
type AgentMessage struct {
	Role    string `json:"role"` // user / assistant
	Content string `json:"content"`
}

// AgentAnswer 一次智能体回答
type AgentAnswer struct {
	SessionID  string      `json:"session_id"`
	Question   string      `json:"question"`
	Answer     string      `json:"answer"`
	Steps      []AgentStep `json:"steps"`
	ToolCalls  int         `json:"tool_calls"`
	Provider   string      `json:"provider"` // llm / local-intent
	Model      string      `json:"model"`
	Fallback   bool        `json:"fallback"`
	Engine     string      `json:"engine"`
	DurationMs int         `json:"duration_ms"`
}
