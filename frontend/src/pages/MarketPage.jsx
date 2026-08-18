// ============================================================
// 技能市场首页 — 科技感重构版
// ============================================================
import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { Input, Spin, Empty, Select } from 'antd';
import {
  SearchOutlined, DownloadOutlined, StarFilled,
  MonitorOutlined, CodeOutlined, SafetyOutlined,
  DatabaseOutlined, CustomerServiceOutlined, AlertOutlined,
  FireOutlined, CrownOutlined, RocketOutlined, AppstoreOutlined
} from '@ant-design/icons';
import { searchSkills, getCategories, getStats } from '../services/api';
import useCountUp from '../components/useCountUp';
import useTilt from '../components/useTilt';
import useTypewriter from '../components/useTypewriter';

const { Search } = Input;

// 分类图标映射（带渐变配色）
const categoryMeta = {
  '智能运维': { icon: <MonitorOutlined />, gradient: 'linear-gradient(135deg, #06b6d4, #3b82f6)' },
  '研发效能': { icon: <CodeOutlined />, gradient: 'linear-gradient(135deg, #8b5cf6, #d946ef)' },
  '安全合规': { icon: <SafetyOutlined />, gradient: 'linear-gradient(135deg, #10b981, #06b6d4)' },
  '数据治理': { icon: <DatabaseOutlined />, gradient: 'linear-gradient(135deg, #f59e0b, #ef4444)' },
  '智能客服': { icon: <CustomerServiceOutlined />, gradient: 'linear-gradient(135deg, #f472b6, #8b5cf6)' },
  '风控分析': { icon: <AlertOutlined />, gradient: 'linear-gradient(135deg, #f43f5e, #f59e0b)' },
};

const categoryOptions = [
  { label: '智能运维', value: '智能运维' },
  { label: '研发效能', value: '研发效能' },
  { label: '安全合规', value: '安全合规' },
  { label: '数据治理', value: '数据治理' },
  { label: '智能客服', value: '智能客服' },
  { label: '风控分析', value: '风控分析' },
];

// ============ 统计卡片 ============
function StatCard({ title, value, suffix, gradient, icon, delay }) {
  const animated = useCountUp(value);
  return (
    <div className="glass-card stat-card fade-up" style={{ animationDelay: `${delay}s` }}>
      <div className="stat-icon" style={{ background: gradient }}>
        {icon}
      </div>
      <div className="stat-title">{title.toUpperCase()}</div>
      <div className="stat-value" style={{ background: gradient, WebkitBackgroundClip: 'text', WebkitTextFillColor: 'transparent' }}>
        {animated}
        <span style={{ fontSize: 16, marginLeft: 4, WebkitTextFillColor: 'var(--text-dim)' }}>{suffix}</span>
      </div>
    </div>
  );
}

// ============ 技能卡片（3D 倾斜） ============
function SkillCard({ skill, onClick }) {
  const tilt = useTilt(6);
  const meta = categoryMeta[skill.category] || { icon: <AppstoreOutlined />, gradient: 'linear-gradient(135deg, #64748b, #334155)' };

  return (
    <div
      ref={tilt.ref}
      onMouseMove={tilt.onMouseMove}
      onMouseLeave={tilt.onMouseLeave}
      className="skill-card"
      onClick={onClick}
      style={{ animationDelay: '0.05s' }}
    >
      <div className="skill-icon" style={{ background: meta.gradient }}>
        {meta.icon}
      </div>
      <div className="skill-name">
        <span>{skill.name}</span>
        {skill.stability === 'stable' && <span className="nexus-tag nexus-tag-green">稳定</span>}
        {skill.stability === 'beta' && <span className="nexus-tag nexus-tag-gold">测试</span>}
      </div>
      <div className="skill-summary">
        {skill.summary?.slice(0, 60)}{skill.summary?.length > 60 ? '...' : ''}
      </div>
      <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginBottom: 12 }}>
        <span className="nexus-tag nexus-tag-cyan">{skill.category}</span>
        {(skill.tags || []).slice(0, 2).map((tag) => (
          <span key={tag} className="nexus-tag nexus-tag-dim">{tag}</span>
        ))}
      </div>
      <div className="skill-meta">
        <span><DownloadOutlined /> <span className="num">{skill.install_count}</span> 安装</span>
        <span><StarFilled style={{ color: 'var(--gold)' }} /> <span className="num">{skill.rating_avg?.toFixed(1)}</span></span>
        <span style={{ marginLeft: 'auto' }}>v{skill.version}</span>
      </div>
    </div>
  );
}

export default function MarketPage() {
  const navigate = useNavigate();
  const [skills, setSkills] = useState([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [stats, setStats] = useState({});
  const [selectedCategory, setSelectedCategory] = useState('');
  const [sortBy, setSortBy] = useState('installs');
  const typed = useTypewriter([
    '数据库智能巡检助手...',
    '日志敏感信息脱敏...',
    '告警收敛分析...',
    'AI 需求分析助手...',
  ]);

  useEffect(() => {
    fetchSkills();
    getStats().then(res => setStats(res.data)).catch(() => {});
  }, [selectedCategory, sortBy]);

  const fetchSkills = async (query = '') => {
    setLoading(true);
    try {
      const params = { query, category: selectedCategory, sort_by: sortBy, page: 1, page_size: 50 };
      const res = await searchSkills(params);
      setSkills(res.data || []);
      setTotal(res.total || 0);
    } catch (e) {
      console.error('获取技能列表失败', e);
    }
    setLoading(false);
  };

  const onSearch = (value) => fetchSkills(value);

  return (
    <div>
      {/* ========== Hero 区 ========== */}
      <div className="hero-section fade-up">
        <div className="hero-eyebrow">
          <span className="dot" />
          PRIVATE AI SKILL MARKETPLACE · 私域 AI 技能市场
        </div>
        <h1 className="hero-title">
          发现 <span className="gradient-text">AI 技能</span>，构建智能组织
        </h1>
        <p className="hero-subtitle">
          搜索、安装、复用 — 让 AI 能力在组织内自由流动
        </p>
        <div className="hero-terminal">
          <span style={{ color: 'var(--cyan)' }}>$</span> skill-nexus search
          <span style={{ color: 'var(--text-dim)' }}> --query</span> {typed}
          <span className="cursor" />
        </div>
        <Search
          className="hero-search"
          placeholder="输入关键词搜索技能, 例如: 数据库巡检、日志脱敏、告警收敛..."
          allowClear
          enterButton={<><SearchOutlined /> 搜索</>}
          size="large"
          onSearch={onSearch}
          style={{ maxWidth: 720, margin: '0 auto' }}
        />
        <div style={{ marginTop: 20, display: 'flex', gap: 10, justifyContent: 'center', flexWrap: 'wrap' }}>
          {['智能运维', '研发效能', '安全合规', '数据治理'].map((c) => (
            <span
              key={c}
              onClick={() => { setSelectedCategory(c); }}
              className="nexus-tag nexus-tag-cyan"
              style={{ cursor: 'pointer', fontSize: 12, padding: '4px 14px' }}
            >
              {c}
            </span>
          ))}
        </div>
      </div>

      {/* ========== 统计卡片 ========== */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 16, marginBottom: 24 }}>
        <StatCard title="技能总数" value={stats.total_skills || 0} suffix="个" icon={<AppstoreOutlined />} gradient="linear-gradient(135deg, #06b6d4, #3b82f6)" delay={0.05} />
        <StatCard title="累计安装" value={stats.total_installs || 0} suffix="次" icon={<RocketOutlined />} gradient="linear-gradient(135deg, #10b981, #06b6d4)" delay={0.12} />
        <StatCard title="月调用量" value={stats.monthly_calls || 0} suffix="次" icon={<FireOutlined />} gradient="linear-gradient(135deg, #f59e0b, #ef4444)" delay={0.19} />
        <StatCard title="月活用户" value={stats.monthly_active_users || 0} suffix="人" icon={<CrownOutlined />} gradient="linear-gradient(135deg, #8b5cf6, #d946ef)" delay={0.26} />
      </div>

      {/* ========== 筛选 + 排序 ========== */}
      <div className="glass-card filter-bar fade-up" style={{ animationDelay: '0.3s' }}>
        <span className="filter-label">▸ CATEGORY</span>
        <Select
          value={selectedCategory}
          onChange={setSelectedCategory}
          style={{ width: 160 }}
          allowClear
          placeholder="全部分类"
          options={categoryOptions}
        />
        <span className="filter-label" style={{ marginLeft: 12 }}>▸ SORT</span>
        <Select
          value={sortBy}
          onChange={setSortBy}
          style={{ width: 140 }}
          options={[
            { label: '🔥 最多安装', value: 'installs' },
            { label: '★ 最高评分', value: 'rating' },
            { label: '🕒 最新发布', value: 'newest' },
          ]}
        />
        <span style={{ marginLeft: 'auto', color: 'var(--text-dim)', fontFamily: 'var(--font-mono)', fontSize: 12.5 }}>
          <span style={{ color: 'var(--cyan-bright)' }}>{total}</span> SKILLS ONLINE
          <span className="dot" style={{ display: 'inline-block', width: 6, height: 6, borderRadius: '50%', background: 'var(--green)', boxShadow: '0 0 8px var(--green)', marginLeft: 8, animation: 'dotBlink 1.6s ease infinite' }} />
        </span>
      </div>

      {/* ========== 技能卡片网格 ========== */}
      <Spin spinning={loading}>
        {skills.length === 0 ? (
          <div className="glass-card nexus-empty" style={{ padding: 80, textAlign: 'center', marginTop: 40 }}>
            <Empty description="暂无技能, 快去开发者中心创建第一个吧!" />
            <button className="glow-btn glow-btn-violet" style={{ padding: '8px 28px', borderRadius: 10, cursor: 'pointer', marginTop: 12 }}
              onClick={() => navigate('/dev')}>
              前往开发者中心 →
            </button>
          </div>
        ) : (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(260px, 1fr))', gap: 18 }}>
            {skills.map((skill, i) => (
              <div key={skill.id} className="fade-up" style={{ animationDelay: `${Math.min(i * 0.04, 0.4)}s` }}>
                <SkillCard skill={skill} onClick={() => navigate(`/skills/${skill.id}`)} />
              </div>
            ))}
          </div>
        )}
      </Spin>
    </div>
  );
}
