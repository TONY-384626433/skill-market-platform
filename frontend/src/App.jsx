import React, { Suspense, useCallback, useEffect, useMemo, useState } from 'react';
import { HashRouter, Link, Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom';
import { Alert, App as AntApp, Avatar, Badge, Button, ConfigProvider, Drawer, Dropdown, Layout, Popover, Skeleton, theme as antdTheme, Tooltip } from 'antd';
import {
  ApiOutlined, AppstoreOutlined, AuditOutlined, BankOutlined, BellOutlined, CodeOutlined,
  LoginOutlined, LogoutOutlined, MenuOutlined, PlusOutlined, SafetyCertificateOutlined,
  SearchOutlined, ToolOutlined, UserOutlined,
} from '@ant-design/icons';
import AuthGate from './components/AuthGate';
import { checkHealth } from './services/api';
import { roleName } from './utils/format';

const { Sider, Content } = Layout;
export const UserContext = React.createContext(null);

const MarketPage = React.lazy(() => import('./pages/MarketPage'));
const SkillDetailPage = React.lazy(() => import('./pages/SkillDetailPage'));
const DeveloperPage = React.lazy(() => import('./pages/DeveloperPage'));
const AdminPage = React.lazy(() => import('./pages/AdminPage'));
const LoginPage = React.lazy(() => import('./pages/LoginPage'));
const MyInstallationsPage = React.lazy(() => import('./pages/MyInstallationsPage'));
const CommandPalette = React.lazy(() => import('./components/CommandPalette'));
const TechBackdrop = React.lazy(() => import('./components/TechBackdrop'));
const BootScreen = React.lazy(() => import('./components/BootScreen'));

const allNavigation = [
  { key: 'market', path: '/', label: '技能市场', icon: AppstoreOutlined },
  { key: 'my', path: '/my', label: '我的技能', icon: ToolOutlined, authenticated: true },
  { key: 'dev', path: '/dev', label: '开发者工作台', icon: CodeOutlined, roles: ['developer', 'admin'] },
  { key: 'admin', path: '/admin', label: '平台治理', icon: AuditOutlined, roles: ['admin'] },
];

const pageNames = { '/': '技能市场', '/my': '我的技能', '/dev': '开发者工作台', '/admin': '平台治理', '/login': '企业账号登录' };

function Brand() {
  return <Link className="brand" to="/" aria-label="九江银行 SkillHub 首页"><span className="brand-mark"><BankOutlined /><i /></span><span><strong>SkillHub</strong><small>AI CAPABILITY OS</small></span></Link>;
}

function Navigation({ items, onNavigate }) {
  const location = useLocation();
  return (
    <nav className="app-nav" aria-label="主导航">
      {items.map(({ key, path, label, icon: Icon }) => {
        const active = path === '/' ? location.pathname === '/' : location.pathname.startsWith(path);
        return <Link key={key} to={path} onClick={onNavigate} className={active ? 'active' : ''}><Icon /><span>{label}</span></Link>;
      })}
    </nav>
  );
}

function Shell() {
  const [user, setUser] = useState(() => {
    try { return JSON.parse(localStorage.getItem('skillhub_user')) || null; } catch { return null; }
  });
  const [apiOnline, setApiOnline] = useState(true);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [commandOpen, setCommandOpen] = useState(false);
  const location = useLocation();
  const navigate = useNavigate();

  useEffect(() => { checkHealth().then(() => setApiOnline(true)).catch(() => setApiOnline(false)); }, [location.pathname]);
  useEffect(() => {
    const handleCommand = (event) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault();
        setCommandOpen(true);
      }
    };
    window.addEventListener('keydown', handleCommand);
    return () => window.removeEventListener('keydown', handleCommand);
  }, []);

  const navItems = useMemo(() => allNavigation.filter((item) => {
    if (item.authenticated && !user) return false;
    if (item.roles && !item.roles.includes(user?.role)) return false;
    return true;
  }), [user]);

  const logout = () => {
    localStorage.removeItem('skillhub_token');
    localStorage.removeItem('skillhub_user');
    setUser(null);
    navigate('/');
  };

  const pathname = location.pathname.startsWith('/skills/') ? '/skills' : location.pathname;
  const title = pathname === '/skills' ? '技能详情' : (pageNames[pathname] || 'SkillHub');
  const isLogin = location.pathname === '/login';
  const userMenu = {
    items: [
      { key: 'identity', label: <div className="identity-menu"><strong>{user?.username}</strong><span>{roleName[user?.role] || user?.role}</span></div>, disabled: true },
      { type: 'divider' },
      { key: 'logout', icon: <LogoutOutlined />, label: '退出登录', danger: true },
    ],
    onClick: ({ key }) => key === 'logout' && logout(),
  };

  const systemPanel = (
    <div className="system-panel">
      <div className="system-panel-head"><span><ApiOutlined /></span><div><strong>系统连接</strong><small>实时服务状态</small></div></div>
      <div className="system-check"><span><i className={apiOnline ? 'online' : 'offline'} />API 服务</span><b>{apiOnline ? '运行正常' : '连接中断'}</b></div>
      <div className="system-check"><span><i className="online" />身份域</span><b>已连接</b></div>
      <div className="system-check"><span><i className="online" />审计网关</span><b>已启用</b></div>
    </div>
  );

  return (
    <UserContext.Provider value={{ user, setUser, logout }}>
      <Layout className={`app-shell ${isLogin ? 'login-shell' : ''}`}>
        <Sider width={240} className="app-sider" breakpoint="lg" collapsedWidth="0" trigger={null}>
          <Brand />
          <div className="nav-section-label">工作区</div>
          <Navigation items={navItems} />
          <button className="sider-command" type="button" onClick={() => setCommandOpen(true)}><SearchOutlined /><span><strong>全局搜索</strong><small>技能与工作区</small></span></button>
          <div className="sider-status"><span className={`status-dot ${apiOnline ? 'online' : 'offline'}`} /><div><strong>{apiOnline ? '服务运行正常' : '服务暂不可用'}</strong><small>API 与治理网关</small></div></div>
          <div className="sider-trust"><SafetyCertificateOutlined /><span>仅限企业内网授权访问<br />操作全程留痕审计</span></div>
        </Sider>

        <Layout>
          <header className="topbar">
            <Button className="mobile-menu" type="text" icon={<MenuOutlined />} onClick={() => setDrawerOpen(true)} aria-label="打开导航" />
            <div className="topbar-title"><span>{title}</span><small><i className={apiOnline ? 'online' : 'offline'} /> 内部服务</small></div>
            <button className="topbar-search" type="button" onClick={() => setCommandOpen(true)}><SearchOutlined /><span>搜索技能、能力或工作区</span></button>
            <div className="topbar-actions">
              {['developer', 'admin'].includes(user?.role) && <Tooltip title="发布新技能"><Button className="topbar-create" type="text" icon={<PlusOutlined />} onClick={() => navigate('/dev')} /></Tooltip>}
              <Popover content={systemPanel} placement="bottomRight" trigger="click"><Tooltip title="系统状态"><Button className="topbar-icon" type="text" icon={<Badge dot status={apiOnline ? 'success' : 'error'}><BellOutlined /></Badge>} /></Tooltip></Popover>
              {user ? (
                <Dropdown menu={userMenu} trigger={['click']}>
                  <Button type="text" className="user-trigger"><Avatar size={30} icon={<UserOutlined />} /><span className="user-copy"><strong>{user.username}</strong><small>{roleName[user.role] || user.role}</small></span></Button>
                </Dropdown>
              ) : <Button type="primary" icon={<LoginOutlined />} onClick={() => navigate('/login')}>登录</Button>}
            </div>
          </header>

          <Content className="app-content">
            {!apiOnline && <Alert className="service-alert" type="warning" showIcon closable message="后端服务未连接，当前无法加载实时数据或执行技能。" />}
            <Suspense fallback={<div className="route-loading"><Skeleton active paragraph={{ rows: 8 }} /></div>}>
              <Routes>
                <Route path="/" element={<MarketPage />} />
                <Route path="/skills/:id" element={<SkillDetailPage />} />
                <Route path="/my" element={<AuthGate><MyInstallationsPage /></AuthGate>} />
                <Route path="/dev" element={<AuthGate roles={['developer', 'admin']}><DeveloperPage /></AuthGate>} />
                <Route path="/admin" element={<AuthGate roles={['admin']}><AdminPage /></AuthGate>} />
                <Route path="/login" element={user ? <Navigate to="/" replace /> : <LoginPage />} />
                <Route path="*" element={<Navigate to="/" replace />} />
              </Routes>
            </Suspense>
          </Content>

          <nav className="mobile-bottom-nav" aria-label="移动端导航">
            {allNavigation.slice(0, 2).map(({ key, path, label, icon: Icon }) => <Link key={key} to={path} className={(path === '/' ? location.pathname === '/' : location.pathname.startsWith(path)) ? 'active' : ''}><Icon /><span>{label}</span></Link>)}
            <Tooltip title={user ? '账号' : '登录'}><button onClick={() => user ? setDrawerOpen(true) : navigate('/login')}><UserOutlined /><span>{user ? '账号' : '登录'}</span></button></Tooltip>
          </nav>
        </Layout>
      </Layout>

      <Drawer className="mobile-drawer" placement="left" open={drawerOpen} onClose={() => setDrawerOpen(false)} width={286} title={<Brand />}>
        <Navigation items={navItems} onNavigate={() => setDrawerOpen(false)} />
        <Button className="drawer-search" block icon={<SearchOutlined />} onClick={() => { setDrawerOpen(false); setCommandOpen(true); }}>搜索技能与工作区</Button>
        {user && <Button danger block icon={<LogoutOutlined />} onClick={logout}>退出登录</Button>}
      </Drawer>
      {commandOpen && <Suspense fallback={null}><CommandPalette open={commandOpen} onClose={() => setCommandOpen(false)} user={user} /></Suspense>}
    </UserContext.Provider>
  );
}

export default function App() {
  const [booting, setBooting] = useState(() => {
    try { return sessionStorage.getItem('skillhub_booted') !== '1'; } catch { return true; }
  });
  const finishBoot = useCallback(() => {
    try { sessionStorage.setItem('skillhub_booted', '1'); } catch { /* storage may be unavailable */ }
    setBooting(false);
  }, []);

  return (
    <><Suspense fallback={null}><TechBackdrop /></Suspense><ConfigProvider theme={{
      algorithm: antdTheme.darkAlgorithm,
      token: {
        colorPrimary: '#39d9ff', colorInfo: '#39d9ff', colorSuccess: '#35e3ae', colorWarning: '#ffbd62', colorError: '#ff6489',
        colorText: '#eaf4ff', colorTextSecondary: '#91a5bc', colorBorder: '#29425f', colorBgLayout: '#050a13', colorBgContainer: '#0b1626', borderRadius: 6,
        fontFamily: "'Inter Variable', 'Noto Sans SC Variable', 'Segoe UI', 'Microsoft YaHei UI', sans-serif",
      },
      components: { Button: { controlHeight: 36, fontWeight: 600 }, Table: { headerBg: '#101d30', headerColor: '#a8bad0' }, Tabs: { itemSelectedColor: '#4ee2ff', inkBarColor: '#4ee2ff' }, Modal: { borderRadiusLG: 8 }, Input: { activeShadow: '0 0 0 2px rgba(65,220,255,.14), 0 0 24px rgba(65,220,255,.12)' } },
    }}>
      <AntApp><HashRouter><Shell /></HashRouter></AntApp>
    </ConfigProvider>{booting && <Suspense fallback={null}><BootScreen onComplete={finishBoot} /></Suspense>}</>
  );
}
