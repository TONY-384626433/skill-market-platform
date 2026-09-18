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
