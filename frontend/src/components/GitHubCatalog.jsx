import React, { useEffect, useMemo, useRef, useState } from 'react';
import { Alert, Button, Empty, Pagination, Select, Skeleton, Tag, Tooltip } from 'antd';
import {
  CheckCircleFilled, CloseCircleFilled, CloudDownloadOutlined, CodeOutlined,
  ExclamationCircleFilled, GithubOutlined, LinkOutlined, ReloadOutlined,
  SafetyCertificateOutlined, StarFilled,
} from '@ant-design/icons';
import { getGitHubSkillDownloadURL, searchGitHubSkills } from '../services/api';
import { formatNumber } from '../utils/format';

const compatibilityMeta = {
  installable: { icon: CheckCircleFilled, tone: 'installable' },
  needs_setup: { icon: ExclamationCircleFilled, tone: 'needs-setup' },
  incompatible: { icon: CloseCircleFilled, tone: 'incompatible' },
};

function RuntimePill({ name, available }) {
  return <span className={available ? 'available' : 'missing'}><i />{name}</span>;
}

function GitHubSkillCard({ skill }) {
  const compatibility = compatibilityMeta[skill.compatibility?.status] || compatibilityMeta.needs_setup;
  const CompatibilityIcon = compatibility.icon;
  const reasons = skill.compatibility?.reasons?.join('；') || '等待兼容性检测';

  return (
    <article className="github-skill-card">
      <div className="github-card-head">
        <span className="github-skill-icon"><GithubOutlined /></span>
        <div className="github-card-badges"><Tag>{skill.format}</Tag>{skill.license && <Tag>{skill.license}</Tag>}</div>
        <Tooltip title={reasons} placement="topRight"><span className={`compatibility-badge ${compatibility.tone}`}><CompatibilityIcon />{skill.compatibility?.label}</span></Tooltip>
      </div>
      <div className="github-card-copy">
        <h3 title={skill.name}>{skill.name}</h3>
        <p>{skill.description}</p>
      </div>
      <div className="github-skill-tags"><span>{skill.category}</span>{(skill.tags || []).slice(0, 4).map((tag) => <span key={tag}>{tag}</span>)}</div>
      <div className="github-repository-line"><GithubOutlined /><strong>{skill.repository}</strong><code>{skill.path}</code></div>
      <div className="github-compatibility-copy"><CompatibilityIcon /><span><strong>{skill.compatibility?.label}</strong><small>{reasons}</small></span></div>
      <footer>
        <span><StarFilled className="rating-star" /> {formatNumber(skill.stars)}</span>
        {skill.language && <span><CodeOutlined /> {skill.language}</span>}
        <div>
          <Tooltip title="查看 GitHub 源码"><Button type="text" icon={<LinkOutlined />} href={skill.skill_url || skill.repository_url} target="_blank" rel="noreferrer" /></Tooltip>
          <Button type="primary" icon={<CloudDownloadOutlined />} href={getGitHubSkillDownloadURL(skill)} disabled={!skill.download_enabled}>下载 Skill</Button>
        </div>
      </footer>
    </article>
  );
}

export default function GitHubCatalog({ query, searchVersion }) {
  const [result, setResult] = useState({ data: [], total: 0, page: 1, page_size: 12, host: { runtimes: {} }, rate_limit: {} });
  const [page, setPage] = useState(1);
  const [category, setCategory] = useState('');
  const [compatibility, setCompatibility] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [reloadVersion, setReloadVersion] = useState(0);
  const criteriaRef = useRef(`${query}|${searchVersion}`);

  useEffect(() => {
    const criteriaKey = `${query}|${searchVersion}`;
    if (criteriaRef.current !== criteriaKey) {
      criteriaRef.current = criteriaKey;
      if (page !== 1) {
        setPage(1);
        return undefined;
      }
    }
    let active = true;
    const timer = window.setTimeout(async () => {
      setLoading(true);
      setError('');
      try {
        const response = await searchGitHubSkills({ query, page, page_size: 12 });
        if (!active) return;
        if (!Array.isArray(response?.data)) throw new Error('GitHub 技能列表格式异常');
        setResult(response);
      } catch (requestError) {
        if (!active) return;
        setError(requestError.response?.data?.error || requestError.message || 'GitHub 技能索引连接失败');
      } finally {
        if (active) setLoading(false);
      }
    }, query ? 420 : 0);
    return () => { active = false; window.clearTimeout(timer); };
  }, [page, query, reloadVersion, searchVersion]);

  const categories = useMemo(() => {
    const counts = new Map();
    result.data.forEach((skill) => counts.set(skill.category, (counts.get(skill.category) || 0) + 1));
    return [...counts.entries()].sort((left, right) => right[1] - left[1]);
  }, [result.data]);
  const visibleSkills = useMemo(() => result.data.filter((skill) => (!category || skill.category === category) && (!compatibility || skill.compatibility?.status === compatibility)), [category, compatibility, result.data]);
  const installableCount = result.data.filter((skill) => skill.compatibility?.status === 'installable').length;
  const runtimeEntries = Object.entries(result.host?.runtimes || {}).sort((left, right) => Number(right[1]) - Number(left[1]) || left[0].localeCompare(right[0]));
  const availableTotal = Math.min(Number(result.total) || 0, Number(result.search_cap) || 1000);

  return (
    <section className="github-catalog" aria-label="GitHub 开源技能目录">
      <div className="github-index-strip">
        <div><span className="github-index-icon"><GithubOutlined /></span><span><strong>GitHub Skill Index</strong><small>{result.authenticated ? '已通过 GitHub 账号连接 Code Search' : '公开仓库降级搜索'}</small></span></div>
        <div><span>匹配公开技能</span><strong>{formatNumber(result.total)}</strong></div>
        <div><span>本页可直接安装</span><strong>{installableCount}<small> / {result.data.length}</small></strong></div>
        <div><span>搜索额度</span><strong>{formatNumber(result.rate_limit?.remaining)}<small> / {formatNumber(result.rate_limit?.limit)}</small></strong></div>
      </div>

      {result.notice && <Alert className="github-notice" type={result.authenticated ? 'info' : 'warning'} showIcon message={result.notice} />}

      <div className="github-catalog-layout">
        <aside className="github-filter-panel">
          <div className="section-label">本页分类</div>
          <button className={!category ? 'active' : ''} type="button" onClick={() => setCategory('')}><span>全部结果</span><b>{result.data.length}</b></button>
          {categories.map(([name, count]) => <button key={name} className={category === name ? 'active' : ''} type="button" onClick={() => setCategory(name)}><span>{name}</span><b>{count}</b></button>)}
          <div className="host-profile">
            <div><SafetyCertificateOutlined /><span><strong>本机兼容性探针</strong><small>{result.host?.os || 'unknown'} / {result.host?.arch || 'unknown'}</small></span></div>
            <div className="runtime-grid">{runtimeEntries.map(([name, available]) => <RuntimePill key={name} name={name} available={available} />)}</div>
          </div>
        </aside>

        <div className="github-results">
          <div className="github-results-toolbar">
            <div><h2>开源技能目录</h2><span>GitHub 匹配 {formatNumber(result.total)} 项，单次查询可浏览前 {formatNumber(result.search_cap || 1000)} 项</span></div>
            <Select value={compatibility || undefined} allowClear placeholder="全部兼容性" onChange={(value) => setCompatibility(value || '')} options={[{ value: 'installable', label: '本机可安装' }, { value: 'needs_setup', label: '需要配置' }, { value: 'incompatible', label: '当前不兼容' }]} />
          </div>

          {loading ? (
            <div className="github-skill-grid">{[1, 2, 3, 4, 5, 6].map((item) => <div className="github-skill-card github-skeleton" key={item}><Skeleton active paragraph={{ rows: 5 }} /></div>)}</div>
          ) : error ? (
            <div className="state-panel"><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={error} /><Button icon={<ReloadOutlined />} onClick={() => setReloadVersion((value) => value + 1)}>重新连接 GitHub</Button></div>
          ) : visibleSkills.length === 0 ? (
            <div className="state-panel"><Empty description="当前页没有符合筛选条件的技能" /><Button onClick={() => { setCategory(''); setCompatibility(''); }}>清除兼容性筛选</Button></div>
          ) : <div className="github-skill-grid">{visibleSkills.map((skill) => <GitHubSkillCard key={skill.id} skill={skill} />)}</div>}

          {!loading && !error && availableTotal > result.page_size && <Pagination className="github-pagination" current={page} pageSize={result.page_size || 12} total={availableTotal} showSizeChanger={false} showQuickJumper onChange={(nextPage) => { setPage(nextPage); window.scrollTo({ top: 520, behavior: 'smooth' }); }} />}
        </div>
      </div>
    </section>
  );
}
