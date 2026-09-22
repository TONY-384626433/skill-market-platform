import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Alert, Button, Card, Col, Input, Progress, Row, Select, Space, Table, Tag, Tooltip, message } from 'antd';
import {
  ApiOutlined, CloudServerOutlined, DeploymentUnitOutlined, ReloadOutlined,
  RobotOutlined, SafetyCertificateOutlined, SendOutlined, ThunderboltOutlined,
} from '@ant-design/icons';
import { getAgentProviders, getAgentStatus, compareProviders, sendAgentMessage } from '../services/api';

const KIND_LABEL = { openai: 'OpenAI 兼容', anthropic: 'Anthropic 协议' };

function fmtTime(unixSec) {
  if (!unixSec) return '—';
  try { return new Date(unixSec * 1000).toLocaleTimeString('zh-CN', { hour12: false }); } catch { return '—'; }
}

export default function ProvidersPage() {
  const [providers, setProviders] = useState([]);
  const [summary, setSummary] = useState({});
  const [status, setStatus] = useState({});
  const [loading, setLoading] = useState(true);
  const [q, setQ] = useState('帮我做一次数据库巡检');
  const [provider, setProvider] = useState('');
  const [chat, setChat] = useState(null);
  const [sending, setSending] = useState(false);
  const [cmpMsg, setCmpMsg] = useState('用一句话说明什么是数据脱敏');
  const [cmp, setCmp] = useState(null);
  const [cmping, setCmping] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [p, s] = await Promise.all([getAgentProviders(), getAgentStatus()]);
      setProviders(p?.data || []);
      setSummary(p?.summary || {});
      setStatus(s?.data || {});
    } catch (err) {
      message.error('加载厂商信息失败：' + (err?.response?.data?.error || err?.message || err));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); const timer = setInterval(load, 15000); return () => clearInterval(timer); }, [load]);

  const stats = status?.provider_stats || { providers: [], total_calls: 0, total_failures: 0 };
  const statMap = useMemo(
    () => Object.fromEntries((stats.providers || []).map((item) => [item.provider, item])),
    [stats],
  );

  const send = async () => {
    if (!q.trim() || sending) return;
    setSending(true);
    setChat(null);
    try {
      const res = await sendAgentMessage(q.trim(), [], provider || undefined);
      setChat(res?.data || res);
      load();
    } catch (err) {
      message.error('调用失败：' + (err?.response?.data?.error || err?.message));
    } finally {
      setSending(false);
    }
  };

  const doCompare = async () => {
    if (!cmpMsg.trim() || cmping) return;
    setCmping(true);
    setCmp(null);
    try {
      const res = await compareProviders(cmpMsg.trim(), []);
      const items = (res?.data || []).slice().sort((a, b) => (a.latency_ms || 0) - (b.latency_ms || 0));
      setCmp(items);
      load();
    } catch (err) {
      message.error('对比失败：' + (err?.response?.data?.error || err?.message));
    } finally {
      setCmping(false);
    }
  };

  const columns = [
    {
      title: '厂商', dataIndex: 'label', key: 'label',
      render: (label, row) => (
        <Space direction="vertical" size={0}>
          <Space size={6}>
            <span className="provider-dot" style={{ background: row.configured ? '#07c160' : '#c9d1d9' }} />
            <strong>{label}</strong>
          </Space>
          <small style={{ color: '#8c9bb0' }}>{row.key}</small>
        </Space>
      ),
    },
    { title: '协议', dataIndex: 'kind', key: 'kind', render: (k) => <Tag color={k === 'anthropic' ? 'purple' : 'blue'}>{KIND_LABEL[k] || k}</Tag> },
    { title: '模型', dataIndex: 'model', key: 'model', render: (m) => <code>{m}</code> },
    {
      title: '状态', dataIndex: 'configured', key: 'configured',
      render: (ok) => (ok ? <Tag color="success">已配置</Tag> : <Tag>未配置</Tag>),
    },
    {
      title: '调用', key: 'calls',
      render: (_, row) => { const s = statMap[row.key]; return s ? <strong>{s.calls}</strong> : <span style={{ color: '#b9c3d1' }}>0</span>; },
    },
    {
      title: '成功率', key: 'rate', width: 160,
      render: (_, row) => {
        const s = statMap[row.key];
        if (!s || !s.calls) return <span style={{ color: '#b9c3d1' }}>—</span>;
        const pct = Math.round(s.success_rate);
        return (
          <Tooltip title={`${s.success} 成功 / ${s.failures} 失败`}>
            <Progress percent={pct} size="small" status={pct >= 99 ? 'success' : pct >= 60 ? 'normal' : 'exception'} />
          </Tooltip>
        );
      },
    },
    {
      title: '平均耗时', key: 'avg', width: 100,
      render: (_, row) => { const s = statMap[row.key]; return s && s.calls ? `${s.avg_ms} ms` : '—'; },
    },
    {
      title: '最近错误', key: 'err',
      render: (_, row) => { const s = statMap[row.key]; return s?.last_error ? <Tooltip title={s.last_error}><Tag color="error">有错误</Tag></Tooltip> : <span style={{ color: '#b9c3d1' }}>—</span>; },
    },
    { title: '最后使用', key: 'last', render: (_, row) => { const s = statMap[row.key]; return s?.last_used_at ? fmtTime(s.last_used_at) : '—'; } },
  ];

  const active = summary.active || {};

  return (
    <div className="providers-page">
      <div className="page-lead">
        <div>
          <span className="page-eyebrow"><DeploymentUnitOutlined /> MODEL PROVIDERS</span>
          <h1>大模型厂商监控</h1>
          <p>接入的多家大模型、配置状态、调用成功率与耗时；任一厂商失败会自动切换候选链中的下一家，全部失败才降级本地意图引擎。</p>
        </div>
        <Button icon={<ReloadOutlined />} onClick={load} loading={loading}>刷新</Button>
      </div>

      <Row gutter={[16, 16]} className="providers-kpi">
        <Col xs={12} md={6}><Card><Space><CloudServerOutlined style={{ fontSize: 22, color: '#0b63ce' }} /><div><small>厂商总数</small><div className="kpi-value">{summary.count ?? providers.length}</div></div></Space></Card></Col>
        <Col xs={12} md={6}><Card><Space><SafetyCertificateOutlined style={{ fontSize: 22, color: '#07c160' }} /><div><small>已配置</small><div className="kpi-value">{summary.configured_count ?? 0}</div></div></Space></Card></Col>
        <Col xs={12} md={6}><Card><Space><RobotOutlined style={{ fontSize: 22, color: '#722ed1' }} /><div><small>当前激活</small><div className="kpi-value kpi-small">{active.label || '本地意图引擎'}</div><small>{active.model || 'intent-router-v1'}</small></div></Space></Card></Col>
        <Col xs={12} md={6}><Card><Space><ThunderboltOutlined style={{ fontSize: 22, color: '#fa8c16' }} /><div><small>累计调用</small><div className="kpi-value">{stats.total_calls ?? 0}</div><small>失败 {stats.total_failures ?? 0} 次</small></div></Space></Card></Col>
      </Row>

      <Card className="providers-table-card" title={<Space><ApiOutlined /> 厂商明细与调用统计</Space>}>
        <Table rowKey="key" size="middle" loading={loading} columns={columns} dataSource={providers} pagination={false} scroll={{ x: 980 }} />
      </Card>

      <Card className="providers-chat-card" title={<Space><SendOutlined /> 厂商连通性测试（真实对话）</Space>}>
        <Space wrap>
          <Select
            style={{ width: 240 }}
            value={provider || ''}
            onChange={setProvider}
            options={[{ value: '', label: '自动择优（默认）' }, ...providers.filter((p) => p.configured).map((p) => ({ value: p.key, label: `${p.label} · ${p.model}` }))]}
          />
          <Input style={{ width: 420 }} value={q} onChange={(e) => setQ(e.target.value)} onPressEnter={send} placeholder="输入一句话，指定厂商调用 Agent" />
          <Button type="primary" icon={<SendOutlined />} loading={sending} onClick={send}>发送</Button>
        </Space>
        {chat && (
          <div className="provider-chat-result">
            <Space wrap>
              <Tag color={chat.provider === 'llm' ? 'success' : 'warning'}>引擎 {chat.provider}</Tag>
              {chat.active_provider && <Tag color="blue">实际厂商 {chat.active_provider}</Tag>}
              {chat.model && <Tag>{chat.model}</Tag>}
              <Tag>工具调用 {chat.tool_calls ?? 0}</Tag>
              <Tag>{chat.duration_ms} ms</Tag>
              {chat.fallback && <Tag color="error">已降级</Tag>}
            </Space>
            {(chat.steps || []).length > 0 && (
              <div className="provider-chat-steps">
                {chat.steps.map((s, i) => (
                  <div key={i} className="provider-step">
                    <Tag color={s.status === 'success' ? 'green' : 'red'}>{s.status}</Tag>
                    <code>{s.skill_key}/{s.tool_name}</code>
                    <span className="sec-muted">{s.duration_ms} ms</span>
                  </div>
                ))}
              </div>
            )}
            <pre className="provider-chat-answer">{chat.answer}</pre>
          </div>
        )}
        {(summary.configured_count ?? 0) === 0 && (
          <Alert style={{ marginTop: 12 }} type="warning" showIcon message="尚未配置任何大模型 API Key" description="设置环境变量（如 DEEPSEEK_API_KEY / OPENAI_API_KEY / ANTHROPIC_API_KEY）并重启后端即可点亮；未配置时 Agent 自动使用本地意图引擎。" />
        )}
      </Card>

      <Card className="providers-cmp-card" title={<Space><ThunderboltOutlined /> 多厂商并行对比（同一问题并发问多家）</Space>}>
        <Space wrap>
          <Input style={{ width: 460 }} value={cmpMsg} onChange={(e) => setCmpMsg(e.target.value)} onPressEnter={doCompare} placeholder="输入一个问题，并发对比所有已配置厂商" />
          <Button type="primary" icon={<ThunderboltOutlined />} loading={cmping} onClick={doCompare}>开始对比</Button>
        </Space>
        {cmp && (
          <div className="providers-cmp-list">
            {cmp.map((it, i) => (
              <div key={i} className={`provider-cmp-item ${it.ok ? 'ok' : 'fail'}`}>
                <div className="provider-cmp-head">
                  <Space wrap>
                    <Tag color={i === 0 ? 'gold' : 'default'}>{i === 0 && it.ok ? '⚡ 最快 ' : ''}{it.latency_ms} ms</Tag>
                    <strong>{it.label}</strong>
                    <code>{it.model}</code>
                    <Tag color={it.kind === 'anthropic' ? 'purple' : 'blue'}>{it.kind}</Tag>
                    {!it.ok && <Tag color="error">失败</Tag>}
                  </Space>
                </div>
                <pre className="provider-cmp-answer">{it.ok ? it.answer : ('错误: ' + (it.error || ''))}</pre>
              </div>
            ))}
          </div>
        )}
      </Card>
    </div>
  );
}
