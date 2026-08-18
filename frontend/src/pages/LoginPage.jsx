// ============================================================
// 登录页 — 科技感重构版
// ============================================================
import React, { useState, useContext } from 'react';
import { useNavigate } from 'react-router-dom';
import { Form, Input, Button, message, Typography } from 'antd';
import { UserOutlined, LockOutlined, LoginOutlined, SafetyCertificateOutlined } from '@ant-design/icons';
import { login } from '../services/api';
import { UserContext } from '../App';

const { Text } = Typography;

export default function LoginPage() {
  const navigate = useNavigate();
  const { setUser } = useContext(UserContext);
  const [loading, setLoading] = useState(false);

  const onFinish = async (values) => {
    setLoading(true);
    try {
      const res = await login(values.username, values.password);
      localStorage.setItem('skillhub_token', res.token);
      localStorage.setItem('skillhub_user', JSON.stringify(res.user));
      setUser(res.user);
      message.success(`欢迎回来, ${res.user.username}!`);
      navigate('/');
    } catch (e) {
      message.error('登录失败, 请检查用户名和密码');
    }
    setLoading(false);
  };

  return (
    <div style={{
      display: 'flex', justifyContent: 'center', alignItems: 'center',
      minHeight: 'calc(100vh - 140px)', position: 'relative'
    }}>
      {/* 光环装饰 */}
      <div style={{
        position: 'absolute', width: 420, height: 420, borderRadius: '50%',
        background: 'radial-gradient(circle, rgba(34,211,238,0.16), transparent 65%)',
        top: '10%', left: '15%', filter: 'blur(10px)', animation: 'floaty 7s ease-in-out infinite'
      }} />
      <div style={{
        position: 'absolute', width: 380, height: 380, borderRadius: '50%',
        background: 'radial-gradient(circle, rgba(139,92,246,0.16), transparent 65%)',
        bottom: '8%', right: '14%', filter: 'blur(10px)', animation: 'floaty 9s ease-in-out infinite reverse'
      }} />

      <div className="gradient-border fade-up" style={{ width: 420, padding: 2, borderRadius: 20 }}>
        <div style={{ background: 'rgba(9, 14, 32, 0.92)', backdropFilter: 'blur(24px)', borderRadius: 19, padding: '44px 40px 36px', position: 'relative', overflow: 'hidden' }}>
          {/* 顶部光条 */}
          <div style={{
            position: 'absolute', top: 0, left: '15%', right: '15%', height: 2,
            background: 'linear-gradient(90deg, transparent, var(--cyan), var(--violet), transparent)',
            boxShadow: '0 0 24px rgba(34,211,238,0.7)'
          }} />

          <div style={{ textAlign: 'center', marginBottom: 36 }}>
            <div style={{
              width: 68, height: 68, borderRadius: 18, margin: '0 auto 18px',
              background: 'var(--gradient-main)', display: 'flex', alignItems: 'center', justifyContent: 'center',
              fontSize: 32, boxShadow: 'var(--shadow-glow-cyan)', animation: 'logoPulse 3s ease infinite'
            }}>⚡</div>
            <h2 style={{
              fontSize: 24, fontWeight: 800, margin: '0 0 6px', color: 'var(--text-primary)',
              letterSpacing: 1
            }}>
              SKILL <span className="gradient-text">NEXUS</span>
            </h2>
            <Text style={{ color: 'var(--text-dim)', fontSize: 13, letterSpacing: 2, fontFamily: 'var(--font-mono)' }}>
              九江银行私域 AI 技能平台
            </Text>
          </div>

          <Form onFinish={onFinish} size="large" className="nexus-form">
            <Form.Item name="username" rules={[{ required: true, message: '请输入用户名' }]}>
              <Input prefix={<UserOutlined style={{ color: 'var(--cyan)' }} />} placeholder="用户名" />
            </Form.Item>
            <Form.Item name="password" rules={[{ required: true, message: '请输入密码' }]}>
              <Input.Password prefix={<LockOutlined style={{ color: 'var(--cyan)' }} />} placeholder="密码" />
            </Form.Item>
            <Form.Item style={{ marginBottom: 12 }}>
              <Button
                className="glow-btn"
                htmlType="submit"
                block
                loading={loading}
                icon={<LoginOutlined />}
                style={{ height: 46, fontSize: 15 }}
              >
                进入技能市场
              </Button>
            </Form.Item>
          </Form>

          <div style={{ textAlign: 'center' }}>
            <Text style={{ color: 'var(--text-dim)', fontSize: 11.5, fontFamily: 'var(--font-mono)' }}>
              <SafetyCertificateOutlined /> 对接九江银行 LDAP 统一认证 · TLS 加密通道
            </Text>
          </div>
        </div>
      </div>
    </div>
  );
}
