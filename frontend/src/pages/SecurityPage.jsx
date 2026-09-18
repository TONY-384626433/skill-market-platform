import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Alert, App as AntApp, Button, Descriptions, Drawer, Empty, Segmented, Skeleton, Space, Statistic, Table, Tabs, Tag, Tooltip, Upload } from 'antd';
import {
  BugOutlined, CheckOutlined, CloseOutlined, CloudDownloadOutlined, ExperimentOutlined, FileProtectOutlined,
  GithubOutlined, InboxOutlined, ReloadOutlined, SafetyCertificateOutlined, ScanOutlined,
  SecurityScanOutlined, WarningOutlined,
} from '@ant-design/icons';
import {
  decideImportRequest, getDownloadAudits, getImportRequests, getSecurityOverview, getSecurityQueue,
  getSecurityRules, getSecurityScanDetail, getSecurityScans, getSkillProvenance, runSkillSecurityScan,
  scanImportRequest, scanPackage, verifySkillProvenance,
} from '../services/api';

const VERDICT_META = {
  safe: { color: 'success', text: '安全', icon: CheckOutlined },
  suspicious: { color: 'warning', text: '可疑待核', icon: WarningOutlined },
  malicious: { color: 'error', text: '高危拦截', icon: BugOutlined },
  unscanned: { color: 'default', text: '未扫描', icon: ScanOutlined },
};

const STATUS_META = {
  pending: { color: 'default', text: '待审查' },
  scanning: { color: 'processing', text: '审查中' },
  pending_review: { color: 'warning', text: '待人工复核' },
  approved: { color: 'success', text: '已通过' },
  blocked: { color: 'error', text: '已阻断' },
  rejected: { color: 'default', text: '已驳回' },
  imported: { color: 'cyan', text: '已入库' },
};

const SEVERITY_META = {
  critical: { color: 'error', text: '严重' },
  high: { color: 'volcano', text: '高' },
  medium: { color: 'warning', text: '中' },
  low: { color: 'default', text: '低' },
};

function VerdictTag({ verdict }) {
  const meta = VERDICT_META[verdict] || VERDICT_META.unscanned;
  const Icon = meta.icon;
  return <Tag color={meta.color} icon={<Icon />}>{meta.text}</Tag>;
}

function StatusTag({ status }) {
  const meta = STATUS_META[status] || { color: 'default', text: status };
  return <Tag color={meta.color}>{meta.text}</Tag>;
}

function SeverityTag({ severity }) {
  const meta = SEVERITY_META[severity] || { color: 'default', text: severity };
  return <Tag color={meta.color}>{meta.text}</Tag>;
}

const SANDBOX_STATUS_META = {
  ok: { color: 'success', text: '已真实验证' },
  skipped: { color: 'default', text: '已跳过' },
  unreachable: { color: 'warning', text: '沙箱不可达' },
  error: { color: 'error', text: '验证出错' },
};

const CHECK_STATUS_META = {
  passed: { color: 'success', text: '通过' },
  failed: { color: 'error', text: '未通过' },
  warning: { color: 'warning', text: '告警' },
  skipped: { color: 'default', text: '跳过' },
};

const DYN_EVENT_CN = {
  network_connect: '外联连接',
  network_dns: '域名解析',
  socket_bind: '监听端口',
  subprocess: '子进程执行',
  shell_command: 'Shell 命令',
  file_write: '写文件',
  file_write_outside: '越权写文件',
  sensitive_read: '读敏感凭据',
  file_delete: '删除文件',
  persistence: '写入持久化项',
  import_module: '动态加载模块',
};

function SandboxPanel({ sandbox }) {
  if (!sandbox) return null;
  const meta = SANDBOX_STATUS_META[sandbox.status] || SANDBOX_STATUS_META.skipped;
  const iso = sandbox.isolated || {};
  const events = sandbox.events || {};
  const checks = sandbox.checks || [];
  const diff = sandbox.filesystem_diff || null;
  const diffRows = diff
    ? Object.entries(diff).map(([k, v]) => ({ key: k, kind: k, files: v || [] })).filter((r) => r.files.length > 0)
    : [];
  return (
    <div className="sec-sandbox">
      <div className="sec-sandbox-head">
        <span className="sec-sandbox-title"><ExperimentOutlined /> 动态沙箱行为验证</span>
        <Space size={4} wrap>
          <Tag color={meta.color}>{meta.text}</Tag>
          {iso.network_reachable === false && <Tooltip title="沙箱无外网可达"><Tag color="cyan">隔离 · 无外网</Tag></Tooltip>}
          {iso.rootfs_read_only && <Tag color="cyan">只读根文件系统</Tag>}
          {iso.uid !== undefined && <Tag color="cyan">非 root(uid {iso.uid})</Tag>}
          {iso.cap_eff === '0000000000000000' && <Tag color="cyan">能力已清空</Tag>}
        </Space>
      </div>
      {sandbox.notice && <Alert type="info" showIcon className="sec-alert" message={sandbox.notice} />}
      <Descriptions size="small" column={2} bordered>
        <Descriptions.Item label="引擎">{sandbox.engine || '-'}</Descriptions.Item>
        <Descriptions.Item label="运行号"><code>{sandbox.run_id || '-'}</code></Descriptions.Item>
        <Descriptions.Item label="入口">{sandbox.entry || '-'}</Descriptions.Item>
        <Descriptions.Item label="退出码">{sandbox.executed ? (sandbox.exit_code ?? '-') : '未执行'}</Descriptions.Item>
        <Descriptions.Item label="耗时">{sandbox.duration_ms ?? 0} ms</Descriptions.Item>
        <Descriptions.Item label="行为事件">{sandbox.trace_lines ?? 0} 条</Descriptions.Item>
      </Descriptions>
      {Object.keys(events).length > 0 && (
        <div className="sec-sandbox-events">
          {Object.entries(events).map(([k, v]) => (
            <span key={k} className="sec-sandbox-event"><code>{k}</code>{DYN_EVENT_CN[k] ? <small>{DYN_EVENT_CN[k]}</small> : null}<b>{v}</b></span>
          ))}
        </div>
      )}
      {checks.length > 0 && (
        <div className="sec-sandbox-checks">
          {checks.map((c, i) => {
            const cm = CHECK_STATUS_META[c.status] || CHECK_STATUS_META.skipped;
            return (
              <div key={i} className="sec-sandbox-check">
                <Tag color={cm.color}>{cm.text}</Tag>
                <code>{c.rule_id}</code>
                <strong>{c.name}</strong>
                <span className="sec-muted">{c.detail}</span>
              </div>
            );
          })}
        </div>
      )}
      {diffRows.length > 0 && (
        <div className="sec-sandbox-diff">
          {diffRows.map((r) => (
            <div key={r.kind}>
              <Tag color="orange">{r.kind}</Tag>
              <span className="sec-muted">{r.files.join('、')}</span>
            </div>
          ))}
        </div>
      )}
      {(sandbox.stdout_tail || sandbox.stderr_tail) && (
        <details className="sec-sandbox-io">
          <summary>运行输出（尾部）</summary>
          {sandbox.stdout_tail && <pre>{sandbox.stdout_tail}</pre>}
          {sandbox.stderr_tail && <pre className="sec-stderr">{sandbox.stderr_tail}</pre>}
        </details>
      )}
    </div>
  );
}

export default function SecurityPage() {
  const { message } = AntApp.useApp();
  const [overview, setOverview] = useState(null);
  const [rules, setRules] = useState([]);
  const [engine, setEngine] = useState({});
  const [policy, setPolicy] = useState({});
  const [queue, setQueue] = useState([]);
  const [scans, setScans] = useState([]);
  const [imports, setImports] = useState([]);
  const [audits, setAudits] = useState([]);
  const [loading, setLoading] = useState(true);
  const [tab, setTab] = useState('imports');
  const [importFilter, setImportFilter] = useState('all');
  const [busy, setBusy] = useState('');
  const [uploading, setUploading] = useState(false);
  const [report, setReport] = useState(null);
  const [provenance, setProvenance] = useState(null);

  const load = useCallback(async () => {
    setLoading(true);
    const [ov, rl, q, sc, im, au] = await Promise.allSettled([
      getSecurityOverview(), getSecurityRules(), getSecurityQueue({ limit: 100 }),
      getSecurityScans({ limit: 60 }), getImportRequests({ limit: 60 }), getDownloadAudits({ limit: 60 }),
    ]);
    if (ov.status === 'fulfilled') setOverview(ov.value);
    if (rl.status === 'fulfilled') { setRules(rl.value?.data || []); setEngine(rl.value?.engine || {}); setPolicy(rl.value?.policy || {}); }
    if (q.status === 'fulfilled') setQueue(q.value?.data || []);
    if (sc.status === 'fulfilled') setScans(sc.value?.data || []);
    if (im.status === 'fulfilled') setImports(im.value?.data || []);
    if (au.status === 'fulfilled') setAudits(au.value?.data || []);
    setLoading(false);
  }, []);

  useEffect(() => { load(); }, [load]);

  const act = async (key, fn) => {
    setBusy(key);
    try { await fn(); await load(); message.success('操作完成'); } catch (e) { message.error(e.response?.data?.error || e.message || '操作失败'); } finally { setBusy(''); }
  };

  const openScan = async (scanId) => {
    try { setReport(await getSecurityScanDetail(scanId)); } catch (e) { message.error('报告加载失败'); }
  };

  const handleUpload = async (file) => {
    setUploading(true);
    try {
      const response = await scanPackage(file);
      const scan = response?.data || response;
      setReport(scan);
      if (scan?.verdict === 'malicious') message.error('已阻断: 命中严重风险, 禁止入库');
      else if (scan?.verdict === 'suspicious') message.warning('结论为可疑, 需人工复核');
      else message.success('安全预检通过');
    } catch (uploadError) {
      message.error(uploadError.response?.data?.error || '预检失败');
    } finally {
      setUploading(false);
    }
    return false;
  };

  const filteredImports = useMemo(() => (importFilter === 'all' ? imports : imports.filter((i) => i.status === importFilter)), [imports, importFilter]);

  const importColumns = [
    { title: '来源技能', dataIndex: 'skill_path', render: (v, r) => (
      <div className="sec-cell">
        <GithubOutlined />
        <div><strong>{r.skill_name || v}</strong><small>{r.repository}@{r.ref} · {v}</small></div>
      </div>
    ) },
    { title: '审查状态', dataIndex: 'status', width: 120, render: (v) => <StatusTag status={v} /> },
    { title: '安全结论', dataIndex: 'verdict', width: 110, render: (v, r) => (v ? <VerdictTag verdict={v} /> : <span className="sec-muted">-</span>) },
    { title: '风险分', dataIndex: 'risk_score', width: 90, render: (v, r) => <span className={v >= 60 ? 'sec-risk-high' : v >= 30 ? 'sec-risk-mid' : 'sec-risk-low'}>{v}</span> },
    { title: '严重项', dataIndex: 'critical_count', width: 80, render: (v) => (v > 0 ? <b className="sec-risk-high">{v}</b> : <span className="sec-muted">0</span>) },
    { title: '相似度', dataIndex: 'reupload', width: 120, render: (v) => (v?.suspected ? <Tooltip title={`疑似复制「${v.matched_skill_name}」`}><Tag color="magenta">{(v.similarity * 100).toFixed(0)}% 查重</Tag></Tooltip> : <span className="sec-muted">-</span>) },
    { title: '时间', dataIndex: 'created_at', width: 160, render: (v) => (v ? new Date(v).toLocaleString('zh-CN', { hour12: false }) : '-') },
    { title: '操作', key: 'ops', width: 260, render: (_, r) => (
      <Space size={4} wrap>
        <Button size="small" icon={<ScanOutlined />} loading={busy === 'scan-' + r.id} onClick={() => act('scan-' + r.id, () => scanImportRequest(r.id))}>重新审查</Button>
        {r.scan_id && <Button size="small" type="link" onClick={() => openScan(r.scan_id)}>报告</Button>}
        {(r.status === 'pending_review' || r.status === 'pending') && (
          <>
            <Button size="small" type="primary" icon={<CheckOutlined />} loading={busy === 'ok-' + r.id}
              onClick={() => act('ok-' + r.id, () => decideImportRequest(r.id, true, '人工复核通过: 已确认来源与权限'))}>通过</Button>
            <Button size="small" danger icon={<CloseOutlined />} loading={busy === 'no-' + r.id}
              onClick={() => act('no-' + r.id, () => decideImportRequest(r.id, false, '人工复核驳回'))}>驳回</Button>
          </>
        )}
        {r.status === 'blocked' && <Tooltip title="安全引擎已判定高危, 不可放行"><Tag color="error">不可放行</Tag></Tooltip>}
      </Space>
    ) },
  ];

  const queueColumns = [
    { title: '技能', dataIndex: 'name', render: (v, r) => (
      <div className="sec-cell"><FileProtectOutlined /><div><strong>{v}</strong><small>{r.skill_key} · v{r.version} · {r.category}</small></div></div>
    ) },
    { title: '安全状态', dataIndex: 'security_status', width: 120, render: (v, r) => (r.quarantined ? <Tag color="error" icon={<BugOutlined />}>已隔离</Tag> : <VerdictTag verdict={v} />) },
    { title: '风险分', dataIndex: 'security_score', width: 90, render: (v) => <span className={v >= 60 ? 'sec-risk-high' : v >= 30 ? 'sec-risk-mid' : 'sec-risk-low'}>{v ?? 0}</span> },
    { title: '最近扫描', dataIndex: 'last_security_scan_at', width: 170, render: (v) => (v ? new Date(v).toLocaleString('zh-CN', { hour12: false }) : <span className="sec-muted">未扫描</span>) },
    { title: '操作', key: 'ops', width: 300, render: (_, r) => (
      <Space size={4} wrap>
        <Button size="small" type="primary" icon={<SecurityScanOutlined />} loading={busy === 'sec-' + r.skill_id}
          onClick={() => act('sec-' + r.skill_id, () => runSkillSecurityScan(r.skill_id))}>安全扫描</Button>
        <Button size="small" icon={<ScanOutlined />} onClick={() => act('prov-' + r.skill_id, async () => setProvenance(await getSkillProvenance(r.skill_id)))}>溯源档案</Button>
        <Button size="small" icon={<SafetyCertificateOutlined />} onClick={() => act('ver-' + r.skill_id, async () => { const res = await verifySkillProvenance(r.skill_id); setProvenance(res?.data); message.info('完整性校验: ' + (res?.data?.verify_status || '-')); })}>完整性校验</Button>
      </Space>
    ) },
  ];

  const scanColumns = [
    { title: '时间', dataIndex: 'created_at', width: 165, render: (v) => new Date(v).toLocaleString('zh-CN', { hour12: false }) },
    { title: '对象', dataIndex: 'skill_name', render: (v, r) => <span>{v || r.target || r.skill_key || r.subject_type}<small className="sec-muted"> {r.subject_type === 'import' ? '(外部导入)' : ''}</small></span> },
    { title: '结论', dataIndex: 'verdict', width: 110, render: (v) => <VerdictTag verdict={v} /> },
    { title: '风险分', dataIndex: 'risk_score', width: 90 },
    { title: '等级', dataIndex: 'grade', width: 70, render: (v) => <Tag>{v || '-'}</Tag> },
    { title: '发现', dataIndex: 'finding_count', width: 100, render: (v, r) => <span>{v} 项{r.critical_count > 0 ? <b className="sec-risk-high"> (严重 {r.critical_count})</b> : ''}</span> },
    { title: '触发', dataIndex: 'trigger_type', width: 100, render: (v) => <Tag color="blue">{v}</Tag> },
    { title: '操作', key: 'ops', width: 90, render: (_, r) => <Button size="small" type="link" onClick={() => openScan(r.id)}>报告</Button> },
  ];

  const ruleColumns = [
    { title: '编号', dataIndex: 'rule_id', width: 100, render: (v) => <code>{v}</code> },
    { title: '级别', dataIndex: 'severity', width: 80, render: (v) => <SeverityTag severity={v} /> },
    { title: '分类', dataIndex: 'category_cn', width: 110 },
    { title: '规则', dataIndex: 'title', render: (v, r) => <div><strong>{v}</strong><div className="sec-muted">{r.detail}</div></div> },
    { title: '适用范围', dataIndex: 'scope', width: 110, render: (v) => <Tag>{v}</Tag> },
  ];

  const auditColumns = [
    { title: '时间', dataIndex: 'created_at', width: 165, render: (v) => new Date(v).toLocaleString('zh-CN', { hour12: false }) },
    { title: '技能', dataIndex: 'skill_key', render: (v) => <span>{v || '-'}</span> },
    { title: '下载人', dataIndex: 'user_id', width: 200, render: (v) => <span className="sec-muted">{v || '匿名'}</span> },
    { title: '交付水印', dataIndex: 'watermark_id', width: 160, render: (v) => <code className="sec-watermark">{v}</code> },
    { title: '来源 IP', dataIndex: 'source_ip', width: 130 },
    { title: '通道', dataIndex: 'channel', width: 120, render: (v) => <Tag color="purple">{v || '-'}</Tag> },
  ];

  if (loading && !overview) {
    return <div className="route-loading"><Skeleton active paragraph={{ rows: 10 }} /></div>;
  }

  const stats = overview || {};

  return (
    <div className="security-page">
      <header className="sec-hero">
        <div className="sec-hero-copy">
          <span className="sec-kicker"><ShieldBadge /> SKILL SUPPLY-CHAIN SECURITY</span>
          <h1>技能安全治理</h1>
          <p>外部技能先审核、后下载；任何技能进入平台前先过静态查毒与危险行为检测；交付包带签名与水印，抄袭与泄漏可追溯。</p>
          <div className="sec-policy">
            <Tag color="cyan">查毒 {engine.count || 36} 条规则</Tag>
            <Tag color="geekblue">引擎 {engine.version || 'SEC-ENGINE'}</Tag>
            <Tag color={stats.auto_approve ? 'green' : 'orange'}>{stats.auto_approve ? '安全即自动放行' : '全部人工审批'}</Tag>
            <Tag color="red">高危一律阻断</Tag>
            <Tag color="purple">HMAC-SHA256 包签名</Tag>
            {engine.dynamic_sandbox && (
              <Tooltip title={engine.dynamic_sandbox.notice || `沙箱地址 ${engine.dynamic_sandbox.url || '-'}`}>
                <Tag color={engine.dynamic_sandbox.reachable ? 'green' : 'default'} icon={<ExperimentOutlined />}>
                  动态沙箱{engine.dynamic_sandbox.reachable ? '在线' : '未接入'}
                </Tag>
              </Tooltip>
            )}
            {engine.source && <Tooltip title={engine.source}><Tag>规则库: {String(engine.source).split(/[\\/]/).pop()}</Tag></Tooltip>}
          </div>
        </div>
        <div className="sec-hero-actions">
          <Button icon={<ReloadOutlined />} onClick={load} loading={loading}>刷新</Button>
          <Button icon={<CloudDownloadOutlined />} href="/api/v1/admin/security-rules/export" target="_blank" rel="noreferrer">导出规则库</Button>
        </div>
      </header>

      <div className="sec-stats">
        <div className="sec-stat"><Statistic title="技能总数" value={stats.total_skills ?? 0} /></div>
        <div className="sec-stat"><Statistic title="已扫描" value={stats.scanned ?? 0} /></div>
        <div className="sec-stat ok"><Statistic title="安全" value={stats.safe ?? 0} /></div>
        <div className="sec-stat warn"><Statistic title="可疑待核" value={stats.suspicious ?? 0} /></div>
        <div className="sec-stat danger"><Statistic title="高危/隔离" value={stats.malicious ?? 0} /></div>
        <div className="sec-stat"><Statistic title="导入待审" value={(stats.import_pending ?? 0) + (stats.import_pending_review ?? 0)} /></div>
        <div className="sec-stat"><Statistic title="查重告警" value={stats.reupload_alerts ?? 0} /></div>
        <div className="sec-stat"><Statistic title="下载审计" value={stats.tracked_downloads ?? 0} /></div>
      </div>

      <Alert
        type="info" showIcon className="sec-alert"
        message="先审核、后下载"
        description={policy.github_import?.description || 'GitHub 等外部来源的技能必须先通过静态安全审查，通过后才允许下载；命中严重风险（病毒/木马/后门/提示注入）一律阻断，且不允许人工放行。'}
      />

      <div className="sec-upload">
        <Upload.Dragger name="file" multiple={false} showUploadList={false} beforeUpload={handleUpload} disabled={uploading}>
          <p className="sec-upload-title"><InboxOutlined /> 上传技能包安全预检</p>
          <p className="sec-muted">
            上传即查毒 + 危险行为 + 提示注入 + 查重（.zip 或单文件，上限 {policy.upload?.max_size_mb || 24} MB）；命中严重风险直接阻断，包内容不会落盘执行。
          </p>
        </Upload.Dragger>
      </div>

      <Tabs
        activeKey={tab}
        onChange={setTab}
        items={[
          {
            key: 'imports', label: `导入审查队列 (${imports.length})`,
            children: (
              <>
                <div className="sec-toolbar">
                  <Segmented
                    value={importFilter}
                    onChange={setImportFilter}
                    options={[
                      { label: '全部', value: 'all' },
                      { label: `待复核 ${imports.filter((i) => i.status === 'pending_review').length}`, value: 'pending_review' },
                      { label: `已通过 ${imports.filter((i) => i.status === 'approved' || i.status === 'imported').length}`, value: 'approved' },
                      { label: `已阻断 ${imports.filter((i) => i.status === 'blocked').length}`, value: 'blocked' },
                      { label: `已驳回 ${imports.filter((i) => i.status === 'rejected').length}`, value: 'rejected' },
                    ]}
                  />
                  <span className="sec-muted">外部技能必须先审查；已通过的包在下载时会注入签名清单与专属水印。</span>
                </div>
                <Table rowKey="id" size="small" loading={loading} columns={importColumns} dataSource={filteredImports}
                  pagination={{ pageSize: 8, hideOnSinglePage: true }}
                  locale={{ emptyText: <Empty description="暂无导入审查单：在 GitHub 开源目录点击「提交安全审查」即可发起" /> }} />
              </>
            ),
          },
          {
            key: 'skills', label: `技能安全队列 (${queue.length})`,
            children: <Table rowKey="skill_id" size="small" loading={loading} columns={queueColumns} dataSource={queue} pagination={{ pageSize: 8, hideOnSinglePage: true }} />,
          },
          {
            key: 'scans', label: `扫描记录 (${scans.length})`,
            children: <Table rowKey="id" size="small" loading={loading} columns={scanColumns} dataSource={scans} pagination={{ pageSize: 10, hideOnSinglePage: true }} />,
          },
          {
            key: 'audit', label: `下载审计 (${audits.length})`,
            children: (
              <>
                <div className="sec-toolbar"><span className="sec-muted">每次下载都会签发唯一交付水印（DL-xxxx），若发现技能包被外泄，可凭水印定位到具体下载人与时间。</span></div>
                <Table rowKey="id" size="small" loading={loading} columns={auditColumns} dataSource={audits} pagination={{ pageSize: 10, hideOnSinglePage: true }} />
              </>
            ),
          },
          {
            key: 'rules', label: `规则库 (${rules.length})`,
            children: (
              <>
                <div className="sec-toolbar"><span className="sec-muted">规则库以数据文件交付（可独立升级、回滚、审计），不编入二进制，避免引擎自身被误报；共 {rules.length} 条规则、{engine.signatures || 1} 组病毒特征。</span></div>
                <Table rowKey="rule_id" size="small" loading={loading} columns={ruleColumns} dataSource={rules} pagination={{ pageSize: 12, hideOnSinglePage: true }} />
              </>
            ),
          },
        ]}
      />

      <Drawer className="sec-report" width={720} open={!!report} onClose={() => setReport(null)} destroyOnHidden
        title={<span className="sec-report-title"><SecurityScanOutlined /> 安全扫描报告 <code>{report?.id}</code></span>}>
        {report && (
          <div className="sec-report-body">
            <Descriptions size="small" column={2} bordered>
              <Descriptions.Item label="对象">{report.skill_name || report.target || report.skill_key}</Descriptions.Item>
              <Descriptions.Item label="结论"><VerdictTag verdict={report.verdict} /></Descriptions.Item>
              <Descriptions.Item label="风险分">{report.risk_score}</Descriptions.Item>
              <Descriptions.Item label="等级">{report.grade}</Descriptions.Item>
              <Descriptions.Item label="扫描文件">{report.files_scanned} 个 / {(report.bytes_scanned / 1024).toFixed(1)} KB</Descriptions.Item>
              <Descriptions.Item label="耗时">{report.duration_ms} ms</Descriptions.Item>
              <Descriptions.Item label="内容指纹"><code className="sec-muted">{(report.content_hash || '').slice(0, 32)}…</code></Descriptions.Item>
              <Descriptions.Item label="引擎">{report.engine_version}</Descriptions.Item>
            </Descriptions>
            {report.summary && <Alert className="sec-alert" type={report.verdict === 'safe' ? 'success' : report.verdict === 'malicious' ? 'error' : 'warning'} showIcon message={report.summary} />}
            {report.reupload?.suspected && (
              <Alert type="warning" showIcon className="sec-alert"
                message="查重命中：疑似复制他人技能"
                description={`与「${report.reupload.matched_skill_name}」相似度 ${(report.reupload.similarity * 100).toFixed(0)}% — ${report.reupload.reason}`} />
            )}
            <SandboxPanel sandbox={report.sandbox} />
            {(report.findings || []).length === 0
              ? <Empty description="未发现风险项" />
              : (
                <div className="sec-findings">
                  {report.findings.map((f, i) => (
                    <div key={i} className={`sec-finding sev-${f.severity}`}>
                      <div className="sec-finding-head">
                        <SeverityTag severity={f.severity} />
                        <code>{f.rule_id}</code>
                        <strong>{f.title}</strong>
                        {f.blocking && <Tag color="error">阻断级</Tag>}
                        {f.file && <span className="sec-muted">{f.file}{f.line ? ':' + f.line : ''}</span>}
                      </div>
                      <p>{f.detail}</p>
                      {f.evidence && <pre>{f.evidence}</pre>}
                    </div>
                  ))}
                </div>
              )}
          </div>
        )}
      </Drawer>

      <Drawer className="sec-report" width={620} open={!!provenance} onClose={() => setProvenance(null)} destroyOnHidden
        title={<span className="sec-report-title"><ScanOutlined /> 溯源档案</span>}>
        {provenance && (
          <Descriptions size="small" column={1} bordered>
            <Descriptions.Item label="技能">{provenance.skill_key || provenance.skill_id}</Descriptions.Item>
            <Descriptions.Item label="水印号"><code className="sec-watermark">{provenance.watermark_id}</code></Descriptions.Item>
            <Descriptions.Item label="内容指纹"><code>{(provenance.content_hash || '').slice(0, 48)}…</code></Descriptions.Item>
            <Descriptions.Item label="SimHash"><code>{provenance.sim_hash}</code></Descriptions.Item>
            <Descriptions.Item label="签名算法">{provenance.algorithm}</Descriptions.Item>
            <Descriptions.Item label="完整性状态">
              <Tag color={provenance.verify_status === 'valid' ? 'success' : provenance.verify_status === 'tampered' ? 'error' : 'default'}>
                {provenance.verify_status === 'valid' ? '有效（与签名一致）' : provenance.verify_status === 'tampered' ? '已篡改（内容哈希不符）' : '未签名'}
              </Tag>
            </Descriptions.Item>
            <Descriptions.Item label="下载次数">{provenance.download_count ?? 0}</Descriptions.Item>
            <Descriptions.Item label="签发交付包">{provenance.signed_packages ?? 0}</Descriptions.Item>
            <Descriptions.Item label="包签名"><pre className="sec-sig">{provenance.signature}</pre></Descriptions.Item>
          </Descriptions>
        )}
      </Drawer>
    </div>
  );
}

function ShieldBadge() {
  return <SafetyCertificateOutlined />;
}
