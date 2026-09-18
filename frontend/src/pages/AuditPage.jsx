import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Alert, App as AntApp, Button, Descriptions, Drawer, Empty, Input, Modal, Progress,
  Skeleton, Space, Table, Tabs, Tag, Tooltip,
} from 'antd';
import {
  CheckCircleOutlined, ClockCircleOutlined, ExperimentOutlined, FileSearchOutlined,
  ReloadOutlined, SafetyCertificateOutlined, StopOutlined, ThunderboltOutlined, WarningOutlined,
} from '@ant-design/icons';
import {
  getAuditOverview, getAuditQueue, getRecentAudits, reviewSkill, runSkillAudit,
} from '../services/api';
import { formatDate, formatNumber } from '../utils/format';

// ============================================================
// 审核引擎检查项说明 (与后端 AVAIL-01..06 一一对应)
// ============================================================
const CHECK_CATALOG = [
  { code: 'AVAIL-01', name: '元数据完整性', category: '静态', level: '致命', desc: '校验 skill_key 命名规范、名称/简介/分类、语义化版本、标签与责任人是否完整。' },
  { code: 'AVAIL-02', name: '接口定义可解析', category: '静态', level: '致命', desc: '校验接入形态、接入地址协议、manifest 是否为合法 JSON 且声明了输入输出契约。' },
  { code: 'AVAIL-03', name: '服务可达与协议握手', category: '运行', level: '致命', desc: '真实发起 initialize + tools/list，确认服务在线、协议版本正常、工具契约完整。' },
  { code: 'AVAIL-04', name: '核心功能可调用', category: '运行', level: '致命', desc: '按工具 inputSchema 自动生成探针入参并真实调用，确认返回有效结果而非空响应。' },
  { code: 'AVAIL-05', name: '响应性能基线', category: '运行', level: '重要', desc: '连续 3 次真实调用，统计成功率与平均/峰值耗时，评估稳定性是否达标。' },
  { code: 'AVAIL-06', name: '安全合规基线', category: '安全', level: '重要', desc: '校验权限声明、输出是否含明文敏感信息，以及网关敏感输入拦截策略是否生效。' },
];

const STATUS_META = {
  passed: { color: 'success', label: '通过', icon: <CheckCircleOutlined /> },
  failed: { color: 'error', label: '不合格', icon: <StopOutlined /> },
  running: { color: 'processing', label: '检测中', icon: <ClockCircleOutlined /> },
  pending: { color: 'default', label: '待检测', icon: <ClockCircleOutlined /> },
};

const CHECK_STATUS_META = {
  passed: { color: 'success', label: '通过' },
  failed: { color: 'error', label: '失败' },
  warning: { color: 'warning', label: '告警' },
  skipped: { color: 'default', label: '跳过' },
};

const CATEGORY_META = { static: '静态', protocol: '协议', functional: '功能', performance: '性能', security: '安全' };

function statusTag(status) {
  const meta = STATUS_META[status] || STATUS_META.pending;
  return <Tag color={meta.color} icon={meta.icon}>{meta.label}</Tag>;
}

function scoreTone(score) {
  if (score === null || score === undefined) return '';
  if (score >= 90) return 'success';
  if (score >= 70) return 'warning';
  return 'danger';
}

function Metric({ label, value, suffix, note, tone = '' }) {
  return <div className={`governance-metric ${tone}`}><span>{label}</span><strong>{formatNumber(value)}{suffix && <small>{suffix}</small>}</strong><em>{note}</em></div>;
}

// 检查项明细表 (报告抽屉内使用)
function CheckList({ checks = [] }) {
  return (
    <div className="audit-checks">
      {checks.map((item) => {
        const meta = CHECK_STATUS_META[item.status] || CHECK_STATUS_META.skipped;
        return (
          <div key={item.code} className={`audit-check ${item.status}`}>
            <div className="audit-check-head">
              <span className="audit-check-code">{item.code}</span>
              <strong>{item.name}</strong>
              <Tag color={meta.color}>{meta.label}</Tag>
              <span className="audit-check-meta">{CATEGORY_META[item.category] || item.category} · 权重 {item.weight} · {item.duration_ms}ms</span>
              <span className={`audit-check-score ${scoreTone(item.score)}`}>{Math.round(item.score)} 分</span>
            </div>
            <p className="audit-check-detail">{item.detail}</p>
            {item.evidence && <pre className="audit-check-evidence">{item.evidence}</pre>}
          </div>
        );
      })}
    </div>
  );
}

function ReportBody({ audit }) {
  if (!audit) return null;
  return (
    <div className="audit-report">
      <div className="audit-report-head">
        <Progress type="dashboard" size={132} percent={Math.round(audit.score || 0)} strokeColor={audit.status === 'passed' ? '#35e3ae' : '#ff6489'} format={(p) => <span className="audit-score-value">{p}<small>分</small></span>} />
        <div className="audit-report-summary">
          <div className="audit-report-title"><strong>{audit.skill_name || audit.skill_key}</strong>{statusTag(audit.status)}{audit.grade && <Tag color="cyan">{audit.grade} 级</Tag>}</div>
          <p>{audit.summary}</p>
          <Descriptions size="small" column={2} items={[
            { key: 'key', label: '技能标识', children: <code>{audit.skill_key}</code> },
            { key: 'version', label: '版本', children: `v${audit.version}` },
            { key: 'trigger', label: '触发方式', children: audit.trigger_type === 'self_check' ? '开发者自检' : '管理员审核' },
            { key: 'operator', label: '发起人', children: audit.triggered_by_name || audit.triggered_by || '系统' },
            { key: 'checks', label: '检查项', children: `${audit.passed_checks}/${audit.total_checks} 通过 · 关键失败 ${audit.critical_failures}` },
            { key: 'duration', label: '检测耗时', children: `${audit.duration_ms} ms` },
            { key: 'time', label: '检测时间', children: formatDate(audit.created_at, true) },
            { key: 'engine', label: '引擎版本', children: `availability-audit v${audit.engine_version}` },
          ]} />
        </div>
      </div>
      <CheckList checks={audit.checks} />
    </div>
  );
}

export default function AuditPage() {
  const { message } = AntApp.useApp();
  const [overview, setOverview] = useState({});
  const [queue, setQueue] = useState([]);
  const [recent, setRecent] = useState([]);
  const [loading, setLoading] = useState(true);
  const [running, setRunning] = useState(null);
  const [report, setReport] = useState(null);
  const [reviewTarget, setReviewTarget] = useState(null);
  const [reviewVerdict, setReviewVerdict] = useState('approve');
  const [reviewComment, setReviewComment] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    const [ov, q, r] = await Promise.allSettled([getAuditOverview(), getAuditQueue(), getRecentAudits({ limit: 30 })]);
    if (ov.status === 'fulfilled') setOverview(ov.value?.data || {});
    if (q.status === 'fulfilled') setQueue(Array.isArray(q.value?.data) ? q.value.data : []);
    if (r.status === 'fulfilled') setRecent(Array.isArray(r.value?.data) ? r.value.data : []);
    if ([ov, q, r].some((x) => x.status === 'rejected')) message.error('部分审核数据加载失败');
    setLoading(false);
  }, [message]);

  useEffect(() => { load(); }, [load]);

  const runAudit = async (skill) => {
    setRunning(skill.id);
    try {
      const res = await runSkillAudit(skill.id);
      const audit = res?.data;
      setReport(audit);
      if (audit?.status === 'passed') message.success(`${skill.name} 可用性审核通过 (${audit.score} 分)`);
      else message.warning(`${skill.name} 可用性审核不合格: ${audit?.summary || ''}`);
      await load();
    } catch (error) {
      message.error(error.response?.data?.error || '审核执行失败, 请检查技能服务是否在线');
    } finally { setRunning(null); }
  };

  const openReview = (skill, verdict) => { setReviewTarget(skill); setReviewVerdict(verdict); setReviewComment(''); };

  const submitReview = async () => {
    if (reviewVerdict === 'reject' && !reviewComment.trim()) return message.warning('驳回时必须填写整改原因');
    setSubmitting(true);
    try {
      await reviewSkill(reviewTarget.id, { verdict: reviewVerdict, comment: reviewComment.trim() });
      message.success(reviewVerdict === 'approve' ? '技能已通过并发布' : '技能已驳回');
      setReviewTarget(null);
      await load();
    } catch (error) { message.error(error.response?.data?.error || '审核提交失败'); }
    finally { setSubmitting(false); }
  };

  const counts = useMemo(() => ({
    failed: queue.filter((item) => item.audit_status === 'failed').length,
    pendingAudit: queue.filter((item) => !['passed', 'failed'].includes(item.audit_status)).length,
  }), [queue]);

  const queueColumns = [
    { title: '技能', dataIndex: 'name', key: 'name', minWidth: 230, render: (value, row) => <div className="table-primary"><span className="table-icon"><FileSearchOutlined /></span><div><strong>{value}</strong><small>{row.skill_key} · v{row.version}</small></div></div> },
    { title: '分类', dataIndex: 'category', key: 'category', width: 110 },
    { title: '接入形态', dataIndex: 'skill_type', key: 'skill_type', width: 100, render: (value) => <Tag>{value?.toUpperCase()}</Tag> },
    { title: '发布状态', dataIndex: 'status', key: 'status', width: 120, render: (value) => <Tag color={value === 'published' ? 'success' : 'default'}>{value === 'published' ? '已上架' : value === 'pending_approval' ? '待发布' : value}</Tag> },
    { title: '审核结论', dataIndex: 'audit_status', key: 'audit_status', width: 110, render: (value) => statusTag(value) },
    { title: '综合得分', dataIndex: 'audit_score', key: 'audit_score', width: 110, render: (value, row) => value === undefined || value === null ? <span className="muted">未检测</span> : <span className={`audit-inline-score ${scoreTone(value)}`}>{Number(value).toFixed(1)}{row.audit_grade ? ` · ${row.audit_grade}` : ''}</span> },
    { title: '最近检测', dataIndex: 'last_audit_at', key: 'last_audit_at', width: 150, render: (value) => value ? formatDate(value, true) : <span className="muted">-</span> },
    { title: '操作', key: 'actions', width: 250, fixed: 'right', render: (_, row) => <div className="table-actions">
      <Tooltip title="真实调用技能, 检测协议/功能/性能/安全">
        <Button type="link" icon={<ExperimentOutlined />} loading={running === row.id} onClick={() => runAudit(row)}>运行检测</Button>
      </Tooltip>
      {row.audit_status === 'passed' && row.status === 'pending_approval' && <Button type="link" icon={<CheckCircleOutlined />} onClick={() => openReview(row, 'approve')}>通过</Button>}
      {row.status === 'pending_approval' && <Button type="link" danger icon={<StopOutlined />} onClick={() => openReview(row, 'reject')}>驳回</Button>}
    </div> },
  ];

  const recentColumns = [
    { title: '检测时间', dataIndex: 'created_at', key: 'created_at', width: 155, render: (value) => formatDate(value, true) },
    { title: '技能', dataIndex: 'skill_name', key: 'skill_name', minWidth: 200, render: (value, row) => <div className="table-primary"><div><strong>{value || row.skill_key}</strong><small>{row.skill_key} · v{row.version}</small></div></div> },
    { title: '触发方式', dataIndex: 'trigger_type', key: 'trigger_type', width: 110, render: (value) => value === 'self_check' ? '开发者自检' : '管理员审核' },
    { title: '发起人', dataIndex: 'triggered_by_name', key: 'triggered_by_name', width: 110, render: (value, row) => value || row.triggered_by || '系统' },
    { title: '结论', dataIndex: 'status', key: 'status', width: 100, render: (value) => statusTag(value) },
    { title: '得分', dataIndex: 'score', key: 'score', width: 100, render: (value, row) => <span className={`audit-inline-score ${scoreTone(value)}`}>{Number(value).toFixed(1)} · {row.grade}</span> },
    { title: '检查项', key: 'checks', width: 120, render: (_, row) => `${row.passed_checks}/${row.total_checks}` },
    { title: '耗时', dataIndex: 'duration_ms', key: 'duration_ms', width: 90, render: (value) => `${value} ms` },
    { title: '操作', key: 'actions', width: 110, fixed: 'right', render: (_, row) => <Button type="link" onClick={() => setReport(row)}>查看报告</Button> },
  ];

  return (
    <div className="page">
      <section className="page-heading">
        <div><h1>技能审核</h1><p>自动检测技能是否"真的可用"：协议握手、真实功能调用、性能基线、安全合规，检测合格才允许发布与安装。</p></div>
        <Space><Button icon={<ReloadOutlined />} onClick={load}>刷新数据</Button></Space>
      </section>

      <section className="governance-metrics audit-metrics">
        <Metric label="已通过" value={overview.passed} suffix=" 个" note="可发布 / 可安装" tone="success" />
        <Metric label="不合格" value={overview.failed} suffix=" 个" note="存在关键项失败" tone="danger" />
        <Metric label="待检测" value={overview.pending} suffix=" 个" note="尚未运行检测" tone="warning" />
        <Metric label="平均得分" value={overview.avg_score} suffix=" 分" note="通过技能平均" />
        <Metric label="检测通过率" value={overview.pass_rate} suffix="%" note={`累计 ${overview.total_audits || 0} 次检测`} tone="success" />
      </section>

      <Alert className="governance-alert" type="info" showIcon icon={<SafetyCertificateOutlined />}
        message="可用性审核门禁已启用"
        description="技能发布前必须先通过自动可用性检测；未通过的技能无法上架，也无法被安装。检测过程会真实调用技能服务，全过程写入审核流水。" />

      {loading ? <Skeleton active paragraph={{ rows: 10 }} /> : <Tabs className="workspace-tabs" items={[
        {
          key: 'queue',
          label: `审核队列 (${queue.length})`,
          children: <section className="workspace-section flush-section">
            <div className="section-heading"><div><h2>技能可用性检测</h2><p>不合格 {counts.failed} 个 · 待检测 {counts.pendingAudit} 个。点击「运行检测」会真实调用技能服务。</p></div></div>
            <Table rowKey="id" dataSource={queue} columns={queueColumns} pagination={false} scroll={{ x: 1400 }} locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无技能" /> }} />
          </section>,
        },
        {
          key: 'recent',
          label: `检测记录 (${recent.length})`,
          children: <section className="workspace-section flush-section">
            <div className="section-heading"><div><h2>最近检测流水</h2><p>每一次自动检测都会留痕，可追溯到发起人、检查项与结果证据。</p></div></div>
            <Table rowKey="id" dataSource={recent} columns={recentColumns} pagination={{ pageSize: 15, showSizeChanger: false }} scroll={{ x: 1200 }} locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无检测记录" /> }} />
          </section>,
        },
        {
          key: 'catalog',
          label: '检测项说明',
          children: <section className="workspace-section flush-section">
            <div className="section-heading"><div><h2>六类检查项</h2><p>加权评分（总分 100），存在"致命"项失败或总分低于 70 分即判定不合格。</p></div></div>
            <div className="audit-catalog">
              {CHECK_CATALOG.map((item) => <div key={item.code} className="audit-catalog-item">
                <div className="audit-catalog-head"><span className="audit-check-code">{item.code}</span><strong>{item.name}</strong><Tag>{item.category}</Tag><Tag color={item.level === '致命' ? 'error' : 'warning'}>{item.level}</Tag></div>
                <p>{item.desc}</p>
              </div>)}
            </div>
          </section>,
        },
      ]} />}

      <Drawer className="audit-drawer" width={820} open={Boolean(report)} onClose={() => setReport(null)}
        title={<span className="drawer-title"><ThunderboltOutlined /> 可用性审核报告</span>}>
        {report && <ReportBody audit={report} />}
      </Drawer>

      <Modal open={Boolean(reviewTarget)} title={reviewVerdict === 'approve' ? '确认通过并发布' : '驳回技能提交'}
        onCancel={() => setReviewTarget(null)}
        footer={[<Button key="cancel" onClick={() => setReviewTarget(null)}>取消</Button>, <Button key="submit" type="primary" danger={reviewVerdict === 'reject'} loading={submitting} onClick={submitReview}>{reviewVerdict === 'approve' ? '确认通过' : '确认驳回'}</Button>]}>
        {reviewTarget && <div className="review-modal">
          <div className="review-target"><strong>{reviewTarget.name}</strong><span>{reviewTarget.skill_key} · v{reviewTarget.version}</span></div>
          {reviewVerdict === 'approve'
            ? <Alert type="success" showIcon message={`可用性审核已通过（${reviewTarget.audit_score ?? '-'} 分），通过后立即进入技能市场。`} />
            : <Alert type="warning" showIcon icon={<WarningOutlined />} message="请提供可执行的整改意见，开发者将在发布记录中看到。" />}
          <label>{reviewVerdict === 'approve' ? '审核备注（选填）' : '驳回原因'}</label>
          <Input.TextArea value={reviewComment} onChange={(event) => setReviewComment(event.target.value)} rows={4} maxLength={300} showCount placeholder={reviewVerdict === 'approve' ? '记录核验结论' : '说明未通过项和整改要求'} />
          <p><ClockCircleOutlined /> 发布门禁：未通过可用性审核的技能无法发布。</p>
        </div>}
      </Modal>
    </div>
  );
}
