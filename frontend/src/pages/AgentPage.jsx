import React, { useEffect, useMemo, useRef, useState } from 'react';
import { Alert, App as AntApp, Button, Empty, Input, Skeleton, Tag, Tooltip } from 'antd';
import {
  ApiOutlined, ClockCircleOutlined, ExperimentOutlined, ReloadOutlined,
  RobotOutlined, SendOutlined, ThunderboltOutlined, ToolOutlined, UserOutlined,
} from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';
import { getAgentStatus, getAgentTools, sendAgentMessage } from '../services/api';
import { formatNumber } from '../utils/format';

const QUICK_PROMPTS = [
  '巡检一下核心银行数据库 core-banking-db-01',
  '分析支付节点最近的告警并给出根因建议',
  '帮我把这段日志脱敏：身份证 110101199003078888 手机号 13800138000',
  '把这条需求整理成结构化分析：客户线上申请对公开户',
];

function StepCard({ step }) {
  const statusMeta = { success: { color: 'success', label: '调用成功' }, failed: { color: 'error', label: '调用失败' }, blocked: { color: 'warning', label: '安全拦截' } }[step.status] || { color: 'default', label: step.status };
  const args = step.args && Object.keys(step.args).length ? JSON.stringify(step.args, null, 2) : '{}';
  const [open, setOpen] = useState(false);
  return (
    <div className={`agent-step ${step.status}`}>
      <div className="agent-step-head">
        <span className="agent-step-index">{step.step}</span>
        <strong>{step.skill_name || step.skill_key}</strong>
        <code>{step.tool_name}</code>
        <Tag color={statusMeta.color}>{statusMeta.label}</Tag>
        <span className="agent-step-time"><ClockCircleOutlined /> {step.duration_ms}ms</span>
        <Button type="link" size="small" onClick={() => setOpen((v) => !v)}>{open ? '收起' : '调用详情'}</Button>
      </div>
      {open && <div className="agent-step-body">
        <div className="agent-step-label">入参</div>
        <pre>{args}</pre>
        {step.trace_id && <><div className="agent-step-label">Trace ID</div><pre>{step.trace_id}</pre></>}
        <div className="agent-step-label">返回结果</div>
        <pre className="agent-step-result">{step.result || '(空)'}</pre>
      </div>}
    </div>
  );
}

function Bubble({ role, children }) {
  return (
    <div className={`agent-bubble ${role}`}>
      <span className="agent-avatar">{role === 'user' ? <UserOutlined /> : <RobotOutlined />}</span>
      <div className="agent-bubble-body">{children}</div>
    </div>
  );
}

export default function AgentPage() {
  const { message } = AntApp.useApp();
  const [status, setStatus] = useState({});
  const [tools, setTools] = useState([]);
  const [loading, setLoading] = useState(true);
  const [input, setInput] = useState('');
  const [sending, setSending] = useState(false);
  const [messages, setMessages] = useState([
    { role: 'assistant', content: '我是 SkillHub 智能体。用自然语言描述需求，我会自动挑选平台上**已通过可用性审核**的技能来执行，并把调用过程与结果一并展示。', steps: [] },
  ]);
  const scroller = useRef(null);

  const load = async () => {
    setLoading(true);
    const [statusResult, toolsResult] = await Promise.allSettled([getAgentStatus(), getAgentTools()]);
    if (statusResult.status === 'fulfilled') setStatus(statusResult.value?.data || {});
    if (toolsResult.status === 'fulfilled') setTools(Array.isArray(toolsResult.value?.data) ? toolsResult.value.data : []);
    if ([statusResult, toolsResult].some((r) => r.status === 'rejected')) message.error('智能体信息加载失败，请确认已登录且后端在线');
    setLoading(false);
  };

  useEffect(() => { load(); }, []);

  useEffect(() => {
    if (scroller.current) scroller.current.scrollTop = scroller.current.scrollHeight;
  }, [messages, sending]);

  const groupedTools = useMemo(() => {
    const map = new Map();
    tools.forEach((tool) => {
      if (!map.has(tool.skill_key)) map.set(tool.skill_key, { name: tool.skill_name, category: tool.category, tools: [] });
      map.get(tool.skill_key).tools.push(tool);
    });
    return Array.from(map.entries());
  }, [tools]);

  const send = async (text) => {
    const question = (text ?? input).trim();
    if (!question || sending) return;
    const history = messages.filter((m) => m.role !== 'system').slice(-6).map((m) => ({ role: m.role, content: m.content }));
    setMessages((prev) => [...prev, { role: 'user', content: question }]);
    setInput('');
    setSending(true);
    try {
      const res = await sendAgentMessage(question, history);
      const data = res?.data || {};
      setMessages((prev) => [...prev, { role: 'assistant', content: data.answer || '(无回复)', steps: data.steps || [], meta: data }]);
    } catch (error) {
      const detail = error.response?.data?.error || error.message || '调用失败';
      setMessages((prev) => [...prev, { role: 'assistant', content: `⚠ 智能体调用失败：${detail}`, steps: [], failed: true }]);
    } finally { setSending(false); }
  };

  const isLLM = status.mode === 'llm';

  return (
    <div className="page agent-page">
      <section className="page-heading">
        <div><h1>AI 智能体</h1><p>自然语言描述需求 → 自动挑选技能 → 真实调用并汇总结果。全部调用走统一网关鉴权与审计。</p></div>
        <Button icon={<ReloadOutlined />} onClick={load}>刷新</Button>
      </section>

      <section className="agent-metrics">
        <div className={`agent-mode ${isLLM ? 'llm' : 'local'}`}>
          <ThunderboltOutlined />
          <div><strong>{isLLM ? '大模型编排模式' : '本地意图引擎'}</strong><small>{isLLM ? `模型 ${status.model}` : '未配置 LLM_API_KEY，已自动降级为关键词意图路由'}</small></div>
        </div>
        <div className="agent-stat"><span><ToolOutlined /></span><div><strong>{formatNumber(tools.length)}</strong><small>可调度工具</small></div></div>
        <div className="agent-stat"><span><ApiOutlined /></span><div><strong>{formatNumber(status.skill_count)}</strong><small>已过审技能</small></div></div>
        <div className="agent-stat"><span><ExperimentOutlined /></span><div><strong>v{status.engine_version || '-'}</strong><small>编排引擎版本</small></div></div>
      </section>

      <Alert className="governance-alert" type="info" showIcon icon={<RobotOutlined />}
        message="智能体只能调度已通过可用性审核的技能"
        description="工具清单由技能服务真实 tools/list 生成；每次调用都会执行敏感信息拦截、写入审计日志（来源标记 agent），并计入技能调用量。" />

      {loading ? <Skeleton active paragraph={{ rows: 10 }} /> : (
        <div className="agent-layout">
          <section className="agent-chat">
            <div className="agent-stream" ref={scroller}>
              {messages.map((item, index) => (
                <Bubble key={index} role={item.role}>
                  {item.role === 'assistant'
                    ? <div className="agent-markdown"><ReactMarkdown>{item.content}</ReactMarkdown></div>
                    : <p className="agent-user-text">{item.content}</p>}
                  {item.steps?.length > 0 && <div className="agent-steps">{item.steps.map((step, i) => <StepCard key={i} step={step} />)}</div>}
                  {item.meta && <div className="agent-foot">
                    <Tooltip title="本次编排使用的引擎与耗时"><span><ThunderboltOutlined /> {item.meta.provider === 'llm' ? '大模型编排' : '本地意图引擎'} · {item.meta.model}</span></Tooltip>
                    <span>{item.meta.tool_calls} 次技能调用 · {item.meta.duration_ms}ms</span>
                  </div>}
                </Bubble>
              ))}
              {sending && <Bubble role="assistant"><div className="agent-thinking"><i /><i /><i /> 正在编排技能…</div></Bubble>}
            </div>

            <div className="agent-quick">
              {QUICK_PROMPTS.map((prompt) => <button key={prompt} type="button" onClick={() => send(prompt)} disabled={sending}>{prompt}</button>)}
            </div>

            <div className="agent-input">
              <Input.TextArea value={input} onChange={(event) => setInput(event.target.value)} autoSize={{ minRows: 1, maxRows: 4 }}
                placeholder="描述你的需求，例如：巡检一下支付数据库，顺便看看最近的慢查询"
                onPressEnter={(event) => { if (!event.shiftKey) { event.preventDefault(); send(); } }} />
              <Button type="primary" icon={<SendOutlined />} loading={sending} onClick={() => send()}>发送</Button>
            </div>
          </section>

          <aside className="agent-aside">
            <div className="aside-section">
              <h3>可调度能力</h3>
              {groupedTools.length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无可调度技能" /> : groupedTools.map(([key, group]) => (
                <div key={key} className="agent-tool-group">
                  <div className="agent-tool-head"><strong>{group.name}</strong><Tag>{group.category}</Tag></div>
                  {group.tools.map((tool) => <Tooltip key={tool.name} title={tool.description} placement="left">
                    <div className="agent-tool-item"><code>{tool.tool}</code><small>{tool.description?.slice(0, 38) || '—'}</small></div>
                  </Tooltip>)}
                </div>
              ))}
            </div>
            <div className="aside-section">
              <h3>编排规则</h3>
              <ul className="agent-rules">
                <li>仅调度<strong>已发布且通过可用性审核</strong>的技能</li>
                <li>每轮最多 {status.max_rounds || 4} 轮工具调用</li>
                <li>敏感输入自动拦截（脱敏类技能白名单放行）</li>
                <li>调用链全程留痕，可在「平台治理」查看审计</li>
                <li>{isLLM ? '当前由大模型自主决策调用哪些工具' : '当前由本地意图引擎按关键词路由技能'}</li>
              </ul>
            </div>
          </aside>
        </div>
      )}
    </div>
  );
}
