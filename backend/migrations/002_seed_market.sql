-- ============================================================
-- SKILL NEXUS 完整技能市场数据 Seed
-- 4 个真实 MCP 技能 + 活跃使用数据
-- ============================================================

-- ============ 1. 补全 s-001 / s-002 元数据 ============
UPDATE skills SET
  manifest = '{"interface": {"inputs": [{"name": "target_db", "type": "string", "required": true}, {"name": "check_scope", "type": "enum", "values": ["full","performance","security","capacity"], "default": "full"}], "outputs": [{"name": "report", "type": "markdown"}, {"name": "score", "type": "number"}, {"name": "issues", "type": "array"}]}}'::jsonb,
  interface_spec = '{"tools": ["inspect_database", "get_slow_queries", "get_connection_stats"]}'::jsonb,
  permissions = '[{"resource": "cmdb:host:read"}, {"resource": "zabbix:metric:read"}, {"resource": "database:metadata:read"}]'::jsonb,
  description = '基于 Zabbix + CMDB 自动执行数据库健康巡检, 生成巡检报告。支持 MySQL/Oracle/PostgreSQL/达梦等多种数据库的每日自动巡检。检查项: CPU/内存/磁盘使用率、连接池状态、慢查询分析、主从复制延迟。',
  tags = '{数据库,巡检,告警,MySQL,Oracle,PostgreSQL}',
  updated_at = NOW()
WHERE id = 's-001';

UPDATE skills SET
  manifest = '{"interface": {"inputs": [{"name": "content", "type": "string", "required": true}], "outputs": [{"name": "masked_content", "type": "string"}, {"name": "entities_found", "type": "array"}]}}'::jsonb,
  interface_spec = '{"tools": ["scan_log", "desensitize", "detect_pii"]}'::jsonb,
  permissions = '[{"resource": "log:read"}, {"resource": "pii:detect"}]'::jsonb,
  description = '对日志中的敏感信息进行智能识别与脱敏, 支持身份证号、手机号、银行卡号、姓名等 PII 实体的自动检测与掩码处理, 满足等保合规要求。',
  tags = '{日志,脱敏,合规,PII,安全}',
  updated_at = NOW()
WHERE id = 's-002';

-- ============ 2. 注册 s-003 告警收敛分析 ============
INSERT INTO skills (id, skill_key, name, version, category, sub_category, tags,
  summary, description, icon_url, skill_type, endpoint_url, endpoint_protocol,
  manifest, dependencies, permissions, interface_spec, stability,
  status, visibility, author_id, maintainers, created_at, updated_at)
SELECT 's-003', 'alert-convergence', '告警收敛分析', '1.0.0', '智能运维', 'AIOps',
  '{告警,收敛,根因,AIOps,运维}',
  '系统出现异常时, 对大量告警进行聚合收敛分析, 识别根因, 减少告警风暴',
  '当系统出现异常时, 对大量告警进行聚合收敛分析。使用 AI 分析历史告警, 识别根因, 减少告警风暴。支持告警聚合、根因定位、收敛报告生成。',
  '', 'mcp', 'http://skill-runner:8081/alert-convergence/mcp', 'MCP/2024-11-05',
  '{"interface": {"inputs": [{"name": "host", "type": "string", "required": true}, {"name": "time_range", "type": "string", "default": "1h"}], "outputs": [{"name": "report", "type": "markdown"}, {"name": "convergence_rate", "type": "number"}]}}'::jsonb,
  '{}'::jsonb,
  '[{"resource": "monitor:alert:read"}]'::jsonb,
  '{"tools": ["analyze_alerts", "find_root_cause", "generate_report"]}'::jsonb,
  'stable', 'published', 'public',
  (SELECT id FROM users WHERE username = 'lisi'),
  ARRAY[(SELECT id FROM users WHERE username = 'lisi')],
  NOW() - INTERVAL '14 days', NOW() - INTERVAL '14 days'
WHERE NOT EXISTS (SELECT 1 FROM skills WHERE id = 's-003');

-- ============ 3. 注册 s-004 AI 需求分析助手 ============
INSERT INTO skills (id, skill_key, name, version, category, sub_category, tags,
  summary, description, icon_url, skill_type, endpoint_url, endpoint_protocol,
  manifest, dependencies, permissions, interface_spec, stability,
  status, visibility, author_id, maintainers, created_at, updated_at)
SELECT 's-004', 'requirement-analysis', 'AI 需求分析助手', '1.0.0', '研发效能', '需求管理',
  '{需求,PRD,AI,研发,文档}',
  '对需求描述开展状态分析、价值及业务规则细化分析, 输出规范化需求文档',
  'AI 辅助需求分析助手: 对需求描述开展状态分析、价值及业务规则细化分析, 输出规范化的需求文档与 PRD, 帮助研发团队提升需求质量。',
  '', 'mcp', 'http://skill-runner:8081/requirement-analysis/mcp', 'MCP/2024-11-05',
  '{"interface": {"inputs": [{"name": "title", "type": "string", "required": true}, {"name": "description", "type": "string", "required": true}], "outputs": [{"name": "analysis", "type": "markdown"}, {"name": "prd", "type": "markdown"}]}}'::jsonb,
  '{}'::jsonb,
  '[{"resource": "requirement:read"}]'::jsonb,
  '{"tools": ["analyze_requirement", "generate_prd", "analyze_business_rules"]}'::jsonb,
  'stable', 'published', 'public',
  (SELECT id FROM users WHERE username = 'zhangsan'),
  ARRAY[(SELECT id FROM users WHERE username = 'zhangsan')],
  NOW() - INTERVAL '10 days', NOW() - INTERVAL '10 days'
WHERE NOT EXISTS (SELECT 1 FROM skills WHERE id = 's-004');

-- ============ 4. 活跃安装数据 ============
-- 已有安装则跳过 (演示账号)
INSERT INTO skill_installations (skill_id, skill_version, user_id, api_key_hash, api_key_prefix, status, installed_at)
SELECT 's-001', '1.2.0', u.id, repeat('a', 64), 'sk-demo-0010000', 'active', NOW() - INTERVAL '20 days'
FROM users u WHERE u.username = 'zhangsan'
  AND NOT EXISTS (SELECT 1 FROM skill_installations si WHERE si.skill_id = 's-001' AND si.user_id = u.id);

INSERT INTO skill_installations (skill_id, skill_version, user_id, api_key_hash, api_key_prefix, status, installed_at)
SELECT 's-004', '1.0.0', u.id, repeat('b', 64), 'sk-demo-0040000', 'active', NOW() - INTERVAL '8 days'
FROM users u WHERE u.username = 'zhangsan'
  AND NOT EXISTS (SELECT 1 FROM skill_installations si WHERE si.skill_id = 's-004' AND si.user_id = u.id);

INSERT INTO skill_installations (skill_id, skill_version, user_id, api_key_hash, api_key_prefix, status, installed_at)
SELECT 's-002', '1.0.0', u.id, repeat('c', 64), 'sk-demo-0020000', 'active', NOW() - INTERVAL '15 days'
FROM users u WHERE u.username = 'lisi'
  AND NOT EXISTS (SELECT 1 FROM skill_installations si WHERE si.skill_id = 's-002' AND si.user_id = u.id);

INSERT INTO skill_installations (skill_id, skill_version, user_id, api_key_hash, api_key_prefix, status, installed_at)
SELECT 's-003', '1.0.0', u.id, repeat('d', 64), 'sk-demo-0030000', 'active', NOW() - INTERVAL '12 days'
FROM users u WHERE u.username = 'lisi'
  AND NOT EXISTS (SELECT 1 FROM skill_installations si WHERE si.skill_id = 's-003' AND si.user_id = u.id);

INSERT INTO skill_installations (skill_id, skill_version, user_id, api_key_hash, api_key_prefix, status, installed_at)
SELECT 's-001', '1.2.0', u.id, repeat('e', 64), 'sk-demo-0011000', 'active', NOW() - INTERVAL '10 days'
FROM users u WHERE u.username = 'wangwu'
  AND NOT EXISTS (SELECT 1 FROM skill_installations si WHERE si.skill_id = 's-001' AND si.user_id = u.id);

INSERT INTO skill_installations (skill_id, skill_version, user_id, api_key_hash, api_key_prefix, status, installed_at)
SELECT 's-002', '1.0.0', u.id, repeat('f', 64), 'sk-demo-0021000', 'active', NOW() - INTERVAL '9 days'
FROM users u WHERE u.username = 'wangwu'
  AND NOT EXISTS (SELECT 1 FROM skill_installations si WHERE si.skill_id = 's-002' AND si.user_id = u.id);

INSERT INTO skill_installations (skill_id, skill_version, user_id, api_key_hash, api_key_prefix, status, installed_at)
SELECT 's-003', '1.0.0', u.id, repeat('0', 64), 'sk-demo-0031000', 'active', NOW() - INTERVAL '7 days'
FROM users u WHERE u.username = 'wangwu'
  AND NOT EXISTS (SELECT 1 FROM skill_installations si WHERE si.skill_id = 's-003' AND si.user_id = u.id);

INSERT INTO skill_installations (skill_id, skill_version, user_id, api_key_hash, api_key_prefix, status, installed_at)
SELECT 's-001', '1.2.0', u.id, repeat('1', 64), 'sk-demo-0012000', 'active', NOW() - INTERVAL '5 days'
FROM users u WHERE u.username = 'admin'
  AND NOT EXISTS (SELECT 1 FROM skill_installations si WHERE si.skill_id = 's-001' AND si.user_id = u.id);

INSERT INTO skill_installations (skill_id, skill_version, user_id, api_key_hash, api_key_prefix, status, installed_at)
SELECT 's-004', '1.0.0', u.id, repeat('2', 64), 'sk-demo-0041000', 'active', NOW() - INTERVAL '3 days'
FROM users u WHERE u.username = 'admin'
  AND NOT EXISTS (SELECT 1 FROM skill_installations si WHERE si.skill_id = 's-004' AND si.user_id = u.id);

-- ============ 5. 评价数据 ============
INSERT INTO skill_ratings (skill_id, user_id, rating, title, comment, created_at)
SELECT 's-001', u.id, 5, '非常实用', '每天自动巡检, 慢查询一眼可见, 帮我们提前发现了支付库的连接池风险!', NOW() - INTERVAL '18 days'
FROM users u WHERE u.username = 'zhangsan'
  AND NOT EXISTS (SELECT 1 FROM skill_ratings sr WHERE sr.skill_id = 's-001' AND sr.user_id = u.id);

INSERT INTO skill_ratings (skill_id, user_id, rating, title, comment, created_at)
SELECT 's-001', u.id, 4, '好用', '巡检报告很详细, 建议支持更多数据库类型。', NOW() - INTERVAL '12 days'
FROM users u WHERE u.username = 'lisi'
  AND NOT EXISTS (SELECT 1 FROM skill_ratings sr WHERE sr.skill_id = 's-001' AND sr.user_id = u.id);

INSERT INTO skill_ratings (skill_id, user_id, rating, title, comment, created_at)
SELECT 's-002', u.id, 5, '合规刚需', '日志脱敏效率大幅提升, 审计检查一次通过!', NOW() - INTERVAL '14 days'
FROM users u WHERE u.username = 'wangwu'
  AND NOT EXISTS (SELECT 1 FROM skill_ratings sr WHERE sr.skill_id = 's-002' AND sr.user_id = u.id);

INSERT INTO skill_ratings (skill_id, user_id, rating, title, comment, created_at)
SELECT 's-003', u.id, 4, '告警风暴救星', '收敛率 92%, 值班手机终于不炸了。', NOW() - INTERVAL '10 days'
FROM users u WHERE u.username = 'lisi'
  AND NOT EXISTS (SELECT 1 FROM skill_ratings sr WHERE sr.skill_id = 's-003' AND sr.user_id = u.id);

INSERT INTO skill_ratings (skill_id, user_id, rating, title, comment, created_at)
SELECT 's-004', u.id, 5, '需求评审神器', 'PRD 生成质量高, 需求遗漏明显减少。', NOW() - INTERVAL '6 days'
FROM users u WHERE u.username = 'admin'
  AND NOT EXISTS (SELECT 1 FROM skill_ratings sr WHERE sr.skill_id = 's-004' AND sr.user_id = u.id);

-- ============ 6. 更新技能统计 ============
UPDATE skills SET
  install_count = (SELECT COUNT(*) FROM skill_installations si WHERE si.skill_id = skills.id AND si.status = 'active'),
  rating_avg = COALESCE((SELECT ROUND(AVG(rating)::numeric, 2) FROM skill_ratings sr WHERE sr.skill_id = skills.id), 0),
  rating_count = (SELECT COUNT(*) FROM skill_ratings sr WHERE sr.skill_id = skills.id),
  call_count = call_count + 128
WHERE id IN ('s-001', 's-002', 's-003', 's-004');

-- ============ 7. 审计日志 (真实调用痕迹) ============
INSERT INTO skill_audit_logs (trace_id, skill_id, user_id, method, response_status, duration_ms, tokens_used, source_ip, pii_detected, created_at)
SELECT 'tr_demo_' || i, s.skill_key, u.id, 'execute', 'success', 180 + (i * 37) % 900, 0,
       (ARRAY['10.10.1.23', '10.10.2.15', '10.10.3.88', '10.10.1.100']::inet[])[1 + i % 4], (i % 7 = 0), NOW() - (i || ' hours')::interval
FROM generate_series(1, 60) AS i
CROSS JOIN (SELECT id, skill_key FROM skills WHERE id = 's-001') s
CROSS JOIN (SELECT id FROM users WHERE username = 'zhangsan') u;

INSERT INTO skill_audit_logs (trace_id, skill_id, user_id, method, response_status, duration_ms, tokens_used, source_ip, pii_detected, created_at)
SELECT 'tr_demo_' || i, s.skill_key, u.id, 'execute', 'success', 150 + (i * 53) % 800, 0,
       (ARRAY['10.10.1.23', '10.10.2.15', '10.10.3.88']::inet[])[1 + i % 3], (i % 5 = 0), NOW() - (i || ' hours')::interval
FROM generate_series(1, 40) AS i
CROSS JOIN (SELECT id, skill_key FROM skills WHERE id = 's-002') s
CROSS JOIN (SELECT id FROM users WHERE username = 'lisi') u;

INSERT INTO skill_audit_logs (trace_id, skill_id, user_id, method, response_status, duration_ms, tokens_used, source_ip, pii_detected, created_at)
SELECT 'tr_demo_' || i, s.skill_key, u.id, 'execute', 'success', 200 + (i * 61) % 700, 0,
       (ARRAY['10.10.2.15', '10.10.3.88']::inet[])[1 + i % 2], false, NOW() - (i || ' hours')::interval
FROM generate_series(1, 30) AS i
CROSS JOIN (SELECT id, skill_key FROM skills WHERE id = 's-003') s
CROSS JOIN (SELECT id FROM users WHERE username = 'wangwu') u;

INSERT INTO skill_audit_logs (trace_id, skill_id, user_id, method, response_status, duration_ms, tokens_used, source_ip, pii_detected, created_at)
SELECT 'tr_demo_' || i, s.skill_key, u.id, 'execute', 'success', 300 + (i * 41) % 600, 0,
       '10.10.1.100'::inet, false, NOW() - (i || ' hours')::interval
FROM generate_series(1, 20) AS i
CROSS JOIN (SELECT id, skill_key FROM skills WHERE id = 's-004') s
CROSS JOIN (SELECT id FROM users WHERE username = 'admin') u;

-- 更新月度统计引用 (skill_audit_logs 总调用数)
UPDATE skills SET call_count = (SELECT COUNT(*) FROM skill_audit_logs WHERE skill_id = skills.skill_key) WHERE id IN ('s-001','s-002','s-003','s-004');

SELECT '✅ 技能市场数据 Seed 完成' AS result;
