// ============================================================
// 登录/注册页 — 科技感重构版
// 支持: 账号密码 / 手机验证码 / 邮箱验证码 / 微信 / QQ / 扫码 / 游客
// ============================================================
import React, { useState, useContext, useEffect, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { Form, Input, Button, message, Typography, Modal, Spin } from 'antd';
import {
  UserOutlined, LockOutlined, LoginOutlined, SafetyCertificateOutlined,
  WechatOutlined, QqOutlined, MailOutlined, QrcodeOutlined,
  MobileOutlined, SmileOutlined, CheckCircleOutlined,
  IdcardOutlined, ThunderboltOutlined
} from '@ant-design/icons';
import {
  login, phoneLogin, register, sendCode, oauthLogin, quickLogin
} from '../services/api';
import { UserContext } from '../App';
import QrMock from '../components/QrMock';

const { Text } = Typography;

// 保存登录态
const saveAuth = (res) => {
  localStorage.setItem('skillhub_token', res.token);
  localStorage.setItem('skillhub_user', JSON.stringify(res.user));
  return res.user;
};

export default function LoginPage() {
  const navigate = useNavigate();
  const { setUser } = useContext(UserContext);

  const [tab, setTab] = useState('login');              // login / register
  const [loginMode, setLoginMode] = useState('password'); // password / phone
  const [regMode, setRegMode] = useState('phone');        // phone / email
  const [loading, setLoading] = useState(false);
  const [oauthLoading, setOauthLoading] = useState('');
  const [countdown, setCountdown] = useState(0);
  const [scanOpen, setScanOpen] = useState(false);
  const [scanState, setScanState] = useState('waiting'); // waiting / confirming / done
  const countdownRef = useRef(null);

  const [loginForm] = Form.useForm();
  const [regForm] = Form.useForm();

  // 验证码倒计时
  useEffect(() => {
    if (countdown > 0) {
      countdownRef.current = setTimeout(() => setCountdown((c) => c - 1), 1000);
    }
    return () => clearTimeout(countdownRef.current);
  }, [countdown]);

  useEffect(() => {
    return () => clearTimeout(countdownRef.current);
  }, []);

  const handleSendCode = async (channel, target) => {
    if (!target) {
      message.warning(channel === 'phone' ? '请先填写手机号' : '请先填写邮箱');
      return;
    }
    try {
      const res = await sendCode(channel, target);
      const tip = res.dev_code ? `验证码: ${res.dev_code}（演示环境直接显示）` : '验证码已发送';
      message.success(tip);
      if (res.dev_code) {
        // 演示环境自动填充验证码
        if (channel === 'phone') {
          loginForm.setFieldsValue({ code: res.dev_code });
          regForm.setFieldsValue({ code: res.dev_code });
        } else {
          regForm.setFieldsValue({ code: res.dev_code });
        }
      }
      setCountdown(60);
    } catch (e) {
      message.error(e.response?.data?.error || '验证码发送失败');
    }
  };

  // ============ 登录 ============
  const onLogin = async (values) => {
    setLoading(true);
    try {
      let res;
      if (loginMode === 'phone') {
        res = await phoneLogin(values.phone, values.code);
      } else {
        res = await login(values.username, values.password);
      }
      const user = saveAuth(res);
      setUser(user);
      message.success(`欢迎回来, ${user.username}!`);
      navigate('/');
    } catch (e) {
      message.error(e.response?.data?.error || '登录失败, 请检查输入');
    }
    setLoading(false);
  };

  // ============ 注册 ============
  const onRegister = async (values) => {
    setLoading(true);
    try {
      const res = await register({
        channel: regMode,
        target: regMode === 'phone' ? values.phone : values.email,
        code: values.code,
        username: values.username,
        password: values.password,
        display_name: values.nickname,
      });
      const user = saveAuth(res);
      setUser(user);
      message.success(`注册成功, 已自动登录! 欢迎 ${user.username} ⚡`);
      navigate('/');
    } catch (e) {
      message.error(e.response?.data?.error || '注册失败');
    }
    setLoading(false);
  };

  // ============ 第三方快捷登录 ============
  const onOAuth = async (provider) => {
    setOauthLoading(provider);
    try {
      const nickname = provider === 'wechat' ? '微信用户' : 'QQ用户';
      const res = await oauthLogin(provider, nickname);
      const user = saveAuth(res);
      setUser(user);
      message.success(`欢迎回来, ${user.username}!`);
      navigate('/');
    } catch (e) {
      message.error('第三方登录失败');
    }
    setOauthLoading('');
  };

  // ============ 游客快捷登录 ============
  const onGuestLogin = async () => {
    setOauthLoading('guest');
    try {
      const res = await quickLogin('guest');
      const user = saveAuth(res);
      setUser(user);
      message.success('已以访客身份进入, 可随时注册完整账号');
      navigate('/');
    } catch (e) {
      message.error('访客登录失败');
    }
    setOauthLoading('');
  };

  // ============ 扫码登录 ============
  const openScan = () => {
    setScanState('waiting');
    setScanOpen(true);
  };

  const mockScan = async () => {
    if (scanState === 'done') return;
    setScanState('confirming');
    setTimeout(async () => {
      setScanState('done');
      try {
        const res = await quickLogin('scan');
        const user = saveAuth(res);
        setUser(user);
        message.success('扫码登录成功!');
        setTimeout(() => { setScanOpen(false); navigate('/'); }, 600);
      } catch (e) {
        message.error('扫码登录失败');
        setScanState('waiting');
      }
    }, 1300);
  };

  const scanStatusText = {
    waiting: '请使用手机扫码登录',
    confirming: '已扫码, 请在手机上确认...',
    done: '登录成功, 正在进入...',
  };

  return (
    <div style={{
      display: 'flex', justifyContent: 'center', alignItems: 'center',
      minHeight: 'calc(100vh - 140px)', position: 'relative', padding: '24px 0'
    }}>
      {/* 光环装饰 */}
      <div style={{
        position: 'absolute', width: 460, height: 460, borderRadius: '50%',
        background: 'radial-gradient(circle, rgba(34,211,238,0.15), transparent 65%)',
        top: '6%', left: '12%', filter: 'blur(10px)', animation: 'floaty 7s ease-in-out infinite'
      }} />
      <div style={{
        position: 'absolute', width: 420, height: 420, borderRadius: '50%',
        background: 'radial-gradient(circle, rgba(139,92,246,0.15), transparent 65%)',
        bottom: '6%', right: '12%', filter: 'blur(10px)', animation: 'floaty 9s ease-in-out infinite reverse'
      }} />

      <div className="gradient-border fade-up" style={{ width: 500, padding: 2, borderRadius: 20 }}>
        <div style={{
          background: 'rgba(9, 14, 32, 0.94)', backdropFilter: 'blur(24px)',
          borderRadius: 19, padding: '36px 40px 30px', position: 'relative', overflow: 'hidden'
        }}>
          {/* 顶部光条 */}
          <div style={{
            position: 'absolute', top: 0, left: '15%', right: '15%', height: 2,
            background: 'linear-gradient(90deg, transparent, var(--cyan), var(--violet), transparent)',
            boxShadow: '0 0 24px rgba(34,211,238,0.7)'
          }} />

          {/* Logo */}
          <div style={{ textAlign: 'center', marginBottom: 24 }}>
            <div style={{
              width: 60, height: 60, borderRadius: 16, margin: '0 auto 14px',
              background: 'var(--gradient-main)', display: 'flex', alignItems: 'center', justifyContent: 'center',
              fontSize: 28, boxShadow: 'var(--shadow-glow-cyan)', animation: 'logoPulse 3s ease infinite'
            }}>⚡</div>
            <h2 style={{ fontSize: 22, fontWeight: 800, margin: '0 0 4px', color: 'var(--text-primary)', letterSpacing: 1 }}>
              SKILL <span className="gradient-text">NEXUS</span>
            </h2>
            <Text style={{ color: 'var(--text-dim)', fontSize: 12, letterSpacing: 2, fontFamily: 'var(--font-mono)' }}>
              私域 AI 技能平台 · 统一身份认证
            </Text>
          </div>

          {/* ============ Tabs ============ */}
          <div style={{ display: 'flex', gap: 8, marginBottom: 22 }}>
            {[{ k: 'login', label: '登录' }, { k: 'register', label: '注册账号' }].map((t) => (
              <button
                key={t.k}
                onClick={() => { setTab(t.k); setLoading(false); }}
                className={`auth-mode-btn ${tab === t.k ? 'active' : ''}`}
                style={{ flex: 1 }}
              >
                {t.label}
              </button>
            ))}
          </div>

          {/* ============ 登录面板 ============ */}
          {tab === 'login' && (
            <div>
              {/* 方式切换 */}
              <div className="auth-mode-switch">
                <button className={`auth-mode-btn ${loginMode === 'password' ? 'active' : ''}`} onClick={() => setLoginMode('password')}>
                  <LockOutlined /> 账号密码
                </button>
                <button className={`auth-mode-btn ${loginMode === 'phone' ? 'active' : ''}`} onClick={() => setLoginMode('phone')}>
                  <MobileOutlined /> 手机验证码
                </button>
              </div>

              <Form form={loginForm} onFinish={onLogin} size="large" className="nexus-form">
                {loginMode === 'password' ? (
                  <>
                    <Form.Item name="username" rules={[{ required: true, message: '请输入用户名' }]}>
                      <Input prefix={<UserOutlined style={{ color: 'var(--cyan)' }} />} placeholder="用户名" />
                    </Form.Item>
                    <Form.Item name="password" rules={[{ required: true, message: '请输入密码' }]}>
                      <Input.Password prefix={<LockOutlined style={{ color: 'var(--cyan)' }} />} placeholder="密码" />
                    </Form.Item>
                  </>
                ) : (
                  <>
                    <Form.Item name="phone" rules={[{ required: true, message: '请输入手机号' }, { pattern: /^1\d{10}$/, message: '手机号格式不正确' }]}>
                      <Input prefix={<MobileOutlined style={{ color: 'var(--cyan)' }} />} placeholder="手机号" maxLength={11} />
                    </Form.Item>
                    <Form.Item name="code" rules={[{ required: true, message: '请输入验证码' }]}>
                      <div className="code-input">
                        <Input prefix={<IdcardOutlined style={{ color: 'var(--cyan)' }} />} placeholder="6 位验证码" maxLength={6} />
                        <Button
                          className="code-btn"
                          disabled={countdown > 0}
                          onClick={() => handleSendCode('phone', loginForm.getFieldValue('phone'))}
                        >
                          {countdown > 0 ? `${countdown}s 后重发` : '获取验证码'}
                        </Button>
                      </div>
                    </Form.Item>
                  </>
                )}
                <Form.Item style={{ marginBottom: 8 }}>
                  <Button
                    className="glow-btn"
                    htmlType="submit"
                    block
                    loading={loading}
                    icon={<LoginOutlined />}
                    style={{ height: 46, fontSize: 15 }}
                  >
                    {loginMode === 'phone' ? '验证码登录' : '登录'}
                  </Button>
                </Form.Item>
              </Form>

              {/* 其他登录方式 */}
              <div className="oauth-divider">其他登录方式</div>
              <div className="oauth-row">
                <div style={{ textAlign: 'center' }}>
                  <div className="oauth-btn wechat" onClick={() => onOAuth('wechat')} title="微信登录">
                    {oauthLoading === 'wechat' ? <Spin size="small" /> : <WechatOutlined />}
                  </div>
                  <div className="oauth-label">微信</div>
                </div>
                <div style={{ textAlign: 'center' }}>
                  <div className="oauth-btn qq" onClick={() => onOAuth('qq')} title="QQ 登录">
                    {oauthLoading === 'qq' ? <Spin size="small" /> : <QqOutlined />}
                  </div>
                  <div className="oauth-label">QQ</div>
                </div>
                <div style={{ textAlign: 'center' }}>
                  <div className="oauth-btn mail" onClick={() => { setTab('register'); setRegMode('email'); }} title="邮箱注册">
                    <MailOutlined />
                  </div>
                  <div className="oauth-label">邮箱</div>
                </div>
                <div style={{ textAlign: 'center' }}>
                  <div className="oauth-btn scan" onClick={openScan} title="扫码登录">
                    <QrcodeOutlined />
                  </div>
                  <div className="oauth-label">扫码</div>
                </div>
              </div>

              <div className="quick-guest" onClick={onGuestLogin}>
                {oauthLoading === 'guest' ? <Spin size="small" style={{ marginRight: 6 }} /> : <SmileOutlined style={{ marginRight: 6 }} />}
                游客快捷体验 · 无需注册
              </div>
            </div>
          )}

          {/* ============ 注册面板 ============ */}
          {tab === 'register' && (
            <div>
              <div className="auth-mode-switch">
                <button className={`auth-mode-btn ${regMode === 'phone' ? 'active' : ''}`} onClick={() => setRegMode('phone')}>
                  <MobileOutlined /> 手机号注册
                </button>
                <button className={`auth-mode-btn ${regMode === 'email' ? 'active' : ''}`} onClick={() => setRegMode('email')}>
                  <MailOutlined /> 邮箱注册
                </button>
              </div>

              <Form form={regForm} onFinish={onRegister} size="large" className="nexus-form">
                <Form.Item name="nickname" rules={[{ required: true, message: '请输入昵称' }]}>
                  <Input prefix={<SmileOutlined style={{ color: 'var(--cyan)' }} />} placeholder="昵称（展示用）" maxLength={20} />
                </Form.Item>
                <Form.Item name="username" rules={[{ required: true, min: 3, max: 20, message: '用户名 3-20 个字符' }]}>
                  <Input prefix={<UserOutlined style={{ color: 'var(--cyan)' }} />} placeholder="用户名（登录用）" />
                </Form.Item>
                {regMode === 'phone' ? (
                  <Form.Item name="phone" rules={[{ required: true, message: '请输入手机号' }, { pattern: /^1\d{10}$/, message: '手机号格式不正确' }]}>
                    <Input prefix={<MobileOutlined style={{ color: 'var(--cyan)' }} />} placeholder="手机号" maxLength={11} />
                  </Form.Item>
                ) : (
                  <Form.Item name="email" rules={[{ required: true, message: '请输入邮箱' }, { type: 'email', message: '邮箱格式不正确' }]}>
                    <Input prefix={<MailOutlined style={{ color: 'var(--cyan)' }} />} placeholder="邮箱地址" />
                  </Form.Item>
                )}
                <Form.Item name="code" rules={[{ required: true, message: '请输入验证码' }]}>
                  <div className="code-input">
                    <Input prefix={<IdcardOutlined style={{ color: 'var(--cyan)' }} />} placeholder="6 位验证码" maxLength={6} />
                    <Button
                      className="code-btn"
                      disabled={countdown > 0}
                      onClick={() => handleSendCode(
                        regMode,
                        regMode === 'phone' ? regForm.getFieldValue('phone') : regForm.getFieldValue('email')
                      )}
                    >
                      {countdown > 0 ? `${countdown}s 后重发` : '获取验证码'}
                    </Button>
                  </div>
                </Form.Item>
                <Form.Item name="password" rules={[{ required: true, min: 6, message: '密码至少 6 位' }]}>
                  <Input.Password prefix={<LockOutlined style={{ color: 'var(--cyan)' }} />} placeholder="设置密码（至少 6 位）" />
                </Form.Item>
                <Form.Item style={{ marginBottom: 8 }}>
                  <Button
                    className="glow-btn glow-btn-violet"
                    htmlType="submit"
                    block
                    loading={loading}
                    icon={<ThunderboltOutlined />}
                    style={{ height: 46, fontSize: 15 }}
                  >
                    注册并自动登录
                  </Button>
                </Form.Item>
              </Form>

              <Text style={{ color: 'var(--text-dim)', fontSize: 11.5, display: 'block', textAlign: 'center', fontFamily: 'var(--font-mono)' }}>
                <SafetyCertificateOutlined /> 注册即代表同意《用户协议》与《隐私政策》 · 也可使用微信 / QQ 快捷注册
              </Text>
            </div>
          )}

          {/* 底部安全说明 */}
          <div style={{ textAlign: 'center', marginTop: 20 }}>
            <Text style={{ color: 'var(--text-dim)', fontSize: 11, fontFamily: 'var(--font-mono)' }}>
              <SafetyCertificateOutlined /> 对接九江银行 LDAP 统一认证 · TLS 加密通道
            </Text>
          </div>
        </div>
      </div>

      {/* ============ 扫码登录 Modal ============ */}
      <Modal
        className="nexus-modal"
        open={scanOpen}
        onCancel={() => setScanOpen(false)}
        footer={null}
        width={360}
        title={<span style={{ color: 'var(--cyan-bright)' }}><QrcodeOutlined /> 扫码登录</span>}
      >
        <div style={{ textAlign: 'center', padding: '8px 0 4px' }}>
          <QrMock />
          <div style={{ marginTop: 18, minHeight: 40, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 8 }}>
            {scanState === 'confirming' ? (
              <>
                <Spin />
                <Text style={{ color: 'var(--gold)', fontFamily: 'var(--font-mono)', fontSize: 13 }}>
                  已扫码, 请在手机上确认...
                </Text>
              </>
            ) : scanState === 'done' ? (
              <>
                <CheckCircleOutlined style={{ color: 'var(--green)', fontSize: 18 }} />
                <Text style={{ color: 'var(--green)', fontFamily: 'var(--font-mono)', fontSize: 13 }}>
                  登录成功, 正在进入...
                </Text>
              </>
            ) : (
              <Text style={{ color: 'var(--text-dim)', fontFamily: 'var(--font-mono)', fontSize: 13 }}>
                {scanStatusText.waiting}
              </Text>
            )}
          </div>
          {scanState === 'waiting' && (
            <Button className="glow-btn" style={{ marginTop: 14, height: 40, padding: '0 28px' }} onClick={mockScan}>
              模拟扫码成功
            </Button>
          )}
          <div style={{ marginTop: 14, color: 'var(--text-dim)', fontSize: 11.5, fontFamily: 'var(--font-mono)' }}>
            演示环境: 扫码后自动完成登录 · 二维码 60s 内有效
          </div>
        </div>
      </Modal>
    </div>
  );
}
