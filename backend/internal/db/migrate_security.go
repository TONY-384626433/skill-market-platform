package db

import "database/sql"

// EnsureSecuritySchema 幂等创建「技能安全治理」相关结构
//   - 安全扫描记录 / 溯源档案 / GitHub 导入审查单 / 下载审计
//   - skills 表补齐安全态字段 (安全徽章 / 隔离标记)
func EnsureSecuritySchema(d *sql.DB) int {
	applied := 0
	for _, stmt := range securitySchemaStatements {
		if _, err := d.Exec(stmt); err != nil {
			continue
		}
		applied++
	}
	return applied
}

var securitySchemaStatements = []string{
	// ---------- 安全扫描记录 ----------
	`CREATE TABLE IF NOT EXISTS skill_security_scans (
		id                 VARCHAR(64) PRIMARY KEY,
		subject_type       VARCHAR(32) NOT NULL DEFAULT 'skill',
		skill_id           VARCHAR(64),
		skill_key          VARCHAR(256),
		skill_name         VARCHAR(256),
		version            VARCHAR(32),
		target             TEXT,
		verdict            VARCHAR(32) NOT NULL DEFAULT 'safe',
		verdict_cn         VARCHAR(32),
		risk_score         INTEGER DEFAULT 0,
		grade              VARCHAR(8),
		findings           JSONB NOT NULL DEFAULT '[]',
		finding_count      INTEGER DEFAULT 0,
		critical_count     INTEGER DEFAULT 0,
		high_count         INTEGER DEFAULT 0,
		files_scanned      INTEGER DEFAULT 0,
		bytes_scanned      BIGINT DEFAULT 0,
		duration_ms        INTEGER DEFAULT 0,
		engine_version     VARCHAR(64),
		content_hash       VARCHAR(128),
		sim_hash           VARCHAR(32),
		trigger_type       VARCHAR(32) DEFAULT 'manual',
		triggered_by       VARCHAR(64),
		summary            TEXT,
		reupload           JSONB,
		reupload_suspected BOOLEAN DEFAULT FALSE,
		created_at         TIMESTAMP WITH TIME ZONE DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_sec_scans_skill ON skill_security_scans(skill_id, created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_sec_scans_created ON skill_security_scans(created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_sec_scans_verdict ON skill_security_scans(verdict)`,
	// 动态沙箱行为验证报告 (后续版本新增, 幂等补齐)
	`ALTER TABLE skill_security_scans ADD COLUMN IF NOT EXISTS sandbox JSONB`,

	// ---------- 溯源档案 (防盗用) ----------
	`CREATE TABLE IF NOT EXISTS skill_provenance (
		skill_id         VARCHAR(64) PRIMARY KEY,
		skill_key        VARCHAR(256),
		owner_id         VARCHAR(64),
		license          VARCHAR(64) DEFAULT 'internal',
		watermark_id     VARCHAR(64),
		content_hash     VARCHAR(128),
		sim_hash         VARCHAR(32),
		signature        TEXT,
		algorithm        VARCHAR(64) DEFAULT 'HMAC-SHA256',
		signed_at        TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		last_verified_at TIMESTAMP WITH TIME ZONE,
		verify_status    VARCHAR(32) DEFAULT 'valid',
		download_count   INTEGER DEFAULT 0,
		signed_packages  INTEGER DEFAULT 0,
		reupload_alerts  INTEGER DEFAULT 0,
		original_of      VARCHAR(64),
		updated_at       TIMESTAMP WITH TIME ZONE DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_provenance_watermark ON skill_provenance(watermark_id)`,
	`CREATE INDEX IF NOT EXISTS idx_provenance_hash ON skill_provenance(content_hash)`,

	// ---------- GitHub 导入审查单 (先审核, 后下载) ----------
	`CREATE TABLE IF NOT EXISTS github_import_requests (
		id                VARCHAR(64) PRIMARY KEY,
		repository        VARCHAR(256) NOT NULL,
		ref               VARCHAR(160),
		skill_path        TEXT,
		skill_name        VARCHAR(256),
		skill_url         TEXT,
		requested_by      VARCHAR(64),
		status            VARCHAR(32) NOT NULL DEFAULT 'pending',
		verdict           VARCHAR(32),
		risk_score        INTEGER DEFAULT 0,
		grade             VARCHAR(8),
		scan_id           VARCHAR(64),
		findings          JSONB NOT NULL DEFAULT '[]',
		critical_count    INTEGER DEFAULT 0,
		content_hash      VARCHAR(128),
		manifest_sha256   VARCHAR(128),
		signature         TEXT,
		watermark_id      VARCHAR(64),
		reupload          JSONB,
		reviewed_by       VARCHAR(64),
		review_note       TEXT,
		reviewed_at       TIMESTAMP WITH TIME ZONE,
		imported_skill_id VARCHAR(64),
		imported_at       TIMESTAMP WITH TIME ZONE,
		expires_at        TIMESTAMP WITH TIME ZONE,
		created_at        TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		updated_at        TIMESTAMP WITH TIME ZONE DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_import_status ON github_import_requests(status, created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_import_repo ON github_import_requests(repository, skill_path)`,

	// ---------- 下载审计 (泄漏可溯源) ----------
	`CREATE TABLE IF NOT EXISTS skill_download_audits (
		id              VARCHAR(64) PRIMARY KEY,
		skill_id        VARCHAR(64),
		skill_key       VARCHAR(256),
		user_id         VARCHAR(64),
		watermark_id    VARCHAR(64),
		manifest_sha256 VARCHAR(128),
		source_ip       VARCHAR(64),
		user_agent      TEXT,
		channel         VARCHAR(32),
		created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_download_skill ON skill_download_audits(skill_id, created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_download_watermark ON skill_download_audits(watermark_id)`,

	// ---------- skills 表补齐安全态 ----------
	`ALTER TABLE skills ADD COLUMN IF NOT EXISTS security_status VARCHAR(32) DEFAULT 'unscanned'`,
	`ALTER TABLE skills ADD COLUMN IF NOT EXISTS security_score INTEGER`,
	`ALTER TABLE skills ADD COLUMN IF NOT EXISTS security_verdict VARCHAR(32)`,
	`ALTER TABLE skills ADD COLUMN IF NOT EXISTS last_security_scan_at TIMESTAMP WITH TIME ZONE`,
	`ALTER TABLE skills ADD COLUMN IF NOT EXISTS quarantined BOOLEAN DEFAULT FALSE`,
	`CREATE INDEX IF NOT EXISTS idx_skills_security ON skills(security_status)`,
}
