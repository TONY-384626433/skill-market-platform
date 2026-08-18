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

export const invokeSkill = (skillKey, method, params) =>
  api.post('/gateway/invoke', { skill_key: skillKey, method, params });
export const checkHealth = () => api.get('/health');

export default api;
