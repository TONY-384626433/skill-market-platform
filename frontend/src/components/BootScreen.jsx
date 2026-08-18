import React, { useEffect, useState } from 'react';
import {
  ApiOutlined, BankOutlined, CheckOutlined, DatabaseOutlined, SafetyCertificateOutlined,
} from '@ant-design/icons';

const stages = [
  { icon: SafetyCertificateOutlined, label: '验证企业身份域' },
  { icon: DatabaseOutlined, label: '载入技能索引' },
  { icon: ApiOutlined, label: '连接治理网关' },
];

export default function BootScreen({ onComplete }) {
  const [stage, setStage] = useState(0);
  const [leaving, setLeaving] = useState(false);

  useEffect(() => {
    const timers = [
      window.setTimeout(() => setStage(1), 220),
      window.setTimeout(() => setStage(2), 470),
      window.setTimeout(() => setStage(3), 720),
      window.setTimeout(() => setLeaving(true), 940),
      window.setTimeout(onComplete, 1160),
    ];
    return () => timers.forEach(window.clearTimeout);
  }, [onComplete]);

  const finish = () => {
    setStage(3);
    setLeaving(true);
    window.setTimeout(onComplete, 180);
  };

  return (
    <div className={`boot-screen ${leaving ? 'leaving' : ''}`} role="status" aria-live="polite">
      <button className="boot-skip" type="button" onClick={finish} aria-label="跳过启动动画">进入系统</button>
      <div className="boot-frame" aria-hidden="true"><i /><i /><i /><i /></div>
      <div className="boot-content">
        <div className="boot-brand"><span><BankOutlined /></span><div><strong>SkillHub</strong><small>JIUJIANG BANK · AI CAPABILITY OS</small></div></div>
        <div className="boot-title"><span className="boot-kicker">SECURE WORKSPACE</span><h1>企业 AI 能力中心</h1><p>正在建立可信工作空间</p></div>
        <div className="boot-stages">
          {stages.map(({ icon: Icon, label }, index) => (
            <div key={label} className={stage > index ? 'complete' : stage === index ? 'active' : ''}>
              <span>{stage > index ? <CheckOutlined /> : <Icon />}</span><b>{label}</b><em>{stage > index ? 'READY' : stage === index ? 'CONNECTING' : 'WAITING'}</em>
            </div>
          ))}
        </div>
        <div className="boot-progress"><span style={{ width: `${Math.min(100, (stage + 1) * 25)}%` }} /></div>
      </div>
    </div>
  );
}
