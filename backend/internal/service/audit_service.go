package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jjbank/skill-market/internal/config"
	"github.com/jjbank/skill-market/internal/model"
)

// ============================================================
// 技能可用性审核引擎 (Availability Audit Engine)
// ============================================================
//
// 目的: 回答"这个技能到底能不能正常用", 而不是只看元数据填得全不全。
// 检查分五类共 6 项:
//   AVAIL-01 元数据完整性      static       必填字段/命名/版本语义化
//   AVAIL-02 接口定义可解析    static       接入形态/端点/manifest 结构
//   AVAIL-03 服务可达与协议握手 protocol     initialize + tools/list 真实握手
//   AVAIL-04 核心功能可调用    functional    按 inputSchema 生成探针参数真实调用
//   AVAIL-05 响应性能基线      performance   连续 3 次调用成功率与耗时
//   AVAIL-06 安全合规基线      security      权限声明/输出无明文 PII/拦截策略一致
//
// 评分: 各检查项 0-100 分加权平均; 存在 critical 项失败 → 直接判定不合格。

const (
	auditEngineVersion = "1.0.0"
	auditPassScore     = 70.0 // 通过分数线
)

// AuditService 技能可用性审核服务
type AuditService struct {
	db  *sql.DB
	cfg *config.Config
}

// NewAuditService 创建审核服务
func NewAuditService(db *sql.DB, cfg *config.Config) *AuditService {
	return &AuditService{db: db, cfg: cfg}
}

// ============================================================
// 审核入口
// ============================================================

// RunAudit 对指定技能执行一次完整的可用性审核并落库
func (s *AuditService) RunAudit(ctx context.Context, skillID, triggerType, triggeredBy string) (*model.SkillAudit, error) {
	if triggerType == "" {
		triggerType = "manual"
	}
	sk, err := s.loadSkill(skillID)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	checks := []model.AuditCheck{
		checkMetadataIntegrity(sk),
		checkInterfaceSpec(sk),
	}
	// 运行期检查需要真实调用技能服务
	reach, tools := s.checkReachability(ctx, sk)
	checks = append(checks, reach)

	if reach.Status == "passed" {
		funcCheck, funcOutput := s.checkFunctionality(ctx, sk, tools)
		checks = append(checks, funcCheck)
		checks = append(checks, s.checkPerformance(ctx, sk, tools))
		checks = append(checks, checkSecurityBaseline(sk, funcOutput))
	} else {
		// 握手都过不了 → 功能/性能检查无意义, 判为失败并说明
		checks = append(checks,
			blockedCheck("AVAIL-04", "核心功能可调用", "functional", "critical", 1.0, "跳过: 服务未通过协议握手, 无法验证功能可用性"),
			blockedCheck("AVAIL-05", "响应性能基线", "performance", "major", 0.6, "跳过: 服务不可达, 无法采集性能数据"),
		)
		checks = append(checks, checkSecurityBaseline(sk, ""))
	}

	audit := summarizeAudit(sk, checks, time.Since(start), triggerType, triggeredBy)

	// 落库 + 回写技能审核态
	if err := s.persistAudit(audit); err != nil {
		return nil, err
	}
	return audit, nil
}

// GetAudit 按审核记录 ID 查询
func (s *AuditService) GetAudit(auditID string) (*model.SkillAudit, error) {
	var a model.SkillAudit
	var checksJSON string
	var score sql.NullFloat64
	var triggeredBy sql.NullString
	err := s.db.QueryRow(`
		SELECT a.id, a.skill_id, a.skill_key, COALESCE(sk.name,''), a.version, a.trigger_type,
		       a.triggered_by, COALESCE(u.display_name,''), a.status, a.score, COALESCE(a.grade,''),
		       a.total_checks, a.passed_checks, a.critical_failures, a.duration_ms,
		       COALESCE(a.checks::text,'[]'), COALESCE(a.summary,''), COALESCE(a.engine_version,''), a.created_at
		FROM skill_audits a
		LEFT JOIN skills sk ON a.skill_id = sk.id
		LEFT JOIN users u ON a.triggered_by = u.id
		WHERE a.id = $1
	`, auditID).Scan(
		&a.ID, &a.SkillID, &a.SkillKey, &a.SkillName, &a.Version, &a.TriggerType,
		&triggeredBy, &a.TriggeredByName, &a.Status, &score, &a.Grade,
		&a.TotalChecks, &a.PassedChecks, &a.CriticalFailures, &a.DurationMs,
		&checksJSON, &a.Summary, &a.EngineVersion, &a.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get audit: %w", err)
	}
	if triggeredBy.Valid {
		a.TriggeredBy = triggeredBy.String
	}
	a.Score = score.Float64
	a.Checks = decodeChecks(checksJSON)
	return &a, nil
}

// GetSkillAudits 某技能的历史审核记录
func (s *AuditService) GetSkillAudits(skillID string, limit int) ([]model.SkillAudit, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.Query(`
		SELECT a.id, a.skill_id, a.skill_key, COALESCE(sk.name,''), a.version, a.trigger_type,
		       COALESCE(a.triggered_by,''), COALESCE(u.display_name,''), a.status, a.score, COALESCE(a.grade,''),
		       a.total_checks, a.passed_checks, a.critical_failures, a.duration_ms,
		       COALESCE(a.checks::text,'[]'), COALESCE(a.summary,''), COALESCE(a.engine_version,''), a.created_at
		FROM skill_audits a
		LEFT JOIN skills sk ON a.skill_id = sk.id
		LEFT JOIN users u ON a.triggered_by = u.id
		WHERE a.skill_id = $1
		ORDER BY a.created_at DESC LIMIT $2
	`, skillID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAudits(rows)
}

// GetRecentAudits 最近审核记录 (全局)
func (s *AuditService) GetRecentAudits(limit int) ([]model.SkillAudit, error) {
	if limit <= 0 || limit > 200 {
		limit = 30
	}
	rows, err := s.db.Query(`
		SELECT a.id, a.skill_id, a.skill_key, COALESCE(sk.name,''), a.version, a.trigger_type,
		       COALESCE(a.triggered_by,''), COALESCE(u.display_name,''), a.status, a.score, COALESCE(a.grade,''),
		       a.total_checks, a.passed_checks, a.critical_failures, a.duration_ms,
		       COALESCE(a.checks::text,'[]'), COALESCE(a.summary,''), COALESCE(a.engine_version,''), a.created_at
		FROM skill_audits a
		LEFT JOIN skills sk ON a.skill_id = sk.id
		LEFT JOIN users u ON a.triggered_by = u.id
		ORDER BY a.created_at DESC LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAudits(rows)
}

// GetAuditQueue 审核队列: 全部技能 + 最近一次审核摘要
func (s *AuditService) GetAuditQueue(status string) ([]map[string]interface{}, error) {
	where := ""
	args := []interface{}{}
	if status != "" {
		where = "WHERE COALESCE(s.audit_status,'pending') = $1"
		args = append(args, status)
	}
	rows, err := s.db.Query(`
		SELECT s.id, s.skill_key, s.name, s.version, s.category, s.skill_type,
		       COALESCE(s.endpoint_url,''), s.status, s.visibility,
		       COALESCE(s.audit_status,'pending'), s.audit_score, s.last_audit_at,
		       COALESCE(u.display_name,''), s.created_at,
		       (SELECT a.id FROM skill_audits a WHERE a.skill_id = s.id ORDER BY a.created_at DESC LIMIT 1),
		       (SELECT COALESCE(a.grade,'') FROM skill_audits a WHERE a.skill_id = s.id ORDER BY a.created_at DESC LIMIT 1),
		       (SELECT a.critical_failures FROM skill_audits a WHERE a.skill_id = s.id ORDER BY a.created_at DESC LIMIT 1),
		       (SELECT COALESCE(a.summary,'') FROM skill_audits a WHERE a.skill_id = s.id ORDER BY a.created_at DESC LIMIT 1)
		FROM skills s
		LEFT JOIN users u ON s.author_id = u.id
		`+where+`
		ORDER BY CASE COALESCE(s.audit_status,'pending')
		           WHEN 'pending' THEN 0 WHEN 'failed' THEN 1 WHEN 'passed' THEN 2 ELSE 3 END,
		         s.updated_at DESC
		LIMIT 200
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]interface{}, 0)
	for rows.Next() {
		var (
			id, skillKey, name, version, category, skillType, endpointURL, pubStatus, visibility string
			auditStatus                                                                          string
			auditScore                                                                           sql.NullFloat64
			lastAuditAt                                                                          *time.Time
			authorName                                                                           string
			createdAt                                                                            time.Time
			lastAuditID, grade, summary                                                          sql.NullString
			criticalFail                                                                         sql.NullInt64
		)
		if err := rows.Scan(&id, &skillKey, &name, &version, &category, &skillType,
			&endpointURL, &pubStatus, &visibility, &auditStatus, &auditScore, &lastAuditAt,
			&authorName, &createdAt, &lastAuditID, &grade, &criticalFail, &summary); err != nil {
			return nil, err
		}
		item := map[string]interface{}{
			"id": id, "skill_key": skillKey, "name": name, "version": version,
			"category": category, "skill_type": skillType, "endpoint_url": endpointURL,
			"status": pubStatus, "visibility": visibility, "author_name": authorName,
			"created_at": createdAt, "audit_status": auditStatus,
			"audit_grade": grade.String, "critical_failures": criticalFail.Int64,
		}
		if auditScore.Valid {
			item["audit_score"] = auditScore.Float64
		}
		if lastAuditAt != nil {
			item["last_audit_at"] = *lastAuditAt
		}
		if lastAuditID.Valid {
			item["last_audit_id"] = lastAuditID.String
		}
		if summary.Valid {
			item["audit_summary"] = summary.String
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// GetOverview 审核概览统计
func (s *AuditService) GetOverview() (*model.AuditOverview, error) {
	o := &model.AuditOverview{EngineVer: auditEngineVersion}
	var lastAudit *time.Time
	var avgScore, passRate sql.NullFloat64
	err := s.db.QueryRow(`
		SELECT
		  (SELECT COUNT(*) FROM skills),
		  (SELECT COUNT(*) FROM skills WHERE COALESCE(audit_status,'pending')='passed'),
		  (SELECT COUNT(*) FROM skills WHERE COALESCE(audit_status,'pending')='failed'),
		  (SELECT COUNT(*) FROM skills WHERE COALESCE(audit_status,'pending') NOT IN ('passed','failed')),
		  (SELECT COUNT(*) FROM skill_audits),
		  (SELECT ROUND(AVG(score)::numeric,1) FROM skill_audits WHERE status='passed'),
		  (SELECT ROUND(100.0 * COUNT(*) FILTER (WHERE status='passed') / NULLIF(COUNT(*),0), 1) FROM skill_audits),
		  (SELECT MAX(created_at) FROM skill_audits),
		  (SELECT COUNT(*) FROM skills WHERE COALESCE(audit_status,'pending')='failed')
	`).Scan(&o.TotalSkills, &o.Passed, &o.Failed, &o.Pending, &o.TotalAudits,
		&avgScore, &passRate, &lastAudit, &o.CriticalOpen)
	if err != nil {
		return nil, fmt.Errorf("audit overview: %w", err)
	}
	o.AvgScore = avgScore.Float64
	o.PassRate = passRate.Float64
	if lastAudit != nil {
		o.LastAuditAt = lastAudit.Format(time.RFC3339)
	}
	return o, nil
}

// LatestPassedAudit 判断技能当前审核态是否可用于发布/安装
func (s *AuditService) LatestAuditState(skillID string) (status string, score float64, critical int, at *time.Time, err error) {
	var sc sql.NullFloat64
	err = s.db.QueryRow(`
		SELECT status, score, critical_failures, created_at FROM skill_audits
		WHERE skill_id = $1 ORDER BY created_at DESC LIMIT 1
	`, skillID).Scan(&status, &sc, &critical, &at)
	if err == sql.ErrNoRows {
		return "", 0, 0, nil, nil
	}
	if err != nil {
		return "", 0, 0, nil, err
	}
	score = sc.Float64
	return status, score, critical, at, nil
}

// ============================================================
// 检查项 1: 元数据完整性 (静态)
// ============================================================

var (
	reSkillKey = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,63}$`)
	reSemver   = regexp.MustCompile(`^\d+\.\d+\.\d+`)
)

func checkMetadataIntegrity(sk *auditSkill) model.AuditCheck {
	c := model.AuditCheck{Code: "AVAIL-01", Name: "元数据完整性", Category: "static", Level: "critical", Weight: 1.0}
	t0 := time.Now()
	var problems []string
	score := 100.0

	if !reSkillKey.MatchString(sk.SkillKey) {
		problems = append(problems, fmt.Sprintf("skill_key 命名不规范: %q (应为小写字母/数字/连字符)", sk.SkillKey))
		score -= 25
	}
	if strings.TrimSpace(sk.Name) == "" {
		problems = append(problems, "name 为空")
		score -= 20
	}
	if strings.TrimSpace(sk.Summary) == "" {
		problems = append(problems, "summary 为空")
		score -= 15
	} else if len([]rune(sk.Summary)) < 8 {
		problems = append(problems, "summary 过短, 不足以说明能力边界")
		score -= 5
	}
	if strings.TrimSpace(sk.Category) == "" {
		problems = append(problems, "category 为空")
		score -= 15
	}
	if !reSemver.MatchString(sk.Version) {
		problems = append(problems, fmt.Sprintf("version %q 不符合语义化版本 (x.y.z)", sk.Version))
		score -= 10
	}
	if len(sk.Tags) == 0 {
		problems = append(problems, "tags 为空, 影响检索与治理分类")
		score -= 5
	}
	if sk.AuthorID == "" {
		problems = append(problems, "缺少责任归属 (author_id)")
		score -= 20
	}
	if score < 0 {
		score = 0
	}

	c.Score = score
	c.DurationMs = int(time.Since(t0).Milliseconds())
	if len(problems) == 0 {
		c.Status = "passed"
		c.Detail = "元数据完整: 命名/名称/分类/版本/标签/责任人均合规"
	} else {
		c.Status = "failed"
		c.Detail = strings.Join(problems, "; ")
		c.Evidence = fmt.Sprintf("skill_key=%s version=%s category=%s tags=%d",
			sk.SkillKey, sk.Version, sk.Category, len(sk.Tags))
	}
	return c
}

// ============================================================
// 检查项 2: 接口定义可解析 (静态)
// ============================================================

var supportedSkillTypes = map[string]bool{"mcp": true, "dify": true, "api": true, "agent": true, "prompt": true}

func checkInterfaceSpec(sk *auditSkill) model.AuditCheck {
	c := model.AuditCheck{Code: "AVAIL-02", Name: "接口定义可解析", Category: "static", Level: "critical", Weight: 0.9}
	t0 := time.Now()
	var problems, notes []string
	score := 100.0

	if !supportedSkillTypes[sk.SkillType] {
		problems = append(problems, fmt.Sprintf("接入形态 %q 不在支持列表 (mcp/dify/api/agent/prompt)", sk.SkillType))
		score -= 40
	}
	if sk.SkillType == "mcp" || sk.SkillType == "api" {
		if strings.TrimSpace(sk.EndpointURL) == "" {
			problems = append(problems, "缺少接入地址 endpoint_url")
			score -= 30
		} else if !strings.HasPrefix(sk.EndpointURL, "http://") && !strings.HasPrefix(sk.EndpointURL, "https://") {
			problems = append(problems, "endpoint_url 协议非 http/https")
			score -= 20
		}
	}
	var manifest map[string]interface{}
	if strings.TrimSpace(sk.Manifest) == "" || sk.Manifest == "{}" {
		notes = append(notes, "manifest 为空, 未声明接口输入输出契约")
		score -= 15
	} else if err := json.Unmarshal([]byte(sk.Manifest), &manifest); err != nil {
		problems = append(problems, "manifest 不是合法 JSON: "+err.Error())
		score -= 35
	} else {
		if iface, ok := manifest["interface"].(map[string]interface{}); ok {
			inputs, hasIn := iface["inputs"].([]interface{})
			if !hasIn || len(inputs) == 0 {
				notes = append(notes, "manifest.interface.inputs 未声明入参")
				score -= 10
			}
		} else {
			notes = append(notes, "manifest 缺少 interface 契约段")
			score -= 10
		}
	}
	if score < 0 {
		score = 0
	}

	c.Score = score
	c.DurationMs = int(time.Since(t0).Milliseconds())
	c.Detail = "接口契约完整可解析"
	c.Evidence = fmt.Sprintf("skill_type=%s endpoint=%s manifest_bytes=%d", sk.SkillType, sk.EndpointURL, len(sk.Manifest))
	if len(problems) > 0 {
		c.Status = "failed"
		c.Detail = strings.Join(problems, "; ")
	} else if len(notes) > 0 {
		c.Status = "warning"
		c.Detail = strings.Join(notes, "; ")
	} else {
		c.Status = "passed"
	}
	return c
}

// ============================================================
// 检查项 3: 服务可达与协议握手 (真实调用)
// ============================================================

type mcpTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

type mcpToolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

type mcpResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  *struct {
		ProtocolVersion string                 `json:"protocolVersion"`
		ServerInfo      map[string]interface{} `json:"serverInfo"`
		Capabilities    map[string]interface{} `json:"capabilities"`
		Tools           []mcpTool              `json:"tools"`
		Content         []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	} `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (s *AuditService) checkReachability(ctx context.Context, sk *auditSkill) (model.AuditCheck, []mcpTool) {
	c := model.AuditCheck{Code: "AVAIL-03", Name: "服务可达与协议握手", Category: "protocol", Level: "critical", Weight: 1.2}
	t0 := time.Now()
	defer func() { c.DurationMs = int(time.Since(t0).Milliseconds()) }()

	if sk.SkillType != "mcp" {
		c.Status = "skipped"
		c.Score = 100
		c.Detail = fmt.Sprintf("接入形态为 %s, 无 MCP 握手环节 (由接入方按契约自检)", sk.SkillType)
		return c, nil
	}

	// 1) initialize
	initResp, _, err := s.callRunner(ctx, sk.SkillKey, "initialize", nil)
	if err != nil {
		c.Status = "failed"
		c.Score = 0
		c.Detail = "初始化握手失败, 技能服务不可达"
		c.Evidence = err.Error()
		return c, nil
	}
	protocol := ""
	serverName := ""
	if initResp.Result != nil {
		protocol = initResp.Result.ProtocolVersion
		if v, ok := initResp.Result.ServerInfo["name"].(string); ok {
			serverName = v
		}
	}
	if protocol == "" && serverName == "" {
		c.Status = "failed"
		c.Score = 20
		c.Detail = "initialize 返回缺少 protocolVersion / serverInfo"
		c.Evidence = truncate(mustJSON(initResp), 400)
		return c, nil
	}

	// 2) tools/list
	listResp, _, err := s.callRunner(ctx, sk.SkillKey, "tools/list", nil)
	if err != nil {
		c.Status = "failed"
		c.Score = 30
		c.Detail = "tools/list 调用失败"
		c.Evidence = err.Error()
		return c, nil
	}
	if listResp.Error != nil {
		c.Status = "failed"
		c.Score = 30
		c.Detail = "tools/list 返回错误: " + listResp.Error.Message
		return c, nil
	}
	var tools []mcpTool
	if listResp.Result != nil {
		tools = listResp.Result.Tools
	}
	if len(tools) == 0 {
		c.Status = "failed"
		c.Score = 40
		c.Detail = "技能未声明任何可调用工具, 实际不可用"
		return c, nil
	}

	var bad []string
	for _, t := range tools {
		if strings.TrimSpace(t.Name) == "" {
			bad = append(bad, "(未命名工具)")
			continue
		}
		if t.InputSchema == nil || t.InputSchema["type"] != "object" {
			bad = append(bad, t.Name+"(inputSchema 非 object)")
		}
	}

	c.Evidence = fmt.Sprintf("protocol=%s server=%s tools=%d [%s]", protocol, serverName, len(tools), toolNames(tools))
	if len(bad) > 0 {
		c.Status = "warning"
		c.Score = 75
		c.Detail = "握手成功, 但部分工具契约不完整: " + strings.Join(bad, ", ")
	} else {
		c.Status = "passed"
		c.Score = 100
		c.Detail = fmt.Sprintf("协议握手正常 (MCP %s), 声明 %d 个可调用工具", orDefault(protocol, "2024-11-05"), len(tools))
	}
	return c, tools
}

// ============================================================
// 检查项 4: 核心功能可调用 (真实调用)
// ============================================================

func (s *AuditService) checkFunctionality(ctx context.Context, sk *auditSkill, tools []mcpTool) (model.AuditCheck, string) {
	c := model.AuditCheck{Code: "AVAIL-04", Name: "核心功能可调用", Category: "functional", Level: "critical", Weight: 1.5}
	t0 := time.Now()
	defer func() { c.DurationMs = int(time.Since(t0).Milliseconds()) }()

	tool := pickProbeTool(sk.SkillKey, tools)
	if tool == nil {
		c.Status = "failed"
		c.Score = 0
		c.Detail = "未找到可探测的工具"
		return c, ""
	}
	args := buildProbeArgs(sk.SkillKey, tool.InputSchema)
	argsJSON := mustJSON(args)

	resp, _, err := s.callRunner(ctx, sk.SkillKey, "tools/call", map[string]interface{}{
		"name":      tool.Name,
		"arguments": args,
	})
	c.Evidence = fmt.Sprintf("tool=%s args=%s", tool.Name, truncate(argsJSON, 200))
	if err != nil {
		c.Status = "failed"
		c.Score = 0
		c.Detail = "功能调用失败: " + err.Error()
		return c, ""
	}
	if resp.Error != nil {
		c.Status = "failed"
		c.Score = 10
		c.Detail = "功能调用返回协议错误: " + resp.Error.Message
		c.Evidence += " | error=" + truncate(resp.Error.Message, 200)
		return c, ""
	}
	if resp.Result == nil {
		c.Status = "failed"
		c.Score = 20
		c.Detail = "功能调用未返回结果体"
		return c, ""
	}
	if resp.Result.IsError {
		c.Status = "failed"
		c.Score = 20
		c.Detail = "功能调用返回 isError=true, 技能内部执行异常"
		c.Evidence += " | " + truncate(mustJSON(resp.Result), 300)
		return c, ""
	}
	text := ""
	for _, part := range resp.Result.Content {
		text += part.Text
	}
	c.Evidence += " | output=" + truncate(text, 300)
	if len(strings.TrimSpace(text)) < 10 {
		c.Status = "failed"
		c.Score = 35
		c.Detail = "功能调用返回空结果, 无法证明能力可用"
		return c, text
	}

	c.Status = "passed"
	c.Score = 100
	c.Detail = fmt.Sprintf("工具 %s 真实调用成功, 返回 %d 字符有效结果", tool.Name, len([]rune(text)))
	return c, text
}

// ============================================================
// 检查项 5: 响应性能基线 (连续调用)
// ============================================================

func (s *AuditService) checkPerformance(ctx context.Context, sk *auditSkill, tools []mcpTool) model.AuditCheck {
	c := model.AuditCheck{Code: "AVAIL-05", Name: "响应性能基线", Category: "performance", Level: "major", Weight: 0.6}
	t0 := time.Now()
	defer func() { c.DurationMs = int(time.Since(t0).Milliseconds()) }()

	tool := pickProbeTool(sk.SkillKey, tools)
	if tool == nil {
		c.Status = "skipped"
		c.Score = 100
		c.Detail = "无可探测工具, 跳过性能基线"
		return c
	}
	args := buildProbeArgs(sk.SkillKey, tool.InputSchema)

	const rounds = 3
	var durations []int
	failures := 0
	for i := 0; i < rounds; i++ {
		resp, ms, err := s.callRunner(ctx, sk.SkillKey, "tools/call", map[string]interface{}{
			"name": tool.Name, "arguments": args,
		})
		durations = append(durations, ms)
		if err != nil || resp == nil || resp.Error != nil || (resp.Result != nil && resp.Result.IsError) {
			failures++
		}
	}
	sort.Ints(durations)
	avg := 0
	for _, d := range durations {
		avg += d
	}
	avg /= len(durations)
	p95 := durations[len(durations)-1]
	successRate := 100.0 * float64(rounds-failures) / float64(rounds)

	c.Evidence = fmt.Sprintf("rounds=%d avg=%dms max=%dms success=%.0f%%", rounds, avg, p95, successRate)
	switch {
	case failures == 0 && p95 <= 1500:
		c.Status = "passed"
		c.Score = 100
		c.Detail = fmt.Sprintf("连续 %d 次调用全部成功, 平均 %dms / 峰值 %dms", rounds, avg, p95)
	case failures == 0 && p95 <= 3000:
		c.Status = "warning"
		c.Score = 75
		c.Detail = fmt.Sprintf("调用全部成功但峰值耗时 %dms 偏高, 建议优化后再上架", p95)
	default:
		c.Status = "failed"
		c.Score = 30
		c.Detail = fmt.Sprintf("连续调用成功率仅 %.0f%% (峰值 %dms), 稳定性不达标", successRate, p95)
	}
	s.recordPerformance(sk.ID, avg, successRate)
	return c
}

// ============================================================
// 检查项 6: 安全合规基线
// ============================================================

// Go regexp (RE2) 不支持环视断言, 用字符类边界代替
var (
	reIDCard   = regexp.MustCompile(`[0-9]{17}[0-9Xx]`)
	rePhoneCN  = regexp.MustCompile(`(^|[^0-9])1[3-9][0-9]{9}([^0-9]|$)`)
	reBankCard = regexp.MustCompile(`(^|[^0-9])[0-9]{16,19}([^0-9]|$)`)
)

// 与网关输入拦截层保持一致的敏感关键词
var auditSensitiveKeywords = []string{
	"id_card", "身份证", "phone", "手机", "password", "密码",
	"token", "secret", "api_key", "access_key",
}

func checkSecurityBaseline(sk *auditSkill, funcOutput string) model.AuditCheck {
	c := model.AuditCheck{Code: "AVAIL-06", Name: "安全合规基线", Category: "security", Level: "major", Weight: 0.9}
	t0 := time.Now()
	defer func() { c.DurationMs = int(time.Since(t0).Milliseconds()) }()

	var problems, notes []string
	score := 100.0

	// 6.1 权限声明
	if strings.TrimSpace(sk.Permissions) == "" || strings.TrimSpace(sk.Permissions) == "[]" {
		problems = append(problems, "未声明任何权限范围 (permissions 为空), 无法评估数据访问边界")
		score -= 30
	}
	// 6.2 输出明文敏感数据检测 (只扫功能调用返回的正文, 不扫探针入参)
	if isDesensitizeSkillKey(sk.SkillKey) {
		notes = append(notes, "脱敏类技能, 输入天然含敏感数据 (网关已按白名单放行), 输出侧做脱敏校验")
		if reIDCard.MatchString(funcOutput) {
			problems = append(problems, "脱敏技能输出中出现未脱敏的身份证号")
			score -= 40
		}
	} else if reIDCard.MatchString(funcOutput) || rePhoneCN.MatchString(funcOutput) {
		problems = append(problems, "功能输出中出现明文个人敏感信息 (身份证/手机号)")
		score -= 40
	}
	// 6.3 输入拦截策略一致性
	if !isDesensitizeSkillKey(sk.SkillKey) {
		probe := map[string]interface{}{"phone": "13800138000", "备注": "客户身份证 110101199003078888"}
		if !hasSensitiveProbe(probe) {
			problems = append(problems, "网关敏感输入拦截规则对该技能未生效 (策略不一致)")
			score -= 25
		}
	}
	if score < 0 {
		score = 0
	}

	c.Score = score
	if len(problems) > 0 {
		c.Status = "failed"
		c.Detail = strings.Join(problems, "; ")
	} else if len(notes) > 0 {
		c.Status = "warning"
		c.Score = 90
		c.Detail = strings.Join(notes, "; ")
	} else {
		c.Status = "passed"
		c.Detail = "权限声明完整, 输出未发现明文敏感信息, 拦截策略一致"
	}
	return c
}

// hasSensitiveProbe 与网关 handler.hasSensitiveData 同规则 (键名 + 值关键词双向检测)
func hasSensitiveProbe(params map[string]interface{}) bool {
	for k, v := range params {
		lower := strings.ToLower(k)
		for _, kw := range auditSensitiveKeywords {
			if strings.Contains(lower, kw) {
				return true
			}
		}
		if s, ok := v.(string); ok {
			ls := strings.ToLower(s)
			for _, kw := range auditSensitiveKeywords {
				if strings.Contains(ls, strings.ToLower(kw)) {
					return true
				}
			}
		}
	}
	return false
}

// ============================================================
// 汇总 / 落库
// ============================================================

func summarizeAudit(sk *auditSkill, checks []model.AuditCheck, elapsed time.Duration, triggerType, triggeredBy string) *model.SkillAudit {
	totalWeight, weighted, passed, criticalFail := 0.0, 0.0, 0, 0
	var failures []string
	for _, chk := range checks {
		if chk.Status == "skipped" {
			continue
		}
		totalWeight += chk.Weight
		weighted += chk.Score * chk.Weight
		if chk.Status == "passed" {
			passed++
		}
		if chk.Status == "failed" {
			if chk.Level == "critical" {
				criticalFail++
			}
			failures = append(failures, chk.Code+" "+chk.Name)
		}
	}
	score := 100.0
	if totalWeight > 0 {
		score = weighted / totalWeight
	}
	score = float64(int(score*10+0.5)) / 10

	grade := "D"
	switch {
	case score >= 90:
		grade = "A"
	case score >= 80:
		grade = "B"
	case score >= 70:
		grade = "C"
	}

	status := "passed"
	summary := fmt.Sprintf("全部 %d 项检查通过, 综合得分 %.1f 分 (%s 级), 技能可正常发布与调用", len(checks), score, grade)
	if criticalFail > 0 || score < auditPassScore {
		status = "failed"
		summary = fmt.Sprintf("可用性审核不合格: %d 项关键检查失败 (%s), 综合得分 %.1f 分",
			criticalFail, strings.Join(failures, ", "), score)
	} else if len(failures) > 0 {
		summary = fmt.Sprintf("审核通过但存在待优化项 (%s), 综合得分 %.1f 分", strings.Join(failures, ", "), score)
	}

	return &model.SkillAudit{
		SkillID:          sk.ID,
		SkillKey:         sk.SkillKey,
		SkillName:        sk.Name,
		Version:          sk.Version,
		TriggerType:      triggerType,
		TriggeredBy:      triggeredBy,
		Status:           status,
		Score:            score,
		Grade:            grade,
		TotalChecks:      len(checks),
		PassedChecks:     passed,
		CriticalFailures: criticalFail,
		DurationMs:       int(elapsed.Milliseconds()),
		Checks:           checks,
		Summary:          summary,
		EngineVersion:    auditEngineVersion,
	}
}

func (s *AuditService) persistAudit(a *model.SkillAudit) error {
	checksJSON := mustJSON(a.Checks)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := tx.QueryRow(`
		INSERT INTO skill_audits (skill_id, skill_key, version, trigger_type, triggered_by,
		       status, score, grade, total_checks, passed_checks, critical_failures,
		       duration_ms, checks, summary, engine_version)
		VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING id, created_at
	`, a.SkillID, a.SkillKey, a.Version, a.TriggerType, a.TriggeredBy,
		a.Status, a.Score, a.Grade, a.TotalChecks, a.PassedChecks, a.CriticalFailures,
		a.DurationMs, checksJSON, a.Summary, a.EngineVersion).Scan(&a.ID, &a.CreatedAt); err != nil {
		return fmt.Errorf("insert skill audit: %w", err)
	}

	if _, err := tx.Exec(`
		UPDATE skills SET audit_status=$1, audit_score=$2, last_audit_at=NOW(), updated_at=NOW()
		WHERE id=$3
	`, a.Status, a.Score, a.SkillID); err != nil {
		return fmt.Errorf("update skill audit state: %w", err)
	}

	// 审核流水 (与既有 skill_reviews 表打通, 便于治理页统一展示)
	stage := "function_test"
	verdict := "pass"
	if a.Status != "passed" {
		verdict = "fail"
	}
	if _, err := tx.Exec(`
		INSERT INTO skill_reviews (skill_id, reviewer_id, stage, verdict, comment, scan_report)
		VALUES ($1, NULLIF($2,''), $3, $4, $5, $6)
	`, a.SkillID, a.TriggeredBy, stage, verdict, a.Summary, checksJSON); err != nil {
		// scan_report 为 JSONB, 若历史表结构不一致则降级为不带报告
		if _, err2 := tx.Exec(`
			INSERT INTO skill_reviews (skill_id, reviewer_id, stage, verdict, comment)
			VALUES ($1, NULLIF($2,''), $3, $4, $5)
		`, a.SkillID, a.TriggeredBy, stage, verdict, a.Summary); err2 != nil {
			return fmt.Errorf("insert review flow: %w (%v)", err2, err)
		}
	}
	return tx.Commit()
}

// recordPerformance 把性能基线回写到技能质量字段
func (s *AuditService) recordPerformance(skillID string, avgMs int, successRate float64) {
	_, _ = s.db.Exec(`
		UPDATE skills SET avg_latency_ms=$1, success_rate=$2 WHERE id=$3
	`, avgMs, successRate, skillID)
}

// ============================================================
// 技能运行服务调用
// ============================================================

func (s *AuditService) runnerBase() string {
	if u := strings.TrimRight(os.Getenv("SKILL_RUNNER_URL"), "/"); u != "" {
		return u
	}
	return "http://localhost:8081"
}

func (s *AuditService) callRunner(ctx context.Context, skillKey, method string, params map[string]interface{}) (*mcpResponse, int, error) {
	if strings.ContainsAny(skillKey, "/\\..") {
		return nil, 0, fmt.Errorf("非法技能标识: %s", skillKey)
	}
	if params == nil {
		params = map[string]interface{}{}
	}
	body, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	})
	url := fmt.Sprintf("%s/%s/mcp", s.runnerBase(), skillKey)

	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, int(time.Since(start).Milliseconds()), fmt.Errorf("技能服务不可达 (%s): %v", url, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	ms := int(time.Since(start).Milliseconds())
	if err != nil {
		return nil, ms, fmt.Errorf("读取响应失败: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, ms, fmt.Errorf("技能服务返回 HTTP %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var out mcpResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, ms, fmt.Errorf("响应非合法 MCP JSON: %v (原始: %s)", err, truncate(string(raw), 120))
	}
	return &out, ms, nil
}

// ============================================================
// 探针参数
// ============================================================

// auditSkill 审核所需的技能快照
type auditSkill struct {
	ID            string
	SkillKey      string
	Name          string
	Version       string
	Category      string
	Summary       string
	SkillType     string
	EndpointURL   string
	Manifest      string
	Dependencies  string
	Permissions   string
	InterfaceSpec string
	Tags          []string
	Status        string
	AuthorID      string
}

func (s *AuditService) loadSkill(skillID string) (*auditSkill, error) {
	var sk auditSkill
	var tags string
	err := s.db.QueryRow(`
		SELECT id, skill_key, name, version, category, summary, skill_type,
		       COALESCE(endpoint_url,''), COALESCE(manifest::text,'{}'),
		       COALESCE(dependencies::text,'{}'), COALESCE(permissions::text,'[]'),
		       COALESCE(interface_spec::text,'{}'), COALESCE(array_to_string(tags,','),''),
		       status, author_id
		FROM skills WHERE id=$1
	`, skillID).Scan(&sk.ID, &sk.SkillKey, &sk.Name, &sk.Version, &sk.Category, &sk.Summary,
		&sk.SkillType, &sk.EndpointURL, &sk.Manifest, &sk.Dependencies, &sk.Permissions,
		&sk.InterfaceSpec, &tags, &sk.Status, &sk.AuthorID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("技能不存在")
	}
	if err != nil {
		return nil, fmt.Errorf("load skill: %w", err)
	}
	if tags != "" {
		sk.Tags = strings.Split(tags, ",")
	}
	return &sk, nil
}

// 已知技能的探针入参 (贴合真实业务语义, 便于演示)
var curatedProbes = map[string]map[string]interface{}{
	"db-inspection": {
		"target_db":   "core-banking-db-01",
		"check_scope": "full",
	},
	"log-desensitization": {
		"log_content": "2026-09-17 用户张三(身份证 110101199003078888) 登录, 手机号 13800138000",
	},
	"alert-convergence": {
		"host":               "payment",
		"time_range_minutes": 60,
	},
	"requirement-analysis": {
		"title":       "对公开户线上化需求",
		"description": "客户通过手机银行提交对公开户申请, 需完成身份核验、风险评级与账户开立",
	},
}

var defaultProbeTools = map[string]string{
	"db-inspection":        "inspect_database",
	"log-desensitization":  "desensitize",
	"alert-convergence":    "analyze_alerts",
	"requirement-analysis": "analyze_requirement",
}

func pickProbeTool(skillKey string, tools []mcpTool) *mcpTool {
	if len(tools) == 0 {
		return nil
	}
	if want, ok := defaultProbeTools[skillKey]; ok {
		for i := range tools {
			if tools[i].Name == want {
				return &tools[i]
			}
		}
	}
	for i := range tools {
		if tools[i].InputSchema != nil {
			return &tools[i]
		}
	}
	return &tools[0]
}

// buildProbeArgs 优先用贴合业务的探针, 否则按 inputSchema 自动推导
func buildProbeArgs(skillKey string, schema map[string]interface{}) map[string]interface{} {
	if curated, ok := curatedProbes[skillKey]; ok {
		return curated
	}
	return deriveArgs(schema)
}

func deriveArgs(schema map[string]interface{}) map[string]interface{} {
	args := map[string]interface{}{}
	if schema == nil {
		return args
	}
	props, _ := schema["properties"].(map[string]interface{})
	if props == nil {
		return args
	}
	required := map[string]bool{}
	if rs, ok := schema["required"].([]interface{}); ok {
		for _, r := range rs {
			if name, ok := r.(string); ok {
				required[name] = true
			}
		}
	}
	for name, raw := range props {
		if len(required) > 0 && !required[name] {
			continue
		}
		p, _ := raw.(map[string]interface{})
		if p == nil {
			args[name] = "probe"
			continue
		}
		if def, ok := p["default"]; ok {
			args[name] = def
			continue
		}
		if enum, ok := p["enum"].([]interface{}); ok && len(enum) > 0 {
			args[name] = enum[0]
			continue
		}
		switch p["type"] {
		case "string":
			args[name] = autoString(name)
		case "integer":
			args[name] = 60
		case "number":
			args[name] = 1.0
		case "boolean":
			args[name] = true
		case "array":
			args[name] = []string{autoString(name)}
		default:
			args[name] = autoString(name)
		}
	}
	return args
}

func autoString(field string) string {
	switch {
	case strings.Contains(field, "db") || strings.Contains(field, "database"):
		return "core-banking-db-01"
	case strings.Contains(field, "host"):
		return "payment"
	case strings.Contains(field, "title"):
		return "可用性审核探针"
	case strings.Contains(field, "content") || strings.Contains(field, "text") || strings.Contains(field, "log"):
		return "可用性审核探针内容"
	case strings.Contains(field, "description") || strings.Contains(field, "scenario"):
		return "可用性审核自动生成的探针描述"
	case strings.Contains(field, "id"):
		return "PROBE-1001"
	default:
		return "probe"
	}
}

// ============================================================
// 辅助
// ============================================================

func isDesensitizeSkillKey(key string) bool {
	switch key {
	case "log-desensitization", "pii-detection", "data-masking":
		return true
	}
	return false
}

func blockedCheck(code, name, category, level string, weight float64, detail string) model.AuditCheck {
	return model.AuditCheck{Code: code, Name: name, Category: category, Level: level,
		Weight: weight, Status: "failed", Score: 0, Detail: detail}
}

func toolNames(tools []mcpTool) string {
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name)
	}
	return strings.Join(names, ", ")
}

func mustJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func decodeChecks(raw string) []model.AuditCheck {
	var checks []model.AuditCheck
	if raw == "" {
		return []model.AuditCheck{}
	}
	if err := json.Unmarshal([]byte(raw), &checks); err != nil {
		return []model.AuditCheck{}
	}
	return checks
}

func scanAudits(rows *sql.Rows) ([]model.SkillAudit, error) {
	out := make([]model.SkillAudit, 0)
	for rows.Next() {
		var a model.SkillAudit
		var checksJSON string
		var score sql.NullFloat64
		err := rows.Scan(&a.ID, &a.SkillID, &a.SkillKey, &a.SkillName, &a.Version, &a.TriggerType,
			&a.TriggeredBy, &a.TriggeredByName, &a.Status, &score, &a.Grade,
			&a.TotalChecks, &a.PassedChecks, &a.CriticalFailures, &a.DurationMs,
			&checksJSON, &a.Summary, &a.EngineVersion, &a.CreatedAt)
		if err != nil {
			return nil, err
		}
		a.Score = score.Float64
		a.Checks = decodeChecks(checksJSON)
		out = append(out, a)
	}
	return out, rows.Err()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
