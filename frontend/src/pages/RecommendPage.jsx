import React, { useCallback, useEffect, useState } from 'react';
import { Alert, App as AntApp, Button, Empty, Input, Skeleton, Space, Tag, Tooltip } from 'antd';
import {
  ArrowRightOutlined, BulbOutlined, CheckCircleFilled, DownloadOutlined, ExperimentOutlined,
  RobotOutlined, SearchOutlined, StarFilled, ThunderboltOutlined,
} from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { getRecommendStatus, recommendSkills } from '../services/api';
import SkillVisual from '../components/SkillVisual';
import { formatNumber } from '../utils/format';

const EXAMPLES = [
  '把生产日志里的身份证和手机号脱敏',
  '核心数据库跑不动了，想看看健康状况和慢查询',
  '把这段业务需求整理成 PRD 要点',
  '分析支付节点最近的告警噪声',
];

function ScoreRing({ score }) {
  const tone = score >= 75 ? 'high' : score >= 50 ? 'mid' : 'low';
  return (
    <div className={`reco-score ${tone}`}>
      <strong>{score}</strong>
      <small>匹配度</small>
    </div>
  );
}

function RecoCard({ item, onOpen, onPlay }) {
  return (
    <article className="reco-card">
      <div className="reco-card-head">
        <SkillVisual category={item.category} />
        <div className="reco-card-title">
          <h3>{item.name}</h3>
          <div className="reco-card-meta">
            <Tag>{item.category}</Tag>
            {item.skill_type && <Tag color="blue">{item.skill_type}</Tag>}
            {item.security_status === 'safe' && <Tag color="success" icon={<CheckCircleFilled />}>安全</Tag>}
          </div>
        </div>
        <ScoreRing score={item.match_score || 0} />
      </div>
      {item.summary && <p className="reco-card-summary">{item.summary}</p>}
      {item.reason && <div className="reco-card-reason"><BulbOutlined /> {item.reason}</div>}
      <div className="reco-card-tags">{(item.tags || []).slice(0, 5).map((t) => <span key={t}>{t}</span>)}</div>
      <footer>
        <span><DownloadOutlined /> {formatNumber(item.install_count)}</span>
        <span><StarFilled className="rating-star" /> {Number(item.rating_avg || 0).toFixed(1)}</span>
        <div className="reco-card-actions">
          <Button size="small" icon={<ExperimentOutlined />} onClick={() => onPlay(item)}>在线试玩</Button>
          <Button type="primary" size="small" icon={<ArrowRightOutlined />} onClick={() => onOpen(item)}>查看详情</Button>
        </div>
      </footer>
    </article>
  );
}

export default function RecommendPage() {
  const { message } = AntApp.useApp();
  const navigate = useNavigate();
  const [text, setText] = useState('');
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState(null);
  const [status, setStatus] = useState(null);

  useEffect(() => { getRecommendStatus().then((res) => setStatus(res?.data || res)).catch(() => {}); }, []);

  const run = useCallback(async (value) => {
    const query = (value ?? text).trim();
    if (!query) { message.warning('先说说你的需求'); return; }
    setLoading(true);
    try {
      const res = await recommendSkills(query);
      const data = res?.data || res;
      setResult(data);
      if ((data?.recommendations || []).length === 0) message.info('没有特别匹配的技能，换个说法试试');
    } catch (error) {
      message.error(error.response?.data?.error || '推荐失败');
    } finally {
      setLoading(false);
    }
  }, [message, text]);

  const mode = status?.mode;
  const recs = result?.recommendations || [];

  return (
    <div className="page reco-page">
      <header className="reco-hero">
        <div className="reco-kicker"><RobotOutlined /> SKILL RECOMMENDATION AGENT</div>
        <h1>说说你的需求，AI 帮你找技能</h1>
        <p>不用记住技能名字。用一句话描述你要做的事，推荐 Agent 会读懂需求，从平台已发布技能中挑出最合适的并说明理由。</p>
        {mode && (
          <Tooltip title={status?.notice || ''}>
            <Tag color={mode === 'llm' ? 'geekblue' : 'cyan'} icon={<ThunderboltOutlined />}>
              {mode === 'llm' ? `大模型推荐 · ${status?.model || ''}` : '本地检索推荐（配置 LLM_API_KEY 可升级）'}
            </Tag>
          </Tooltip>
        )}
        <div className="reco-input">
          <Input.TextArea rows={3} value={text} onChange={(e) => setText(e.target.value)}
            placeholder="例如：把生产日志里的身份证和手机号脱敏；或：核心数据库跑不动了想看看慢查询…"
            onPressEnter={(e) => { if (!e.shiftKey) { e.preventDefault(); run(); } }} maxLength={500} showCount />
          <Button type="primary" size="large" icon={<SearchOutlined />} loading={loading} onClick={() => run()}>帮我找技能</Button>
        </div>
        <div className="reco-examples">
          <span>试试：</span>
          {EXAMPLES.map((ex) => <button key={ex} type="button" onClick={() => { setText(ex); run(ex); }}>{ex}</button>)}
        </div>
      </header>

      <div className="reco-result">
        {loading && <div className="reco-loading"><Skeleton active paragraph={{ rows: 4 }} /></div>}
        {!loading && result && (
          <>
            {result.interpretation && (
              <Alert className="reco-interpret" type="info" showIcon icon={<BulbOutlined />}
                message="需求理解" description={result.interpretation} />
            )}
            {recs.length > 0 ? (
              <>
                <div className="reco-result-head">
                  <strong>为你推荐 {recs.length} 个技能</strong>
                  <span className="sec-muted">引擎 {result.engine} · 候选 {result.candidate_count} 个 · 耗时 {result.duration_ms} ms</span>
                </div>
                <div className="reco-cards">
                  {recs.map((item) => (
                    <RecoCard key={item.skill_id} item={item}
                      onOpen={() => navigate(`/skills/${item.skill_id}`)}
                      onPlay={() => navigate(`/skills/${item.skill_id}?tab=playground`)} />
                  ))}
                </div>
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
