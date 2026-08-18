// ============================================================
// 管理后台 — 科技感重构版
// ============================================================
import React, { useState, useEffect } from 'react';
import { Table, Tag, Typography } from 'antd';
import {
  BarChartOutlined, AuditOutlined,
  RiseOutlined, TeamOutlined, ApiOutlined, StarOutlined,
  DatabaseOutlined, WarningOutlined
} from '@ant-design/icons';
import { getStats, getAuditLogs } from '../services/api';
import useCountUp from '../components/useCountUp';

const { Text } = Typography;

// ============ 发光统计卡 ============
function AdminStatCard({ title, value, suffix, gradient, icon, delay }) {
  const animated = useCountUp(value);
  return (
    <div className="glass-card stat-card fade-up" style={{ animationDelay: `${delay}s` }}>
      <div className="stat-icon" style={{ background: gradient }}>{icon}</div>
      <div className="stat-title">{title.toUpperCase()}</div>
      <div className="stat-value" style={{ background: gradient, WebkitBackgroundClip: 'text', WebkitTextFillColor: 'transparent' }}>
        {typeof value === 'number' ? animated : value}
        {suffix && <span style={{ fontSize: 15, marginLeft: 4, WebkitTextFillColor: 'var(--text-dim)' }}>{suffix}</span>}
      </div>
    </div>
  );
}

export default function AdminPage() {
  const [stats, setStats] = useState({});
  const [auditLogs, setAuditLogs] = useState([]);
  const [auditTotal, setAuditTotal] = useState(0);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    getStats().then(res => setStats(res.data)).catch(() => {});
    fetchAuditLogs();
  }, []);

  const fetchAuditLogs = async (userId = '') => {
    setLoading(true);
    try {
      const res = await getAuditLogs({ user_id: userId, page: 1, page_size: 50 });
      setAuditLogs(res.data || []);
      setAuditTotal(res.total || 0);
    } catch (e) {}
    setLoading(false);
  };

  const auditColumns = [
    {
      title: 'TRACE ID', dataIndex: 'trace_id', key: 'trace_id', width: 150, ellipsis: true,
      render: (text) => <Text style={{ color: 'var(--cyan-bright)', fontFamily: 'var(--font-mono)', fontSize: 11.5 }}>{text?.slice(0, 16)}...</Text>,
    },
    { title: '用户', dataIndex: 'user_id', key: 'user_id', width: 90 },
    { title: '技能', dataIndex: 'skill_id', key: 'skill_id', width: 110, ellipsis: true },
    {
      title: '方法', dataIndex: 'method', key: 'method', width: 100,
      render: (m) => <Tag className="nexus-tag nexus-tag-violet" style={{ fontFamily: 'var(--font-mono)' }}>{m}</Tag>,
    },
    {
      title: '状态', dataIndex: 'response_status', key: 'status', width: 85,
      render: (s) => s === 'success'
        ? <Tag className="nexus-tag nexus-tag-green">● SUCCESS</Tag>
        : <Tag className="nexus-tag nexus-tag-pink">● FAILED</Tag>,
    },
    {
      title: '耗时', dataIndex: 'duration_ms', key: 'duration', width: 75,
      render: (v) => <Text style={{ color: v > 2000 ? 'var(--gold)' : 'var(--text-secondary)', fontFamily: 'var(--font-mono)' }}>{v}ms</Text>,
    },
    { title: 'TOKEN', dataIndex: 'tokens_used', key: 'tokens', width: 70, render: (v) => <Text style={{ fontFamily: 'var(--font-mono)' }}>{v}</Text> },
    {
      title: 'PII', dataIndex: 'pii_detected', key: 'pii', width: 60,
      render: (v) => v ? <Tag className="nexus-tag nexus-tag-pink">是</Tag> : <Tag className="nexus-tag nexus-tag-dim">否</Tag>,
    },
    { title: '来源 IP', dataIndex: 'source_ip', key: 'ip', width: 120, render: (v) => <Text style={{ fontFamily: 'var(--font-mono)', fontSize: 11.5 }}>{v}</Text> },
    { title: '时间', dataIndex: 'created_at', key: 'time', width: 160, render: (v) => <Text style={{ fontFamily: 'var(--font-mono)', fontSize: 11.5 }}>{v}</Text> },
  ];

  return (
    <div>
      <div className="fade-up">
        <div className="page-title">
          <span className="title-icon"><BarChartOutlined /></span>
          管理后台
        </div>
        <p className="page-subtitle">
          全链路运营监控 · 审计追踪 · 系统治理
        </p>
      </div>

      {/* ========== 核心指标 ========== */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 16, marginBottom: 24 }}>
        <AdminStatCard title="技能总数" value={stats.total_skills || 0} suffix="个" icon={<ApiOutlined />} gradient="linear-gradient(135deg, #06b6d4, #3b82f6)" delay={0.05} />
        <AdminStatCard title="累计安装" value={stats.total_installs || 0} suffix="次" icon={<RiseOutlined />} gradient="linear-gradient(135deg, #10b981, #06b6d4)" delay={0.12} />
        <AdminStatCard title="月活用户" value={stats.monthly_active_users || 0} suffix="人" icon={<TeamOutlined />} gradient="linear-gradient(135deg, #8b5cf6, #d946ef)" delay={0.19} />
        <AdminStatCard title="平均评分" value={stats.avg_rating || 0} suffix="/5" icon={<StarOutlined />} gradient="linear-gradient(135deg, #f59e0b, #ef4444)" delay={0.26} />
      </div>

      {/* ========== 审计日志 ========== */}
      <div className="glass-card fade-up" style={{ padding: '8px 24px 24px', animationDelay: '0.3s' }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '16px 0 12px', borderBottom: '1px solid rgba(34,211,238,0.1)', marginBottom: 16 }}>
          <h4 style={{ margin: 0, color: 'var(--cyan-bright)', fontWeight: 700 }}>
            <AuditOutlined style={{ marginRight: 8, color: 'var(--cyan)' }} />
            全链路审计日志
          </h4>
          <Text style={{ color: 'var(--text-dim)', fontFamily: 'var(--font-mono)', fontSize: 12 }}>
            <DatabaseOutlined /> 共 <span style={{ color: 'var(--cyan-bright)' }}>{auditTotal}</span> 条记录
          </Text>
        </div>
        <Table
          className="nexus-table"
          dataSource={auditLogs}
          columns={auditColumns}
          rowKey="id"
          loading={loading}
          size="small"
          scroll={{ x: 1200 }}
          pagination={{
            pageSize: 20,
            className: 'nexus-pagination',
            showSizeChanger: false,
            showTotal: (t) => <Text style={{ color: 'var(--text-dim)', fontSize: 12 }}>共 {t} 条</Text>,
          }}
        />
      </div>
    </div>
  );
}
