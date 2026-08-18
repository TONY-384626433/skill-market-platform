import React, { useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { App as AntApp, Button, Empty, Input, Segmented, Select, Skeleton, Tag, Tooltip } from 'antd';
import {
  ApiOutlined, AppstoreOutlined, ArrowRightOutlined, BankOutlined, CheckCircleFilled, CodeOutlined,
  DownloadOutlined, HeartFilled, HeartOutlined, PlusOutlined, PlayCircleOutlined,
  ReloadOutlined, SafetyCertificateOutlined, SearchOutlined, StarFilled,
  ThunderboltOutlined, UnorderedListOutlined,
} from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { getCategories, getStats, searchSkills } from '../services/api';
import { UserContext } from '../App';
import SkillVisual from '../components/SkillVisual';
import { formatNumber } from '../utils/format';

const emptyStats = { total_skills: 0, total_installs: 0, monthly_calls: 0, success_rate: 100, monthly_active_users: 0, avg_duration_ms: 0 };
const typeLabels = { mcp: 'MCP 服务', dify: 'Dify 工作流', api: 'API 服务', agent: '智能体', prompt: '提示词' };
const suggestions = ['日志脱敏', '数据库巡检', '告警分析', '需求分析'];

function Metric({ icon: Icon, label, value, suffix, detail, tone }) {
  return (
    <div className={`market-metric ${tone || ''}`}>
      <span className="metric-icon"><Icon /></span>
      <div><span>{label}</span><strong>{formatNumber(value)}{suffix && <small>{suffix}</small>}</strong><em>{detail}</em></div>
    </div>
  );
}

function SkillCard({ skill, onOpen, onPlay, favorited, onToggleFavorite, view }) {
  return (
    <article className={`skill-card ${view === 'list' ? 'skill-card-list' : ''}`} data-category={skill.category} onClick={onOpen} tabIndex={0} onKeyDown={(event) => event.key === 'Enter' && event.target === event.currentTarget && onOpen()}>
      <div className="skill-card-accent" />
      <div className="skill-card-top">
        <SkillVisual category={skill.category} />
        <div className="skill-card-tags"><Tag>{typeLabels[skill.skill_type] || skill.skill_type}</Tag>{skill.stability === 'stable' && <Tag color="success"><CheckCircleFilled /> 生产稳定</Tag>}</div>
        <Tooltip title={favorited ? '取消收藏' : '收藏技能'}>
          <button className={`favorite-button ${favorited ? 'active' : ''}`} type="button" aria-label={favorited ? '取消收藏' : '收藏技能'} onClick={(event) => { event.stopPropagation(); onToggleFavorite(); }}>{favorited ? <HeartFilled /> : <HeartOutlined />}</button>
        </Tooltip>
      </div>
      <div className="skill-card-copy"><h3>{skill.name}</h3><p>{skill.summary}</p></div>
      <div className="skill-tag-list"><span>{skill.category}</span>{(skill.tags || []).slice(0, 3).map((tag) => <span key={tag}>{tag}</span>)}</div>
      <div className="skill-owner"><span>维护方</span><strong>{skill.team_name || skill.author_name || '平台团队'}</strong></div>
      <footer>
        <span><DownloadOutlined /> {formatNumber(skill.install_count)}</span>
        <span><StarFilled className="rating-star" /> {Number(skill.rating_avg || 0).toFixed(1)}</span>
        <span>v{skill.version}</span>
        <div className="skill-card-actions">
          <Button size="small" icon={<PlayCircleOutlined />} onClick={(event) => { event.stopPropagation(); onPlay(); }}>试玩</Button>
          <Button type="primary" size="small" icon={<ArrowRightOutlined />} onClick={(event) => { event.stopPropagation(); onOpen(); }} aria-label={`打开${skill.name}`} />
        </div>
      </footer>
    </article>
  );
}

export default function MarketPage() {
  const { message } = AntApp.useApp();
  const navigate = useNavigate();
  const { user } = useContext(UserContext);
  const [skills, setSkills] = useState([]);
  const [categories, setCategories] = useState([]);
  const [stats, setStats] = useState(emptyStats);
  const [filters, setFilters] = useState({ query: '', category: '', skill_type: '', sort_by: 'installs' });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [total, setTotal] = useState(0);
  const [view, setView] = useState(() => localStorage.getItem('skillhub_market_view') || 'grid');
  const [favoritesOnly, setFavoritesOnly] = useState(false);
  const [favorites, setFavorites] = useState(() => {
    try { return new Set(JSON.parse(localStorage.getItem('skillhub_favorites')) || []); } catch { return new Set(); }
  });

  const loadSkills = useCallback(async (nextFilters) => {
    setLoading(true);
    setError('');
    try {
      const response = await searchSkills({ ...nextFilters, page: 1, page_size: 50 });
      if (!Array.isArray(response?.data)) throw new Error('技能列表数据格式异常');
      setSkills(response.data);
      setTotal(Number(response.total) || 0);
    } catch (requestError) {
      setSkills([]);
      setError(requestError.response?.data?.error || requestError.message || '技能列表加载失败');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    Promise.allSettled([getCategories(), getStats()]).then(([categoryResult, statResult]) => {
      if (categoryResult.status === 'fulfilled' && Array.isArray(categoryResult.value?.data)) setCategories(categoryResult.value.data);
      if (statResult.status === 'fulfilled' && statResult.value?.data && typeof statResult.value.data === 'object') setStats({ ...emptyStats, ...statResult.value.data });
    });
  }, []);

  useEffect(() => {
    const timer = window.setTimeout(() => loadSkills(filters), filters.query ? 280 : 0);
    return () => window.clearTimeout(timer);
  }, [filters, loadSkills]);

  const visibleSkills = useMemo(() => favoritesOnly ? skills.filter((skill) => favorites.has(String(skill.id))) : skills, [favorites, favoritesOnly, skills]);
  const activeFilterCount = [filters.query, filters.category, filters.skill_type, favoritesOnly].filter(Boolean).length;
  const quickSkills = skills.slice(0, 3);

  const updateFilter = (key, value) => setFilters((current) => ({ ...current, [key]: value || '' }));
  const resetFilters = () => {
    setFilters({ query: '', category: '', skill_type: '', sort_by: 'installs' });
    setFavoritesOnly(false);
  };
  const changeView = (nextView) => {
    setView(nextView);
    localStorage.setItem('skillhub_market_view', nextView);
  };
  const toggleFavorite = (id, name) => {
    const key = String(id);
    const next = new Set(favorites);
    const removing = next.has(key);
    if (removing) next.delete(key); else next.add(key);
    setFavorites(next);
    localStorage.setItem('skillhub_favorites', JSON.stringify([...next]));
    message.success(removing ? `已取消收藏 ${name}` : `已收藏 ${name}`);
  };

  return (
    <div className="page market-page">
      <section className="market-command" aria-label="技能发现控制台">
        <div className="market-command-copy">
          <div className="command-eyebrow"><span><i /> GOVERNED AI MARKETPLACE</span><em>企业内部</em></div>
          <h1>让可信 AI 能力，在业务一线即取即用</h1>
          <p>发现、评估并接入经过安全审核的 MCP 服务、工作流与 API。</p>
          <div className="market-search">
            <Input size="large" prefix={<SearchOutlined />} placeholder="输入能力、场景或标签，实时检索" value={filters.query} allowClear onChange={(event) => updateFilter('query', event.target.value)} onPressEnter={() => loadSkills(filters)} />
            <Button type="primary" size="large" icon={<SearchOutlined />} onClick={() => loadSkills(filters)}>搜索能力</Button>
          </div>
          <div className="search-suggestions"><span>热门能力</span>{suggestions.map((item) => <button key={item} type="button" onClick={() => updateFilter('query', item)}>{item}</button>)}</div>
        </div>
        <div className="capability-radar" aria-hidden="true">
          <div className="radar-ring ring-one" /><div className="radar-ring ring-two" /><div className="radar-axis axis-one" /><div className="radar-axis axis-two" />
          <div className="radar-core"><BankOutlined /><span>SkillHub</span></div>
          <span className="radar-node node-one"><SafetyCertificateOutlined /></span>
          <span className="radar-node node-two"><ApiOutlined /></span>
          <span className="radar-node node-three"><CodeOutlined /></span>
          <div className="radar-status"><i /> {stats.total_skills} 项可信能力在线</div>
        </div>
      </section>

      <section className="market-metrics" aria-label="平台运营指标">
        <Metric icon={AppstoreOutlined} label="已上架技能" value={stats.total_skills} suffix=" 个" detail="全部通过发布审核" tone="blue" />
        <Metric icon={DownloadOutlined} label="累计安装" value={stats.total_installs} suffix=" 次" detail="企业内部授权接入" tone="teal" />
        <Metric icon={ThunderboltOutlined} label="近 30 日调用" value={stats.monthly_calls} suffix=" 次" detail={`${formatNumber(stats.monthly_active_users)} 位活跃用户`} tone="amber" />
        <Metric icon={SafetyCertificateOutlined} label="调用成功率" value={stats.success_rate} suffix="%" detail={`平均响应 ${formatNumber(stats.avg_duration_ms)} ms`} tone="green" />
      </section>

      {quickSkills.length > 0 && <section className="quick-launch"><div><ThunderboltOutlined /><span><strong>快速启动</strong><small>常用能力直达在线工作区</small></span></div>{quickSkills.map((skill) => <button key={skill.id} type="button" onClick={() => navigate(`/skills/${skill.id}?tab=playground`)}><SkillVisual category={skill.category} /><span>{skill.name}</span><PlayCircleOutlined /></button>)}</section>}

      <section className="catalog-section">
        <aside className="catalog-sidebar">
          <div className="section-label">能力分类</div>
          <button className={!filters.category ? 'active' : ''} onClick={() => updateFilter('category', '')}><span>全部技能</span><b>{stats.total_skills}</b></button>
          {categories.map((category) => <button key={category.key} className={filters.category === category.name ? 'active' : ''} onClick={() => updateFilter('category', category.name)}><span>{category.name}</span><b>{category.count}</b></button>)}
          <div className="catalog-help"><CodeOutlined /><strong>能力接入通道</strong><p>提交标准接口后进入安全与功能审核。</p>{['developer', 'admin'].includes(user?.role) ? <Button size="small" icon={<PlusOutlined />} onClick={() => navigate('/dev')}>提交能力</Button> : <Button size="small" onClick={() => navigate('/login')}>登录开发者账号</Button>}</div>
        </aside>

        <div className="catalog-main">
          <div className="catalog-toolbar">
            <div><h2>技能目录</h2><span>{favoritesOnly ? `${visibleSkills.length} 项收藏` : `共 ${total} 项可用能力`}</span>{activeFilterCount > 0 && <button className="clear-filter" type="button" onClick={resetFilters}>清除 {activeFilterCount} 项筛选</button>}</div>
            <div className="toolbar-controls">
              <Tooltip title={favoritesOnly ? '显示全部' : '仅看收藏'}><Button className={favoritesOnly ? 'favorite-filter active' : 'favorite-filter'} icon={favoritesOnly ? <HeartFilled /> : <HeartOutlined />} onClick={() => setFavoritesOnly((value) => !value)} /></Tooltip>
              <Select value={filters.skill_type || undefined} allowClear placeholder="全部形态" onChange={(value) => updateFilter('skill_type', value)} options={Object.entries(typeLabels).map(([value, label]) => ({ value, label }))} />
              <Select value={filters.sort_by} onChange={(value) => updateFilter('sort_by', value)} options={[{ value: 'installs', label: '最多安装' }, { value: 'rating', label: '最高评分' }, { value: 'newest', label: '最新发布' }]} />
              <Segmented className="view-switch" value={view} onChange={changeView} options={[{ value: 'grid', icon: <AppstoreOutlined /> }, { value: 'list', icon: <UnorderedListOutlined /> }]} />
            </div>
          </div>

          {loading ? (
            <div className={`skill-grid ${view === 'list' ? 'list-view' : ''}`}>{[1, 2, 3, 4].map((item) => <div className="skill-card skeleton-card" key={item}><Skeleton active paragraph={{ rows: 4 }} /></div>)}</div>
          ) : error ? (
            <div className="state-panel"><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={error} /><Button icon={<ReloadOutlined />} onClick={() => loadSkills(filters)}>重新加载</Button></div>
          ) : visibleSkills.length === 0 ? (
            <div className="state-panel"><Empty description={favoritesOnly ? '还没有收藏技能' : '没有找到符合条件的技能'} /><Button onClick={resetFilters}>清除筛选</Button></div>
          ) : (
            <div className={`skill-grid ${view === 'list' ? 'list-view' : ''}`}>{visibleSkills.map((skill) => <SkillCard key={skill.id} view={view} skill={skill} favorited={favorites.has(String(skill.id))} onToggleFavorite={() => toggleFavorite(skill.id, skill.name)} onOpen={() => navigate(`/skills/${skill.id}`)} onPlay={() => navigate(`/skills/${skill.id}?tab=playground`)} />)}</div>
          )}
        </div>
      </section>
    </div>
  );
}
