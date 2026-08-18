// ============================================================
// API 服务层 — 统一封装后端接口调用
// 支持构建时指定后端地址: VITE_API_BASE=https://xxx/api/v1
// 默认同源 (/api/v1, 由 vite proxy 转发)
// ============================================================
import axios from 'axios';

const api = axios.create({
  baseURL: import.meta.env.VITE_API_BASE || '/api/v1',
  timeout: 30000,
});

// 请求拦截器 — 自动附加 Token
api.interceptors.request.use((config) => {
  const token = localStorage.getItem('skillhub_token');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// 响应拦截器 — 统一错误处理
api.interceptors.response.use(
  (res) => res.data,
  (err) => {
    if (err.response?.status === 401) {
      localStorage.removeItem('skillhub_token');
      window.location.href = '/login';
    }
    return Promise.reject(err);
  }
);

// ============================================================
// 认证
// ============================================================
export const login = (username, password) =>
  api.post('/auth/login', { username, password });

// 发送验证码 (channel: phone / email)
export const sendCode = (channel, target) =>
  api.post('/auth/send-code', { channel, target });

// 注册 (手机号/邮箱 + 验证码)
export const register = (data) =>
  api.post('/auth/register', data);

// 手机验证码登录 (无账号自动创建)
export const phoneLogin = (phone, code) =>
  api.post('/auth/phone-login', { phone, code });

// 第三方快捷登录 (provider: wechat / qq)
export const oauthLogin = (provider, nickname) =>
  api.post('/auth/oauth', { provider, nickname });

// 快捷登录 (mode: scan / guest)
export const quickLogin = (mode = 'guest') =>
  api.post('/auth/quick', { mode });

// ============================================================
// 技能市场
// ============================================================
export const searchSkills = (params) =>
  api.get('/skills', { params });

export const getSkillDetail = (id) =>
  api.get(`/skills/${id}`);

export const getCategories = () =>
  api.get('/skills/categories');

export const getStats = () =>
  api.get('/skills/stats/overview');

// ============================================================
// 技能管理
// ============================================================
export const createSkill = (data) =>
  api.post('/skills', data);

export const installSkill = (id, version) =>
  api.post(`/skills/${id}/install`, { version });

export const getMyInstallations = () =>
  api.get('/skills/my/installations');

export const revokeInstallation = (instId) =>
  api.delete(`/skills/installations/${instId}`);

// ============================================================
// 评价
// ============================================================
export const rateSkill = (id, data) =>
  api.post(`/skills/${id}/rate`, data);

export const getSkillRatings = (id, params) =>
  api.get(`/skills/${id}/ratings`, { params });

// ============================================================
// 审计
// ============================================================
export const getAuditLogs = (params) =>
  api.get('/skills/audit-logs', { params });

// ============================================================
// 网关调用
// ============================================================
export const invokeSkill = (skillKey, method, params) =>
  api.post('/gateway/invoke', { skill_key: skillKey, method, params });

// ============================================================
// 系统
// ============================================================
export const checkHealth = () =>
  api.get('/health');

export default api;
