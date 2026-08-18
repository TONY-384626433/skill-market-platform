export const formatNumber = (value) => new Intl.NumberFormat('zh-CN', {
  notation: Number(value) >= 10000 ? 'compact' : 'standard',
  maximumFractionDigits: 1,
}).format(Number(value) || 0);

export const formatDate = (value, withTime = false) => {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '-';
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit',
    ...(withTime ? { hour: '2-digit', minute: '2-digit', hour12: false } : {}),
  }).format(date);
};

export const safeJson = (value, fallback = {}) => {
  if (!value) return fallback;
  if (typeof value === 'object') return value;
  try { return JSON.parse(value); } catch { return fallback; }
};

export const roleName = { user: '普通用户', developer: '开发者', admin: '平台管理员' };

export const statusMeta = {
  published: { label: '已发布', color: 'success' },
  pending_approval: { label: '待审核', color: 'processing' },
  rejected: { label: '已驳回', color: 'error' },
  draft: { label: '草稿', color: 'default' },
  deprecated: { label: '已弃用', color: 'warning' },
  active: { label: '运行中', color: 'success' },
};

export const categoryMeta = {
  '智能运维': { tone: 'blue', short: '运' },
  '研发效能': { tone: 'indigo', short: '研' },
  '安全合规': { tone: 'green', short: '安' },
  '数据治理': { tone: 'amber', short: '数' },
  '智能客服': { tone: 'cyan', short: '客' },
  '风控分析': { tone: 'red', short: '风' },
};
