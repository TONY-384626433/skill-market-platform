import React, { useContext, useEffect, useState } from 'react';
import { Alert, App as AntApp, Button, Form, Input, Segmented, Tabs } from 'antd';
import {
  ApiOutlined, AuditOutlined, BankOutlined, CheckCircleFilled, CodeOutlined,
  IdcardOutlined, KeyOutlined, LockOutlined, LoginOutlined, MobileOutlined,
  SafetyCertificateOutlined, TeamOutlined, UserOutlined,
} from '@ant-design/icons';
import { useLocation, useNavigate } from 'react-router-dom';
import { UserContext } from '../App';
import { login, phoneLogin, quickLogin, register, sendCode } from '../services/api';

const demoAccounts = [
  { username: 'admin', role: '平台管理员', scope: '审核与治理', icon: AuditOutlined },
  { username: 'zhangsan', role: '技能开发者', scope: '发布与调试', icon: CodeOutlined },
  { username: 'zhaoliu', role: '业务用户', scope: '安装与调用', icon: TeamOutlined },
];

export default function LoginPage() {
  const { message } = AntApp.useApp();
  const { setUser } = useContext(UserContext);
  const navigate = useNavigate();
  const location = useLocation();
  const [loginForm] = Form.useForm();
  const [registerForm] = Form.useForm();
  const [mode, setMode] = useState('password');
  const [loading, setLoading] = useState(false);
  const [demoLoading, setDemoLoading] = useState('');
  const [countdown, setCountdown] = useState(0);

  useEffect(() => {
    if (!countdown) return undefined;
    const timer = window.setInterval(() => setCountdown((value) => Math.max(0, value - 1)), 1000);
    return () => window.clearInterval(timer);
  }, [countdown]);

  const completeLogin = (response) => {
    if (!response?.token || !response?.user) throw new Error('登录响应不完整');
    localStorage.setItem('skillhub_token', response.token);
    localStorage.setItem('skillhub_user', JSON.stringify(response.user));
    setUser(response.user);
    message.success(`欢迎进入 SkillHub，${response.user.display_name || response.user.username}`);
    navigate(location.state?.from || '/', { replace: true });
  };

  const submitLogin = async (values) => {
    setLoading(true);
    try {
      completeLogin(mode === 'password' ? await login(values.username, values.password) : await phoneLogin(values.phone, values.code));
    } catch (requestError) { message.error(requestError.response?.data?.error || requestError.message || '登录失败'); }
    finally { setLoading(false); }
  };

  const requestCode = async (target, channel = 'phone', form = loginForm) => {
    if (!target) return message.warning(channel === 'phone' ? '请先输入手机号' : '请先输入邮箱');
    try {
      const response = await sendCode(channel, target);
      setCountdown(60);
      if (response.dev_code) {
        form.setFieldValue('code', response.dev_code);
        message.success(`演示验证码：${response.dev_code}`);
      } else message.success('验证码已发送');
    } catch (requestError) { message.error(requestError.response?.data?.error || '验证码发送失败'); }
  };

  const submitRegister = async (values) => {
    setLoading(true);
    try {
      completeLogin(await register({ channel: 'email', target: values.email, code: values.code, username: values.username, password: values.password, display_name: values.display_name }));
    } catch (requestError) { message.error(requestError.response?.data?.error || '注册失败'); }
    finally { setLoading(false); }
  };

  const guestLogin = async () => {
    setLoading(true);
    try { completeLogin(await quickLogin('guest')); }
    catch (requestError) { message.error(requestError.response?.data?.error || '访客登录失败'); }
    finally { setLoading(false); }
  };

  const demoLogin = async (username) => {
    setDemoLoading(username);
    try { completeLogin(await login(username, 'demo')); }
    catch (requestError) { message.error(requestError.response?.data?.error || '演示账号登录失败'); }
    finally { setDemoLoading(''); }
  };

  const loginPanel = (
    <div>
      <Segmented block value={mode} onChange={setMode} options={[{ value: 'password', label: '账号密码' }, { value: 'phone', label: '手机验证' }]} />
      <Form form={loginForm} layout="vertical" onFinish={submitLogin} requiredMark={false} className="login-form">
        {mode === 'password' ? <>
          <Form.Item name="username" label="企业账号" rules={[{ required: true, message: '请输入企业账号' }]}><Input size="large" prefix={<UserOutlined />} placeholder="请输入用户名" autoComplete="username" /></Form.Item>
          <Form.Item name="password" label="密码" rules={[{ required: true, message: '请输入密码' }]}><Input.Password size="large" prefix={<LockOutlined />} placeholder="请输入密码" autoComplete="current-password" /></Form.Item>
          <div className="login-options"><span><SafetyCertificateOutlined /> 仅限已授权企业账号</span><Button type="link" onClick={() => message.info('请联系企业 IAM 管理员重置密码')}>忘记密码</Button></div>
        </> : <>
          <Form.Item name="phone" label="手机号" rules={[{ required: true, message: '请输入手机号' }, { pattern: /^1\d{10}$/, message: '手机号格式不正确' }]}><Input size="large" prefix={<MobileOutlined />} maxLength={11} placeholder="企业预留手机号" /></Form.Item>
          <Form.Item label="验证码" required><div className="code-field"><Form.Item name="code" noStyle rules={[{ required: true, message: '请输入验证码' }]}><Input size="large" prefix={<IdcardOutlined />} maxLength={6} placeholder="6 位验证码" /></Form.Item><Button size="large" disabled={countdown > 0} onClick={() => requestCode(loginForm.getFieldValue('phone'))}>{countdown ? `${countdown}s` : '获取验证码'}</Button></div></Form.Item>
        </>}
        <Button className="login-submit" type="primary" size="large" block htmlType="submit" icon={<LoginOutlined />} loading={loading}>安全登录</Button>
      </Form>
      <div className="login-divider"><span>或选择演示身份快速进入</span></div>
      <div className="login-account-grid">
        {demoAccounts.map(({ username, role, scope, icon: Icon }) => <button key={username} type="button" onClick={() => demoLogin(username)} disabled={Boolean(demoLoading)}><span><Icon /></span><strong>{role}</strong><small>{demoLoading === username ? '正在进入...' : scope}</small></button>)}
      </div>
      <Button className="guest-button" type="text" block loading={loading} onClick={guestLogin}>以访客身份浏览市场</Button>
    </div>
  );

  const registerPanel = (
    <Form form={registerForm} layout="vertical" onFinish={submitRegister} requiredMark={false} className="login-form register-form">
      <Alert type="info" showIcon message="体验环境注册" description="生产环境账号由企业统一身份系统同步。" />
      <div className="form-grid two"><Form.Item name="display_name" label="姓名" rules={[{ required: true }]}><Input prefix={<UserOutlined />} placeholder="展示姓名" /></Form.Item><Form.Item name="username" label="用户名" rules={[{ required: true }, { min: 3 }]}><Input prefix={<KeyOutlined />} placeholder="至少 3 个字符" /></Form.Item></div>
      <Form.Item name="email" label="企业邮箱" rules={[{ required: true }, { type: 'email' }]}><Input placeholder="name@jjbank.com" /></Form.Item>
      <Form.Item label="邮箱验证码" required><div className="code-field"><Form.Item name="code" noStyle rules={[{ required: true }]}><Input maxLength={6} placeholder="6 位验证码" /></Form.Item><Button disabled={countdown > 0} onClick={() => requestCode(registerForm.getFieldValue('email'), 'email', registerForm)}>{countdown ? `${countdown}s` : '获取验证码'}</Button></div></Form.Item>
      <Form.Item name="password" label="密码" rules={[{ required: true }, { min: 6 }]}><Input.Password prefix={<LockOutlined />} placeholder="至少 6 位" /></Form.Item>
      <Button type="primary" size="large" block htmlType="submit" loading={loading}>注册并进入</Button>
    </Form>
  );

  return (
    <div className="login-page">
      <section className="login-context">
        <div className="login-context-head"><span className="login-bank-mark"><BankOutlined /></span><div><strong>九江银行 SkillHub</strong><small>AI CAPABILITY OPERATING SYSTEM</small></div><em><i /> SECURE</em></div>
        <div className="login-context-copy"><p className="eyebrow"><span />企业 AI 能力控制台</p><h1>连接可信能力<br />驱动业务执行</h1><p>统一身份、统一网关、统一审计，让每一次 AI 调用都有边界、有凭证、可追溯。</p></div>
        <div className="access-map" aria-hidden="true">
          <div className="access-core"><BankOutlined /><strong>SkillHub</strong><small>TRUST CORE</small></div>
          <span className="access-line line-one" /><span className="access-line line-two" /><span className="access-line line-three" />
          <div className="access-node node-auth"><SafetyCertificateOutlined /><span>IAM</span></div>
          <div className="access-node node-api"><ApiOutlined /><span>API</span></div>
          <div className="access-node node-audit"><AuditOutlined /><span>AUDIT</span></div>
          <i className="access-pulse pulse-one" /><i className="access-pulse pulse-two" /><i className="access-pulse pulse-three" />
        </div>
        <div className="login-assurance"><span><CheckCircleFilled />企业身份认证</span><span><CheckCircleFilled />访问凭据隔离</span><span><CheckCircleFilled />全链路审计</span></div>
      </section>
      <section className="login-panel">
        <div className="login-mobile-brand"><span><BankOutlined /></span><div><strong>SkillHub</strong><small>九江银行 AI 能力中心</small></div></div>
        <div className="login-panel-heading"><span className="login-panel-icon"><LockOutlined /></span><div><h2>进入企业工作区</h2><p>身份验证通过后加载对应角色权限</p></div><em><i /> 服务在线</em></div>
        <Tabs items={[{ key: 'login', label: '企业登录', children: loginPanel }, { key: 'register', label: '体验账号注册', children: registerPanel }]} />
        <footer><SafetyCertificateOutlined /> TLS 加密连接 · 登录行为已纳入安全审计</footer>
      </section>
    </div>
  );
}
