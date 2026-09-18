package db

import "database/sql"

// EnsureAuditSchema 幂等创建「技能可用性审核」相关结构。
// 说明: docker-entrypoint-initdb.d 只在数据卷首次初始化时执行,
// 已存在的库不会再跑 00x 迁移脚本, 因此这里用 IF NOT EXISTS 语句在启动时补齐。
func EnsureAuditSchema(d *sql.DB) int {
	applied := 0
	for _, stmt := range auditSchemaStatements {
		if _, err := d.Exec(stmt); err != nil {
			// 交由调用方打印告警, 不阻塞服务启动
			continue
		}
		applied++
	}
	return applied
}

// auditSchemaStatements 全部语句均为幂等 (IF NOT EXISTS / 重复执行安全)
var auditSchemaStatements = []string{
	// ---------- 技能可用性审核记录 ----------
	`CREATE TABLE IF NOT EXISTS skill_audits (
		id                VARCHAR(64) PRIMARY KEY DEFAULT uuid_generate_v4()::text,
		skill_id          VARCHAR(64) NOT NULL REFERENCES skills(id),
		skill_key         VARCHAR(256) NOT NULL,
		version           VARCHAR(32) NOT NULL DEFAULT '',
		trigger_type      VARCHAR(32) NOT NULL DEFAULT 'manual',
		triggered_by      VARCHAR(64),
		status            VARCHAR(32) NOT NULL DEFAULT 'running',
		score             DECIMAL(5,2) DEFAULT 0,
		grade             VARCHAR(8),
		total_checks      INTEGER DEFAULT 0,
		passed_checks     INTEGER DEFAULT 0,
		critical_failures INTEGER DEFAULT 0,
		duration_ms       INTEGER DEFAULT 0,
		checks            JSONB NOT NULL DEFAULT '[]',
		summary           TEXT,
		engine_version    VARCHAR(32) DEFAULT '1.0.0',
		created_at        TIMESTAMP WITH TIME ZONE DEFAULT NOW()
	)`,

	`CREATE INDEX IF NOT EXISTS idx_skill_audits_skill ON skill_audits(skill_id, created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_skill_audits_status ON skill_audits(status)`,
	`CREATE INDEX IF NOT EXISTS idx_skill_audits_created ON skill_audits(created_at DESC)`,

	// ---------- skills 表补齐审核态字段 ----------
	`ALTER TABLE skills ADD COLUMN IF NOT EXISTS audit_status VARCHAR(32) DEFAULT 'pending'`,
	`ALTER TABLE skills ADD COLUMN IF NOT EXISTS audit_score DECIMAL(5,2)`,
	`ALTER TABLE skills ADD COLUMN IF NOT EXISTS last_audit_at TIMESTAMP WITH TIME ZONE`,

	`CREATE INDEX IF NOT EXISTS idx_skills_audit_status ON skills(audit_status)`,
}
