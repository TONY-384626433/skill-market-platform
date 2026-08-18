import React, { useEffect, useState } from 'react';
import {
  Alert, App as AntApp, Button, Checkbox, Descriptions, Empty, Form, Input, Select, Steps, Table, Tabs, Tag,
} from 'antd';
import {
  ApiOutlined, CheckCircleOutlined, CloudUploadOutlined, CodeOutlined, FileTextOutlined,
  PlusOutlined, ReloadOutlined, SafetyCertificateOutlined,
} from '@ant-design/icons';
import { createSkill, getMySubmissions } from '../services/api';
import { formatDate, statusMeta } from '../utils/format';

const categories = ['智能运维', '研发效能', '安全合规', '数据治理', '智能客服', '风控分析'];
const typeOptions = [
  { value: 'mcp', label: 'MCP 服务' }, { value: 'api', label: 'REST API' },
  { value: 'dify', label: 'Dify 工作流' }, { value: 'agent', label: '智能体模板' },
  { value: 'prompt', label: '提示词模板' },
];
const stepFields = [
  ['name', 'skill_key', 'category', 'summary', 'description', 'tags'],
  ['skill_type', 'version', 'endpoint_protocol', 'endpoint_url', 'stability'],
  ['permissions', 'dependencies', 'interface_spec', 'compliance_confirmed'],
];

function SubmissionTable({ items, loading, onRefresh }) {
  const columns = [
    { title: '技能', dataIndex: 'name', key: 'name', render: (value, row) => <div className="table-primary"><span className="table-icon"><CodeOutlined /></span><div><strong>{value}</strong><small>{row.skill_key}</small></div></div> },
    { title: '版本', dataIndex: 'version', key: 'version', width: 90, render: (value) => `v${value}` },
    { title: '分类', dataIndex: 'category', key: 'category', width: 110 },
    { title: '提交时间', dataIndex: 'created_at', key: 'created_at', width: 140, render: (value) => formatDate(value) },
    { title: '状态', dataIndex: 'status', key: 'status', width: 110, render: (value) => { const meta = statusMeta[value] || { label: value, color: 'default' }; return <Tag color={meta.color}>{meta.label}</Tag>; } },
    { title: '审核意见', dataIndex: 'review_comment', key: 'review_comment', ellipsis: true, render: (value) => value || <span className="muted">暂无</span> },
  ];
  return (
    <section className="workspace-section flush-section">
      <div className="section-heading"><div><h2>发布记录</h2><p>查看技能版本、审核状态和平台反馈。</p></div><Button icon={<ReloadOutlined />} onClick={onRefresh}>刷新</Button></div>
      <Table rowKey="id" dataSource={items} columns={columns} loading={loading} pagination={false} scroll={{ x: 820 }} locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="尚未提交技能" /> }} />
    </section>
  );
}

export default function DeveloperPage() {
  const { message } = AntApp.useApp();
  const [form] = Form.useForm();
  const [activeTab, setActiveTab] = useState('submissions');
  const [step, setStep] = useState(0);
  const [submissions, setSubmissions] = useState([]);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [draft, setDraft] = useState({});
  const values = draft;

  const loadSubmissions = async () => {
    setLoading(true);
    try {
      const response = await getMySubmissions();
      setSubmissions(Array.isArray(response?.data) ? response.data : []);
    } catch (requestError) { message.error(requestError.response?.data?.error || '发布记录加载失败'); }
    finally { setLoading(false); }
  };

  useEffect(() => { loadSubmissions(); }, []);

  const next = async () => {
    try {
      await form.validateFields(stepFields[step]);
      setDraft((current) => ({ ...current, ...form.getFieldsValue(true) }));
      setStep((current) => current + 1);
    } catch { /* Ant Design displays field errors. */ }
  };

  const submit = async () => {
    setSubmitting(true);
    try {
      const currentValues = await form.validateFields();
      const data = { ...draft, ...form.getFieldsValue(true), ...currentValues };
      let interfaceSpec;
      try { interfaceSpec = JSON.parse(data.interface_spec || '{}'); } catch { message.error('接口定义不是合法 JSON'); setStep(2); return; }
      const dependencies = Object.fromEntries((data.dependencies || []).map((name) => [name, 'required']));
      await createSkill({
        skill_key: data.skill_key,
        name: data.name,
        version: data.version,
        category: data.category,
        summary: data.summary,
        description: data.description,
        tags: data.tags || [],
        skill_type: data.skill_type,
        endpoint_url: data.endpoint_url,
        endpoint_protocol: data.endpoint_protocol,
        stability: data.stability,
        visibility: 'private',
        permissions: JSON.stringify(data.permissions || []),
        dependencies: JSON.stringify(dependencies),
        interface_spec: JSON.stringify(interfaceSpec),
        manifest: JSON.stringify({ schema_version: '1.0', skill_key: data.skill_key, interface: interfaceSpec }),
      });
      message.success('技能已提交审核');
      form.resetFields();
	  setDraft({});
      setStep(0);
      setActiveTab('submissions');
      await loadSubmissions();
    } catch (requestError) {
      if (requestError?.errorFields) return;
      message.error(requestError.response?.data?.error || '提交失败');
    } finally { setSubmitting(false); }
  };

  const formPanel = (
    <div className="publish-layout">
      <aside className="publish-aside">
        <Steps direction="vertical" current={step} items={[
          { title: '基本信息', description: '名称、分类与用途' },
          { title: '接入配置', description: '协议、版本与地址' },
          { title: '权限治理', description: '依赖、权限与接口' },
          { title: '确认提交', description: '核对发布内容' },
        ]} />
        <div className="review-process"><SafetyCertificateOutlined /><strong>标准审核流程</strong><p>格式校验、安全扫描、功能验证、人工复核。</p></div>
      </aside>
      <section className="publish-form">
        <Form form={form} layout="vertical" initialValues={{ version: '0.1.0', skill_type: 'mcp', endpoint_protocol: 'http', stability: 'beta', interface_spec: '{\n  "inputs": [],\n  "outputs": []\n}' }}>
          {step === 0 && <div className="form-step"><div className="form-step-heading"><FileTextOutlined /><div><h2>基本信息</h2><p>清楚说明能力边界，帮助使用方做出正确选择。</p></div></div><div className="form-grid two"><Form.Item name="name" label="技能名称" rules={[{ required: true, message: '请输入技能名称' }, { max: 50 }]}><Input placeholder="例如：数据库智能巡检助手" /></Form.Item><Form.Item name="skill_key" label="唯一标识" extra="发布后不可修改，仅允许小写字母、数字和连字符。" rules={[{ required: true }, { pattern: /^[a-z][a-z0-9-]{2,63}$/, message: '格式如 db-inspection' }]}><Input placeholder="db-inspection" /></Form.Item></div><Form.Item name="category" label="能力分类" rules={[{ required: true, message: '请选择分类' }]}><Select options={categories.map((value) => ({ value, label: value }))} placeholder="选择最匹配的业务分类" /></Form.Item><Form.Item name="summary" label="一句话简介" rules={[{ required: true }, { max: 120 }]}><Input showCount maxLength={120} placeholder="说明解决什么问题和主要价值" /></Form.Item><Form.Item name="description" label="详细说明" rules={[{ required: true, message: '请补充详细说明' }]}><Input.TextArea rows={5} placeholder="包括适用场景、能力边界、数据来源和已知限制" /></Form.Item><Form.Item name="tags" label="检索标签"><Select mode="tags" tokenSeparators={[',']} maxCount={6} placeholder="输入标签后回车" /></Form.Item></div>}

          {step === 1 && <div className="form-step"><div className="form-step-heading"><ApiOutlined /><div><h2>接入配置</h2><p>平台将使用这些信息执行连通性和协议校验。</p></div></div><div className="form-grid two"><Form.Item name="skill_type" label="技能形态" rules={[{ required: true }]}><Select options={typeOptions} /></Form.Item><Form.Item name="version" label="语义化版本" rules={[{ required: true }, { pattern: /^\d+\.\d+\.\d+$/, message: '格式如 1.0.0' }]}><Input /></Form.Item><Form.Item name="endpoint_protocol" label="接入协议" rules={[{ required: true }]}><Select options={[{ value: 'http', label: 'HTTP / REST' }, { value: 'mcp', label: 'MCP' }, { value: 'grpc', label: 'gRPC' }]} /></Form.Item><Form.Item name="stability" label="稳定性" rules={[{ required: true }]}><Select options={[{ value: 'experimental', label: '实验性' }, { value: 'beta', label: '公开测试' }, { value: 'stable', label: '生产稳定' }]} /></Form.Item></div><Form.Item name="endpoint_url" label="内部接入地址" rules={[{ required: true, message: '请输入接入地址' }, { type: 'url', message: '请输入完整 URL' }]}><Input placeholder="http://service.internal:8080/mcp" /></Form.Item><Alert type="info" showIcon message="请确保平台审核网络可访问该地址，且健康检查不会触发业务写操作。" /></div>}

          {step === 2 && <div className="form-step"><div className="form-step-heading"><SafetyCertificateOutlined /><div><h2>权限与接口治理</h2><p>声明实际所需权限和依赖，遵循最小授权原则。</p></div></div><Form.Item name="permissions" label="权限声明" extra="例如：cmdb.read、log.mask。无额外权限时可留空。"><Select mode="tags" tokenSeparators={[',']} placeholder="输入权限后回车" /></Form.Item><Form.Item name="dependencies" label="运行依赖"><Select mode="tags" tokenSeparators={[',']} placeholder="输入依赖服务或技能后回车" /></Form.Item><Form.Item name="interface_spec" label="接口定义（JSON）" rules={[{ required: true }]}><Input.TextArea rows={10} className="json-editor" spellCheck={false} /></Form.Item><Form.Item name="compliance_confirmed" valuePropName="checked" rules={[{ validator: (_, checked) => checked ? Promise.resolve() : Promise.reject(new Error('请确认合规声明')) }]}><Checkbox>我确认提交内容不包含密钥、客户数据或生产环境敏感参数，并已完成团队内部评审。</Checkbox></Form.Item></div>}

          {step === 3 && <div className="form-step"><div className="form-step-heading"><CheckCircleOutlined /><div><h2>确认提交</h2><p>提交后将进入平台审核，审核完成前不会在市场公开。</p></div></div><Descriptions bordered column={1} size="small" items={[{ key: 'name', label: '技能名称', children: values.name }, { key: 'key', label: '唯一标识', children: values.skill_key }, { key: 'category', label: '分类与形态', children: `${values.category || '-'} / ${values.skill_type || '-'}` }, { key: 'version', label: '版本与稳定性', children: `v${values.version || '-'} / ${values.stability || '-'}` }, { key: 'endpoint', label: '接入地址', children: values.endpoint_url }, { key: 'permissions', label: '权限声明', children: (values.permissions || []).join(', ') || '无额外权限' }]} /><Alert type="warning" showIcon message="预计审核时间：1 个工作日" description="安全扫描或功能验证不通过时，平台会在发布记录中返回具体整改意见。" /></div>}

          <div className="form-actions"><Button disabled={step === 0} onClick={() => setStep((current) => current - 1)}>上一步</Button>{step < 3 ? <Button type="primary" onClick={next}>下一步</Button> : <Button type="primary" icon={<CloudUploadOutlined />} loading={submitting} onClick={submit}>提交审核</Button>}</div>
        </Form>
      </section>
    </div>
  );

  return (
    <div className="page">
      <section className="page-heading"><div><h1>开发者工作台</h1><p>管理内部能力版本，并通过标准化流程发布到技能市场。</p></div><Button type="primary" icon={<PlusOutlined />} onClick={() => { setActiveTab('publish'); setStep(0); }}>发布新技能</Button></section>
      <Tabs className="workspace-tabs" activeKey={activeTab} onChange={setActiveTab} items={[{ key: 'submissions', label: `我的发布 (${submissions.length})`, children: <SubmissionTable items={submissions} loading={loading} onRefresh={loadSubmissions} /> }, { key: 'publish', label: '发布技能', children: formPanel }]} />
    </div>
  );
}
