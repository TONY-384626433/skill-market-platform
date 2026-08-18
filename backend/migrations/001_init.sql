-- ============================================================
-- Skill 市场平台 — 数据库初始化脚本
-- 数据库: PostgreSQL 15
-- 执行时机: Docker Compose 首次启动时自动运行
-- ============================================================

-- 扩展
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";          -- 模糊搜索

-- ============================================================
-- 用户与团队
-- ============================================================

CREATE TABLE users (
    id              VARCHAR(64) PRIMARY KEY DEFAULT uuid_generate_v4()::text,
    username        VARCHAR(128) NOT NULL UNIQUE,
    display_name    VARCHAR(256) NOT NULL,
    email           VARCHAR(256),
    phone           VARCHAR(32),
	password_hash   TEXT,
    department      VARCHAR(256),
    role            VARCHAR(64) NOT NULL DEFAULT 'user',  -- user / developer / admin
    avatar_url      VARCHAR(512),
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE teams (
    id              VARCHAR(64) PRIMARY KEY DEFAULT uuid_generate_v4()::text,
    name            VARCHAR(256) NOT NULL,
    department      VARCHAR(256),
    description     TEXT,
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE team_members (
    team_id         VARCHAR(64) NOT NULL REFERENCES teams(id),
    user_id         VARCHAR(64) NOT NULL REFERENCES users(id),
    role            VARCHAR(64) DEFAULT 'member',  -- owner / admin / member
    joined_at       TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (team_id, user_id)
);

-- ============================================================
-- 技能 (核心实体)
-- ============================================================

CREATE TABLE skills (
    id              VARCHAR(64) PRIMARY KEY DEFAULT uuid_generate_v4()::text,
    skill_key       VARCHAR(256) NOT NULL,                -- 唯一标识 key, 如 "db-inspection"
    name            VARCHAR(256) NOT NULL,                -- 显示名称
    version         VARCHAR(32) NOT NULL DEFAULT '0.1.0',
    category        VARCHAR(64) NOT NULL,                -- 分类
    sub_category    VARCHAR(64),
    tags            TEXT[] DEFAULT '{}',                  -- 标签数组

    -- 描述信息
    summary         VARCHAR(512) NOT NULL,                -- 一句话描述
    description     TEXT,                                 -- 详细描述
    icon_url        VARCHAR(512),

    -- 技能形态
    skill_type      VARCHAR(64) NOT NULL DEFAULT 'mcp',   -- mcp / dify / api / agent / prompt
    endpoint_url    VARCHAR(512),                         -- 接入地址
    endpoint_protocol VARCHAR(64) DEFAULT 'mcp',          -- mcp / http / grpc

    -- 完整 Manifest (JSON)
    manifest        JSONB NOT NULL DEFAULT '{}',

    -- 依赖声明 (JSON)
    dependencies    JSONB DEFAULT '{}',

    -- 权限声明 (JSON)
    permissions     JSONB DEFAULT '[]',

    -- 接口定义 (JSON)
    interface_spec  JSONB DEFAULT '{}',

    -- 质量信息
    stability       VARCHAR(32) DEFAULT 'beta',           -- experimental / beta / stable / deprecated
    test_coverage   DECIMAL(5,2),
    avg_latency_ms  INTEGER,
    success_rate    DECIMAL(5,2),

    -- 统计
    install_count   BIGINT DEFAULT 0,
    call_count      BIGINT DEFAULT 0,
    rating_avg      DECIMAL(3,2) DEFAULT 0,
    rating_count    INTEGER DEFAULT 0,

    -- 状态
    status          VARCHAR(32) NOT NULL DEFAULT 'draft', -- draft/pending_approval/approved/rejected/published/deprecated/archived
    visibility      VARCHAR(32) DEFAULT 'private',        -- private / team / public
    review_comment  TEXT,

    -- 所有权
    author_id       VARCHAR(64) NOT NULL REFERENCES users(id),
    team_id         VARCHAR(64) REFERENCES teams(id),
    maintainers     TEXT[] DEFAULT '{}',

    -- 版本链
    previous_version_id VARCHAR(64) REFERENCES skills(id),

    -- 嵌入向量 (pgvector 可选, 启用时取消注释)
    -- embedding    vector(1536),

    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    UNIQUE(skill_key, version)
);

-- ============================================================
-- 技能版本历史
-- ============================================================

CREATE TABLE skill_versions (
    id              VARCHAR(64) PRIMARY KEY DEFAULT uuid_generate_v4()::text,
    skill_id        VARCHAR(64) NOT NULL REFERENCES skills(id),
    version         VARCHAR(32) NOT NULL,
    manifest_snapshot JSONB NOT NULL,
    changelog       TEXT,
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(skill_id, version)
);

-- ============================================================
-- 技能安装记录
-- ============================================================

CREATE TABLE skill_installations (
    id              VARCHAR(64) PRIMARY KEY DEFAULT uuid_generate_v4()::text,
    skill_id        VARCHAR(64) NOT NULL REFERENCES skills(id),
    skill_version   VARCHAR(32) NOT NULL,
    user_id         VARCHAR(64) NOT NULL REFERENCES users(id),
    api_key_hash    VARCHAR(256) NOT NULL,                  -- SHA256(Token)
    api_key_prefix  VARCHAR(16) NOT NULL,                   -- 前 8 位明文, 方便识别
    token_expires_at TIMESTAMP WITH TIME ZONE,
    ip_whitelist    TEXT[] DEFAULT '{}',
    rate_limit_qps  INTEGER DEFAULT 100,
    status          VARCHAR(32) DEFAULT 'active',           -- active / revoked / expired
    installed_at    TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    revoked_at      TIMESTAMP WITH TIME ZONE,
    UNIQUE(skill_id, user_id)
);

-- ============================================================
-- 技能审核流水线
-- ============================================================

CREATE TABLE skill_reviews (
    id              VARCHAR(64) PRIMARY KEY DEFAULT uuid_generate_v4()::text,
    skill_id        VARCHAR(64) NOT NULL REFERENCES skills(id),
    reviewer_id     VARCHAR(64) REFERENCES users(id),
    stage           VARCHAR(64) NOT NULL,                  -- format_check / security_scan / function_test / manual_review
    verdict         VARCHAR(32),                           -- pass / fail / pending
    comment         TEXT,
    scan_report     JSONB,                                 -- 自动化扫描结果
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- ============================================================
-- 技能调用全链路审计
-- ============================================================

CREATE TABLE skill_audit_logs (
    id              VARCHAR(64) PRIMARY KEY DEFAULT uuid_generate_v4()::text,
    trace_id        VARCHAR(64) NOT NULL,                   -- 分布式追踪 ID
    parent_trace_id VARCHAR(64),                            -- 编排场景的父调用

    skill_id        VARCHAR(64) NOT NULL,
    skill_version   VARCHAR(32),
    user_id         VARCHAR(64) NOT NULL,
    installation_id VARCHAR(64),

    method          VARCHAR(256),
    request_params  JSONB,
    request_size    INTEGER,

    response_status VARCHAR(32),                            -- success / error / timeout / blocked
    response_code   INTEGER,
    response_size   INTEGER,
    error_message   TEXT,

    duration_ms     INTEGER,
    tokens_used     INTEGER DEFAULT 0,

    source_ip       INET,
    user_agent      VARCHAR(512),

    -- 数据访问记录
    data_access     JSONB DEFAULT '[]',

    -- 合规
    pii_detected    BOOLEAN DEFAULT FALSE,
    pii_details     JSONB,

    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- ============================================================
-- 用户评价
-- ============================================================

CREATE TABLE skill_ratings (
    id              VARCHAR(64) PRIMARY KEY DEFAULT uuid_generate_v4()::text,
    skill_id        VARCHAR(64) NOT NULL REFERENCES skills(id),
    user_id         VARCHAR(64) NOT NULL REFERENCES users(id),
    rating          INTEGER NOT NULL CHECK (rating >= 1 AND rating <= 5),
    title           VARCHAR(256),
    comment         TEXT,
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(skill_id, user_id)
);

-- ============================================================
-- 技能收藏
-- ============================================================

CREATE TABLE skill_bookmarks (
    user_id         VARCHAR(64) NOT NULL REFERENCES users(id),
    skill_id        VARCHAR(64) NOT NULL REFERENCES skills(id),
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (user_id, skill_id)
);

-- ============================================================
-- 运营统计 (每日快照)
-- ============================================================

CREATE TABLE daily_stats (
    id              VARCHAR(64) PRIMARY KEY DEFAULT uuid_generate_v4()::text,
    stat_date       DATE NOT NULL,
    skill_id        VARCHAR(64) REFERENCES skills(id),
    call_count      BIGINT DEFAULT 0,
    unique_users    INTEGER DEFAULT 0,
    avg_duration_ms INTEGER DEFAULT 0,
    error_count     INTEGER DEFAULT 0,
    tokens_used     BIGINT DEFAULT 0,
    UNIQUE(stat_date, skill_id)
);

-- ============================================================
-- 系统配置
-- ============================================================

CREATE TABLE system_configs (
    key             VARCHAR(128) PRIMARY KEY,
    value           JSONB NOT NULL,
    description     VARCHAR(512),
    updated_by      VARCHAR(64),
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- ============================================================
-- 索引
-- ============================================================

-- 技能查询
CREATE INDEX idx_skills_status ON skills(status);
CREATE INDEX idx_skills_category ON skills(category);
CREATE INDEX idx_skills_rating ON skills(rating_avg DESC);
CREATE INDEX idx_skills_tags ON skills USING gin(tags);
CREATE INDEX idx_skills_author ON skills(author_id);
CREATE INDEX idx_skills_team ON skills(team_id);
CREATE INDEX idx_skills_name_search ON skills USING gin(name gin_trgm_ops);

-- 审计日志 (按时间分区可在生产中进一步优化)
CREATE INDEX idx_audit_trace ON skill_audit_logs(trace_id);
CREATE INDEX idx_audit_skill_time ON skill_audit_logs(skill_id, created_at DESC);
CREATE INDEX idx_audit_user_time ON skill_audit_logs(user_id, created_at DESC);
CREATE INDEX idx_audit_created ON skill_audit_logs(created_at DESC);

-- 安装记录
CREATE INDEX idx_installs_user ON skill_installations(user_id);
CREATE INDEX idx_installs_skill ON skill_installations(skill_id);

-- 评价
CREATE INDEX idx_ratings_skill ON skill_ratings(skill_id);

-- ============================================================
-- 种子数据
-- ============================================================

-- 管理员
INSERT INTO users (id, username, display_name, email, department, role) VALUES
('u-admin-001', 'admin', '系统管理员', 'admin@jjbank.com', '信息科技部', 'admin'),
('u-zhangsan', 'zhangsan', '张三', 'zhangsan@jjbank.com', '信息科技部-研发中心', 'developer'),
('u-lisi', 'lisi', '李四', 'lisi@jjbank.com', '信息科技部-运维中心', 'developer'),
('u-wangwu', 'wangwu', '王五', 'wangwu@jjbank.com', '信息科技部-安全中心', 'developer'),
('u-zhaoliu', 'zhaoliu', '赵六', 'zhaoliu@jjbank.com', '信息科技部-数据平台', 'user');

-- 示例团队
INSERT INTO teams (id, name, department, description) VALUES
('t-dba-001', '数据库管理团队', '信息科技部-基础架构中心', '负责全行数据库运维管理'),
('t-devops-001', 'DevOps 团队', '信息科技部-研发中心', '负责 CI/CD 与自动化部署'),
('t-security-001', '安全合规团队', '信息科技部-安全中心', '负责信息安全与合规审计');

INSERT INTO team_members (team_id, user_id, role) VALUES
('t-dba-001', 'u-lisi', 'owner'),
('t-security-001', 'u-wangwu', 'owner');

-- 示例技能
INSERT INTO skills (id, skill_key, name, version, category, tags, summary, description,
    skill_type, endpoint_url, stability, status, visibility, author_id, team_id, manifest) VALUES
('s-001', 'db-inspection', '数据库智能巡检助手', '1.2.0', '智能运维',
    ARRAY['数据库', '巡检', '告警', 'Mysql', 'Oracle'],
    '基于 Zabbix + CMDB 自动执行数据库健康检查，生成巡检报告',
    '通过 MCP 协议对接 Zabbix 监控与 CMDB 配置库，支持 Mysql/Oracle/达梦等多种数据库的每日自动巡检。',
    'mcp', 'http://skill-runner:8081/db-inspection/mcp', 'stable', 'published', 'public',
    'u-lisi', 't-dba-001',
    '{"interface":{"inputs":[{"name":"target_db","type":"string","required":true}],"outputs":[{"name":"report","type":"markdown"}]}}'::jsonb),

('s-002', 'log-desensitization', '日志敏感信息识别', '1.0.0', '安全合规',
    ARRAY['日志', '脱敏', 'PII', '安全'],
    '面向应用日志的敏感信息智能识别与脱敏处理',
    '融合正则匹配与上下文语义推理，精准识别人名、身份证号、手机号、银行卡号等敏感信息，支持自动脱敏。',
    'mcp', 'http://skill-runner:8081/log-desensitization/mcp', 'stable', 'published', 'public',
    'u-wangwu', 't-security-001',
    '{"interface":{"inputs":[{"name":"log_content","type":"string","required":true}],"outputs":[{"name":"masked_content","type":"string"}]}}'::jsonb);
