import React, { useEffect, useState } from 'react';
import { App as AntApp, Button, Empty, Popconfirm, Skeleton, Table, Tag } from 'antd';
import {
  ApiOutlined, AppstoreAddOutlined, DeleteOutlined, EyeOutlined, KeyOutlined, SafetyCertificateOutlined,
} from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { getMyInstallations, revokeInstallation } from '../services/api';
import { formatDate } from '../utils/format';

export default function MyInstallationsPage() {
  const { message } = AntApp.useApp();
  const navigate = useNavigate();
  const [items, setItems] = useState([]);
  const [loading, setLoading] = useState(true);
  const [revoking, setRevoking] = useState('');
  const [error, setError] = useState('');

  const load = async () => {
    setLoading(true);
    setError('');
    try {
      const response = await getMyInstallations();
      if (!Array.isArray(response?.data)) throw new Error('安装列表数据格式异常');
      setItems(response.data);
    } catch (requestError) {
      setError(requestError.response?.data?.error || requestError.message || '安装列表加载失败');
    } finally { setLoading(false); }
  };

  useEffect(() => { load(); }, []);

  const revoke = async (id) => {
    setRevoking(id);
    try {
      await revokeInstallation(id);
      message.success('技能已卸载，原访问令牌立即失效');
      setItems((current) => current.filter((item) => item.id !== id));
    } catch (requestError) { message.error(requestError.response?.data?.error || '卸载失败'); }
    finally { setRevoking(''); }
  };

  const columns = [
    {
      title: '技能', dataIndex: 'skill_name', key: 'skill_name', minWidth: 220,
      render: (name, record) => <div className="table-primary"><span className="table-icon"><ApiOutlined /></span><div><strong>{name}</strong><small>{record.skill_id}</small></div></div>,
    },
    { title: '版本', dataIndex: 'skill_version', key: 'skill_version', width: 100, render: (value) => <Tag>v{value}</Tag> },
    { title: '令牌标识', dataIndex: 'api_key_prefix', key: 'api_key_prefix', width: 180, render: (value) => <code className="key-prefix"><KeyOutlined /> {value}...</code> },
    { title: '安装时间', dataIndex: 'installed_at', key: 'installed_at', width: 150, render: (value) => formatDate(value) },
    { title: '状态', dataIndex: 'status', key: 'status', width: 100, render: () => <Tag color="success">运行中</Tag> },
    {
      title: '操作', key: 'actions', width: 190, fixed: 'right', render: (_, record) => (
        <div className="table-actions">
          <Button type="link" icon={<EyeOutlined />} onClick={() => navigate(`/skills/${record.skill_id}`)}>详情</Button>
          <Popconfirm title="确认卸载此技能？" description="卸载后对应访问令牌会立即失效。" okText="确认卸载" cancelText="取消" onConfirm={() => revoke(record.id)}>
            <Button type="link" danger icon={<DeleteOutlined />} loading={revoking === record.id}>卸载</Button>
          </Popconfirm>
        </div>
      ),
    },
  ];

  return (
    <div className="page">
      <section className="page-heading">
        <div><h1>我的技能</h1><p>管理已安装能力、访问令牌标识和运行状态。</p></div>
        <Button type="primary" icon={<AppstoreAddOutlined />} onClick={() => navigate('/')}>浏览技能市场</Button>
      </section>

      <section className="summary-strip">
        <div><span>已安装</span><strong>{items.length}</strong><small>个技能</small></div>
        <div><span>有效令牌</span><strong>{items.filter((item) => item.status === 'active').length}</strong><small>个凭据</small></div>
        <div><span>安全状态</span><strong className="summary-status">正常</strong><small><SafetyCertificateOutlined /> 令牌均受统一鉴权</small></div>
      </section>

      <section className="workspace-section">
        <div className="section-heading"><div><h2>安装记录</h2><p>完整令牌不会在此页面回显。如有泄露风险，请立即卸载后重新安装。</p></div><Button onClick={load}>刷新</Button></div>
        {loading ? <Skeleton active paragraph={{ rows: 8 }} /> : error ? <div className="state-panel"><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={error} /><Button type="primary" onClick={load}>重新加载</Button></div> : items.length === 0 ? <div className="state-panel"><Empty description="尚未安装任何技能" /><Button type="primary" onClick={() => navigate('/')}>前往技能市场</Button></div> : <Table className="workspace-table" rowKey="id" dataSource={items} columns={columns} pagination={false} scroll={{ x: 900 }} />}
      </section>
    </div>
  );
}
