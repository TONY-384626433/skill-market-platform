// ============================================================
// 开发者工作台 — 科技感重构版
// ============================================================
import React, { useState, useContext } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Form, Input, Select, Button, Steps, message, Row, Col,
  Typography, Divider, Alert
} from 'antd';
import {
  PlusOutlined, CheckCircleOutlined, CodeOutlined,
  ApiOutlined, RobotOutlined, ArrowLeftOutlined,
  RocketOutlined, SafetyCertificateOutlined
} from '@ant-design/icons';
import { createSkill } from '../services/api';
import { UserContext } from '../App';

const { Title, Text } = Typography;
const { TextArea } = Input;
const { Option } = Select;

// 技能形态
const SKILL_TYPES = [
  { key: 'mcp', title: 'MCP Server', desc: '基于 MCP 协议的工具服务, 支持标准 Tool/Resource 接口', icon: '🔌' },
  { key: 'dify', title: 'Dify 工作流', desc: '在 Dify 平台上构建的 AI 工作流应用', icon: '⚡' },
  { key: 'agent', title: 'Agent 模板', desc: 'AI Agent 配置模板, 含 Prompt + Tools 配置', icon: '🤖' },
  { key: 'api', title: 'API 封装', desc: 'REST/gRPC API 封装, 适用于已有服务的快速集成', icon: '🔗' },
  { key: 'prompt', title: 'Prompt 模板', desc: '经过验证的高质量 Prompt 模板集合', icon: '💬' },
  { key: 'knowledge', title: '知识库', desc: '行业/业务知识库, 可供 RAG 应用使用', icon: '📚' },
];

export default function DeveloperPage() {
  const navigate = useNavigate();
  const { user } = useContext(UserContext);
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [currentStep, setCurrentStep] = useState(0);
  const [selectedType, setSelectedType] = useState('');

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

  const onFinish = async (values) => {
    setLoading(true);
    try {
      await createSkill({
        skill_key: values.name.toLowerCase().replace(/\s+/g, '-'),
        name: values.name,
        category: values.category,
        summary: values.summary,
        description: values.description,
        skill_type: values.skill_type,
        endpoint_url: values.endpoint_url,
        stability: 'beta',
        tags: values.tags ? values.tags.split(',').map(s => s.trim()) : [],
      });
      message.success('技能已提交审核!');
      form.resetFields();
      setCurrentStep(0);
      setSelectedType('');
    } catch (e) {
      message.error(e.response?.data?.error || '创建失败');
    }
    setLoading(false);
  };

  const steps = [
    { title: '选择形态', icon: <RobotOutlined /> },
    { title: '填写信息', icon: <CodeOutlined /> },
    { title: '配置接口', icon: <ApiOutlined /> },
    { title: '提交审核', icon: <CheckCircleOutlined /> },
  ];

  const selectType = (key) => {
    setSelectedType(key);
    form.setFieldsValue({ skill_type: key });
    setCurrentStep(1);
  };

  return (
    <div style={{ maxWidth: 960, margin: '0 auto' }}>
      <div className="fade-up">
        <div className="page-title">
          <span className="title-icon"><CodeOutlined /></span>
          开发者工作台
        </div>
        <p className="page-subtitle">
          将你的 AI 能力封装为标准技能, 分享给全行同事使用
        </p>
      </div>

      <Steps className="nexus-steps fade-up" current={currentStep} items={steps} style={{ marginBottom: 32 }} />

      <div className="glass-card fade-up" style={{ padding: '32px', animationDelay: '0.1s' }}>
        {/* 步骤 0: 选择技能形态 */}
        {currentStep === 0 && (
          <div>
            <h4 style={{ color: 'var(--cyan-bright)', fontWeight: 700, marginBottom: 20 }}>
              <span style={{ color: 'var(--cyan)' }}>▸</span> 选择技能形态
            </h4>
            <Row gutter={[16, 16]}>
              {SKILL_TYPES.map((item) => (
                <Col xs={24} sm={12} md={8} key={item.key}>
                  <div
                    className={`form-type-card ${selectedType === item.key ? 'active' : ''}`}
                    onClick={() => selectType(item.key)}
                  >
                    <span className="type-icon">{item.icon}</span>
                    <div className="type-title">{item.title}</div>
                    <div className="type-desc">{item.desc}</div>
                  </div>
                </Col>
              ))}
            </Row>
          </div>
        )}

        {/* 步骤 1-3: 表单 */}
        {currentStep >= 1 && (
          <Form form={form} layout="vertical" onFinish={onFinish} className="nexus-form">
            <h4 style={{ color: 'var(--cyan-bright)', fontWeight: 700, marginBottom: 20 }}>
              <span style={{ color: 'var(--cyan)' }}>▸</span> 基本信息
            </h4>
            <Row gutter={16}>
              <Col span={16}>
                <Form.Item name="name" label="技能名称" rules={[{ required: true }]}>
                  <Input placeholder="如: 数据库智能巡检助手" />
                </Form.Item>
              </Col>
              <Col span={8}>
                <Form.Item name="category" label="分类" rules={[{ required: true }]}>
                  <Select placeholder="选择分类">
                    <Option value="智能运维">智能运维</Option>
                    <Option value="研发效能">研发效能</Option>
                    <Option value="安全合规">安全合规</Option>
                    <Option value="数据治理">数据治理</Option>
                    <Option value="智能客服">智能客服</Option>
                    <Option value="风控分析">风控分析</Option>
                  </Select>
                </Form.Item>
              </Col>
            </Row>
            <Form.Item name="summary" label="一句话描述" rules={[{ required: true, max: 200 }]}>
              <Input placeholder="用一句话描述技能的功能和价值" maxLength={200} showCount />
            </Form.Item>
            <Form.Item name="description" label="详细描述">
              <TextArea rows={4} placeholder="实现原理、适用场景、使用方法等" />
            </Form.Item>
            <Form.Item name="tags" label="标签">
              <Input placeholder="用逗号分隔, 如: 数据库, 巡检, Mysql" />
            </Form.Item>

            <Divider style={{ borderColor: 'rgba(34,211,238,0.12)' }} />
            <h4 style={{ color: 'var(--cyan-bright)', fontWeight: 700, marginBottom: 20 }}>
              <span style={{ color: 'var(--cyan)' }}>▸</span> 接口配置
            </h4>
            <Form.Item name="endpoint_url" label="接入地址">
              <Input placeholder="如: http://service.internal:8080/api/mcp" />
            </Form.Item>
            <Form.Item name="skill_type" label="技能形态" hidden>
              <Input />
            </Form.Item>

            <Alert
              className="nexus-alert"
              type="info"
              showIcon
              message="提交后将进入自动化审核流水线: 格式校验 → 安全扫描 → 功能验证"
              style={{ marginBottom: 24 }}
            />

            <div style={{ display: 'flex', gap: 12 }}>
              {currentStep > 1 && (
                <Button
                  className="glow-btn glow-btn-ghost"
                  icon={<ArrowLeftOutlined />}
                  onClick={() => setCurrentStep(currentStep - 1)}
                >
                  上一步
                </Button>
              )}
              {currentStep === 1 && (
                <Button
                  className="glow-btn glow-btn-ghost"
                  icon={<ApiOutlined />}
                  onClick={() => setCurrentStep(2)}
                >
                  下一步: 接口配置
                </Button>
              )}
              <Button
                className="glow-btn"
                htmlType="submit"
                loading={loading}
                icon={<RocketOutlined />}
                style={{ marginLeft: 'auto' }}
              >
                提交审核
              </Button>
            </div>
            {currentStep === 2 && (
              <div style={{ marginTop: 14, color: 'var(--text-dim)', fontSize: 12.5, fontFamily: 'var(--font-mono)' }}>
                <SafetyCertificateOutlined /> 提交即进入安全扫描队列, 预计 2 分钟内完成
              </div>
            )}
          </Form>
        )}
      </div>
    </div>
  );
}
