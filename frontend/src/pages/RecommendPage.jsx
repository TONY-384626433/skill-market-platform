import React, { useCallback, useEffect, useState } from 'react';
import { Alert, App as AntApp, Button, Empty, Input, Segmented, Skeleton, Space, Tag, Tooltip } from 'antd';
import {
  ArrowRightOutlined, BulbOutlined, CheckCircleFilled, CloudDownloadOutlined, DownloadOutlined, ExperimentOutlined,
  GithubOutlined, LinkOutlined, RobotOutlined, SafetyCertificateOutlined, SearchOutlined,
  StarFilled, StopOutlined, ThunderboltOutlined, WarningOutlined,
} from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { getGitHubSkillDownloadURL, getRecommendStatus, recommendSkills, submitImportRequest } from '../services/api';
import SkillVisual from '../components/SkillVisual';
import { formatNumber } from '../utils/format';

const EXAMPLES = [
  '把生产日志里的身份证和手机号脱敏',
  '核心数据库跑不动了，想看看健康状况和慢查询',
  '翻译一篇 PDF 文档',
  'data analysis of sales csv',
];

const SEC_META = {
  safe: { color: 'success', text: '安全(已核查)', icon: CheckCircleFilled },
  suspicious: { color: 'warning', text: '可疑(已核查)', icon: WarningOutlined },
  unverified: { color: 'default', text: '未核查', icon: StopOutlined },
  malicious: { color: 'error', text: '恶意', icon: StopOutlined },
};

function SecTag({ item }) {
  if (item.source !== 'github') return null;
  const meta = SEC_META[item.security_status] || SEC_META.unverified;
  const Icon = meta.icon;
  return (
    <Tooltip title={item.security_summary || (item.security_status === 'unverified' ? '未能抓取技能包，未完成安全核查' : '推荐前已做静态查毒 + 语义审计')}>
      <Tag color={meta.color} icon={<Icon />}>{meta.text}</Tag>
    </Tooltip>
  );
}

const REVIEW_META = {
  pending: { color: 'default', text: '待审查' },
  scanning: { color: 'processing', text: '审查中' },
  pending_review: { color: 'warning', text: '待人工复核' },
  approved: { color: 'success', text: '已通过安全审查' },
  imported: { color: 'cyan', text: '已入库' },
  blocked: { color: 'error', text: '安全审查已阻断' },
  rejected: { color: 'default', text: '已驳回' },
  scan_failed: { color: 'volcano', text: '审查失败(可重试)' },
};

function ScoreRing({ score }) {
  const tone = score >= 75 ? 'high' : score >= 50 ? 'mid' : 'low';
  return <div className={`reco-score ${tone}`}><strong>{score}</strong><small>匹配度</small></div>;
}

function RecoCard({ item, onOpen, onPlay, onReview, reviewing, review }) {
  const isGithub = item.source === 'github';
  const reviewMeta = review ? (REVIEW_META[review.status] || { color: 'default', text: review.status }) : null;
  const downloadURL = review?.download_allowed
    ? `${getGitHubSkillDownloadURL(item)}&request_id=${encodeURIComponent(review.id)}`
    : undefined;
  return (
    <article className="reco-card">
      <div className="reco-card-head">
        <SkillVisual category={item.category} />
        <div className="reco-card-title">
          <h3>{item.name}</h3>
          <div className="reco-card-meta">
            {isGithub
              ? <Tag color="black" icon={<GithubOutlined />}>GitHub</Tag>
              : <Tag color="geekblue" icon={<SafetyCertificateOutlined />}>企业库</Tag>}
            <Tag>{item.category}</Tag>
            {item.skill_type && <Tag color="blue">{item.skill_type}</Tag>}
            <SecTag item={item} />
          </div>
        </div>
        <ScoreRing score={item.match_score || 0} />
      </div>
      {item.summary && <p className="reco-card-summary">{item.summary}</p>}
      {item.reason && <div className="reco-card-reason"><BulbOutlined /> {item.reason}</div>}
      {isGithub && item.repository && (
        <div className="reco-card-repo"><GithubOutlined /> <span>{item.repository}</span>{item.path ? <small>{item.path}</small> : null}</div>
      )}
      {isGithub && review && (
        <div className="reco-card-review">
          <Tag color={reviewMeta.color}>{reviewMeta.text}</Tag>
          {typeof review.risk_score === 'number' && <span className="sec-muted">风险分 {review.risk_score}</span>}
          {review.review_note && <span className="sec-muted">{review.review_note}</span>}
        </div>
      )}
      {!isGithub && <div className="reco-card-tags">{(item.tags || []).slice(0, 5).map((t) => <span key={t}>{t}</span>)}</div>}
      <footer>
        {isGithub
          ? <span><StarFilled className="rating-star" /> {formatNumber(item.stars || 0)} star</span>
          : <><span><DownloadOutlined /> {formatNumber(item.install_count)}</span><span><StarFilled className="rating-star" /> {Number(item.rating_avg || 0).toFixed(1)}</span></>}
        <div className="reco-card-actions">
          {isGithub ? (
            <>
              <Button size="small" icon={<LinkOutlined />} onClick={() => window.open(item.repository_url, '_blank', 'noopener')}>查看仓库</Button>
              {review?.download_allowed ? (
                <Button type="primary" size="small" icon={<CloudDownloadOutlined />} href={downloadURL}>下载已审通过包</Button>
              ) : (
                <Button type="primary" size="small" icon={<SafetyCertificateOutlined />} loading={reviewing} onClick={() => onReview(item)}>{review ? '重新提交审查' : '提交安全审查'}</Button>
              )}
            </>
          ) : (
            <>
              <Button size="small" icon={<ExperimentOutlined />} onClick={() => onPlay(item)}>在线试玩</Button>
              <Button type="primary" size="small" icon={<ArrowRightOutlined />} onClick={() => onOpen(item)}>查看详情</Button>
            </>
          )}
        </div>
      </footer>
    </article>
  );
}

export default function RecommendPage() {
  const { message } = AntApp.useApp();
  const navigate = useNavigate();
  const [text, setText] = useState('');
  const [source, setSource] = useState('all');
  const [topN, setTopN] = useState(15);
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState(null);
  const [status, setStatus] = useState(null);
  const [reviewing, setReviewing] = useState('');
  const [reviews, setReviews] = useState({});

  useEffect(() => { getRecommendStatus().then((res) => setStatus(res?.data || res)).catch(() => {}); }, []);

  const run = useCallback(async (value, src) => {
    const query = (value ?? text).trim();
    if (!query) { message.warning('先说说你的需求'); return; }
    setLoading(true);
    try {
      const useSource = src ?? source;
      const opts = { top_n: topN, verify_security: true };
      if (useSource !== 'all') opts.sources = [useSource];
      const res = await recommendSkills(query, opts);
      const data = res?.data || res;
      setResult(data);
      if ((data?.recommendations || []).length === 0) message.info('没有特别匹配的技能，换个说法试试');
    } catch (error) {
      message.error(error.response?.data?.error || '推荐失败');
    } finally {
      setLoading(false);
    }
  }, [message, source, text, topN]);

  const submitReview = useCallback(async (item) => {
    setReviewing(item.skill_id);
    try {
      const res = await submitImportRequest(item.repository, item.ref || 'main', item.path || 'SKILL.md', item.repository_url);
      const req = res?.data || res;
      setReviews((prev) => ({ ...prev, [item.skill_id]: req }));
      const statusText = (REVIEW_META[req?.status] || {}).text || req?.status;
      if (req?.download_allowed) message.success(`已通过安全审查，可下载（风险分 ${req?.risk_score ?? '-'}）`);
      else message.success(`已提交安全审查: ${statusText}（风险分 ${req?.risk_score ?? '-'}）`);
    } catch (error) {
      message.error(error.response?.data?.error || '提交审查失败');
    } finally {
      setReviewing('');
    }
  }, [message]);

  const mode = status?.mode;
  const recs = result?.recommendations || [];

  return (
    <div className="page reco-page">
      <header className="reco-hero">
        <div className="reco-kicker"><RobotOutlined /> SKILL RECOMMENDATION AGENT</div>
        <h1>说说你的需求，AI 帮你找技能</h1>
        <p>跨源推荐：<b>企业审核库 + GitHub 开源</b>。GitHub 技能推荐前先核查安全性；同等条件下优先推荐 star 多、下载/安装多的技能。</p>
        <div className="reco-controls">
          <Segmented value={source} onChange={setSource} options={[
            { label: '全部来源', value: 'all' },
            { label: <span><SafetyCertificateOutlined /> 企业库</span>, value: 'internal' },
            { label: <span><GithubOutlined /> GitHub</span>, value: 'github' },
          ]} />
          <Segmented value={topN} onChange={setTopN} options={[
            { label: '推荐 10', value: 10 }, { label: '推荐 20', value: 20 }, { label: '推荐 30', value: 30 },
          ]} />
          {mode && (
            <Tooltip title={status?.notice || ''}>
              <Tag color={mode === 'llm' ? 'geekblue' : 'cyan'} icon={<ThunderboltOutlined />}>
                {mode === 'llm' ? `大模型推荐 · ${status?.model || ''}` : '本地检索推荐'}
              </Tag>
            </Tooltip>
          )}
        </div>
        <div className="reco-input">
          <Input.TextArea rows={3} value={text} onChange={(e) => setText(e.target.value)}
            placeholder="例如：把生产日志里的身份证和手机号脱敏；或：data analysis of sales csv…"
            onPressEnter={(e) => { if (!e.shiftKey) { e.preventDefault(); run(); } }} maxLength={500} showCount />
          <Button type="primary" size="large" icon={<SearchOutlined />} loading={loading} onClick={() => run()}>帮我找技能</Button>
        </div>
      </header>

      <div className="reco-result">
        {loading && <div className="reco-loading"><Skeleton active paragraph={{ rows: 4 }} /></div>}
        {!loading && result && (
          <>
            {result.interpretation && (
              <Alert className="reco-interpret" type="info" showIcon icon={<BulbOutlined />} message="需求理解" description={result.interpretation} />
            )}
            {recs.length > 0 ? (
              <>
                <div className="reco-result-head">
                  <strong>为你推荐 {recs.length} 个技能</strong>
                  <span className="sec-muted">引擎 {result.engine} · 候选 {result.candidate_count} · 已核查 {result.verified_count || 0} · 耗时 {result.duration_ms} ms</span>
                </div>
                {result.github_notice && <div className="sec-muted reco-gh-notice"><GithubOutlined /> {result.github_notice}</div>}
                <div className="reco-cards">
                  {recs.map((item) => (
                    <RecoCard key={`${item.source}-${item.skill_id}`} item={item} reviewing={reviewing === item.skill_id} review={reviews[item.skill_id]}
                      onOpen={() => navigate(`/skills/${item.skill_id}`)}
                      onPlay={() => navigate(`/skills/${item.skill_id}?tab=playground`)}
                      onReview={submitReview} />
                  ))}
                </div>
                <Alert type="info" showIcon className="reco-footnote"
                  message="GitHub 技能来自公开来源，需先通过安全审查（先审核后下载）才能接入企业库使用；推荐结果中的安全结论为快速核查，正式接入仍走完整审查。" />
              </>
            ) : (
              <div className="reco-empty">
                <Empty description={result.notice || '没有找到匹配的技能'} />
                {(result.suggested_queries || []).length > 0 && (
                  <div className="reco-examples">
                    <span>可以这样问：</span>
                    {result.suggested_queries.map((s) => <button key={s} type="button" onClick={() => { setText(s); run(s); }}>{s}</button>)}
                  </div>
                )}
              </div>
            )}
          </>
        )}
        {!loading && !result && (
          <Alert className="reco-hint" type="info" showIcon icon={<RobotOutlined />}
            message="举几个例子" description={<Space wrap>{EXAMPLES.map((ex) => <Tag key={ex} className="reco-hint-tag" onClick={() => { setText(ex); run(ex); }}>{ex}</Tag>)}</Space>} />
        )}
      </div>
    </div>
  );
}
