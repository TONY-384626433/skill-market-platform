import React, { useEffect, useMemo, useRef, useState } from 'react';
import { Empty, Input, Modal, Spin, Tag } from 'antd';
import {
  AppstoreOutlined, AuditOutlined, CodeOutlined, SearchOutlined, ToolOutlined,
} from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { searchSkills } from '../services/api';

const workspaces = [
  { key: 'market', title: '技能市场', description: '浏览、筛选和试玩已发布能力', path: '/', icon: AppstoreOutlined },
  { key: 'my', title: '我的技能', description: '管理安装、令牌和授权状态', path: '/my', icon: ToolOutlined, authenticated: true },
  { key: 'dev', title: '开发者工作台', description: '提交能力并跟踪审核进度', path: '/dev', icon: CodeOutlined, roles: ['developer', 'admin'] },
  { key: 'admin', title: '平台治理', description: '审核技能并查看操作审计', path: '/admin', icon: AuditOutlined, roles: ['admin'] },
];

export default function CommandPalette({ open, onClose, user }) {
  const navigate = useNavigate();
  const inputRef = useRef(null);
  const [query, setQuery] = useState('');
  const [skills, setSkills] = useState([]);
  const [loading, setLoading] = useState(false);
  const [activeIndex, setActiveIndex] = useState(0);

  const visibleWorkspaces = useMemo(() => workspaces.filter((item) => {
    if (item.authenticated && !user) return false;
    if (item.roles && !item.roles.includes(user?.role)) return false;
    return !query || `${item.title}${item.description}`.toLowerCase().includes(query.toLowerCase());
  }), [query, user]);

  useEffect(() => {
    if (!open) return undefined;
    setQuery('');
    setActiveIndex(0);
    const focusTimer = window.setTimeout(() => inputRef.current?.focus(), 80);
    return () => window.clearTimeout(focusTimer);
  }, [open]);

  useEffect(() => {
    if (!open) return undefined;
    let current = true;
    const timer = window.setTimeout(async () => {
      setLoading(true);
      try {
        const response = await searchSkills({ query, page: 1, page_size: 6, sort_by: query ? 'rating' : 'installs' });
        if (current) setSkills(Array.isArray(response?.data) ? response.data : []);
      } catch {
        if (current) setSkills([]);
      } finally {
        if (current) setLoading(false);
      }
    }, query ? 180 : 0);
    return () => { current = false; window.clearTimeout(timer); };
  }, [open, query]);

  const results = [
    ...visibleWorkspaces.map((item) => ({ ...item, kind: 'workspace' })),
    ...skills.map((skill) => ({
      key: `skill-${skill.id}`, title: skill.name, description: skill.summary,
      path: `/skills/${skill.id}`, kind: 'skill', category: skill.category,
    })),
  ];

  useEffect(() => setActiveIndex(0), [query]);

  const select = (item) => {
    if (!item) return;
    navigate(item.path);
    onClose();
  };

  const handleKeyDown = (event) => {
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      setActiveIndex((index) => Math.min(results.length - 1, index + 1));
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      setActiveIndex((index) => Math.max(0, index - 1));
    } else if (event.key === 'Enter') {
      event.preventDefault();
      select(results[activeIndex]);
    }
  };

  return (
    <Modal className="command-modal" open={open} onCancel={onClose} footer={null} width={680} closable={false} centered destroyOnHidden>
      <div className="command-search">
        <SearchOutlined />
        <Input ref={inputRef} variant="borderless" value={query} onChange={(event) => setQuery(event.target.value)} onKeyDown={handleKeyDown} placeholder="搜索技能、能力或工作区" allowClear />
        {loading && <Spin size="small" />}
      </div>
      <div className="command-meta"><span>{query ? '搜索结果' : '快速访问'}</span><em>{results.length} 项</em></div>
      <div className="command-results">
        {results.length ? results.map((item, index) => {
          const Icon = item.icon;
          return (
            <button key={item.key} type="button" className={index === activeIndex ? 'active' : ''} onMouseEnter={() => setActiveIndex(index)} onClick={() => select(item)}>
              <span className="command-result-icon">{Icon ? <Icon /> : <span className="skill-command-mark">AI</span>}</span>
              <span className="command-result-copy"><strong>{item.title}</strong><small>{item.description}</small></span>
              <Tag>{item.kind === 'skill' ? (item.category || '技能') : '工作区'}</Tag>
            </button>
          );
        }) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有匹配的技能或工作区" />}
      </div>
      <div className="command-footer"><span><i /> 技能索引实时同步</span><span>统一权限边界</span><span>访问留痕审计</span></div>
    </Modal>
  );
}
