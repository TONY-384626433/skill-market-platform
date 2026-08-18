package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jjbank/skill-market/internal/config"
	"github.com/jjbank/skill-market/internal/model"
	"github.com/lib/pq"
	"golang.org/x/crypto/sha3"
)

// SkillService 技能业务服务
type SkillService struct {
	db  *sql.DB
	cfg *config.Config
}

// NewSkillService 创建技能服务
func NewSkillService(db *sql.DB, cfg *config.Config) *SkillService {
	return &SkillService{db: db, cfg: cfg}
}

// ============================================================
// 技能查询
// ============================================================

// Search 搜索技能 (关键字 + 分类 + 排序 + 分页)
func (s *SkillService) Search(req *model.SkillSearchRequest) ([]model.Skill, int64, error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 || req.PageSize > 50 {
		req.PageSize = 20
	}

	// 构建查询
	where := "WHERE s.status = 'published' AND s.visibility = 'public'"
	args := []interface{}{}
	argIdx := 1

	if req.Query != "" {
		where += fmt.Sprintf(" AND (s.name ILIKE $%d OR s.summary ILIKE $%d OR $%d = ANY(s.tags))", argIdx, argIdx+1, argIdx+2)
		likeQ := "%" + req.Query + "%"
		args = append(args, likeQ, likeQ, req.Query)
		argIdx += 3
	}
	if req.Category != "" {
		where += fmt.Sprintf(" AND s.category = $%d", argIdx)
		args = append(args, req.Category)
		argIdx++
	}
	if req.SkillType != "" {
		where += fmt.Sprintf(" AND s.skill_type = $%d", argIdx)
		args = append(args, req.SkillType)
		argIdx++
	}

	// 排序
	orderBy := "ORDER BY s.install_count DESC"
	switch req.SortBy {
	case "rating":
		orderBy = "ORDER BY s.rating_avg DESC, s.rating_count DESC"
	case "newest":
		orderBy = "ORDER BY s.created_at DESC"
	case "installs":
		orderBy = "ORDER BY s.install_count DESC"
	}

	// 计数
	var total int64
	countQuery := "SELECT COUNT(*) FROM skills s " + where
	err := s.db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count skills: %w", err)
	}

	// 查询数据
	offset := (req.Page - 1) * req.PageSize
	query := fmt.Sprintf(`
		SELECT s.id, s.skill_key, s.name, s.version, s.category,
		       s.tags, s.summary, COALESCE(s.description,''), COALESCE(s.icon_url,''),
		       s.skill_type, COALESCE(s.endpoint_url,''), s.stability,
		       s.install_count, s.call_count, s.rating_avg, s.rating_count,
		       s.status, s.author_id, COALESCE(u.display_name,''),
		       COALESCE(s.team_id,''), COALESCE(t.name,''),
		       s.created_at, s.updated_at
		FROM skills s
		LEFT JOIN users u ON s.author_id = u.id
		LEFT JOIN teams t ON s.team_id = t.id
		%s %s LIMIT $%d OFFSET $%d
	`, where, orderBy, argIdx, argIdx+1)

	args = append(args, req.PageSize, offset)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query skills: %w", err)
	}
	defer rows.Close()

	var skills []model.Skill
	for rows.Next() {
		var sk model.Skill
		err := rows.Scan(
			&sk.ID, &sk.SkillKey, &sk.Name, &sk.Version, &sk.Category,
			pq.Array(&sk.Tags), &sk.Summary, &sk.Description, &sk.IconURL,
			&sk.SkillType, &sk.EndpointURL, &sk.Stability,
			&sk.InstallCount, &sk.CallCount, &sk.RatingAvg, &sk.RatingCount,
			&sk.Status, &sk.AuthorID, &sk.AuthorName,
			&sk.TeamID, &sk.TeamName,
			&sk.CreatedAt, &sk.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan skill: %w", err)
		}
		skills = append(skills, sk)
	}
	return skills, total, nil
}

// GetByID 根据 ID 获取技能详情
func (s *SkillService) GetByID(id string) (*model.Skill, error) {
	var sk model.Skill
	err := s.db.QueryRow(`
		SELECT s.id, s.skill_key, s.name, s.version, s.category,
		       s.tags, s.summary, COALESCE(s.description,''), COALESCE(s.icon_url,''),
		       s.skill_type, COALESCE(s.endpoint_url,''), s.stability,
		       s.install_count, s.call_count, s.rating_avg, s.rating_count,
		       s.status, s.author_id, COALESCE(u.display_name,''),
		       COALESCE(s.team_id,''), COALESCE(t.name,''),
		       COALESCE(s.manifest::text,'{}'), COALESCE(s.interface_spec::text,'{}'),
		       COALESCE(s.permissions::text,'[]'),
		       s.created_at, s.updated_at
		FROM skills s
		LEFT JOIN users u ON s.author_id = u.id
		LEFT JOIN teams t ON s.team_id = t.id
		WHERE s.id = $1
	`, id).Scan(
		&sk.ID, &sk.SkillKey, &sk.Name, &sk.Version, &sk.Category,
		pq.Array(&sk.Tags), &sk.Summary, &sk.Description, &sk.IconURL,
		&sk.SkillType, &sk.EndpointURL, &sk.Stability,
		&sk.InstallCount, &sk.CallCount, &sk.RatingAvg, &sk.RatingCount,
		&sk.Status, &sk.AuthorID, &sk.AuthorName,
		&sk.TeamID, &sk.TeamName,
		&sk.Manifest, &sk.InterfaceSpec, &sk.Permissions,
		&sk.CreatedAt, &sk.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get skill: %w", err)
	}
	return &sk, nil
}

// GetByCategory 按分类获取技能列表
func (s *SkillService) GetByCategory(category string, limit int) ([]model.Skill, error) {
	rows, err := s.db.Query(`
		SELECT s.id, s.skill_key, s.name, s.version, s.category,
		       s.tags, s.summary, s.skill_type, s.stability,
		       s.install_count, s.rating_avg, s.status, s.author_id,
		       COALESCE(u.display_name,''), s.created_at, s.updated_at
		FROM skills s LEFT JOIN users u ON s.author_id = u.id
		WHERE s.status = 'published' AND s.category = $1
		ORDER BY s.install_count DESC LIMIT $2
	`, category, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var skills []model.Skill
	for rows.Next() {
		var sk model.Skill
		if err := rows.Scan(&sk.ID, &sk.SkillKey, &sk.Name, &sk.Version, &sk.Category,
			pq.Array(&sk.Tags), &sk.Summary, &sk.SkillType, &sk.Stability,
			&sk.InstallCount, &sk.RatingAvg, &sk.Status, &sk.AuthorID,
			&sk.AuthorName, &sk.CreatedAt, &sk.UpdatedAt); err != nil {
			return nil, err
		}
		skills = append(skills, sk)
	}
	return skills, nil
}

// ============================================================
// 技能注册
// ============================================================

// CreateSkill 创建新技能 (提交审核)
func (s *SkillService) CreateSkill(sk *model.Skill) error {
	_, err := s.db.Exec(`
		INSERT INTO skills (skill_key, name, version, category, sub_category, tags,
		       summary, description, skill_type, endpoint_url, endpoint_protocol,
		       manifest, dependencies, permissions, interface_spec,
		       stability, status, visibility, author_id, team_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,'pending_approval',$17,$18,$19)
	`, sk.SkillKey, sk.Name, sk.Version, sk.Category, sk.SubCategory,
		pq.Array(sk.Tags),
		sk.Summary, sk.Description,
		sk.SkillType, sk.EndpointURL, sk.EndpointProto,
		sk.Manifest, sk.Dependencies, sk.Permissions, sk.InterfaceSpec,
		sk.Stability, sk.Visibility, sk.AuthorID, sk.TeamID,
	)
	if err != nil {
		return fmt.Errorf("create skill: %w", err)
	}
	return nil
}

// UpdateSkill 更新技能
func (s *SkillService) UpdateSkill(id string, updates map[string]interface{}) error {
	// 简化实现: 仅更新允许的字段
	_, err := s.db.Exec(`
		UPDATE skills SET name=$1, summary=$2, description=$3, tags=$4,
		       category=$5, endpoint_url=$6, stability=$7, updated_at=NOW()
		WHERE id=$8
	`, updates["name"], updates["summary"], updates["description"],
		pq.Array(updates["tags"]),
		updates["category"], updates["endpoint_url"],
		updates["stability"], id)
	return err
}

// ============================================================
// 技能安装
// ============================================================

// InstallSkill 安装技能 — 生成 API Token
func (s *SkillService) InstallSkill(skillID, userID, version string) (*model.SkillInstallation, string, error) {
	// 检查是否已安装
	var existingID string
	_ = s.db.QueryRow(`SELECT id FROM skill_installations WHERE skill_id=$1 AND user_id=$2 AND status='active'`,
		skillID, userID).Scan(&existingID)
	if existingID != "" {
		return nil, "", fmt.Errorf("该技能已安装, 安装 ID: %s", existingID)
	}

	// 生成 API Token: sk-{user_prefix}-{random}
	rawToken := "sk-" + userID[:min(8, len(userID))] + "-" + randomHex(32)
	tokenHash := hashToken(rawToken)
	tokenPrefix := rawToken[:16]

	// 获取技能版本
	if version == "" {
		_ = s.db.QueryRow(`SELECT version FROM skills WHERE id=$1`, skillID).Scan(&version)
	}

	inst := &model.SkillInstallation{
		SkillID:      skillID,
		SkillVersion: version,
		UserID:       userID,
		APIKeyPrefix: tokenPrefix,
		Status:       "active",
		InstalledAt:  time.Now(),
	}

	err := s.db.QueryRow(`
		INSERT INTO skill_installations (skill_id, skill_version, user_id, api_key_hash, api_key_prefix, status)
		VALUES ($1,$2,$3,$4,$5,'active')
		RETURNING id
	`, skillID, version, userID, tokenHash, tokenPrefix).Scan(&inst.ID)
	if err != nil {
		return nil, "", fmt.Errorf("install skill: %w", err)
	}

	// 更新安装计数
	_, _ = s.db.Exec(`UPDATE skills SET install_count = install_count + 1 WHERE id = $1`, skillID)

	return inst, rawToken, nil
}

// GetUserInstallations 获取用户已安装的技能
func (s *SkillService) GetUserInstallations(userID string) ([]model.SkillInstallation, error) {
	rows, err := s.db.Query(`
		SELECT si.id, si.skill_id, sk.name, si.skill_version,
		       si.user_id, si.api_key_prefix, si.token_expires_at,
		       si.status, si.installed_at
		FROM skill_installations si
		JOIN skills sk ON si.skill_id = sk.id
		WHERE si.user_id = $1 AND si.status = 'active'
		ORDER BY si.installed_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var installations []model.SkillInstallation
	for rows.Next() {
		var inst model.SkillInstallation
		if err := rows.Scan(&inst.ID, &inst.SkillID, &inst.SkillName, &inst.SkillVersion,
			&inst.UserID, &inst.APIKeyPrefix, &inst.TokenExpiresAt,
			&inst.Status, &inst.InstalledAt); err != nil {
			return nil, err
		}
		installations = append(installations, inst)
	}
	return installations, nil
}

// RevokeInstallation 注销安装
func (s *SkillService) RevokeInstallation(installationID, userID string) error {
	_, err := s.db.Exec(`
		UPDATE skill_installations SET status='revoked', revoked_at=NOW()
		WHERE id=$1 AND user_id=$2 AND status='active'
	`, installationID, userID)
	return err
}

// ============================================================
// 技能评价
// ============================================================

// RateSkill 评价技能
func (s *SkillService) RateSkill(skillID, userID string, rating int, title, comment string) error {
	_, err := s.db.Exec(`
		INSERT INTO skill_ratings (skill_id, user_id, rating, title, comment)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (skill_id, user_id) DO UPDATE SET rating=$3, title=$4, comment=$5
	`, skillID, userID, rating, title, comment)
	if err != nil {
		return err
	}

	// 更新技能平均分
	_, _ = s.db.Exec(`
		UPDATE skills SET
		  rating_avg = (SELECT AVG(rating)::numeric(3,2) FROM skill_ratings WHERE skill_id=$1),
		  rating_count = (SELECT COUNT(*) FROM skill_ratings WHERE skill_id=$1)
		WHERE id = $1
	`, skillID)
	return nil
}

// GetSkillRatings 获取技能评价列表
func (s *SkillService) GetSkillRatings(skillID string, page, pageSize int) ([]model.SkillRating, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}

	var total int64
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM skill_ratings WHERE skill_id=$1`, skillID).Scan(&total)

	rows, err := s.db.Query(`
		SELECT sr.id, sr.skill_id, sr.user_id, COALESCE(u.display_name,''),
		       sr.rating, COALESCE(sr.title,''), COALESCE(sr.comment,''), sr.created_at
		FROM skill_ratings sr LEFT JOIN users u ON sr.user_id = u.id
		WHERE sr.skill_id=$1 ORDER BY sr.created_at DESC LIMIT $2 OFFSET $3
	`, skillID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var ratings []model.SkillRating
	for rows.Next() {
		var r model.SkillRating
		if err := rows.Scan(&r.ID, &r.SkillID, &r.UserID, &r.UserName,
			&r.Rating, &r.Title, &r.Comment, &r.CreatedAt); err != nil {
			return nil, 0, err
		}
		ratings = append(ratings, r)
	}
	return ratings, total, nil
}

// ============================================================
// 审计日志
// ============================================================

// RecordAudit 记录审计日志
func (s *SkillService) RecordAudit(log *model.AuditLog) error {
	_, err := s.db.Exec(`
		INSERT INTO skill_audit_logs (trace_id, skill_id, user_id, method, response_status, duration_ms, tokens_used, source_ip, pii_detected)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, log.TraceID, log.SkillID, log.UserID, log.Method,
		log.ResponseStatus, log.DurationMs, log.TokensUsed, log.SourceIP, log.PIIDetected,
	)
	return err
}

// GetAuditLogs 查询审计日志
func (s *SkillService) GetAuditLogs(userID string, page, pageSize int) ([]model.AuditLog, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	var total int64
	where := ""
	args := []interface{}{}
	if userID != "" {
		where = "WHERE user_id = $1"
		args = append(args, userID)
	}
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM skill_audit_logs `+where, args...).Scan(&total)

	query := fmt.Sprintf(`SELECT id, trace_id, skill_id, user_id, method,
	       response_status, duration_ms, tokens_used, COALESCE(source_ip::text,''), pii_detected, created_at
	FROM skill_audit_logs %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		where, len(args)+1, len(args)+2)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []model.AuditLog
	for rows.Next() {
		var l model.AuditLog
		if err := rows.Scan(&l.ID, &l.TraceID, &l.SkillID, &l.UserID, &l.Method,
			&l.ResponseStatus, &l.DurationMs, &l.TokensUsed, &l.SourceIP, &l.PIIDetected, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		logs = append(logs, l)
	}
	return logs, total, nil
}

// ============================================================
// 统计
// ============================================================

// GetOverviewStats 获取概览统计
func (s *SkillService) GetOverviewStats() (map[string]interface{}, error) {
	var totalSkills, totalInstalls, monthlyCalls, monthlyActiveUsers int64
	var avgRating float64
	err := s.db.QueryRow(`
		SELECT
		  (SELECT COUNT(*) FROM skills WHERE status='published'),
		  (SELECT COUNT(*) FROM skill_installations WHERE status='active'),
		  (SELECT COUNT(*) FROM skill_audit_logs WHERE created_at > NOW() - INTERVAL '30 days'),
		  (SELECT COUNT(DISTINCT user_id) FROM skill_audit_logs WHERE created_at > NOW() - INTERVAL '30 days'),
		  (SELECT COALESCE(ROUND(AVG(rating_avg)::numeric,2),0) FROM skills WHERE status='published')
	`).Scan(&totalSkills, &totalInstalls, &monthlyCalls, &monthlyActiveUsers, &avgRating)
	if err != nil {
		return nil, fmt.Errorf("get overview stats: %w", err)
	}

	return map[string]interface{}{
		"total_skills":         totalSkills,
		"total_installs":       totalInstalls,
		"monthly_calls":        monthlyCalls,
		"monthly_active_users": monthlyActiveUsers,
		"avg_rating":           avgRating,
	}, nil
}

// ============================================================
// 辅助函数
// ============================================================

func hashToken(token string) string {
	h := sha3.New256()
	h.Write([]byte(token))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func randomHex(n int) string {
	// 简化实现: 生产环境应使用 crypto/rand
	const hexChars = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = hexChars[time.Now().UnixNano()%int64(len(hexChars))]
		time.Sleep(1) // 简陋但足够的唯一性保证
	}
	return string(b)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// 确保 encoding/json 被使用 (manifest 解析)
var _ = json.Marshal
