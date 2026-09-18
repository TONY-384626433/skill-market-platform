import React, { useEffect, useMemo, useRef, useState } from 'react';
import { Alert, Button, Drawer, Empty, Pagination, Select, Skeleton, Spin, Tabs, Tag, Tooltip } from 'antd';
import {
  CheckCircleFilled, CloseCircleFilled, CloudDownloadOutlined, CodeOutlined,
  CopyOutlined, ExclamationCircleFilled, EyeOutlined, FileTextOutlined, GithubOutlined,
  LinkOutlined, ReloadOutlined, SafetyCertificateOutlined, StarFilled,
} from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { getGitHubSkillDownloadURL, getGitHubSkillPreview, searchGitHubSkills, submitImportRequest } from '../services/api';
import { formatNumber } from '../utils/format';

const compatibilityMeta = {
  installable: { icon: CheckCircleFilled, tone: 'installable' },
  needs_setup: { icon: ExclamationCircleFilled, tone: 'needs-setup' },
  incompatible: { icon: CloseCircleFilled, tone: 'incompatible' },
};

function RuntimePill({ name, available }) {
  return <span className={available ? 'available' : 'missing'}><i />{name}</span>;
}

function formatBytes(value) {
  const bytes = Number(value) || 0;
  if (bytes < 1024) return `${bytes} B`;
  return `${(bytes / 1024).toFixed(bytes < 10240 ? 1 : 0)} KB`;
}

function GitHubSkillCard({ skill, onPreview }) {
  const compatibility = compatibilityMeta[skill.compatibility?.status] || compatibilityMeta.needs_setup;
  const CompatibilityIcon = compatibility.icon;
  const reasons = skill.compatibility?.reasons?.join('；') || '等待兼容性检测';

  return (
    <article className="github-skill-card" role="button" tabIndex={0} aria-label={`预览 ${skill.name}`} onClick={() => onPreview(skill)} onKeyDown={(event) => event.key === 'Enter' && event.target === event.currentTarget && onPreview(skill)}>
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
          <Tooltip title="查看 GitHub 源码"><Button type="text" icon={<LinkOutlined />} href={skill.skill_url || skill.repository_url} target="_blank" rel="noreferrer" onClick={(event) => event.stopPropagation()} /></Tooltip>
          <Button type="primary" icon={<EyeOutlined />} onClick={(event) => { event.stopPropagation(); onPreview(skill); }}>预览详情</Button>
        </div>
      </footer>
    </article>
  );
}

function GitHubSkillPreview({ skill, open, onClose }) {
  const [preview, setPreview] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [activeTab, setActiveTab] = useState('rendered');
  const [copied, setCopied] = useState(false);
  const [review, setReview] = useState(null);     // 安全审查单 (先审核, 后下载)
  const [reviewing, setReviewing] = useState(false);
  const [gateError, setGateError] = useState('');

  const submitReview = async () => {
    if (!skill) return;
    setReviewing(true);
    setGateError('');
    try {
      const response = await submitImportRequest(skill.repository, skill.ref, skill.path, skill.skill_url);
      setReview(response?.data || response);
    } catch (requestError) {
      setGateError(requestError.response?.data?.error || requestError.message || '提交安全审查失败');
    } finally {
      setReviewing(false);
    }
  };

  const reviewMeta = {
    pending: { color: 'default', text: '待审查' },
    scanning: { color: 'processing', text: '审查中' },
    pending_review: { color: 'warning', text: '待人工复核' },
    approved: { color: 'success', text: '已通过安全审查' },
    blocked: { color: 'error', text: '安全审查已阻断' },
    rejected: { color: 'default', text: '已驳回' },
    imported: { color: 'cyan', text: '已入库' },
  }[review?.status] || { color: 'default', text: '未审查' };
  const downloadURL = review?.download_allowed
    ? `${getGitHubSkillDownloadURL(skill)}&request_id=${encodeURIComponent(review.id)}`
    : undefined;

  useEffect(() => {
    if (!open || !skill) return undefined;
    let active = true;
    setPreview(null);
    setError('');
    setLoading(true);
    setActiveTab('rendered');
    setCopied(false);
    setReview(null);
    setGateError('');
    getGitHubSkillPreview(skill)
      .then((response) => { if (active) setPreview(response); })
      .catch((requestError) => { if (active) setError(requestError.response?.data?.error || requestError.message || '预览内容加载失败'); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [open, skill]);

  if (!skill) return null;
  const compatibility = compatibilityMeta[skill.compatibility?.status] || compatibilityMeta.needs_setup;
  const CompatibilityIcon = compatibility.icon;
  const reasons = skill.compatibility?.reasons?.join('；') || '等待兼容性检测';
  const requirements = skill.compatibility?.requirements || [];
  const copySource = async () => {
    if (!preview?.content) return;
    try {
      await navigator.clipboard.writeText(preview.content);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1800);
    } catch { setCopied(false); }
  };
  const markdown = preview?.body || preview?.content || '';
  const markdownView = loading ? (
    <div className="github-preview-loading"><Spin /><span>正在安全读取 SKILL.md</span></div>
  ) : error ? (
    <div className="state-panel github-preview-error"><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={error} /><Button icon={<ReloadOutlined />} onClick={() => { setError(''); setLoading(true); getGitHubSkillPreview(skill).then(setPreview).catch((requestError) => setError(requestError.response?.data?.error || '预览内容加载失败')).finally(() => setLoading(false)); }}>重新加载</Button></div>
  ) : (
    <div className="github-markdown"><ReactMarkdown remarkPlugins={[remarkGfm]} components={{ a: ({ children, ...props }) => <a {...props} target="_blank" rel="noreferrer">{children}</a> }}>{markdown}</ReactMarkdown></div>
  );

  return (
    <Drawer
      className="github-preview-drawer"
      open={open}
      onClose={onClose}
      width={920}
      destroyOnHidden
      title={<span className="github-preview-title"><GithubOutlined /><span><strong>Skill 项目预览</strong><small>{skill.repository}</small></span></span>}
      extra={<Button type="text" icon={<LinkOutlined />} href={skill.skill_url || skill.repository_url} target="_blank" rel="noreferrer">GitHub</Button>}
      footer={<div className="github-preview-footer"><span><SafetyCertificateOutlined /> 安全门禁: 外部技能需先审查 (查毒/危险行为/提示注入) 才能下载</span><div className="github-preview-actions"><Button onClick={onClose}>关闭</Button>{review?.download_allowed ? <Button type="primary" icon={<CloudDownloadOutlined />} href={downloadURL}>下载已审通过包</Button> : <Button type="primary" icon={<SafetyCertificateOutlined />} loading={reviewing} disabled={!skill.download_enabled} onClick={submitReview}>{review ? '重新提交审查' : '提交安全审查'}</Button>}</div></div>}
    >
      <div className="github-preview-shell">
        <header className="github-preview-hero">
          <span className="github-preview-icon"><FileTextOutlined /></span>
          <div><div className="github-preview-tags"><Tag>{skill.format}</Tag><Tag>{skill.category}</Tag>{skill.license && <Tag>{skill.license}</Tag>}</div><h2>{preview?.name || skill.name}</h2><p>{preview?.description || skill.description}</p></div>
          <span className={`compatibility-badge ${compatibility.tone}`}><CompatibilityIcon />{skill.compatibility?.label}</span>
        </header>

        {(review || gateError) && (
          <div className={`github-review-panel ${review?.status === 'blocked' ? 'blocked' : review?.download_allowed ? 'passed' : 'pending'}`}>
            <div className="github-review-head">
              <Tag color={reviewMeta.color}>{reviewMeta.text}</Tag>
              {review?.verdict_cn && <Tag>{review.verdict_cn}</Tag>}
              {typeof review?.risk_score === 'number' && <span className="sec-muted">风险分 {review.risk_score}</span>}
              {review?.grade && <span className="sec-muted">等级 {review.grade}</span>}
              {review?.id && <code>{review.id}</code>}
            </div>
            {gateError && <p>{gateError}</p>}
            {review?.review_note && <p>{review.review_note}</p>}
            {review?.reupload?.suspected && <p>查重提示: 与「{review.reupload.matched_skill_name}」相似度 {(review.reupload.similarity * 100).toFixed(0)}%, 需人工确认原创性后放行。</p>}
            {(review?.findings || []).slice(0, 4).map((finding, index) => (
              <div key={index} className="github-review-finding"><b>{finding.rule_id}</b><span>{finding.title}</span>{finding.file && <code>{finding.file}</code>}</div>
            ))}
            {(review?.findings || []).length > 4 && <p className="sec-muted">另有 {review.findings.length - 4} 项风险, 详见安全治理页报告</p>}
          </div>
        )}

        <Alert className="github-preview-notice" type="warning" showIcon message={preview?.security_notice || '第三方公开内容正在以只读方式预览，不会执行仓库中的代码。'} />

        <div className="github-preview-layout">
          <main className="github-preview-document">
            <Tabs activeKey={activeTab} onChange={setActiveTab} items={[
              { key: 'rendered', label: '内容预览', children: markdownView },
              { key: 'source', label: '原始 SKILL.md', children: loading ? markdownView : <div className="github-raw-source"><Button icon={<CopyOutlined />} onClick={copySource}>{copied ? '已复制' : '复制源码'}</Button><pre>{preview?.content || error || '暂无原始内容'}</pre></div> },
            ]} />
          </main>

          <aside className="github-preview-aside">
            <section className={`preview-verdict ${compatibility.tone}`}><span><CompatibilityIcon /></span><div><small>本机安装判定</small><strong>{skill.compatibility?.label}</strong><p>{reasons}</p></div></section>
            <section><h3>项目档案</h3><dl><div><dt>仓库</dt><dd>{skill.repository}</dd></div><div><dt>分支</dt><dd>{skill.ref}</dd></div><div><dt>文件</dt><dd>{skill.path}</dd></div><div><dt>大小</dt><dd>{preview ? formatBytes(preview.size_bytes) : '--'}</dd></div><div><dt>行数</dt><dd>{preview?.line_count || '--'}</dd></div><div><dt>Stars</dt><dd>{formatNumber(skill.stars)}</dd></div></dl></section>
            <section><h3>运行要求</h3><div className="preview-requirements">{requirements.length ? requirements.map((item) => <span key={item}>{item}</span>) : <span>未检测到额外运行时</span>}</div>{preview?.declared_compatibility && <p className="preview-declared">仓库声明：{preview.declared_compatibility}</p>}</section>
            <section className="preview-review-checklist"><h3>下载前检查</h3><p><i />确认仓库所有者与许可证</p><p><i />检查脚本所需文件及网络权限</p><p><i />不要向未知 Skill 提供敏感凭据</p></section>
          </aside>
        </div>
      </div>
    </Drawer>
  );
}

export default function GitHubCatalog({ query, searchVersion }) {
  const [result, setResult] = useState({ data: [], total: 0, page: 1, page_size: 12, host: { runtimes: {} }, rate_limit: {} });
  const [page, setPage] = useState(1);
  const [category, setCategory] = useState('');
  const [compatibility, setCompatibility] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [previewSkill, setPreviewSkill] = useState(null);
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
          ) : <div className="github-skill-grid">{visibleSkills.map((skill) => <GitHubSkillCard key={skill.id} skill={skill} onPreview={setPreviewSkill} />)}</div>}

          {!loading && !error && availableTotal > result.page_size && <Pagination className="github-pagination" current={page} pageSize={result.page_size || 12} total={availableTotal} showSizeChanger={false} showQuickJumper onChange={(nextPage) => { setPage(nextPage); window.scrollTo({ top: 520, behavior: 'smooth' }); }} />}
        </div>
      </div>
      <GitHubSkillPreview skill={previewSkill} open={Boolean(previewSkill)} onClose={() => setPreviewSkill(null)} />
    </section>
  );
}
