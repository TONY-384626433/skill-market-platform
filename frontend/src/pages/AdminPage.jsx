import React, { useEffect, useState } from 'react';
import { Alert, App as AntApp, Button, Empty, Input, Modal, Skeleton, Table, Tabs, Tag } from 'antd';
import {
  CheckOutlined, ClockCircleOutlined, FileSearchOutlined, ReloadOutlined,
  SafetyCertificateOutlined, StopOutlined, WarningOutlined,
} from '@ant-design/icons';
import { getAuditLogs, getReviewQueue, getStats, reviewSkill } from '../services/api';
import { formatDate, formatNumber } from '../utils/format';

function GovernanceMetric({ label, value, suffix, note, tone = '' }) {
  return <div className={`governance-metric ${tone}`}><span>{label}</span><strong>{formatNumber(value)}{suffix && <small>{suffix}</small>}</strong><em>{note}</em></div>;
}

export default function AdminPage() {
  const { message } = AntApp.useApp();
  const [stats, setStats] = useState({});
  const [queue, setQueue] = useState([]);
  const [logs, setLogs] = useState([]);
  const [loading, setLoading] = useState(true);
  const [reviewing, setReviewing] = useState(null);
  const [verdict, setVerdict] = useState('approve');
  const [comment, setComment] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const load = async () => {
    setLoading(true);
    const [statResult, queueResult, logResult] = await Promise.allSettled([
      getStats(), getReviewQueue(), getAuditLogs({ page: 1, page_size: 50 }),
    ]);
    if (statResult.status === 'fulfilled') setStats(statResult.value?.data || {});
    if (queueResult.status === 'fulfilled') setQueue(Array.isArray(queueResult.value?.data) ? queueResult.value.data : []);
    if (logResult.status === 'fulfilled') setLogs(Array.isArray(logResult.value?.data) ? logResult.value.data : []);
    if ([statResult, queueResult, logResult].some((result) => result.status === 'rejected')) message.error('部分治理数据加载失败');
    setLoading(false);
  };

  useEffect(() => { load(); }, []);

  const openReview = (skill, nextVerdict) => {
    setReviewing(skill);
    setVerdict(nextVerdict);
    setComment('');
  };

  const submitReview = async () => {
    if (verdict === 'reject' && !comment.trim()) return message.warning('驳回时必须填写整改原因');
    setSubmitting(true);
    try {
      await reviewSkill(reviewing.id, { verdict, comment: comment.trim() });
      message.success(verdict === 'approve' ? '技能已通过并发布' : '技能已驳回');
      setReviewing(null);
      await load();
    } catch (requestError) { message.error(requestError.response?.data?.error || '审核提交失败'); }
    finally { setSubmitting(false); }
  };

  const queueColumns = [
    { title: '技能', dataIndex: 'name', key: 'name', minWidth: 220, render: (value, row) => <div className="table-primary"><span className="table-icon"><FileSearchOutlined /></span><div><strong>{value}</strong><small>{row.skill_key} · v{row.version}</small></div></div> },
    { title: '分类', dataIndex: 'category', key: 'category', width: 110 },
    { title: '形态', dataIndex: 'skill_type', key: 'skill_type', width: 90, render: (value) => <Tag>{value?.toUpperCase()}</Tag> },
    { title: '提交人', dataIndex: 'author_name', key: 'author_name', width: 110 },
    { title: '提交时间', dataIndex: 'created_at', key: 'created_at', width: 150, render: (value) => formatDate(value, true) },
    { title: '简介', dataIndex: 'summary', key: 'summary', ellipsis: true },
    { title: '操作', key: 'actions', width: 180, fixed: 'right', render: (_, row) => <div className="table-actions"><Button type="link" icon={<CheckOutlined />} onClick={() => openReview(row, 'approve')}>通过</Button><Button type="link" danger icon={<StopOutlined />} onClick={() => openReview(row, 'reject')}>驳回</Button></div> },
  ];

  const logColumns = [
    { title: '时间', dataIndex: 'created_at', key: 'created_at', width: 155, render: (value) => formatDate(value, true) },
    { title: 'Trace ID', dataIndex: 'trace_id', key: 'trace_id', width: 180, render: (value) => <code className="trace-id">{value || '-'}</code> },
    { title: '用户', dataIndex: 'user_id', key: 'user_id', width: 130, ellipsis: true },
    { title: '技能', dataIndex: 'skill_id', key: 'skill_id', width: 120 },
    { title: '方法', dataIndex: 'method', key: 'method', width: 110, render: (value) => <Tag>{value}</Tag> },
    { title: '结果', dataIndex: 'response_status', key: 'response_status', width: 100, render: (value) => <Tag color={value === 'success' ? 'success' : 'error'}>{value === 'success' ? '成功' : '失败'}</Tag> },
    { title: '耗时', dataIndex: 'duration_ms', key: 'duration_ms', width: 90, render: (value) => `${value || 0} ms` },
    { title: 'PII', dataIndex: 'pii_detected', key: 'pii_detected', width: 90, render: (value) => value ? <Tag color="warning">已识别</Tag> : <span className="muted">无</span> },
  ];

  return (
    <div className="page">
      <section className="page-heading"><div><h1>平台治理</h1><p>管理发布审核、运行质量、合规风险和全链路调用审计。</p></div><Button icon={<ReloadOutlined />} onClick={load}>刷新数据</Button></section>

      <section className="governance-metrics">
        <GovernanceMetric label="待审核" value={stats.pending_reviews} suffix=" 项" note="需要人工复核" tone="warning" />
        <GovernanceMetric label="调用成功率" value={stats.success_rate} suffix="%" note="近 30 日" tone="success" />
        <GovernanceMetric label="平均响应" value={stats.avg_duration_ms} suffix=" ms" note="全量技能" />
        <GovernanceMetric label="敏感信息识别" value={stats.pii_blocked} suffix=" 次" note="近 30 日" tone="danger" />
      </section>

      <Alert className="governance-alert" type="info" showIcon icon={<SafetyCertificateOutlined />} message="治理策略运行中" description="所有技能发布需通过格式校验、安全扫描、功能验证和人工审核；运行调用会记录 Trace ID 与合规识别结果。" />

      {loading ? <Skeleton active paragraph={{ rows: 10 }} /> : <Tabs className="workspace-tabs" items={[
        { key: 'review', label: `审核队列 (${queue.length})`, children: <section className="workspace-section flush-section"><div className="section-heading"><div><h2>待人工复核</h2><p>核查功能边界、接入地址、权限声明与责任归属。</p></div></div><Table rowKey="id" dataSource={queue} columns={queueColumns} pagination={false} scroll={{ x: 1000 }} locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前没有待审核技能" /> }} /></section> },
        { key: 'audit', label: `调用审计 (${logs.length})`, children: <section className="workspace-section flush-section"><div className="section-heading"><div><h2>最近调用记录</h2><p>查看身份、技能、执行状态、耗时和敏感信息识别结果。</p></div></div><Table rowKey="id" dataSource={logs} columns={logColumns} pagination={{ pageSize: 15, showSizeChanger: false }} scroll={{ x: 1000 }} locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无调用审计记录" /> }} /></section> },
      ]} />}

      <Modal open={Boolean(reviewing)} title={verdict === 'approve' ? '确认通过并发布' : '驳回技能提交'} onCancel={() => setReviewing(null)} footer={[<Button key="cancel" onClick={() => setReviewing(null)}>取消</Button>, <Button key="submit" type="primary" danger={verdict === 'reject'} loading={submitting} onClick={submitReview}>{verdict === 'approve' ? '确认通过' : '确认驳回'}</Button>]}>
        {reviewing && <div className="review-modal"><div className="review-target"><strong>{reviewing.name}</strong><span>{reviewing.skill_key} · v{reviewing.version}</span></div>{verdict === 'approve' ? <Alert type="success" showIcon message="通过后将立即进入技能市场，对全行用户可见。" /> : <Alert type="warning" showIcon icon={<WarningOutlined />} message="请提供可执行的整改意见，开发者将在发布记录中看到。" />}<label>{verdict === 'approve' ? '审核备注（选填）' : '驳回原因'}</label><Input.TextArea value={comment} onChange={(event) => setComment(event.target.value)} rows={4} maxLength={300} showCount placeholder={verdict === 'approve' ? '记录核验结论' : '说明未通过项和整改要求'} /><p><ClockCircleOutlined /> 审核结论会记录审核人和时间。</p></div>}
      </Modal>
    </div>
  );
}
