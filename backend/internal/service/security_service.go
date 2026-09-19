package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jjbank/skill-market/internal/config"
	"github.com/jjbank/skill-market/internal/model"
	sec "github.com/jjbank/skill-market/internal/security"
)

// ============================================================
// 技能安全治理服务
//   1) 扫描:   进平台前 / 发布前 / 安装前 都过静态安检 (查毒+危险行为+提示注入+供应链)
//   2) 溯源:   指纹查重防抄袭、水印绑定、HMAC 签名防篡改、下载审计可追责
//   3) 门禁:   GitHub 技能必须「先审核、后下载」, 高危一律阻断
// ============================================================

// SecurityService 安全治理
type SecurityService struct {
	db              *sql.DB
	skillsDir       string
	importsDir      string
	autoApprove     bool
	blockOnCritical bool
	simThreshold    float64
	sandbox         *SandboxClient
	semantic        *SemanticAuditService
}

// NewSecurityService 构建服务
func NewSecurityService(db *sql.DB, cfg *config.Config) *SecurityService {
	skillsDir := strings.TrimSpace(os.Getenv("SEED_SKILLS_DIR"))
	if skillsDir == "" {
		skillsDir = filepath.Join("..", "seed-skills")
	}
	importsDir := strings.TrimSpace(os.Getenv("SKILLHUB_IMPORTS_DIR"))
	if importsDir == "" {
		importsDir = filepath.Join("..", "data", "imports")
	}
	auto := os.Getenv("IMPORT_AUTO_APPROVE")
	return &SecurityService{
		db:              db,
		skillsDir:       skillsDir,
		importsDir:      importsDir,
		autoApprove:     auto == "" || auto == "1" || strings.EqualFold(auto, "true"),
		blockOnCritical: true,
		simThreshold:    0.75,
		sandbox:         NewSandboxClient(),
		semantic:        NewSemanticAuditService(cfg),
	}
}

// AutoApprove 是否开启「安全即自动放行」策略
func (s *SecurityService) AutoApprove() bool { return s.autoApprove }

// ---------- 文件装载 ----------

// ReadDirFiles 读取目录下参与审查的文件
func ReadDirFiles(root string) ([]model.SkillFile, error) {
	var files []model.SkillFile
	var total int64
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "__pycache__" || name == ".venv" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		f := model.SkillFile{Path: rel, Size: info.Size()}
		if info.Size() <= 2<<20 {
			if data, err := os.ReadFile(p); err == nil {
				f.Content = data
				if utf8.Valid(data) {
					f.Text = string(data)
				} else {
					f.Skipped = true
				}
			}
		} else {
			f.Skipped = true
		}
		total += info.Size()
		if total > 16<<20 {
			return filepath.SkipDir
		}
		files = append(files, f)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// SkillFiles 读取某个技能的待审文件 (先查导入区, 再查本地技能区)
func (s *SecurityService) SkillFiles(skillID, skillKey string) ([]model.SkillFile, string, error) {
	candidates := []string{
		filepath.Join(s.importsDir, skillID, skillKey),
		filepath.Join(s.importsDir, skillKey),
		filepath.Join(s.skillsDir, skillKey),
	}
	for _, dir := range candidates {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			files, err := ReadDirFiles(dir)
			if err != nil {
				return nil, "", err
			}
			return files, dir, nil
		}
	}
	return nil, "", fmt.Errorf("技能 %s 的包文件不存在 (已查询: %s)", skillKey, strings.Join(candidates, ", "))
}

// ---------- 扫描 ----------

// ScanSkill 扫描已入库技能并落库
func (s *SecurityService) ScanSkill(ctx context.Context, skillID, trigger, by, byName string) (*model.SecurityScan, error) {
	var key, name, version string
	if err := s.db.QueryRowContext(ctx, `SELECT skill_key, name, COALESCE(version,'') FROM skills WHERE id=$1`, skillID).
		Scan(&key, &name, &version); err != nil {
		return nil, fmt.Errorf("技能不存在: %w", err)
	}
	files, dir, err := s.SkillFiles(skillID, key)
	if err != nil {
		return nil, err
	}
	subject := sec.ScanSubject{Type: "skill", SkillID: skillID, SkillKey: key, SkillName: name,
		Version: version, Target: dir, Trigger: trigger, TriggerBy: by, TriggerNam: byName}
	return s.runScan(ctx, subject, files)
}

// runScan 执行扫描 + 查重 + 落库 + 回写技能状态
func (s *SecurityService) runScan(ctx context.Context, subject sec.ScanSubject, files []model.SkillFile) (*model.SecurityScan, error) {
	if !sec.RulesLoaded() {
		return nil, fmt.Errorf("安全规则库未装载, 拒绝在无规则状态下放行 (fail-closed)")
	}
	scan := sec.ScanFiles(subject, files)
	scan.ContentHash = sec.ContentHash(files)
	scan.SimHashHex = sec.SimHashHex(files)
	scan.EngineVersion = sec.EngineVersion()

	// 查重 (防盗用)
	if report := s.detectReupload(ctx, files, subject.SkillID, subject.OwnerID); report != nil && report.Suspected {
		scan.Reupload = report
		scan.ReuploadSuspected = true
		scan.Findings = append(scan.Findings, model.SecurityFinding{
			RuleID: "IP-01", Category: "compliance", CategoryCN: "合规一致性", Severity: "high",
			Title: "疑似重复上传他人技能", Detail: report.Reason, File: "", Blocking: false, Score: 18,
		})
		scan.FindingCount = len(scan.Findings)
		scan.HighCount++
		if scan.Verdict == "safe" {
			scan.Verdict, scan.VerdictCN = "suspicious", "可疑待核"
		}
		if scan.RiskScore < 45 {
			scan.RiskScore = 45
		}
		if scan.Grade == "A" || scan.Grade == "B" {
			scan.Grade = "C"
		}
		scan.Summary = fmt.Sprintf("命中与已有技能 %s 的高度相似内容 (相似度 %.0f%%), 需人工确认原创性; %s",
			report.MatchedName, report.Similarity*100, scan.Summary)
	}

	// 动态沙箱验证 (静态查毒之外的第二道): 在无外网/只读根/非 root 的隔离容器里
	// 真跑一次, 观测是否外联、执行命令、越权写文件、窃取凭据、写持久化项。
	if s.sandbox.Enabled() {
		if report := s.sandbox.Verify(ctx, subject.SkillKey, files); report != nil {
			scan.Sandbox = report
			if len(report.Findings) > 0 {
				sec.MergeFindings(scan, report.Findings)
			}
		}
	}

	// 第三道防线 · AI 语义审计: 识别社工话术 + 文档意图与代码能力的深度对撞
	if s.semantic != nil {
		report := s.semantic.Audit(ctx, subject, files, scan.Facts)
		scan.Semantic = report
		if len(report.Findings) > 0 {
			sec.MergeFindings(scan, report.Findings)
		}
	}

	if err := s.persistScan(ctx, scan); err != nil {
		return nil, err
	}
	if subject.SkillID != "" {
		_ = s.applySkillSecurityState(ctx, subject.SkillID, scan)
		_ = s.touchProvenance(ctx, subject.SkillID, subject.SkillKey, subject.OwnerID, files, scan)
	}
	return scan, nil
}

func (s *SecurityService) detectReupload(ctx context.Context, files []model.SkillFile, excludeSkillID, ownerID string) *model.ReuploadReport {
	rows, err := s.db.QueryContext(ctx, `SELECT skill_id, skill_key, name, author_id FROM skills WHERE id <> $1 AND status <> 'archived'`, excludeSkillID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	best := 0.0
	var report *model.ReuploadReport
	for rows.Next() {
		var id, key, name string
		var author sql.NullString
		if err := rows.Scan(&id, &key, &name, &author); err != nil {
			continue
		}
		other, _, err := s.SkillFiles(id, key)
		if err != nil || len(other) == 0 {
			continue
		}
		sim := sec.Similarity(files, other)
		if sim > best {
			best = sim
			report = &model.ReuploadReport{Suspected: sim >= s.simThreshold, Similarity: sim,
				MatchedSkill: id, MatchedName: name, MatchedOwner: author.String,
				Reason: fmt.Sprintf("与技能「%s」内容相似度 %.0f%% (阈值 %.0f%%), 疑似复制他人作品改名上传",
					name, sim*100, s.simThreshold*100)}
		}
	}
	if report != nil && ownerID != "" && report.MatchedOwner == ownerID {
		report.Suspected = false
	}
	return report
}

// ---------- 落库 ----------

func (s *SecurityService) persistScan(ctx context.Context, scan *model.SecurityScan) error {
	findings, _ := json.Marshal(scan.Findings)
	reupload, _ := json.Marshal(scan.Reupload)
	sandbox := sql.NullString{}
	if scan.Sandbox != nil {
		if raw, err := json.Marshal(scan.Sandbox); err == nil {
			sandbox = sql.NullString{String: string(raw), Valid: true}
		}
	}
	semantic := sql.NullString{}
	if scan.Semantic != nil {
		if raw, err := json.Marshal(scan.Semantic); err == nil {
			semantic = sql.NullString{String: string(raw), Valid: true}
		}
	}
	facts := sql.NullString{}
	if scan.Facts != nil {
		if raw, err := json.Marshal(scan.Facts); err == nil {
			facts = sql.NullString{String: string(raw), Valid: true}
		}
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO skill_security_scans (id, subject_type, skill_id, skill_key, skill_name, version, target,
			verdict, verdict_cn, risk_score, grade, findings, finding_count, critical_count, high_count,
			files_scanned, bytes_scanned, duration_ms, engine_version, content_hash, sim_hash, trigger_type,
			triggered_by, summary, reupload, reupload_suspected, sandbox, semantic, semantic_facts)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29)`,
		scan.ID, scan.SubjectType, nullIfEmpty(scan.SkillID), nullIfEmpty(scan.SkillKey), nullIfEmpty(scan.SkillName),
		nullIfEmpty(scan.Version), nullIfEmpty(scan.Target), scan.Verdict, scan.VerdictCN, scan.RiskScore, scan.Grade,
		string(findings), scan.FindingCount, scan.CriticalCount, scan.HighCount, scan.FilesScanned, scan.BytesScanned,
		scan.DurationMs, scan.EngineVersion, scan.ContentHash, scan.SimHashHex, scan.TriggerType,
		nullIfEmpty(scan.TriggeredBy), scan.Summary, string(reupload), scan.ReuploadSuspected, sandbox, semantic, facts)
	return err
}

func (s *SecurityService) applySkillSecurityState(ctx context.Context, skillID string, scan *model.SecurityScan) error {
	status := "safe"
	switch scan.Verdict {
	case "malicious":
		status = "malicious"
	case "suspicious":
		status = "suspicious"
	}
	quarantine := scan.CriticalCount > 0 && s.blockOnCritical
	_, err := s.db.ExecContext(ctx, `
		UPDATE skills SET security_status=$2, security_score=$3, security_verdict=$4,
			last_security_scan_at=NOW(), quarantined=$5, updated_at=NOW() WHERE id=$1`,
		skillID, status, scan.RiskScore, scan.Verdict, quarantine)
	return err
}

// ScanByID 查询单次扫描
func (s *SecurityService) ScanByID(ctx context.Context, scanID string) (*model.SecurityScan, error) {
	scan := &model.SecurityScan{}
	var findings, reupload, sandbox, semantic, facts sql.NullString
	var skillID, skillKey, skillName, version, target, triggerBy sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, subject_type, skill_id, skill_key, skill_name, version, target, verdict, verdict_cn,
		       risk_score, grade, findings, finding_count, critical_count, high_count, files_scanned,
		       bytes_scanned, duration_ms, engine_version, content_hash, trigger_type, triggered_by,
		       summary, created_at, reupload, sandbox, semantic, semantic_facts
		FROM skill_security_scans WHERE id=$1`, scanID).
		Scan(&scan.ID, &scan.SubjectType, &skillID, &skillKey, &skillName, &version, &target, &scan.Verdict,
			&scan.VerdictCN, &scan.RiskScore, &scan.Grade, &findings, &scan.FindingCount, &scan.CriticalCount,
			&scan.HighCount, &scan.FilesScanned, &scan.BytesScanned, &scan.DurationMs, &scan.EngineVersion,
			&scan.ContentHash, &scan.TriggerType, &triggerBy, &scan.Summary, &scan.CreatedAt, &reupload, &sandbox,
			&semantic, &facts)
	if err != nil {
		return nil, err
	}
	if sandbox.Valid && sandbox.String != "" {
		var sb model.SandboxReport
		if json.Unmarshal([]byte(sandbox.String), &sb) == nil {
			scan.Sandbox = &sb
		}
	}
	if semantic.Valid && semantic.String != "" {
		var sm model.SemanticAuditReport
		if json.Unmarshal([]byte(semantic.String), &sm) == nil {
			scan.Semantic = &sm
		}
	}
	if facts.Valid && facts.String != "" {
		var sf model.SemanticFacts
		if json.Unmarshal([]byte(facts.String), &sf) == nil {
			scan.Facts = &sf
		}
	}
	scan.SkillID, scan.SkillKey, scan.SkillName = skillID.String, skillKey.String, skillName.String
	scan.Version, scan.Target, scan.TriggeredBy = version.String, target.String, triggerBy.String
	_ = json.Unmarshal([]byte(findings.String), &scan.Findings)
	if reupload.Valid && reupload.String != "" {
		var r model.ReuploadReport
		if json.Unmarshal([]byte(reupload.String), &r) == nil {
			scan.Reupload = &r
			scan.ReuploadSuspected = r.Suspected
			scan.ReuploadSimilarity = r.Similarity
		}
	}
	return scan, nil
}

// RecentScans 最近扫描记录
func (s *SecurityService) RecentScans(ctx context.Context, limit int) ([]model.SecurityScan, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, subject_type, COALESCE(skill_id,''), COALESCE(skill_key,''), COALESCE(skill_name,''),
		       verdict, verdict_cn, risk_score, COALESCE(grade,''), finding_count, critical_count, high_count,
		       files_scanned, duration_ms, COALESCE(trigger_type,''), COALESCE(triggered_by,''),
		       COALESCE(summary,''), created_at, reupload_suspected
		FROM skill_security_scans ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.SecurityScan{}
	for rows.Next() {
		var sc model.SecurityScan
		if err := rows.Scan(&sc.ID, &sc.SubjectType, &sc.SkillID, &sc.SkillKey, &sc.SkillName, &sc.Verdict,
			&sc.VerdictCN, &sc.RiskScore, &sc.Grade, &sc.FindingCount, &sc.CriticalCount, &sc.HighCount,
			&sc.FilesScanned, &sc.DurationMs, &sc.TriggerType, &sc.TriggeredBy, &sc.Summary, &sc.CreatedAt,
			&sc.ReuploadSuspected); err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, nil
}

// ScansForSkill 某技能的历史扫描
func (s *SecurityService) ScansForSkill(ctx context.Context, skillID string, limit int) ([]model.SecurityScan, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, verdict, verdict_cn, risk_score, COALESCE(grade,''), finding_count, critical_count,
		       created_at, COALESCE(trigger_type,''), COALESCE(triggered_by,'')
		FROM skill_security_scans WHERE skill_id=$1 ORDER BY created_at DESC LIMIT $2`, skillID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.SecurityScan{}
	for rows.Next() {
		var sc model.SecurityScan
		sc.SkillID = skillID
		if err := rows.Scan(&sc.ID, &sc.Verdict, &sc.VerdictCN, &sc.RiskScore, &sc.Grade, &sc.FindingCount,
			&sc.CriticalCount, &sc.CreatedAt, &sc.TriggerType, &sc.TriggeredBy); err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, nil
}

// Overview 安全治理概览
func (s *SecurityService) Overview(ctx context.Context) (*model.SecurityOverview, error) {
	o := &model.SecurityOverview{EngineVersion: sec.EngineVersion(), RuleCount: sec.RuleCount(),
		AutoApprove: s.autoApprove, BlockOnCritical: s.blockOnCritical}
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM skills WHERE status <> 'archived'`).Scan(&o.TotalSkills)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM skills WHERE security_status IS NULL OR security_status='unscanned'`).Scan(&o.Unscanned)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM skills WHERE security_status='safe'`).Scan(&o.Safe)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM skills WHERE security_status='suspicious'`).Scan(&o.Suspicious)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM skills WHERE security_status='malicious'`).Scan(&o.Malicious)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM skills WHERE quarantined = TRUE`).Scan(&o.Blocked)
	o.Scanned = o.Safe + o.Suspicious + o.Malicious
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM skill_security_scans`).Scan(&o.TotalScans)
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(ROUND(AVG(risk_score)::numeric,1),0) FROM skill_security_scans`).Scan(&o.AvgRisk)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM skill_security_scans WHERE reupload_suspected = TRUE`).Scan(&o.ReuploadAlerts)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM skill_download_audits`).Scan(&o.Downloads)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM skill_provenance`).Scan(&o.TrackedSkills)
	for _, item := range []struct {
		status string
		dest   *int64
	}{
		{"pending", &o.ImportPending}, {"scanning", &o.ImportPending}, {"pending_review", &o.ImportPendingReview},
		{"blocked", &o.ImportBlocked}, {"approved", &o.ImportApproved}, {"rejected", &o.ImportRejected},
		{"imported", &o.ImportImported},
	} {
		var n int64
		_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM github_import_requests WHERE status=$1`, item.status).Scan(&n)
		*item.dest += n
	}
	var last sql.NullTime
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(created_at) FROM skill_security_scans`).Scan(&last); err == nil && last.Valid {
		o.LastScanAt = last.Time.Format(time.RFC3339)
	}
	return o, nil
}

// Queue 安全待办队列 (未扫描/命中高危的技能)
func (s *SecurityService) Queue(ctx context.Context, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, skill_key, name, COALESCE(version,''), COALESCE(category,''), COALESCE(status,''),
		       COALESCE(security_status,'unscanned'), COALESCE(security_score,0), COALESCE(security_verdict,''),
		       quarantined, last_security_scan_at
		FROM skills WHERE status <> 'archived'
		ORDER BY (quarantined IS TRUE) DESC, (security_status IS NULL OR security_status='unscanned') DESC, security_score DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id, key, name, version, category, status, sstatus, verdict string
		var score int
		var quarantined bool
		var lastScan sql.NullTime
		if err := rows.Scan(&id, &key, &name, &version, &category, &status, &sstatus, &score, &verdict, &quarantined, &lastScan); err != nil {
			return nil, err
		}
		item := map[string]interface{}{"skill_id": id, "skill_key": key, "name": name, "version": version,
			"category": category, "status": status, "security_status": sstatus, "security_score": score,
			"security_verdict": verdict, "quarantined": quarantined}
		if lastScan.Valid {
			item["last_security_scan_at"] = lastScan.Time.Format(time.RFC3339)
		}
		out = append(out, item)
	}
	return out, nil
}

// ---------- 溯源 ----------

func (s *SecurityService) touchProvenance(ctx context.Context, skillID, skillKey, ownerID string, files []model.SkillFile, scan *model.SecurityScan) error {
	if skillID == "" {
		return nil
	}
	var existing string
	err := s.db.QueryRowContext(ctx, `SELECT watermark_id FROM skill_provenance WHERE skill_id=$1`, skillID).Scan(&existing)
	if err == nil && existing != "" {
		_, err = s.db.ExecContext(ctx, `UPDATE skill_provenance SET content_hash=$2, sim_hash=$3, verify_status='valid', updated_at=NOW() WHERE skill_id=$1`,
			skillID, scan.ContentHash, scan.SimHashHex)
		return err
	}
	issued := time.Now().Format("2006-01-02")
	watermark := sec.NewWatermark(skillKey, ownerID, issued)
	signature := sec.SignManifest(sec.PackageManifest{Schema: sec.ManifestSchema, SkillKey: skillKey,
		OwnerID: ownerID, WatermarkID: watermark, IssuedAt: issued, ContentHash: scan.ContentHash,
		SimHash: scan.SimHashHex, RiskScore: scan.RiskScore, ScanVerdict: scan.Verdict}, sec.SigningKey())
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO skill_provenance (skill_id, skill_key, owner_id, license, watermark_id, content_hash, sim_hash,
			signature, algorithm, signed_at, verify_status)
		VALUES ($1,$2,$3,'internal',$4,$5,$6,$7,$8,NOW(),'valid')
		ON CONFLICT (skill_id) DO UPDATE SET content_hash=$5, sim_hash=$6, updated_at=NOW()`, skillID, skillKey,
		ownerID, watermark, scan.ContentHash, scan.SimHashHex, signature, sec.Algorithm)
	return err
}

// Provenance 技能溯源档案
func (s *SecurityService) Provenance(ctx context.Context, skillID string) (*model.SkillProvenance, error) {
	p := &model.SkillProvenance{SkillID: skillID}
	var lastVerified sql.NullTime
	var originalOf sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(skill_key,''), COALESCE(owner_id,''), COALESCE(license,'internal'), COALESCE(watermark_id,''),
		       COALESCE(content_hash,''), COALESCE(sim_hash,''), COALESCE(signature,''), COALESCE(algorithm,''),
		       signed_at, last_verified_at, COALESCE(verify_status,'unsigned'), download_count, signed_packages,
		       reupload_alerts, original_of
		FROM skill_provenance WHERE skill_id=$1`, skillID).
		Scan(&p.SkillKey, &p.OwnerID, &p.License, &p.WatermarkID, &p.ContentHash, &p.SimHash, &p.Signature,
			&p.Algorithm, &p.SignedAt, &lastVerified, &p.VerifyStatus, &p.DownloadCount, &p.SignedPackages,
			&p.ReuploadAlerts, &originalOf)
	if err != nil {
		return nil, err
	}
	if lastVerified.Valid {
		t := lastVerified.Time
		p.LastVerifiedAt = &t
	}
	p.OriginalOf = originalOf.String
	return p, nil
}

// VerifyProvenance 校验技能包完整性 (重算指纹比对)
func (s *SecurityService) VerifyProvenance(ctx context.Context, skillID string) (*model.SkillProvenance, error) {
	p, err := s.Provenance(ctx, skillID)
	if err != nil {
		return nil, err
	}
	files, _, err := s.SkillFiles(skillID, p.SkillKey)
	if err != nil {
		return nil, err
	}
	actual := sec.ContentHash(files)
	if p.Signature == "" {
		p.VerifyStatus = "unsigned"
	} else if actual != p.ContentHash {
		p.VerifyStatus = "tampered"
	} else {
		p.VerifyStatus = "valid"
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE skill_provenance SET verify_status=$2, last_verified_at=NOW() WHERE skill_id=$1`, skillID, p.VerifyStatus)
	return p, nil
}

// Downloads 下载审计
func (s *SecurityService) Downloads(ctx context.Context, skillID string, limit int) ([]model.SkillDownload, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := `SELECT id, COALESCE(skill_id,''), COALESCE(skill_key,''), COALESCE(user_id,''), COALESCE(watermark_id,''),
	       COALESCE(manifest_sha256,''), COALESCE(source_ip,''), COALESCE(channel,''), created_at
		FROM skill_download_audits`
	args := []interface{}{}
	if skillID != "" {
		query += ` WHERE skill_id=$1`
		args = append(args, skillID)
	}
	query += fmt.Sprintf(` ORDER BY created_at DESC LIMIT %d`, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.SkillDownload{}
	for rows.Next() {
		var d model.SkillDownload
		if err := rows.Scan(&d.ID, &d.SkillID, &d.SkillKey, &d.UserID, &d.WatermarkID, &d.Manifest, &d.SourceIP, &d.Channel, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// RecordDownload 记录下载并返回本次交付水印
func (s *SecurityService) RecordDownload(ctx context.Context, skillID, skillKey, userID, watermark, manifest, ip, ua, channel string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO skill_download_audits (id, skill_id, skill_key, user_id, watermark_id, manifest_sha256, source_ip, user_agent, channel)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		newID("dl"), nullIfEmpty(skillID), nullIfEmpty(skillKey), nullIfEmpty(userID), watermark, manifest, ip, ua, channel)
	if err == nil && skillID != "" {
		_, _ = s.db.ExecContext(ctx, `UPDATE skill_provenance SET download_count=download_count+1, signed_packages=signed_packages+1 WHERE skill_id=$1`, skillID)
	}
	return err
}

// ---------- GitHub 导入门禁 ----------

// CreateImportRequest 提交导入审查单 (此时不下载任何内容)
func (s *SecurityService) CreateImportRequest(ctx context.Context, repository, ref, skillPath, skillURL, userID, userName string) (*model.ImportRequest, error) {
	repository = strings.TrimSpace(repository)
	skillPath = strings.TrimSpace(skillPath)
	if repository == "" || skillPath == "" {
		return nil, fmt.Errorf("repository 与 path 必填")
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		ref = "main"
	}
	id := newID("imp")
	expires := time.Now().Add(24 * time.Hour)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO github_import_requests (id, repository, ref, skill_path, skill_name, skill_url, requested_by, status, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'pending',$8)`,
		id, repository, ref, skillPath, "", skillURL, nullIfEmpty(userID), expires)
	if err != nil {
		return nil, err
	}
	req, err := s.ImportRequest(ctx, id)
	if err != nil {
		return nil, err
	}
	req.RequestedName = userName
	return req, nil
}

// ScanImportRequest 对审查单执行「审核」: 抓包 → 静态查毒 → 指纹查重 → 出结论
func (s *SecurityService) ScanImportRequest(ctx context.Context, reqID string, github *GitHubService) (*model.ImportRequest, error) {
	req, err := s.ImportRequest(ctx, reqID)
	if err != nil {
		return nil, err
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE github_import_requests SET status='scanning', updated_at=NOW() WHERE id=$1`, reqID)
	files, err := github.FetchSkillFiles(ctx, req.Repository, req.Ref, req.SkillPath)
	if err != nil {
		_, _ = s.db.ExecContext(ctx, `UPDATE github_import_requests SET status='blocked', verdict='error', risk_score=100,
			review_note=$2, updated_at=NOW() WHERE id=$1`, reqID, "审查失败: "+err.Error())
		return s.ImportRequest(ctx, reqID)
	}
	subject := sec.ScanSubject{Type: "import", SkillKey: req.SkillPath, SkillName: req.SkillName,
		Target: req.Repository + "@" + req.Ref + ":" + req.SkillPath, Trigger: "import",
		TriggerBy: req.RequestedBy, TriggerNam: req.RequestedName}
	scan, err := s.runScan(ctx, subject, files)
	if err != nil {
		return nil, err
	}

	status := "approved"
	note := fmt.Sprintf("静态安全审查通过 (风险分 %d, %s), 规则库 %s", scan.RiskScore, scan.VerdictCN, scan.EngineVersion)
	if scan.ReuploadSuspected && scan.Reupload != nil {
		status = "pending_review"
		note = fmt.Sprintf("查重命中相似技能「%s」(相似度 %.0f%%), 需人工确认原创性后放行",
			scan.Reupload.MatchedName, scan.Reupload.Similarity*100)
	}
	switch scan.Verdict {
	case "malicious":
		status = "blocked"
		note = fmt.Sprintf("安全审查拦截: 命中 %d 项严重风险, 已阻断下载; %s", scan.CriticalCount, scan.Summary)
	case "suspicious":
		if status == "approved" {
			status = "pending_review"
			note = fmt.Sprintf("存在中高风险项 (风险分 %d), 需人工复核后放行: %s", scan.RiskScore, scan.Summary)
		}
	}
	if status == "approved" && !s.autoApprove {
		status = "pending_review"
		note += " (平台策略: 关闭自动放行, 需人工审批)"
	}

	watermark := sec.NewWatermark(req.SkillPath, req.RequestedBy, time.Now().Format("2006-01-02"))
	manifest := sec.BuildManifest(req.SkillPath, req.SkillName, "", req.RequestedBy, "review-required",
		watermark, "", time.Now().Format(time.RFC3339), "github:"+req.Repository, scan.Verdict, scan.RiskScore, files)
	manifestBytes := sec.MarshalManifest(manifest)
	manifestSHA := sec.FileSHA256(manifestBytes)
	signature := manifest.Signature
	if status == "blocked" {
		signature = ""
	}
	findings, _ := json.Marshal(scan.Findings)
	reupload, _ := json.Marshal(scan.Reupload)

	_, err = s.db.ExecContext(ctx, `
		UPDATE github_import_requests SET status=$2, verdict=$3, risk_score=$4, grade=$5, scan_id=$6, findings=$7,
			critical_count=$8, content_hash=$9, manifest_sha256=$10, signature=$11, watermark_id=$12, reupload=$13,
			skill_name=COALESCE(NULLIF(skill_name,''), $14), updated_at=NOW() WHERE id=$1`,
		reqID, status, scan.Verdict, scan.RiskScore, scan.Grade, scan.ID, string(findings), scan.CriticalCount,
		scan.ContentHash, manifestSHA, signature, watermark, string(reupload), req.SkillName)
	if err != nil {
		return nil, err
	}
	// 人工复核项入队 (保留审查意见)
	if status == "pending_review" {
		_, _ = s.db.ExecContext(ctx, `UPDATE github_import_requests SET review_note=$2 WHERE id=$1`, reqID, note)
	}
	out, err := s.ImportRequest(ctx, reqID)
	if err != nil {
		return nil, err
	}
	out.Findings = scan.Findings
	out.Reupload = scan.Reupload
	return out, nil
}

// DecideImport 人工审批
func (s *SecurityService) DecideImport(ctx context.Context, reqID string, approve bool, note, by, byName string) (*model.ImportRequest, error) {
	req, err := s.ImportRequest(ctx, reqID)
	if err != nil {
		return nil, err
	}
	if req.Status == "blocked" && approve {
		return nil, fmt.Errorf("该审查单已被安全引擎阻断 (命中严重风险), 不允许人工放行")
	}
	if req.Status == "malicious" {
		return nil, fmt.Errorf("高危技能不允许放行")
	}
	status := "rejected"
	if approve {
		status = "approved"
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE github_import_requests SET status=$2, reviewed_by=$3, review_note=$4, reviewed_at=NOW(), updated_at=NOW() WHERE id=$1`,
		reqID, status, nullIfEmpty(by), note)
	if err != nil {
		return nil, err
	}
	out, err := s.ImportRequest(ctx, reqID)
	if err != nil {
		return nil, err
	}
	out.ReviewedName = byName
	return out, nil
}

// ImportRequest 查询审查单
func (s *SecurityService) ImportRequest(ctx context.Context, reqID string) (*model.ImportRequest, error) {
	req := &model.ImportRequest{}
	var findings, reupload, note, reviewedBy, importedSkill sql.NullString
	var reviewedAt, importedAt, expiresAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT id, repository, ref, skill_path, COALESCE(skill_name,''), COALESCE(skill_url,''),
		       COALESCE(requested_by,''), status, COALESCE(verdict,''), risk_score, COALESCE(grade,''),
		       COALESCE(scan_id,''), findings, critical_count, COALESCE(content_hash,''), COALESCE(manifest_sha256,''),
		       COALESCE(signature,''), COALESCE(watermark_id,''), reupload, reviewed_by, review_note, reviewed_at,
		       imported_skill_id, imported_at, expires_at, created_at, updated_at
		FROM github_import_requests WHERE id=$1`, reqID).
		Scan(&req.ID, &req.Repository, &req.Ref, &req.SkillPath, &req.SkillName, &req.SkillURL, &req.RequestedBy,
			&req.Status, &req.Verdict, &req.RiskScore, &req.Grade, &req.ScanID, &findings, &req.CriticalCount,
			&req.ContentHash, &req.ManifestSHA256, &req.Signature, &req.WatermarkID, &reupload, &reviewedBy,
			&note, &reviewedAt, &importedSkill, &importedAt, &expiresAt, &req.CreatedAt, &req.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(findings.String), &req.Findings)
	if reupload.Valid && reupload.String != "" && reupload.String != "null" {
		var r model.ReuploadReport
		if json.Unmarshal([]byte(reupload.String), &r) == nil {
			req.Reupload = &r
		}
	}
	req.ReviewNote, req.ReviewedBy, req.ImportedSkill = note.String, reviewedBy.String, importedSkill.String
	if reviewedAt.Valid {
		t := reviewedAt.Time
		req.ReviewedAt = &t
	}
	if importedAt.Valid {
		t := importedAt.Time
		req.ImportedAt = &t
	}
	if expiresAt.Valid {
		t := expiresAt.Time
		req.ExpiresAt = &t
	}
	req.DownloadAllowed = req.Status == "approved" || req.Status == "imported"
	req.StatusCN, req.VerdictCN = importStatusCN(req.Status), verdictCN(req.Verdict)
	return req, nil
}

// ListImportRequests 审查队列
func (s *SecurityService) ListImportRequests(ctx context.Context, status string, limit int) ([]model.ImportRequest, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := `SELECT id FROM github_import_requests`
	args := []interface{}{}
	if strings.TrimSpace(status) != "" {
		query += ` WHERE status=$1`
		args = append(args, status)
	}
	query += fmt.Sprintf(` ORDER BY created_at DESC LIMIT %d`, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	out := []model.ImportRequest{}
	for _, id := range ids {
		item, err := s.ImportRequest(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, *item)
	}
	return out, nil
}

// DownloadGate 下载门禁: 仅放行「已审批通过」的审查单
func (s *SecurityService) DownloadGate(ctx context.Context, requestID, repository, ref, skillPath string) (*model.ImportRequest, error) {
	if strings.TrimSpace(requestID) == "" {
		return nil, fmt.Errorf("该技能来自 GitHub 外部来源, 必须先提交安全审查并通过审核后才能下载 (缺少 request_id)")
	}
	req, err := s.ImportRequest(ctx, requestID)
	if err != nil {
		return nil, fmt.Errorf("审查单不存在: %w", err)
	}
	if !sameLocator(req, repository, ref, skillPath) {
		return nil, fmt.Errorf("审查单与下载目标不一致 (审查单 %s@%s:%s)", req.Repository, req.Ref, req.SkillPath)
	}
	switch req.Status {
	case "approved", "imported":
		return req, nil
	case "blocked":
		return nil, fmt.Errorf("安全审查已拦截该技能 (风险分 %d, 命中 %d 项严重风险), 禁止下载", req.RiskScore, req.CriticalCount)
	case "pending_review":
		return nil, fmt.Errorf("该技能审核结论为「待人工复核」, 请管理员在安全治理页审批后再下载")
	case "rejected":
		return nil, fmt.Errorf("该技能审查未通过, 已驳回, 禁止下载")
	default:
		return nil, fmt.Errorf("该技能尚未完成安全审查 (当前状态: %s), 请先运行审查", importStatusCN(req.Status))
	}
}

// MarkImported 标记审查单已导入 (进入企业技能库)
func (s *SecurityService) MarkImported(ctx context.Context, reqID, skillID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE github_import_requests SET status='imported', imported_skill_id=$2, imported_at=NOW(), updated_at=NOW() WHERE id=$1`, reqID, skillID)
	return err
}

// ---------- 小工具 ----------

func sameLocator(req *model.ImportRequest, repository, ref, skillPath string) bool {
	if strings.TrimSpace(repository) == "" {
		return true // 仅凭 request_id 下载 (前端预览场景)
	}
	return strings.EqualFold(strings.TrimSpace(repository), strings.TrimSpace(req.Repository)) &&
		strings.EqualFold(strings.TrimSpace(ref), strings.TrimSpace(req.Ref)) &&
		strings.TrimSpace(skillPath) == strings.TrimSpace(req.SkillPath)
}

func importStatusCN(status string) string {
	switch status {
	case "pending":
		return "待审查"
	case "scanning":
		return "审查中"
	case "pending_review":
		return "待人工复核"
	case "approved":
		return "已通过"
	case "blocked":
		return "已阻断"
	case "rejected":
		return "已驳回"
	case "imported":
		return "已入库"
	default:
		return status
	}
}

func verdictCN(v string) string {
	switch v {
	case "safe":
		return "安全"
	case "suspicious":
		return "可疑待核"
	case "malicious":
		return "高危拦截"
	default:
		return v
	}
}

func nullIfEmpty(v string) interface{} {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

func newID(prefix string) string {
	return fmt.Sprintf("%s_%x%04x", prefix, time.Now().UnixNano()&0xFFFFFFFF, time.Now().UnixNano()&0xFFFF)
}
