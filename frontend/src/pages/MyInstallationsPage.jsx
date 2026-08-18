// ============================================================
// 我的技能 — 科技感重构版
// ============================================================
import React, { useState, useEffect, useContext } from 'react';
import { useNavigate } from 'react-router-dom';
import { List, Button, message, Empty, Spin, Typography, Popconfirm, Tag } from 'antd';
import {
  DeleteOutlined, EyeOutlined, KeyOutlined,
  AppstoreOutlined, CheckCircleOutlined, StopOutlined
} from '@ant-design/icons';
import { getMyInstallations, revokeInstallation } from '../services/api';
import { UserContext } from '../App';

const { Title, Text } = Typography;

export default function MyInstallationsPage() {
  const navigate = useNavigate();
  const { user } = useContext(UserContext);
  const [installations, setInstallations] = useState([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (user) fetchInstallations();
    else setLoading(false);
  }, [user]);

  const fetchInstallations = async () => {
    setLoading(true);
    try {
      const res = await getMyInstallations();
      setInstallations(res.data || []);
    } catch (e) {
      message.error('获取安装列表失败');
    }
    setLoading(false);
  };

  const handleRevoke = async (instId) => {
    try {
      await revokeInstallation(instId);
      message.success('已卸载');
      fetchInstallations();
    } catch (e) {
      message.error('卸载失败');
    }
  };

  if (!user) {
    return (
      <div style={{ textAlign: 'center', padding: 100 }}>
        <h3 style={{ color: 'var(--text-primary)', fontSize: 22 }}>🔒 请先登录</h3>
        <Button className="glow-btn" style={{ marginTop: 12, height: 42, padding: '0 32px' }}
          onClick={() => navigate('/login')}>
          前往登录
        </Button>
      </div>
    );
  }

  return (
    <div style={{ maxWidth: 960, margin: '0 auto' }}>
      <div className="fade-up">
        <div className="page-title">
          <span className="title-icon"><AppstoreOutlined /></span>
          我的技能
        </div>
        <p className="page-subtitle">
          管理已安装的 AI 技能和 API Token
        </p>
      </div>

      <Spin spinning={loading}>
        {installations.length === 0 ? (
          <div className="glass-card nexus-empty fade-up" style={{ padding: 70, textAlign: 'center' }}>
            <Empty description="还没有安装任何技能, 去市场看看吧!" />
            <Button className="glow-btn" style={{ marginTop: 16, height: 42, padding: '0 32px' }}
              onClick={() => navigate('/')}>
              前往技能市场 →
            </Button>
          </div>
        ) : (
          <List
            className="nexus-list"
            dataSource={installations}
            renderItem={(item, i) => (
              <div
                className="glass-card fade-up"
                style={{ padding: '20px 24px', marginBottom: 14, animationDelay: `${Math.min(i * 0.06, 0.3)}s` }}
              >
                <div className="install-item">
                  <div style={{ display: 'flex', alignItems: 'center', gap: 16, flex: 1, minWidth: 260 }}>
                    <span className="inst-icon"><AppstoreOutlined /></span>
                    <div>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
                        <Title level={5} style={{ margin: 0, color: 'var(--text-primary)' }}>
                          {item.skill_name || item.skill_id}
                        </Title>
                        <Tag className="nexus-tag nexus-tag-cyan">v{item.skill_version}</Tag>
                        {item.status === 'active' ? (
                          <Tag className="nexus-tag nexus-tag-green"><CheckCircleOutlined /> ACTIVE</Tag>
                        ) : (
                          <Tag className="nexus-tag nexus-tag-gold"><StopOutlined /> {item.status}</Tag>
                        )}
                      </div>
                      <div style={{ marginTop: 8, display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap' }}>
                        <Text style={{ color: 'var(--text-dim)', fontFamily: 'var(--font-mono)', fontSize: 12 }}>
                          <KeyOutlined style={{ color: 'var(--cyan)', marginRight: 4 }} />
                          Token: <span style={{ color: 'var(--cyan-bright)' }}>{item.api_key_prefix}...</span>
                        </Text>
                        <Text style={{ color: 'var(--text-dim)', fontFamily: 'var(--font-mono)', fontSize: 12 }}>
                          安装于: {item.installed_at}
                        </Text>
                      </div>
                    </div>
                  </div>
                  <div style={{ display: 'flex', gap: 8, flexShrink: 0 }}>
                    <Button className="glow-btn glow-btn-ghost" icon={<EyeOutlined />}
                      onClick={() => navigate(`/skills/${item.skill_id}`)}>
                      查看
                    </Button>
                    <Popconfirm
                      title="确定要卸载此技能吗?"
                      description="API Token 将被吊销。"
                      okText="卸载"
                      cancelText="取消"
                      okButtonProps={{ danger: true }}
                      onConfirm={() => handleRevoke(item.id)}
                    >
                      <Button danger icon={<DeleteOutlined />}
                        style={{ background: 'rgba(244,63,94,0.08)', border: '1px solid rgba(244,63,94,0.35)', color: '#f87171', borderRadius: 10 }}>
                        卸载
                      </Button>
                    </Popconfirm>
                  </div>
                </div>
              </div>
            )}
          />
        )}
      </Spin>
    </div>
  );
}
