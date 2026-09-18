package model

import "time"

// ============================================================
// 技能可用性审核 (Availability Audit)
// ============================================================

// AuditCheck 单项检查结果
type AuditCheck struct {
	Code       string  `json:"code"`               // 检查项编号, 如 AVAIL-02
	Name       string  `json:"name"`               // 检查项名称
	Category   string  `json:"category"`           // static / protocol / functional / performance / security
	Level      string  `json:"level"`              // critical / major / minor
	Status     string  `json:"status"`             // passed / failed / warning / skipped
	Score      float64 `json:"score"`              // 0-100
	Weight     float64 `json:"weight"`             // 加权权重
	Detail     string  `json:"detail"`             // 结论说明
	Evidence   string  `json:"evidence,omitempty"` // 原始证据 (响应片段/错误信息)
	DurationMs int     `json:"duration_ms"`        // 该项耗时
}

// SkillAudit 一次完整的可用性审核记录
type SkillAudit struct {
	ID               string       `json:"id"`
	SkillID          string       `json:"skill_id"`
	SkillKey         string       `json:"skill_key"`
	SkillName        string       `json:"skill_name,omitempty"`
	Version          string       `json:"version"`
	TriggerType      string       `json:"trigger_type"` // manual / self_check / auto
	TriggeredBy      string       `json:"triggered_by"`
	TriggeredByName  string       `json:"triggered_by_name,omitempty"`
	Status           string       `json:"status"` // running / passed / failed
	Score            float64      `json:"score"`
	Grade            string       `json:"grade"` // A / B / C / D
	TotalChecks      int          `json:"total_checks"`
	PassedChecks     int          `json:"passed_checks"`
	CriticalFailures int          `json:"critical_failures"`
	DurationMs       int          `json:"duration_ms"`
	Checks           []AuditCheck `json:"checks"`
	Summary          string       `json:"summary"`
	EngineVersion    string       `json:"engine_version"`
	CreatedAt        time.Time    `json:"created_at"`
}

// SkillAuditBrief 审核状态摘要 (挂在技能列表上)
type SkillAuditBrief struct {
	AuditStatus  string     `json:"audit_status"`
	AuditScore   *float64   `json:"audit_score,omitempty"`
	AuditGrade   string     `json:"audit_grade,omitempty"`
	LastAuditAt  *time.Time `json:"last_audit_at,omitempty"`
	LastAuditID  string     `json:"last_audit_id,omitempty"`
	CriticalFail int        `json:"critical_failures,omitempty"`
	Summary      string     `json:"audit_summary,omitempty"`
}

// AuditOverview 审核概览统计
type AuditOverview struct {
	TotalSkills  int64   `json:"total_skills"`
	Passed       int64   `json:"passed"`
	Failed       int64   `json:"failed"`
	Pending      int64   `json:"pending"`
	TotalAudits  int64   `json:"total_audits"`
	AvgScore     float64 `json:"avg_score"`
	PassRate     float64 `json:"pass_rate"`
	LastAuditAt  string  `json:"last_audit_at,omitempty"`
	EngineVer    string  `json:"engine_version"`
	CriticalOpen int64   `json:"critical_open"`
}
