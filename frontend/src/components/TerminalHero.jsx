// ============================================================
// TerminalHero — 高级模拟终端组件
// 多行命令脚本循环演示：打字 → 输出 → 进度条 → 自动切换
// ============================================================
import React, { useEffect, useRef, useState } from 'react';

// 演示脚本（模拟真实使用流程）
const SCRIPT = [
  {
    cmd: 'skill-nexus search --query "数据库巡检" --sort rating',
    outputs: [
      { type: 'muted', text: '◇ 正在检索私有技能市场...' },
      { type: 'success', text: '✓ 发现 12 个技能 · 耗时 0.42s' },
    ],
  },
  {
    cmd: 'skill-nexus install db-inspection@v1.2.0',
    outputs: [
      { type: 'progress', text: '安装中' },
      { type: 'success', text: '✓ 安装成功 · API Token sk-••••9f2a 已生成' },
    ],
  },
  {
    cmd: 'skill-nexus invoke db-inspection --target prod-db',
    outputs: [
      { type: 'progress', text: '执行中' },
      { type: 'warn', text: '⚠ 发现 3 个风险项: 慢查询 ×2 · 死锁 ×1' },
      { type: 'success', text: '✓ 审计日志已写入 · trace 7f3a9c2b' },
    ],
  },
  {
    cmd: 'skill-nexus stats --monthly --format table',
    outputs: [
      { type: 'info', text: '本月调用 1,284 次 · 活跃用户 46 人 · 平均响应 0.38s' },
    ],
  },
];

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const TYPE_SPEED = 38;
const OUTPUT_GAP = 340;
const END_PAUSE = 2400;

export default function TerminalHero() {
  const [scriptIdx, setScriptIdx] = useState(0);
  const [cmdText, setCmdText] = useState('');
  const [outputs, setOutputs] = useState([]);
  const [progress, setProgress] = useState(0);
  const cancelledRef = useRef(false);

  useEffect(() => {
    cancelledRef.current = false;

    const run = async () => {
      const step = SCRIPT[scriptIdx];

      // 1. 逐字敲入命令
      for (let i = 1; i <= step.cmd.length; i++) {
        if (cancelledRef.current) return;
        setCmdText(step.cmd.slice(0, i));
        await sleep(TYPE_SPEED);
      }
      await sleep(280);

      // 2. 逐行输出结果
      for (const out of step.outputs) {
        if (cancelledRef.current) return;
        if (out.type === 'progress') {
          setOutputs((o) => [...o, out]);
          for (let p = 0; p <= 100; p += 4) {
            if (cancelledRef.current) return;
            setProgress(p);
            await sleep(26);
          }
          setProgress(0);
        } else {
          setOutputs((o) => [...o, out]);
          await sleep(OUTPUT_GAP);
        }
      }

      await sleep(END_PAUSE);
      if (cancelledRef.current) return;

      // 3. 清屏切换下一条
      setOutputs([]);
      setCmdText('');
      setScriptIdx((s) => (s + 1) % SCRIPT.length);
    };

    run();
    return () => { cancelledRef.current = true; };
  }, [scriptIdx]);

  // 命令语法高亮着色
  const renderCmd = (text) => {
    const tokens = text.split(' ');
    return tokens.map((token, i) => {
      let cls = 'terminal-cmd';
      if (token.startsWith('--') || token.startsWith('"')) cls = 'terminal-arg';
      return (
        <span key={i} className={cls}>
          {token}{i < tokens.length - 1 ? ' ' : ''}
        </span>
      );
    });
  };

  const outClass = {
    success: 'terminal-out-success',
    warn: 'terminal-out-warn',
    muted: 'terminal-out-muted',
    info: 'terminal-out-info',
  };

  return (
    <div className="hero-terminal-window">
      <div className="terminal-bar">
        <span className="terminal-dot red" />
        <span className="terminal-dot yellow" />
        <span className="terminal-dot green" />
        <span className="terminal-title">skill-nexus — zsh — 80×24</span>
        <span className="terminal-tag">LIVE</span>
      </div>
      <div className="terminal-body">
        {/* 命令行 */}
        <div className="terminal-line">
          <span className="terminal-prompt">❯</span>
          <span>
            {renderCmd(cmdText)}
            <span className="terminal-cursor" />
          </span>
        </div>

        {/* 输出行 */}
        {outputs.map((out, i) => {
          if (out.type === 'progress') {
            return (
              <div className="terminal-line" key={i} style={{ marginTop: 5 }}>
                <span className="terminal-out-muted">◇ {out.text}</span>
                <span className="terminal-progress-track">
                  <span className="terminal-progress-fill" style={{ width: `${progress}%` }} />
                </span>
                <span className="terminal-out-success">{progress}%</span>
              </div>
            );
          }
          return (
            <div className="terminal-line" key={i} style={{ marginTop: 5 }}>
              <span className={outClass[out.type] || 'terminal-out-muted'}>{out.text}</span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
