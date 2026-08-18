// ============================================================
// Skill Nexus — 主应用入口（科技感重构版）
// ============================================================
import React, { useState, useEffect } from 'react';
import { HashRouter, Routes, Route, Link, useLocation } from 'react-router-dom';
import { ConfigProvider, theme, Layout, Button, Avatar, Dropdown, Space, App as AntApp, Alert } from 'antd';
import { UserOutlined, LogoutOutlined, LoginOutlined, ThunderboltFilled } from '@ant-design/icons';
import { Navigate } from 'react-router-dom';

import MarketPage from './pages/MarketPage';
import SkillDetailPage from './pages/SkillDetailPage';
import DeveloperPage from './pages/DeveloperPage';
import AdminPage from './pages/AdminPage';
import LoginPage from './pages/LoginPage';
import MyInstallationsPage from './pages/MyInstallationsPage';
import ParticleBackground from './components/ParticleBackground';
import { checkHealth } from './services/api';

const { Header, Content, Footer } = Layout;

// 用户上下文
export const UserContext = React.createContext(null);

// ============ 导航项 ============
const NAV_ITEMS = [
  { key: 'market', path: '/', label: '技能市场', icon: '▣' },
  { key: 'my', path: '/my', label: '我的技能', icon: '◈' },
  { key: 'dev', path: '/dev', label: '开发者', icon: '⌘' },
  { key: 'admin', path: '/admin', label: '管理后台', icon: '◉' },
];

function NavItem({ item, active, onClick }) {
  return (
    <Link
      to={item.path}
      onClick={onClick}
      className={`nexus-nav-item ${active ? 'active' : ''}`}
    >
      <span className="nav-icon">{item.icon}</span>
      {item.label}
    </Link>
  );
}

function App() {
  const [user, setUser] = useState(null);
  const [apiOnline, setApiOnline] = useState(true);
  const location = useLocation();

  useEffect(() => {
    const saved = localStorage.getItem('skillhub_user');
    if (saved) {
      try { setUser(JSON.parse(saved)); } catch (e) {}
    }
  }, []);

  // 检测后端 API 是否可达 (静态托管时无后端)
  useEffect(() => {
    checkHealth()
      .then(() => setApiOnline(true))
      .catch(() => setApiOnline(false));
  }, [location.pathname]);

  const handleLogout = () => {
    localStorage.removeItem('skillhub_token');
    localStorage.removeItem('skillhub_user');
    setUser(null);
  };

  const activeKey = NAV_ITEMS.find((n) =>
    n.path === '/' ? location.pathname === '/' : location.pathname.startsWith(n.path)
  )?.key;

  return (
    <ConfigProvider
      theme={{
        algorithm: theme.darkAlgorithm,
        token: {
          colorPrimary: '#22d3ee',
          colorBgBase: '#04060f',
          colorBgContainer: '#0d142c',
          colorText: '#e2e8f0',
          colorTextSecondary: '#94a3b8',
          borderRadius: 10,
          fontFamily: "-apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif",
        },
        components: {
          Layout: { bodyBg: 'transparent', headerBg: 'transparent' },
          Card: { headerBg: 'transparent' },
        },
      }}
    >
      <AntApp>
        <UserContext.Provider value={{ user, setUser }}>
          {/* ========== 背景层 ========== */}
          <div className="nexus-bg" />
          <div className="nexus-grid" />
          <ParticleBackground />
          <div className="nexus-scanline" />

          {/* ========== 主布局 ========== */}
          <Layout className="nexus-layout">
            {/* ========== 顶部导航 ========== */}
            <Header className="nexus-header">
              <Link to="/" className="nexus-logo">
                <span className="nexus-logo-icon">⚡</span>
                <span>
                  <span className="nexus-logo-text">SKILL NEXUS</span>
                  <span className="nexus-logo-badge">v1.0 · NEXUS OS</span>
                </span>
              </Link>

              <nav className="nexus-nav">
                {NAV_ITEMS.map((item) => (
                  <NavItem key={item.key} item={item} active={item.key === activeKey} />
                ))}
              </nav>

              <Space size={12}>
                {user ? (
                  <Dropdown
                    menu={{
                      items: [
                        {
                          key: 'profile',
                          icon: <UserOutlined />,
                          label: (
                            <span>
                              {user.username}
                              <span className="nexus-role-badge">{user.role}</span>
                            </span>
                          ),
                        },
                        { type: 'divider' },
                        { key: 'logout', icon: <LogoutOutlined />, label: '退出登录', danger: true },
                      ],
                      onClick: ({ key }) => { if (key === 'logout') handleLogout(); },
                    }}
                  >
                    <Button className="nexus-user-btn">
                      <Avatar size="small" style={{ background: 'linear-gradient(135deg,#22d3ee,#8b5cf6)', marginRight: 8, fontSize: 12 }}>
                        {user.username?.[0]?.toUpperCase()}
                      </Avatar>
                      {user.username}
                    </Button>
                  </Dropdown>
                ) : (
                  <Link to="/login">
                    <Button className="glow-btn glow-btn-ghost" icon={<LoginOutlined />}>登录</Button>
                  </Link>
                )}
              </Space>
            </Header>

            {/* ========== 内容区 ========== */}
            <Content className="nexus-content">
              {!apiOnline && (
                <Alert
                  type="warning"
                  showIcon
                  message="当前为静态预览模式: 后端 API 未连接, 技能数据与调用功能需本地运行完整环境"
                  description="本地运行: cd docker && docker compose up -d --build → cd backend && go run cmd/main.go → cd frontend && npm start"
                  style={{ marginBottom: 16, borderRadius: 10, background: 'rgba(251,191,36,0.08)', border: '1px solid rgba(251,191,36,0.3)' }}
                />
              )}
              <Routes>
                <Route path="/" element={<MarketPage />} />
                <Route path="/skills/:id" element={<SkillDetailPage />} />
                <Route path="/dev" element={<DeveloperPage />} />
                <Route path="/admin" element={<AdminPage />} />
                <Route path="/my" element={<MyInstallationsPage />} />
                <Route path="/login" element={<LoginPage />} />
                <Route path="*" element={<Navigate to="/" replace />} />
              </Routes>
            </Content>

            {/* ========== 底部 ========== */}
            <Footer className="nexus-footer">
              <span className="footer-glow">◈ SKILL NEXUS</span> · 私有 AI 技能市场与配套机制 · v1.0
              <br />
              中南财经政法大学 × 九江银行 · 金融科技专项赛
            </Footer>
          </Layout>
        </UserContext.Provider>
      </AntApp>
    </ConfigProvider>
  );
}

// 包一层 HashRouter（GitHub Pages 等静态托管兼容, 刷新/子路由不 404）
function AppWithRouter() {
  return (
    <HashRouter>
      <App />
    </HashRouter>
  );
}

export default AppWithRouter;
