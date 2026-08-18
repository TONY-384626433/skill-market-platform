import React from 'react';
import {
  AlertOutlined, CodeOutlined, CustomerServiceOutlined, DatabaseOutlined,
  MonitorOutlined, SafetyCertificateOutlined, AppstoreOutlined,
} from '@ant-design/icons';

const iconMap = {
  '智能运维': MonitorOutlined,
  '研发效能': CodeOutlined,
  '安全合规': SafetyCertificateOutlined,
  '数据治理': DatabaseOutlined,
  '智能客服': CustomerServiceOutlined,
  '风控分析': AlertOutlined,
};

export default function SkillVisual({ category, size = 'md' }) {
  const Icon = iconMap[category] || AppstoreOutlined;
  return <span className={`skill-visual skill-visual-${size}`} data-category={category || 'default'}><Icon /></span>;
}
