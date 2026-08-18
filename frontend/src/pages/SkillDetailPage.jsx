// ============================================================
// 技能详情页 — 科技感重构版
// ============================================================
import React, { useState, useEffect, useContext } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Button, Tag, Spin, message, Modal, Rate,
  List, Space, Typography, Tabs, Alert, Descriptions, Empty
} from 'antd';
import {
  DownloadOutlined, StarFilled, ClockCircleOutlined,
  CheckCircleOutlined, CopyOutlined, CodeOutlined,
  BookOutlined, ApiOutlined, SafetyCertificateOutlined,
  ThunderboltOutlined, RocketOutlined, ShareAltOutlined,
  PlayCircleOutlined, BugOutlined
} from '@ant-design/icons';
import { getSkillDetail, installSkill, rateSkill, getSkillRatings, invokeSkill } from '../services/api';
import { UserContext } from '../App';
import ReactMarkdown from 'react-markdown';

const { Title, Paragraph, Text } = Typography;

const categoryGradient = {
  '智能运维': 'linear-gradient(135deg, #06b6d4, #3b82f6)',
  '研发效能': 'linear-gradient(135deg, #8b5cf6, #d946ef)',
  '安全合规': 'linear-gradient(135deg, #10b981, #06b6d4)',
  '数据治理': 'linear-gradient(135deg, #f59e0b, #ef4444)',
  '智能客服': 'linear-gradient(135deg, #f472b6, #8b5cf6)',
  '风控分析': 'linear-gradient(135deg, #f43f5e, #f59e0b)',
};

// 在线试玩默认参数模板
const PLAYGROUND_TEMPLATES = {
  'db-inspection': {
    params: { target_db: 'core-banking-db-01', check_scope: 'full' },
    hint: '支持实例: core-banking-db-01 / payment-db-02 / risk-control-db-01',
  },
  'log-desensitization': {
    params: { log_content: '用户张三的身份证号是110101199003078888, 手机号13800138000' },
    hint: '演示: 身份证号 / 手机号 / 银行卡号自动检测并脱敏',
  },
  'alert-convergence': {
    params: { host: 'payment-node-01', time_range_minutes: 60 },
    hint: '演示: 对 payment-node-01 最近 1 小时告警做收敛分析',
  },
  'requirement-analysis': {
    params: { title: '余额查询', description: '实现余额查询功能, 支持实时余额与昨日余额对比' },
    hint: '演示: AI 需求分析, 输出结构化分析结果',
  },
};

export default function SkillDetailPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const { user } = useContext(UserContext);

  const [skill, setSkill] = useState(null);
  const [loading, setLoading] = useState(true);
  const [installing, setInstalling] = useState(false);
  const [tokenModal, setTokenModal] = useState({ open: false, token: '' });
  const [ratings, setRatings] = useState([]);
  const [userRating, setUserRating] = useState(0);
  const [playParams, setPlayParams] = useState('');
  const [playResult, setPlayResult] = useState(null);
  const [playLoading, setPlayLoading] = useState(false);

  useEffect(() => {
    fetchSkill();
    fetchRatings();
  }, [id]);

  // 加载试玩默认参数
  useEffect(() => {
    if (skill?.skill_key) {
      const tpl = PLAYGROUND_TEMPLATES[skill.skill_key];
      setPlayParams(JSON.stringify(tpl?.params || { text: '请输入参数' }, null, 2));
      setPlayResult(null);
    }
  }, [skill?.skill_key]);

  const fetchSkill = async () => {
    setLoading(true);
    try {
      const res = await getSkillDetail(id);
      setSkill(res.data);
    } catch (e) {
      message.error('获取技能详情失败');
    }
    setLoading(false);
  };

  const fetchRatings = async () => {
    try {
      const res = await getSkillRatings(id, { page: 1, page_size: 20 });
      setRatings(res.data || []);
    } catch (e) {}
  };

  const handleInstall = async () => {
    if (!user) {
      message.warning('请先登录');
      return navigate('/login');
    }
    setInstalling(true);
    try {
      const res = await installSkill(id);
      setTokenModal({ open: true, token: res.api_token });
      message.success('安装成功!');
      fetchSkill();
    } catch (e) {
      message.error(e.response?.data?.error || '安装失败');
    }
    setInstalling(false);
  };

  const handleRate = async (rating) => {
    if (!user) return message.warning('请先登录');
    try {
      await rateSkill(id, { rating, title: '', comment: '' });
      setUserRating(rating);
      message.success('评价成功');
      fetchSkill();
      fetchRatings();
    } catch (e) {
      message.error('评价失败');
    }
  };

  const copyToken = () => {
    navigator.clipboard.writeText(tokenModal.token);
    message.success('Token 已复制到剪贴板');
  };

  // 在线试玩: 真实调用技能
  const handlePlay = async () => {
    if (!user) {
      message.warning('请先登录后试玩');
      return navigate('/login');
    }
    let params;
    try {
      params = JSON.parse(playParams || '{}');
    } catch (e) {
      return message.error('参数不是合法 JSON, 请检查格式');
    }
    setPlayLoading(true);
    setPlayResult(null);
    try {
      const res = await invokeSkill(skill.skill_key, 'execute', params);
      setPlayResult(res);
      message.success(`调用成功! 耗时 ${res.duration_ms}ms`);
    } catch (e) {
      message.error(e.response?.data?.error || '调用失败');
      setPlayResult({ status: 'failed', error: e.response?.data?.error || '调用失败', duration_ms: 0 });
    }
    setPlayLoading(false);
  };

  if (loading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '50vh' }}>
        <Spin size="large" style={{ color: 'var(--cyan)' }} />
      </div>
    );
  }
  if (!skill) return <div style={{ textAlign: 'center', marginTop: 100, color: 'var(--text-secondary)' }}>技能不存在</div>;

  const grad = categoryGradient[skill.category] || 'linear-gradient(135deg, #64748b, #334155)';

  return (
    <div style={{ maxWidth: 1180, margin: '0 auto' }}>
      {/* ========== 头部信息 ========== */}
      <div className="glass-card detail-hero fade-up">
        <div className="big-icon" style={{ background: grad }}>⚡</div>
        <div style={{ flex: 1 }}>
          <Space size={8} wrap style={{ marginBottom: 10 }}>
            <span className="nexus-tag nexus-tag-cyan">{skill.category}</span>
            <span className="nexus-tag nexus-tag-violet">{skill.skill_type?.toUpperCase()}</span>
            {skill.stability === 'stable' ? (
              <span className="nexus-tag nexus-tag-green">STABLE</span>
            ) : (
              <span className="nexus-tag nexus-tag-gold">BETA</span>
            )}
            <span className="nexus-tag nexus-tag-dim">v{skill.version}</span>
          </Space>
          <h1 style={{ fontSize: 30, fontWeight: 800, margin: '0 0 8px', color: 'var(--text-primary)' }}>
            {skill.name}
          </h1>
          <Paragraph style={{ color: 'var(--text-secondary)', fontSize: 15, marginBottom: 16 }}>
            {skill.summary}
          </Paragraph>
          <div>
            <span className="detail-stat"><StarFilled style={{ color: 'var(--gold)' }} /> {skill.rating_avg?.toFixed(1)} ({skill.rating_count} 评价)</span>
            <span className="detail-stat"><DownloadOutlined /> {skill.install_count} 次安装</span>
            <span className="detail-stat"><ThunderboltOutlined /> 作者: {skill.author_name}</span>
            <span className="detail-stat"><ShareAltOutlined /> 团队: {skill.team_name || '-'}</span>
          </div>
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 12, justifyContent: 'center', minWidth: 160 }}>
          <Button
            className="glow-btn"
            size="large"
            icon={<RocketOutlined />}
            onClick={handleInstall}
            loading={installing}
            style={{ height: 46 }}
          >
            一键安装
          </Button>
          <Button
            className="glow-btn glow-btn-ghost"
            icon={<CopyOutlined />}
            onClick={() => message.info('收藏功能即将上线')}
          >
            收藏
          </Button>
        </div>
      </div>

      {/* ========== 详细内容 Tabs ========== */}
      <div className="glass-card fade-up" style={{ padding: '8px 28px 28px', animationDelay: '0.1s' }}>
        <Tabs
          className="nexus-tabs"
          defaultActiveKey="overview"
          items={[
            {
              key: 'overview',
              label: <span><BookOutlined /> 概述</span>,
              children: (
                <div>
                  <h4 style={{ color: 'var(--cyan-bright)', fontWeight: 700, marginBottom: 12 }}>
                    <span style={{ color: 'var(--cyan)' }}>▸</span> 技能描述
                  </h4>
                  <Paragraph style={{ color: 'var(--text-secondary)', fontSize: 14, lineHeight: 1.9 }}>
                    {skill.description || skill.summary}
                  </Paragraph>

                  <h4 style={{ color: 'var(--cyan-bright)', fontWeight: 700, margin: '28px 0 12px' }}>
                    <span style={{ color: 'var(--cyan)' }}>▸</span> 接口定义
                  </h4>
                  <Descriptions
                    className="nexus-descriptions"
                    bordered
                    column={{ xs: 1, sm: 2 }}
                    size="small"
                  >
                    <Descriptions.Item label="接入协议">{skill.endpoint_protocol || 'MCP'}</Descriptions.Item>
                    <Descriptions.Item label="接入地址">
                      <code style={{ color: 'var(--cyan-bright)', background: 'rgba(34,211,238,0.08)', padding: '2px 8px', borderRadius: 6, fontSize: 12 }}>
                        {skill.endpoint_url || '(安装后自动生成)'}
                      </code>
                    </Descriptions.Item>
                    <Descriptions.Item label="认证方式">API Key (Bearer Token)</Descriptions.Item>
                    <Descriptions.Item label="超时时间">30s</Descriptions.Item>
                  </Descriptions>

                  <h4 style={{ color: 'var(--cyan-bright)', fontWeight: 700, margin: '28px 0 12px' }}>
                    <span style={{ color: 'var(--cyan)' }}>▸</span> 权限声明
                  </h4>
                  <pre className="nexus-code">
                    {skill.permissions || '无特殊权限要求'}
                  </pre>
                </div>
              ),
            },
            {
              key: 'api',
              label: <span><CodeOutlined /> 快速开始</span>,
              children: (
                <div>
                  <h4 style={{ color: 'var(--cyan-bright)', fontWeight: 700, marginBottom: 12 }}>
                    <span style={{ color: 'var(--cyan)' }}>▸</span> 方式一: 在 Dify 中使用
                  </h4>
                  <Paragraph style={{ color: 'var(--text-secondary)' }}>
                    安装后技能自动注册为 Dify 自定义工具, 在工作流中拖拽即可使用。
                  </Paragraph>
                  <Alert
                    className="nexus-alert"
                    type="info"
                    message="安装后会自动生成 API Token, 在 Dify 工具配置中填入即可。"
                    showIcon
                  />

                  <h4 style={{ color: 'var(--cyan-bright)', fontWeight: 700, margin: '28px 0 12px' }}>
                    <span style={{ color: 'var(--cyan)' }}>▸</span> 方式二: API 调用
                  </h4>
                  <pre className="nexus-code">{`curl -X POST https://skill-hub.internal/api/v1/gateway/invoke \\
  -H "Authorization: Bearer <YOUR_TOKEN>" \\
  -H "Content-Type: application/json" \\
  -d '{
    "skill_key": "${skill.skill_key}",
    "method": "execute",
    "params": {
      "target": "your-param"
    }
  }'`}</pre>

                  <h4 style={{ color: 'var(--cyan-bright)', fontWeight: 700, margin: '28px 0 12px' }}>
                    <span style={{ color: 'var(--cyan)' }}>▸</span> 方式三: Python SDK 调用
                  </h4>
                  <pre className="nexus-code">{`from skillhub import SkillClient

client = SkillClient(token="<YOUR_TOKEN>")
result = client.invoke("${skill.skill_key}", method="execute", params={
    "target": "your-param"
})
print(result)`}</pre>
                </div>
              ),
            },
            {
              key: 'ratings',
              label: <span><StarFilled /> 评价 ({ratings.length})</span>,
              children: (
                <div>
                  <div style={{ marginBottom: 24, padding: 20, borderRadius: 12, background: 'rgba(13,20,44,0.5)', border: '1px solid rgba(148,163,184,0.12)', display: 'flex', alignItems: 'center', gap: 16 }}>
                    <Text style={{ color: 'var(--text-secondary)', fontWeight: 600 }}>你的评分:</Text>
                    <Rate value={userRating} onChange={handleRate} style={{ fontSize: 20 }} />
                    {userRating > 0 && (
                      <Text style={{ color: 'var(--cyan-bright)', fontFamily: 'var(--font-mono)' }}>
                        ★ {userRating.toFixed(1)} / 5.0
                      </Text>
                    )}
                  </div>
                  <List
                    className="nexus-list"
                    dataSource={ratings}
                    locale={{ emptyText: <Empty description="暂无评价, 快来抢沙发!" /> }}
                    renderItem={(item) => (
                      <List.Item>
                        <List.Item.Meta
                          avatar={
                            <div className="rate-avatar">{item.user_name?.[0]}</div>
                          }
                          title={
                            <Space size={12}>
                              <Rate disabled value={item.rating} style={{ fontSize: 14 }} />
                              <Text style={{ color: 'var(--text-primary)', fontWeight: 600 }}>{item.user_name}</Text>
                              <Text style={{ color: 'var(--text-dim)', fontSize: 12, fontFamily: 'var(--font-mono)' }}>{item.created_at}</Text>
                            </Space>
                          }
                          description={<Text style={{ color: 'var(--text-secondary)' }}>{item.comment || item.title}</Text>}
                        />
                      </List.Item>
                    )}
                  />
                </div>
              ),
            },
            {
              key: 'playground',
              label: <span><PlayCircleOutlined /> 在线试玩</span>,
              children: (
                <div>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 14 }}>
                    <span className="nexus-tag nexus-tag-green" style={{ fontSize: 12 }}>● LIVE</span>
                    <Text style={{ color: 'var(--text-dim)', fontSize: 12.5 }}>
                      真实调用技能服务 · 参数将发送至 MCP Server 执行
                    </Text>
                  </div>
                  <div style={{ marginBottom: 14 }}>
                    <Text style={{ color: 'var(--cyan-bright)', fontWeight: 600, fontSize: 13, display: 'block', marginBottom: 8 }}>
                      ▸ 调用参数 (JSON)
                    </Text>
                    <Input.TextArea
                      className="nexus-input"
                      value={playParams}
                      onChange={(e) => setPlayParams(e.target.value)}
                      rows={5}
                      style={{ fontFamily: 'var(--font-mono)', fontSize: 12.5, borderRadius: 10 }}
                    />
                    <Text style={{ color: 'var(--text-dim)', fontSize: 11.5, display: 'block', marginTop: 6 }}>
                      {PLAYGROUND_TEMPLATES[skill.skill_key]?.hint || ''}
                    </Text>
                  </div>
                  <Button
                    className="glow-btn glow-btn-violet"
                    icon={<BugOutlined />}
                    loading={playLoading}
                    onClick={handlePlay}
                    style={{ height: 42, padding: '0 30px', marginBottom: 20 }}
                  >
                    {playLoading ? '技能执行中...' : '▶ 执行技能'}
                  </Button>

                  {playResult && (
                    <div style={{ borderRadius: 14, border: playResult.status === 'success' ? '1px solid rgba(52,211,153,0.25)' : '1px solid rgba(244,63,94,0.3)', background: 'rgba(8,13,30,0.7)', padding: '18px 22px' }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 14, marginBottom: 14, flexWrap: 'wrap' }}>
                        <span className={`nexus-tag ${playResult.status === 'success' ? 'nexus-tag-green' : 'nexus-tag-pink'}`}>
                          {playResult.status === 'success' ? '● SUCCESS' : '● FAILED'}
                        </span>
                        {playResult.duration_ms > 0 && (
                          <Text style={{ color: 'var(--text-dim)', fontFamily: 'var(--font-mono)', fontSize: 12 }}>
                            <ClockCircleOutlined /> 耗时 {playResult.duration_ms}ms
                          </Text>
                        )}
                        {playResult.trace_id && (
                          <Text style={{ color: 'var(--text-dim)', fontFamily: 'var(--font-mono)', fontSize: 12 }}>
                            trace: {playResult.trace_id}
                          </Text>
                        )}
                        {playResult.data?.tool && (
                          <Text style={{ color: 'var(--text-dim)', fontFamily: 'var(--font-mono)', fontSize: 12 }}>
                            tool: {playResult.data.tool}
                          </Text>
                        )}
                      </div>
                      {playResult.status === 'success' ? (
                        <div style={{ fontSize: 13.5, lineHeight: 1.8, color: 'var(--text-secondary)' }}>
                          <ReactMarkdown>{playResult.data?.result || ''}</ReactMarkdown>
                        </div>
                      ) : (
                        <Text style={{ color: '#f87171', fontSize: 13 }}>{playResult.error}</Text>
                      )}
                    </div>
                  )}
                </div>
              ),
            },
          ]}
        />
      </div>

      {/* ========== 安装成功 Token 弹窗 ========== */}
      <Modal
        className="nexus-modal"
        title={<><CheckCircleOutlined style={{ color: 'var(--green)', marginRight: 8 }} /> 安装成功</>}
        open={tokenModal.open}
        onCancel={() => setTokenModal({ open: false, token: '' })}
        footer={[
          <Button key="close" className="glow-btn glow-btn-ghost" onClick={() => setTokenModal({ open: false, token: '' })}>关闭</Button>,
          <Button key="copy" className="glow-btn" icon={<CopyOutlined />} onClick={copyToken}>复制 Token</Button>,
        ]}
      >
        <Alert
          className="nexus-alert"
          type="warning"
          message="请立即保存 Token，关闭后不再显示!"
          showIcon
          style={{ marginBottom: 16, borderColor: 'rgba(251,191,36,0.3)' }}
        />
        <div className="token-box">{tokenModal.token}</div>
        <Paragraph style={{ color: 'var(--text-dim)', marginTop: 14, fontSize: 12.5 }}>
          <SafetyCertificateOutlined /> 可在 Dify 工具配置或 API 调用中使用此 Token。
        </Paragraph>
      </Modal>
    </div>
  );
}
