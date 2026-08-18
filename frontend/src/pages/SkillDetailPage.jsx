import React, { useContext, useEffect, useMemo, useState } from 'react';
import {
  Alert, App as AntApp, Button, Descriptions, Empty, Input, List, Modal, Rate, Skeleton, Space, Tabs, Tag,
} from 'antd';
import {
  ArrowLeftOutlined, CheckCircleOutlined, ClockCircleOutlined, CodeOutlined, CopyOutlined,
  DownloadOutlined, LockOutlined, PlayCircleOutlined, SafetyCertificateOutlined, StarFilled, TeamOutlined,
} from '@ant-design/icons';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import ReactMarkdown from 'react-markdown';
import { UserContext } from '../App';
import SkillVisual from '../components/SkillVisual';
import { getSkillDetail, getSkillRatings, installSkill, invokeSkill, rateSkill } from '../services/api';
import { formatDate, formatNumber, safeJson } from '../utils/format';

const playTemplates = {
  'db-inspection': { target_db: 'core-banking-db-01', check_scope: 'full' },
  'log-desensitization': { log_content: '客户手机号 13800138000，身份证号 110101199003078888' },
  'alert-convergence': { host: 'payment-node-01', time_range_minutes: 60 },
  'requirement-analysis': { title: '余额查询', description: '支持实时余额与昨日余额对比' },
};

function CodeBlock({ children, onCopy }) {
  return <div className="code-block"><Button type="text" icon={<CopyOutlined />} onClick={onCopy}>复制</Button><pre>{children}</pre></div>;
}

export default function SkillDetailPage() {
	const { message } = AntApp.useApp();
  const { id } = useParams();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const { user } = useContext(UserContext);
  const [skill, setSkill] = useState(null);
  const [ratings, setRatings] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [installing, setInstalling] = useState(false);
  const [token, setToken] = useState('');
  const [playParams, setPlayParams] = useState('{}');
  const [playResult, setPlayResult] = useState(null);
  const [playing, setPlaying] = useState(false);
  const [ratingValue, setRatingValue] = useState(0);
  const [ratingComment, setRatingComment] = useState('');
  const [activeTab, setActiveTab] = useState(() => searchParams.get('tab') || 'overview');

  const load = async () => {
    setLoading(true);
    setError('');
    try {
      const [skillResponse, ratingResponse] = await Promise.all([getSkillDetail(id), getSkillRatings(id, { page: 1, page_size: 20 })]);
      if (!skillResponse?.data) throw new Error('技能详情数据格式异常');
      setSkill(skillResponse.data);
      setRatings(Array.isArray(ratingResponse?.data) ? ratingResponse.data : []);
      setPlayParams(JSON.stringify(playTemplates[skillResponse.data.skill_key] || { input: '请输入测试参数' }, null, 2));
      try {
        const recent = JSON.parse(localStorage.getItem('skillhub_recent_skills')) || [];
        const next = [{ id: skillResponse.data.id, name: skillResponse.data.name, category: skillResponse.data.category }, ...recent.filter((item) => String(item.id) !== String(skillResponse.data.id))].slice(0, 6);
        localStorage.setItem('skillhub_recent_skills', JSON.stringify(next));
      } catch { /* recent history is optional */ }
    } catch (requestError) {
      setError(requestError.response?.data?.error || requestError.message || '详情加载失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, [id]);
  useEffect(() => setActiveTab(searchParams.get('tab') || 'overview'), [searchParams]);

  const permissions = useMemo(() => safeJson(skill?.permissions, []), [skill?.permissions]);
  const manifest = useMemo(() => safeJson(skill?.manifest, {}), [skill?.manifest]);
  const interfaceSpec = useMemo(() => {
    const direct = safeJson(skill?.interface_spec, {});
    return Object.keys(direct).length ? direct : (manifest.interface || {});
  }, [skill?.interface_spec, manifest]);

  const copy = async (value) => {
    try { await navigator.clipboard.writeText(value); message.success('已复制到剪贴板'); } catch { message.error('复制失败，请手动选择内容'); }
  };

  const requireLogin = () => {
    if (user) return true;
    message.info('请先登录企业账号');
    navigate('/login', { state: { from: `/skills/${id}` } });
    return false;
  };

  const handleInstall = async () => {
    if (!requireLogin()) return;
    setInstalling(true);
    try {
      const response = await installSkill(id, skill.version);
      setToken(response.api_token || '');
      message.success('技能已安装');
      setSkill((current) => ({ ...current, install_count: Number(current.install_count || 0) + 1 }));
    } catch (requestError) {
      message.error(requestError.response?.data?.error || '安装失败');
    } finally { setInstalling(false); }
  };

  const handlePlay = async () => {
    if (!requireLogin()) return;
    let params;
    try { params = JSON.parse(playParams); } catch { message.error('请输入合法的 JSON 参数'); return; }
    setPlaying(true);
    setPlayResult(null);
    try {
      setPlayResult(await invokeSkill(skill.skill_key, 'execute', params));
    } catch (requestError) {
      setPlayResult({ status: 'failed', error: requestError.response?.data?.error || '调用失败' });
    } finally { setPlaying(false); }
  };

  const submitRating = async () => {
    if (!requireLogin() || !ratingValue) return message.warning('请先选择评分');
    try {
      await rateSkill(id, { rating: ratingValue, title: '', comment: ratingComment });
      message.success('评价已提交');
      setRatingComment('');
      await load();
    } catch (requestError) { message.error(requestError.response?.data?.error || '评价提交失败'); }
  };

  if (loading) return <div className="page"><Skeleton active paragraph={{ rows: 12 }} /></div>;
  if (error || !skill) return <div className="state-panel full-height"><Empty description={error || '技能不存在'} /><Space><Button onClick={() => navigate('/')}>返回市场</Button><Button type="primary" onClick={load}>重新加载</Button></Space></div>;

  const curl = `curl -X POST https://skillhub.internal/api/v1/gateway/invoke \\\n+  -H "Authorization: Bearer <企业访问令牌>" \\\n+  -H "Content-Type: application/json" \\\n+  -d '{"skill_key":"${skill.skill_key}","method":"execute","params":{}}'`;

  const changeTab = (key) => {
    setActiveTab(key);
    const next = new URLSearchParams(searchParams);
    if (key === 'overview') next.delete('tab'); else next.set('tab', key);
    setSearchParams(next, { replace: true });
  };

  const tabItems = [
    {
      key: 'overview', label: '能力概览', children: (
        <div className="detail-prose">
          <h2>能力说明</h2>
          <p>{skill.description || skill.summary}</p>
          <h2>适用场景</h2>
          <ul><li>在标准化工作流中复用该能力，减少重复建设。</li><li>通过统一网关调用，保留身份、参数摘要与执行结果审计。</li><li>适用于已获得对应数据和系统权限的内部应用。</li></ul>
          <h2>输入与输出</h2>
          {Object.keys(interfaceSpec).length ? <CodeBlock onCopy={() => copy(JSON.stringify(interfaceSpec, null, 2))}>{JSON.stringify(interfaceSpec, null, 2)}</CodeBlock> : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="维护方暂未提供结构化接口定义" />}
        </div>
      ),
    },
    {
      key: 'api', label: '接口文档', children: (
        <div className="detail-prose">
          <Alert type="info" showIcon message="调用前需要先安装技能，安装令牌仅在首次安装后显示一次。" />
          <h2>HTTP 调用示例</h2>
          <CodeBlock onCopy={() => copy(curl)}>{curl}</CodeBlock>
          <h2>接口约束</h2>
          <Descriptions bordered size="small" column={{ xs: 1, md: 2 }} items={[
            { key: 'protocol', label: '接入协议', children: (skill.endpoint_protocol || 'HTTP').toUpperCase() },
            { key: 'auth', label: '认证方式', children: 'Bearer Token' },
            { key: 'timeout', label: '默认超时', children: '30 秒' },
            { key: 'trace', label: '链路追踪', children: '自动生成 Trace ID' },
          ]} />
        </div>
      ),
    },
    {
      key: 'playground', label: '在线试玩', children: (
        <div className="playground">
          <div className="playground-head"><div><h2>测试请求</h2><p>请求将由统一网关发送到真实技能服务，并写入审计日志。</p></div><Tag color="success">服务可用</Tag></div>
          <label>调用参数（JSON）</label>
          <Input.TextArea value={playParams} onChange={(event) => setPlayParams(event.target.value)} autoSize={{ minRows: 7, maxRows: 14 }} spellCheck={false} />
          <Button type="primary" icon={<PlayCircleOutlined />} loading={playing} onClick={handlePlay}>{playing ? '执行中' : '执行技能'}</Button>
          {playResult && <div className={`play-result ${playResult.status === 'success' ? 'success' : 'error'}`}><div><strong>{playResult.status === 'success' ? '执行成功' : '执行失败'}</strong>{playResult.duration_ms != null && <span><ClockCircleOutlined /> {playResult.duration_ms} ms</span>}{playResult.trace_id && <code>{playResult.trace_id}</code>}</div>{playResult.status === 'success' ? <ReactMarkdown>{playResult.data?.result || JSON.stringify(playResult.data, null, 2)}</ReactMarkdown> : <p>{playResult.error}</p>}</div>}
        </div>
      ),
    },
    {
      key: 'ratings', label: `评价 (${ratings.length})`, children: (
        <div className="rating-section">
          <div className="rating-compose"><div><strong>使用体验</strong><Rate value={ratingValue} onChange={setRatingValue} /></div><Input.TextArea value={ratingComment} onChange={(event) => setRatingComment(event.target.value)} placeholder="说明使用场景、效果或改进建议" maxLength={300} showCount autoSize={{ minRows: 2, maxRows: 4 }} /><Button type="primary" onClick={submitRating}>提交评价</Button></div>
          <List dataSource={ratings} locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无评价" /> }} renderItem={(item) => <List.Item><List.Item.Meta avatar={<span className="rating-avatar">{item.user_name?.[0] || '用'}</span>} title={<Space><strong>{item.user_name || '企业用户'}</strong><Rate disabled value={item.rating} /><small>{formatDate(item.created_at, true)}</small></Space>} description={item.comment || item.title || '未填写文字评价'} /></List.Item>} />
        </div>
      ),
    },
  ];

  return (
    <div className="page detail-page">
      <Button className="back-button" type="text" icon={<ArrowLeftOutlined />} onClick={() => navigate('/')}>返回技能市场</Button>
      <section className="detail-header">
        <SkillVisual category={skill.category} size="lg" />
        <div className="detail-title"><div className="detail-badges"><Tag color="blue">{skill.category}</Tag><Tag>{skill.skill_type?.toUpperCase()}</Tag><Tag color={skill.stability === 'stable' ? 'success' : 'warning'}>{skill.stability === 'stable' ? '稳定版本' : '测试版本'}</Tag></div><h1>{skill.name}</h1><p>{skill.summary}</p><div className="detail-inline-meta"><span><StarFilled className="rating-star" /> {Number(skill.rating_avg || 0).toFixed(1)} ({skill.rating_count || 0})</span><span><DownloadOutlined /> {formatNumber(skill.install_count)} 次安装</span><span><TeamOutlined /> {skill.team_name || skill.author_name}</span><span>v{skill.version}</span></div></div>
        <div className="detail-header-actions"><Button icon={<PlayCircleOutlined />} onClick={() => changeTab('playground')}>在线试玩</Button><Button type="primary" icon={<DownloadOutlined />} loading={installing} onClick={handleInstall}>安装技能</Button></div>
        <Button className="detail-install-mobile" type="primary" icon={<DownloadOutlined />} loading={installing} onClick={handleInstall}>安装</Button>
      </section>

      <div className="detail-layout">
        <main className="detail-main"><Tabs items={tabItems} activeKey={activeTab} onChange={changeTab} /></main>
        <aside className="detail-aside">
          <Button type="primary" size="large" block icon={<DownloadOutlined />} loading={installing} onClick={handleInstall}>安装此技能</Button>
          <p className="install-note">安装后生成独立访问令牌，可随时在“我的技能”中撤销。</p>
          <div className="aside-section"><h3>治理信息</h3><dl><div><dt>发布状态</dt><dd><span className="status-dot online" /> 已发布</dd></div><div><dt>稳定性</dt><dd>{skill.stability === 'stable' ? '生产稳定' : '公开测试'}</dd></div><div><dt>可见范围</dt><dd>企业内部</dd></div><div><dt>更新时间</dt><dd>{formatDate(skill.updated_at)}</dd></div></dl></div>
          <div className="aside-section"><h3>权限与合规</h3>{Array.isArray(permissions) && permissions.length ? permissions.map((permission, index) => { const label = typeof permission === 'object' ? (permission.resource || permission.name || JSON.stringify(permission)) : String(permission); return <Tag key={`${label}-${index}`}>{label}</Tag>; }) : <p className="muted-line"><LockOutlined /> 无额外权限声明</p>}<p className="compliance-note"><SafetyCertificateOutlined /> 调用由企业网关鉴权并留存审计记录。</p></div>
          <div className="aside-section"><h3>维护方</h3><strong>{skill.team_name || skill.author_name || '平台团队'}</strong><p className="muted-line">负责人：{skill.author_name || '未指定'}</p></div>
        </aside>
      </div>

      <Modal open={Boolean(token)} title={<span><CheckCircleOutlined className="success-icon" /> 安装成功</span>} onCancel={() => setToken('')} footer={<Button type="primary" onClick={() => { copy(token); setToken(''); }}>复制并关闭</Button>}>
        <Alert type="warning" showIcon message="请立即保存访问令牌" description="出于安全考虑，关闭后不再显示完整令牌。" />
        <div className="token-box"><code>{token}</code><Button type="text" icon={<CopyOutlined />} onClick={() => copy(token)} /></div>
      </Modal>
    </div>
  );
}
