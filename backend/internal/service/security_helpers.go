package service

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jjbank/skill-market/internal/model"
	sec "github.com/jjbank/skill-market/internal/security"
)

// ---------- 引擎元信息 / 策略 (供前端与审计展示) ----------

// SecurityRuleDocs 规则说明
func SecurityRuleDocs() []model.SecurityRuleDoc { return sec.Rules() }

// SecurityEngineMeta 引擎元信息
func SecurityEngineMeta() map[string]interface{} {
	meta := sec.RulesMeta()
	meta["version"] = sec.EngineVersion()
	meta["dynamic_sandbox"] = SandboxEngineMeta()
	return meta
}

// SecurityPolicy 平台安全策略 (对外可审计)
func SecurityPolicy() map[string]interface{} {
	return map[string]interface{}{
		"github_import": map[string]interface{}{
			"mode":             "review_before_download",
			"description":      "GitHub 外部技能必须先提交安全审查, 通过后才允许下载",
			"block_on":         "任何 critical(严重) 风险项",
			"manual_review_on": []string{"verdict=suspicious", "similarity>=0.75 (疑似盗用)"},
		},
		"upload": map[string]interface{}{
			"mode":        "scan_before_accept",
			"description": "上传/上架外部技能包前先做静态安全预检 (查毒 + 危险行为 + 提示注入 + 查重)",
			"max_size_mb": uploadMaxBytes >> 20,
			"api":         "POST /api/v1/admin/security/scan-package",
		},
		"install": map[string]interface{}{
			"require_signed_manifest": true,
			"description":             "交付包必须携带 HMAC-SHA256 签名清单, 验签失败不可安装",
		},
		"dynamic_sandbox": map[string]interface{}{
			"mode":        "behaviour_verification",
			"description": "可执行技能在隔离容器(无外网/只读根/非 root/cap-drop ALL/限额)中真跑一次, 观测外联、命令执行、越权写文件、凭据读取、持久化",
			"rules":       "DYN-01 ~ DYN-08",
			"api":         SandboxEngineMeta(),
		},
		"ai_semantic_audit": map[string]interface{}{
			"mode":        "intent_and_social_engineering",
			"description": "对文档/代码做语义审计: 识别社工话术(冒充权威/制造紧迫/情感诱导/隐瞒目的/索取凭据/规避审查) 并对撞宣称意图与真实意图",
			"rules":       "SEM-01 ~ SEM-07",
			"engine":      (&SemanticAuditService{}).Status(),
		},
		"provenance": map[string]interface{}{
			"watermark":   "每个技能与每次交付均带唯一水印, 泄漏可追溯到下载人",
			"fingerprint": "内容指纹 + SimHash 相似度, 改名重传同样会被识别",
		},
		"key_management": sec.KeyMeta(),
		"av_engines":     sec.AVSummary(sec.ExternalAVStatus()),
	}
}

// SecurityRulesExport 导出规则库 (审计/备份)
func SecurityRulesExport() ([]byte, error) { return sec.MarshalRulesFile() }

// DefenseStatus 三道防线总体状态 (供前端与审计展示)
func (s *SecurityService) DefenseStatus() map[string]interface{} {
	sandbox := SandboxEngineMeta()
	sandboxMode := "active"
	if enabled, ok := sandbox["enabled"].(bool); ok && !enabled {
		sandboxMode = "disabled"
	} else if reachable, ok := sandbox["reachable"].(bool); ok && !reachable {
		sandboxMode = "unreachable"
	}
	semantic := map[string]interface{}{}
	if s != nil && s.semantic != nil {
		semantic = s.semantic.Status()
	} else {
		semantic = (&SemanticAuditService{}).Status()
	}
	return map[string]interface{}{
		"engine_version": sec.EngineVersion(),
		"defense_in_depth": true,
		"lines": []map[string]interface{}{
			{
				"line": 1, "key": "static",
				"name":        "代码级检测 (静态分析 + AST/能力语义分析)",
				"engine":      sec.SemanticEngine(),
				"rules":       "AST-01 ~ AST-08 (语义) + 正则规则库",
				"rule_count":  sec.RuleCount(),
				"mode":        "active",
				"description": "去注释/合并拼接/解码 base64·hex·字符码后重新语义扫描; 以能力(执行/外联/读凭据/破坏/持久化/反沙箱)而非关键词判定",
				"capabilities": []string{"危险执行", "网络外联", "凭据读取", "破坏性操作", "持久化", "编码载荷", "反沙箱"},
			},
			{
				"line": 2, "key": "sandbox",
				"name":        "动态沙箱验证 (隔离环境 + 全链路监控)",
				"engine":      sec.SandboxEngine(),
				"rules":       "DYN-01 ~ DYN-08",
				"mode":        sandboxMode,
				"description": "无外网出口 / 只读根 / 非 root / cap-drop ALL / 资源限额 的隔离容器中真跑一次, 全链路监控行为",
				"channels":    []string{"文件系统", "网络", "进程", "凭据", "持久化", "常规文件"},
				"runtime":     sandbox,
			},
			{
				"line": 3, "key": "semantic",
				"name":        "AI 语义审计 (社工话术 + 意图深度分析)",
				"engine":      semanticAuditEngine,
				"rules":       "SEM-01 ~ SEM-07",
				"mode":        semantic["mode"],
				"description": "识别社工话术, 并对撞「文档宣称意图」与「代码/行为真实意图」, 判定隐藏目的",
				"engine_status": semantic,
			},
		},
	}
}

// ---------- 安全徽章 ----------

// Badge 技能安全态摘要
func (s *SecurityService) Badge(ctx context.Context, skillID string) (*model.SecurityBadge, error) {
	badge := &model.SecurityBadge{SkillID: skillID, Verdict: "unscanned", VerdictCN: "未扫描", SecurityStatus: "unscanned"}
	var lastScan sql.NullTime
	var scanID sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(security_status,'unscanned'), COALESCE(security_score,0), COALESCE(security_verdict,''),
		       COALESCE(quarantined,FALSE), last_security_scan_at
		FROM skills WHERE id=$1`, skillID).
		Scan(&badge.SecurityStatus, &badge.RiskScore, &badge.Verdict, &badge.Blocked, &lastScan)
	if err != nil {
		return nil, fmt.Errorf("技能不存在")
	}
	badge.VerdictCN = verdictCN(badge.Verdict)
	if lastScan.Valid {
		t := lastScan.Time
		badge.LastScanAt = &t
	}
	_ = s.db.QueryRowContext(ctx, `
		SELECT id, grade, critical_count, summary FROM skill_security_scans
		WHERE skill_id=$1 ORDER BY created_at DESC LIMIT 1`, skillID).
		Scan(&scanID, &badge.Grade, &badge.CriticalCount, &badge.Summary)
	if scanID.Valid {
		badge.LastScanID = scanID.String
	}
	var provenance model.SkillProvenance
	err = s.db.QueryRowContext(ctx, `
		SELECT COALESCE(watermark_id,''), COALESCE(owner_id,''), download_count, signed_packages, reupload_alerts
		FROM skill_provenance WHERE skill_id=$1`, skillID).
		Scan(&provenance.WatermarkID, &provenance.OwnerID, &badge.Downloads, &badge.SignedPackages, &badge.ReuploadAlerts)
	if err == nil {
		badge.WatermarkID = provenance.WatermarkID
		badge.OwnerID = provenance.OwnerID
	}
	return badge, nil
}

// EnsureProvenanceByScan 若尚无溯源档案, 先扫描一次再生成
func (s *SecurityService) EnsureProvenanceByScan(ctx context.Context, skillID string) (*model.SkillProvenance, error) {
	if _, err := s.ScanSkill(ctx, skillID, "provenance", "", ""); err != nil {
		return nil, err
	}
	return s.Provenance(ctx, skillID)
}

// ---------- 交付包签名 (防盗用: 水印 + 清单 + 签名) ----------

// ArchiveMeta 交付包元信息
type ArchiveMeta struct {
	SkillKey  string
	SkillName string
	Version   string
	OwnerID   string
	License   string
	Watermark string
	Delivery  string
	Source    string
	Verdict   string
	RiskScore int
	Channel   string
	SkillID   string
	UserID    string
}

// SignArchive 为交付包注入签名清单与溯源水印, 返回新的压缩包与清单
func (s *SecurityService) SignArchive(zipBytes []byte, meta ArchiveMeta) ([]byte, sec.PackageManifest, error) {
	reader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, sec.PackageManifest{}, fmt.Errorf("压缩包解析失败: %w", err)
	}
	files := make([]model.SkillFile, 0, len(reader.File))
	for _, f := range reader.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(rc, 4<<20))
		rc.Close()
		if err != nil {
			continue
		}
		item := model.SkillFile{Path: f.Name, Size: int64(len(data)), Content: data}
		if sec.IsScannableFile(f.Name) {
			item.Text = string(data)
		}
		files = append(files, item)
	}
	if len(files) == 0 {
		return nil, sec.PackageManifest{}, fmt.Errorf("压缩包为空")
	}
	issued := time.Now().UTC().Format(time.RFC3339)
	if meta.License == "" {
		meta.License = "internal-confidential"
	}
	manifest := sec.BuildManifest(meta.SkillKey, meta.SkillName, meta.Version, meta.OwnerID, meta.License,
		meta.Watermark, meta.Delivery, issued, meta.Source, meta.Verdict, meta.RiskScore, files)

	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for _, f := range reader.File {
		w, err := writer.Create(f.Name)
		if err != nil {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		_, _ = io.Copy(w, rc)
		rc.Close()
	}
	manifestWriter, err := writer.Create(sec.ManifestFileName())
	if err != nil {
		return nil, manifest, err
	}
	_, _ = manifestWriter.Write(sec.MarshalManifest(manifest))
	watermarkWriter, err := writer.Create(sec.WatermarkFileName())
	if err != nil {
		return nil, manifest, err
	}
	_, _ = watermarkWriter.Write(sec.WatermarkDocument(manifest))
	if err := writer.Close(); err != nil {
		return nil, manifest, err
	}
	return out.Bytes(), manifest, nil
}

// ---------- 上传技能包安全预检 (入库/上架前的第一道闸门) ----------

const (
	uploadMaxBytes = 24 << 20
	uploadMaxFiles = 200
	uploadMaxFile  = 4 << 20
)

// PackageFilesFromBytes 把上传内容解析为待审文件集 (zip 或单文件)
func PackageFilesFromBytes(filename string, data []byte) ([]model.SkillFile, error) {
	if bytes.HasPrefix(data, []byte("PK\x03\x04")) || strings.HasSuffix(strings.ToLower(filename), ".zip") {
		reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, fmt.Errorf("压缩包解析失败: %w", err)
		}
		files := []model.SkillFile{}
		for _, f := range reader.File {
			if f.FileInfo().IsDir() {
				continue
			}
			if len(files) >= uploadMaxFiles {
				break
			}
			rc, err := f.Open()
			if err != nil {
				continue
			}
			content, err := io.ReadAll(io.LimitReader(rc, uploadMaxFile))
			rc.Close()
			if err != nil {
				continue
			}
			// 注意: 故意保留原始路径, 以便检出 Zip Slip 路径穿越
			item := model.SkillFile{Path: f.Name, Size: int64(len(content)), Content: content}
			if sec.IsScannableFile(f.Name) && utf8.Valid(content) {
				item.Text = string(content)
			} else if !utf8.Valid(content) {
				item.Skipped = true
			}
			files = append(files, item)
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("压缩包内没有可审查文件")
		}
		return files, nil
	}
	name := filename
	if strings.TrimSpace(name) == "" {
		name = "uploaded-file"
	}
	item := model.SkillFile{Path: name, Size: int64(len(data)), Content: data}
	if sec.IsScannableFile(name) && utf8.Valid(data) {
		item.Text = string(data)
	} else if !utf8.Valid(data) {
		item.Skipped = true
	}
	return []model.SkillFile{item}, nil
}

// ScanUploadedPackage 上传包安全预检: 查毒 + 危险行为 + 提示注入 + 查重
func (s *SecurityService) ScanUploadedPackage(ctx context.Context, filename string, data []byte, userID, userName string) (*model.SecurityScan, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("上传内容为空")
	}
	if len(data) > uploadMaxBytes {
		return nil, fmt.Errorf("上传包过大 (上限 %d MB)", uploadMaxBytes>>20)
	}
	files, err := PackageFilesFromBytes(filename, data)
	if err != nil {
		return nil, err
	}
	subject := sec.ScanSubject{Type: "package", SkillKey: filename, SkillName: filename,
		Target: "upload://" + filename, Trigger: "upload", TriggerBy: userID, TriggerNam: userName}
	return s.runScan(ctx, subject, files)
}

// SemanticAuditSkill 对已入库技能单独跑一次第三道防线 (AI 语义审计)
func (s *SecurityService) SemanticAuditSkill(ctx context.Context, skillID string) (*model.SemanticAuditReport, error) {
	files, _, err := s.SkillFiles(skillID, "")
	if err != nil {
		return nil, err
	}
	_, facts := sec.AnalyzeSemantics(files)
	subject := sec.ScanSubject{Type: "skill", SkillID: skillID, Trigger: "semantic"}
	return s.semantic.Audit(ctx, subject, files, facts), nil
}

// SemanticAuditText 对一段自由文本跑 AI 语义审计 (在线演示/即席审查)
func (s *SecurityService) SemanticAuditText(ctx context.Context, text string) (*model.SemanticAuditReport, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("请输入待审计文本")
	}
	if len(text) > 20000 {
		text = text[:20000]
	}
	files := []model.SkillFile{{Path: "pasted-text.md", Size: int64(len(text)), Content: []byte(text), Text: text}}
	subject := sec.ScanSubject{Type: "text", SkillName: "即席文本审计", Trigger: "semantic"}
	return s.semantic.Audit(ctx, subject, files, nil), nil
}

// EnsureProvenanceSignature 用已有清单重算签名 (安装前验签用)
func (s *SecurityService) VerifyArchiveSignature(zipBytes []byte, manifestRaw []byte) (bool, string) {
	var manifest sec.PackageManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return false, "清单解析失败"
	}
	reader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return false, "压缩包解析失败"
	}
	files := []model.SkillFile{}
	for _, f := range reader.File {
		if f.FileInfo().IsDir() || f.Name == sec.ManifestFileName() || f.Name == sec.WatermarkFileName() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(io.LimitReader(rc, 4<<20))
		rc.Close()
		files = append(files, model.SkillFile{Path: f.Name, Size: int64(len(data)), Content: data})
	}
	return sec.VerifyManifest(manifest, files, sec.SigningKey())
}
