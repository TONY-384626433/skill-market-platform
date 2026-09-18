package model

import "time"

// ============================================================
// 技能安全治理 (Security Governance)
//   - 静态安全扫描 (查毒 / 危险行为 / 提示注入 / 供应链)
//   - 溯源防盗用 (内容指纹 / 水印 / 签名 / 下载审计)
//   - GitHub 导入门禁 (先审核, 后下载)
// ============================================================

// SkillFile 参与扫描的技能文件 (内存态, 内容不回传前端)
type SkillFile struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Content []byte `json:"-"`
	Text    string `json:"-"`
	Skipped bool   `json:"-"` // 二进制/超大文件
}

// SecurityFinding 单条安全发现
type SecurityFinding struct {
	RuleID     string `json:"rule_id"`  // 规则编号, 如 EXEC-01
	Category   string `json:"category"` // execution/network/credential/obfuscation/persistence/destructive/injection/supply_chain/malware/integrity/compliance
	CategoryCN string `json:"category_cn"`
	Severity   string `json:"severity"` // critical / high / medium / low
	Title      string `json:"title"`
	Detail     string `json:"detail"`
	File       string `json:"file,omitempty"`
	Line       int    `json:"line,omitempty"`
	Evidence   string `json:"evidence,omitempty"` // 已脱敏的证据片段
	Blocking   bool   `json:"blocking"`           // 是否属于阻断级
	Score      int    `json:"score"`              // 该发现扣分
}

// SandboxCheck 沙箱单项检查
type SandboxCheck struct {
	RuleID string `json:"rule_id"`
	Name   string `json:"name"`
	Status string `json:"status"` // passed / failed / warning / skipped
	Detail string `json:"detail"`
}

// SandboxReport 动态沙箱行为验证报告
type SandboxReport struct {
	Engine         string                 `json:"engine"`
	Status         string                 `json:"status"` // ok / skipped / unreachable / error
	Notice         string                 `json:"notice,omitempty"`
	RunID          string                 `json:"run_id,omitempty"`
	Entry          string                 `json:"entry,omitempty"`
	Executed       bool                   `json:"executed"`
	ExitCode       *int                   `json:"exit_code,omitempty"`
	DurationMs     int                    `json:"duration_ms"`
	VerdictHint    string                 `json:"verdict_hint,omitempty"`
	CriticalCount  int                    `json:"critical_count,omitempty"`
	HighCount      int                    `json:"high_count,omitempty"`
	TraceLines     int                    `json:"trace_lines,omitempty"`
	Isolated       map[string]interface{} `json:"isolated,omitempty"`
	Events         map[string]int         `json:"events,omitempty"`
	MCP            map[string]interface{} `json:"mcp,omitempty"`
	Checks         []SandboxCheck         `json:"checks,omitempty"`
	Findings       []SecurityFinding      `json:"findings,omitempty"`
	FilesystemDiff map[string][]string    `json:"filesystem_diff,omitempty"`
	StdoutTail     string                 `json:"stdout_tail,omitempty"`
	StderrTail     string                 `json:"stderr_tail,omitempty"`
}

// AVEngine 外部查毒引擎状态 (ClamAV / YARA)
type AVEngine struct {
	Engine    string `json:"engine"`
	Available bool   `json:"available"`
	Binary    string `json:"binary,omitempty"`
	Rules     string `json:"rules,omitempty"`
	Version   string `json:"version,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// SecurityScan 一次完整的安全扫描记录
type SecurityScan struct {
	ID             string            `json:"id"`
	SubjectType    string            `json:"subject_type"` // skill / import / package
	SkillID        string            `json:"skill_id,omitempty"`
	SkillKey       string            `json:"skill_key,omitempty"`
	SkillName      string            `json:"skill_name,omitempty"`
	Version        string            `json:"version,omitempty"`
	Target         string            `json:"target,omitempty"` // 仓库/路径等定位
	Verdict        string            `json:"verdict"`          // safe / suspicious / malicious
	VerdictCN      string            `json:"verdict_cn"`
	RiskScore      int               `json:"risk_score"` // 0-100, 越高越危险
	Grade          string            `json:"grade"`      // A / B / C / D
	Findings       []SecurityFinding `json:"findings"`
	FindingCount   int               `json:"finding_count"`
	CriticalCount  int               `json:"critical_count"`
	HighCount      int               `json:"high_count"`
	FilesScanned   int               `json:"files_scanned"`
	BytesScanned   int64             `json:"bytes_scanned"`
	DurationMs     int               `json:"duration_ms"`
	EngineVersion  string            `json:"engine_version"`
	ManifestSHA256 string            `json:"manifest_sha256,omitempty"`
	Signature      string            `json:"signature,omitempty"` // 包签名 (HMAC-SHA256)
	ContentHash    string            `json:"content_hash,omitempty"`
	SimHashHex     string            `json:"sim_hash,omitempty"`
	WatermarkID    string            `json:"watermark_id,omitempty"`
	TriggerType    string            `json:"trigger_type"` // manual / upload / import / install / schedule
	TriggeredBy    string            `json:"triggered_by,omitempty"`
	TriggeredName  string            `json:"triggered_by_name,omitempty"`
	Summary        string            `json:"summary"`
	AVEngines      []AVEngine        `json:"av_engines,omitempty"`
	Sandbox        *SandboxReport    `json:"sandbox,omitempty"`
	KeyID          string            `json:"key_id,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	// 查重 (防盗用)
	ReuploadSimilarity float64         `json:"reupload_similarity,omitempty"`
	ReuploadSkillID    string          `json:"reupload_skill_id,omitempty"`
	ReuploadSkillName  string          `json:"reupload_skill_name,omitempty"`
	ReuploadOwnerID    string          `json:"reupload_owner_id,omitempty"`
	ReuploadSuspected  bool            `json:"reupload_suspected"`
	Reupload           *ReuploadReport `json:"reupload,omitempty"`
}

// SecurityBadge 挂在技能上的安全摘要
type SecurityBadge struct {
	SkillID        string     `json:"skill_id"`
	SecurityStatus string     `json:"security_status"` // unscanned / safe / suspicious / malicious / blocked
	Verdict        string     `json:"verdict"`
	VerdictCN      string     `json:"verdict_cn"`
	RiskScore      int        `json:"risk_score"`
	Grade          string     `json:"grade"`
	CriticalCount  int        `json:"critical_count"`
	LastScanAt     *time.Time `json:"last_scan_at,omitempty"`
	LastScanID     string     `json:"last_scan_id,omitempty"`
	Blocked        bool       `json:"blocked"`
	Summary        string     `json:"summary,omitempty"`
	// 溯源
	WatermarkID    string `json:"watermark_id,omitempty"`
	OwnerID        string `json:"owner_id,omitempty"`
	SignedPackages int    `json:"signed_packages"`
	Downloads      int    `json:"downloads"`
	ReuploadAlerts int    `json:"reupload_alerts"`
}

// SkillProvenance 技能溯源档案 (防盗用)
type SkillProvenance struct {
	SkillID        string     `json:"skill_id"`
	SkillKey       string     `json:"skill_key,omitempty"`
	SkillName      string     `json:"skill_name,omitempty"`
	OwnerID        string     `json:"owner_id"`
	OwnerName      string     `json:"owner_name,omitempty"`
	License        string     `json:"license"`
	WatermarkID    string     `json:"watermark_id"`
	ContentHash    string     `json:"content_hash"`
	SimHash        string     `json:"simhash"`
	Signature      string     `json:"signature"`
	Algorithm      string     `json:"algorithm"`
	SignedAt       time.Time  `json:"signed_at"`
	LastVerifiedAt *time.Time `json:"last_verified_at,omitempty"`
	VerifyStatus   string     `json:"verify_status"` // valid / tampered / unsigned
	DownloadCount  int        `json:"download_count"`
	SignedPackages int        `json:"signed_packages"`
	ReuploadAlerts int        `json:"reupload_alerts"`
	OriginalOf     string     `json:"original_of,omitempty"` // 若疑似盗用, 指向原技能
}

// SkillDownload 下载审计 (泄漏可溯源)
type SkillDownload struct {
	ID          string    `json:"id"`
	SkillID     string    `json:"skill_id"`
	SkillKey    string    `json:"skill_key,omitempty"`
	UserID      string    `json:"user_id,omitempty"`
	UserName    string    `json:"user_name,omitempty"`
	WatermarkID string    `json:"watermark_id"`
	Manifest    string    `json:"manifest_sha256,omitempty"`
	Signature   string    `json:"signature,omitempty"`
	SourceIP    string    `json:"source_ip,omitempty"`
	UserAgent   string    `json:"user_agent,omitempty"`
	Channel     string    `json:"channel,omitempty"` // github_import / market
	CreatedAt   time.Time `json:"created_at"`
}

// ImportRequest GitHub 技能导入审核单 (门禁核心: 先审核, 后下载)
type ImportRequest struct {
	ID              string            `json:"id"`
	Repository      string            `json:"repository"`
	Ref             string            `json:"ref"`
	SkillPath       string            `json:"skill_path"`
	SkillName       string            `json:"skill_name,omitempty"`
	SkillURL        string            `json:"skill_url,omitempty"`
	RequestedBy     string            `json:"requested_by,omitempty"`
	RequestedName   string            `json:"requested_by_name,omitempty"`
	Status          string            `json:"status"` // pending / scanning / blocked / pending_review / approved / rejected / imported / expired
	StatusCN        string            `json:"status_cn"`
	Verdict         string            `json:"verdict,omitempty"`
	VerdictCN       string            `json:"verdict_cn,omitempty"`
	RiskScore       int               `json:"risk_score"`
	Grade           string            `json:"grade,omitempty"`
	ScanID          string            `json:"scan_id,omitempty"`
	Findings        []SecurityFinding `json:"findings,omitempty"`
	CriticalCount   int               `json:"critical_count"`
	ContentHash     string            `json:"content_hash,omitempty"`
	ManifestSHA256  string            `json:"manifest_sha256,omitempty"`
	Signature       string            `json:"signature,omitempty"`
	WatermarkID     string            `json:"watermark_id,omitempty"`
	Reupload        *ReuploadReport   `json:"reupload,omitempty"`
	ReviewedBy      string            `json:"reviewed_by,omitempty"`
	ReviewedName    string            `json:"reviewed_by_name,omitempty"`
	ReviewNote      string            `json:"review_note,omitempty"`
	ReviewedAt      *time.Time        `json:"reviewed_at,omitempty"`
	ImportedSkill   string            `json:"imported_skill_id,omitempty"`
	ImportedAt      *time.Time        `json:"imported_at,omitempty"`
	DownloadAllowed bool              `json:"download_allowed"`
	ExpiresAt       *time.Time        `json:"expires_at,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// ReuploadReport 查重结果 (防止他人上传/导入盗用作品)
type ReuploadReport struct {
	Suspected    bool    `json:"suspected"`
	Similarity   float64 `json:"similarity"`
	MatchedSkill string  `json:"matched_skill_id,omitempty"`
	MatchedName  string  `json:"matched_skill_name,omitempty"`
	MatchedOwner string  `json:"matched_owner_id,omitempty"`
	MatchedWater string  `json:"matched_watermark_id,omitempty"`
	Reason       string  `json:"reason,omitempty"`
}

// SecurityOverview 安全治理概览
type SecurityOverview struct {
	TotalSkills         int64   `json:"total_skills"`
	Scanned             int64   `json:"scanned"`
	Unscanned           int64   `json:"unscanned"`
	Safe                int64   `json:"safe"`
	Suspicious          int64   `json:"suspicious"`
	Malicious           int64   `json:"malicious"`
	Blocked             int64   `json:"blocked"`
	CriticalOpen        int64   `json:"critical_open"`
	ImportPending       int64   `json:"import_pending"`
	ImportPendingReview int64   `json:"import_pending_review"`
	ImportApproved      int64   `json:"import_approved"`
	ImportRejected      int64   `json:"import_rejected"`
	ImportBlocked       int64   `json:"import_blocked"`
	ImportImported      int64   `json:"import_imported"`
	TotalScans          int64   `json:"total_scans"`
	AvgRisk             float64 `json:"avg_risk"`
	Downloads           int64   `json:"tracked_downloads"`
	ReuploadAlerts      int64   `json:"reupload_alerts"`
	TrackedSkills       int64   `json:"tracked_skills"`
	EngineVersion       string  `json:"engine_version"`
	RuleCount           int     `json:"rule_count"`
	AutoApprove         bool    `json:"auto_approve"`
	BlockOnCritical     bool    `json:"block_on_critical"`
	LastScanAt          string  `json:"last_scan_at,omitempty"`
}

// SecurityRuleDoc 规则说明 (前端展示用)
type SecurityRuleDoc struct {
	RuleID     string `json:"rule_id"`
	Category   string `json:"category"`
	CategoryCN string `json:"category_cn"`
	Severity   string `json:"severity"`
	Title      string `json:"title"`
	Detail     string `json:"detail"`
	Scope      string `json:"scope"` // code / markdown / dependency / package
}
