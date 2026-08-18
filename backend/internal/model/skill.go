package model

import (
	"time"
)

// Skill 技能核心实体
type Skill struct {
	ID            string    `json:"id"`
	SkillKey      string    `json:"skill_key"`
	Name          string    `json:"name"`
	Version       string    `json:"version"`
	Category      string    `json:"category"`
	SubCategory   string    `json:"sub_category,omitempty"`
	Tags          []string  `json:"tags"`
	Summary       string    `json:"summary"`
	Description   string    `json:"description,omitempty"`
	IconURL       string    `json:"icon_url,omitempty"`
	SkillType     string    `json:"skill_type"`
	EndpointURL   string    `json:"endpoint_url,omitempty"`
	EndpointProto string    `json:"endpoint_protocol"`
	Manifest      string    `json:"manifest"`       // JSONB -> string
	Dependencies  string    `json:"dependencies"`   // JSONB
	Permissions   string    `json:"permissions"`    // JSONB
	InterfaceSpec string    `json:"interface_spec"` // JSONB
	Stability     string    `json:"stability"`
	InstallCount  int64     `json:"install_count"`
	CallCount     int64     `json:"call_count"`
	RatingAvg     float64   `json:"rating_avg"`
	RatingCount   int       `json:"rating_count"`
	Status        string    `json:"status"`
	Visibility    string    `json:"visibility"`
	ReviewComment string    `json:"review_comment,omitempty"`
	AuthorID      string    `json:"author_id"`
	AuthorName    string    `json:"author_name,omitempty"`
	TeamID        string    `json:"team_id,omitempty"`
	TeamName      string    `json:"team_name,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// CategoryStat 市场分类的实时统计。
type CategoryStat struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// SkillInstallation 技能安装记录
type SkillInstallation struct {
	ID             string     `json:"id"`
	SkillID        string     `json:"skill_id"`
	SkillName      string     `json:"skill_name,omitempty"`
	SkillVersion   string     `json:"skill_version"`
	UserID         string     `json:"user_id"`
	APIKeyPrefix   string     `json:"api_key_prefix"`
	TokenExpiresAt *time.Time `json:"token_expires_at,omitempty"`
	Status         string     `json:"status"`
	InstalledAt    time.Time  `json:"installed_at"`
}

// SkillRating 用户评价
type SkillRating struct {
	ID        string    `json:"id"`
	SkillID   string    `json:"skill_id"`
	UserID    string    `json:"user_id"`
	UserName  string    `json:"user_name,omitempty"`
	Rating    int       `json:"rating"`
	Title     string    `json:"title,omitempty"`
	Comment   string    `json:"comment,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// AuditLog 审计日志
type AuditLog struct {
	ID             string    `json:"id"`
	TraceID        string    `json:"trace_id"`
	SkillID        string    `json:"skill_id"`
	UserID         string    `json:"user_id"`
	Method         string    `json:"method"`
	ResponseStatus string    `json:"response_status"`
	DurationMs     int       `json:"duration_ms"`
	TokensUsed     int       `json:"tokens_used"`
	SourceIP       string    `json:"source_ip"`
	PIIDetected    bool      `json:"pii_detected"`
	CreatedAt      time.Time `json:"created_at"`
}

// SkillSearchRequest 技能搜索请求
type SkillSearchRequest struct {
	Query     string `json:"query" form:"query"`
	Category  string `json:"category" form:"category"`
	SkillType string `json:"skill_type" form:"skill_type"`
	Status    string `json:"status" form:"status"`
	SortBy    string `json:"sort_by" form:"sort_by"` // rating / installs / newest
	Page      int    `json:"page" form:"page"`
	PageSize  int    `json:"page_size" form:"page_size"`
}

// SkillInstallRequest 安装请求
type SkillInstallRequest struct {
	SkillID     string `json:"skill_id" binding:"required"`
	Version     string `json:"version"`
	IPWhitelist string `json:"ip_whitelist,omitempty"`
}

// SkillInvokeRequest 技能调用请求
type SkillInvokeRequest struct {
	SkillID string                 `json:"skill_id" binding:"required"`
	Method  string                 `json:"method" binding:"required"`
	Params  map[string]interface{} `json:"params"`
}
