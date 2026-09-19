import axios from 'axios';

const api = axios.create({
  baseURL: import.meta.env.VITE_API_BASE || '/api/v1',
  timeout: 30000,
  headers: { 'Content-Type': 'application/json' },
});

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('skillhub_token');
  if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});

api.interceptors.response.use(
  (response) => response.data,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('skillhub_token');
      localStorage.removeItem('skillhub_user');
      if (!window.location.hash.includes('/login')) window.location.hash = '#/login';
    }
    return Promise.reject(error);
  },
);

export const login = (username, password) => api.post('/auth/login', { username, password });
export const sendCode = (channel, target) => api.post('/auth/send-code', { channel, target });
export const register = (data) => api.post('/auth/register', data);
export const phoneLogin = (phone, code) => api.post('/auth/phone-login', { phone, code });
export const quickLogin = (mode = 'guest') => api.post('/auth/quick', { mode });

export const searchSkills = (params) => api.get('/skills', { params });
export const getSkillDetail = (id) => api.get(`/skills/${id}`);
export const getCategories = () => api.get('/skills/categories');
export const getStats = () => api.get('/skills/stats/overview');
export const getGitHubStatus = () => api.get('/github/status');
export const searchGitHubSkills = (params) => api.get('/github/skills/search', { params, timeout: 45000 });
export const getGitHubSkillPreview = (skill) => api.get('/github/skills/preview', {
  params: { repo: skill.repository, ref: skill.ref, path: skill.path },
  timeout: 30000,
});
export const getGitHubSkillDownloadURL = (skill) => {
  const base = (import.meta.env.VITE_API_BASE || '/api/v1').replace(/\/$/, '');
  const params = new URLSearchParams({ repo: skill.repository, ref: skill.ref, path: skill.path });
  return `${base}/github/skills/download?${params.toString()}`;
};

export const createSkill = (data) => api.post('/skills', data);
export const getMySubmissions = () => api.get('/skills/my/submissions');
export const installSkill = (id, version) => api.post(`/skills/${id}/install`, { version });
export const getMyInstallations = () => api.get('/skills/my/installations');
export const revokeInstallation = (id) => api.delete(`/skills/installations/${id}`);

export const rateSkill = (id, data) => api.post(`/skills/${id}/rate`, data);
export const getSkillRatings = (id, params) => api.get(`/skills/${id}/ratings`, { params });

export const getReviewQueue = () => api.get('/admin/review-queue');
export const reviewSkill = (id, data) => api.post(`/admin/skills/${id}/review`, data);
export const getAuditLogs = (params) => api.get('/admin/audit-logs', { params });

// ---------- 技能可用性审核 ----------
export const getAuditOverview = () => api.get('/admin/audit-overview');export const getAuditQueue = (params) => api.get('/admin/audit-queue', { params });
export const getRecentAudits = (params) => api.get('/admin/audits/recent', { params });
export const getAuditDetail = (auditId) => api.get(`/admin/audits/${auditId}`);
export const getSkillAudits = (id, params) => api.get(`/admin/skills/${id}/audits`, { params });
export const runSkillAudit = (id) => api.post(`/admin/skills/${id}/audit`, {}, { timeout: 120000 });
export const selfCheckSkill = (id) => api.post(`/skills/${id}/self-check`, {}, { timeout: 120000 });
export const getSkillAuditBadge = (id) => api.get(`/skills/${id}/audit-badge`);

export const invokeSkill = (skillKey, method, params) =>
  api.post('/gateway/invoke', { skill_key: skillKey, method, params });
export const checkHealth = () => api.get('/health');

// ---------- AI 智能体 ----------
export const getAgentStatus = () => api.get('/agent/status');
export const getAgentTools = () => api.get('/agent/tools');
export const sendAgentMessage = (text, history) => api.post('/agent/chat', { message: text, history }, { timeout: 190000 });

// 技能推荐 Agent (需求 → 推荐技能)
export const getRecommendStatus = () => api.get('/agent/recommend/status');
export const recommendSkills = (message, topN) => api.post('/agent/recommend', { message, top_n: topN }, { timeout: 120000 });

// ---------- 技能安全治理 (查毒 / 防盗用溯源 / 导入门禁) ----------
export const getSecurityRules = () => api.get('/security/rules');
export const getSecurityOverview = () => api.get('/admin/security-overview');
export const getSecurityQueue = (params) => api.get('/admin/security-queue', { params });
export const getSecurityScans = (params) => api.get('/admin/security-scans', { params });
export const getSecurityScanDetail = (scanId) => api.get(`/admin/security-scans/${scanId}`);
export const runSkillSecurityScan = (id) => api.post(`/admin/skills/${id}/security-scan`, {}, { timeout: 180000 });
export const getSkillSecurityScans = (id, params) => api.get(`/admin/skills/${id}/security-scans`, { params });
export const getSkillProvenance = (id) => api.get(`/admin/skills/${id}/provenance`);
export const verifySkillProvenance = (id) => api.post(`/admin/skills/${id}/provenance/verify`, {});
export const getDownloadAudits = (params) => api.get('/admin/download-audits', { params });
export const getImportRequests = (params) => api.get('/admin/import-requests', { params });
export const scanImportRequest = (reqId) => api.post(`/admin/import-requests/${reqId}/scan`, {}, { timeout: 180000 });
export const decideImportRequest = (reqId, approve, note) => api.post(`/admin/import-requests/${reqId}/decision`, { approve, note });
export const getSkillSecurityBadge = (id) => api.get(`/skills/${id}/security-badge`);
export const submitImportRequest = (repo, ref, path, skillUrl) =>
  api.post('/github/import-requests', { repository: repo, ref, path, skill_url: skillUrl }, { timeout: 180000 });
export const getImportRequest = (reqId) => api.get(`/github/import-requests/${reqId}`);
export const getSecurityRulesExportURL = () => `${(import.meta.env.VITE_API_BASE || '/api/v1').replace(/\/$/, '')}/admin/security-rules/export`;
// 三道防线 (第一道 静态/AST 语义 · 第二道 沙箱 · 第三道 AI 语义审计)
export const getDefenseStatus = () => api.get('/admin/security/defense-status');
export const runSemanticAudit = (payload) => api.post('/admin/security/semantic-audit', payload, { timeout: 120000 });
export const scanPackage = (file) => {
  const form = new FormData();
  form.append('file', file);
  return api.post('/admin/security/scan-package', form, { headers: { 'Content-Type': 'multipart/form-data' }, timeout: 180000 });
};

export default api;
