// ============================================================
// QrMock — 模拟二维码组件
// 演示用: 伪随机图案 + 三个定位角标 + 中间 Logo
// ============================================================
import React, { useMemo } from 'react';

// 简易伪随机数生成器（保证每次渲染图案稳定）
function mulberry32(seed) {
  return function () {
    let t = (seed += 0x6d2b79f5);
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

export default function QrMock({ size = 180, seed = 20260818 }) {
  const cells = useMemo(() => {
    const rand = mulberry32(seed);
    const grid = [];
    for (let y = 0; y < 25; y++) {
      for (let x = 0; x < 25; x++) {
        grid.push({ x, y, on: rand() > 0.48 });
      }
    }
    return grid;
  }, [seed]);

  const cell = size / 25;

  // 三个定位角标区域
  const isFinder = (x, y) =>
    (x < 7 && y < 7) || (x >= 18 && y < 7) || (x < 7 && y >= 18);

  const finderPattern = (fx, fy) => {
    const pts = [];
    for (let y = 0; y < 7; y++) {
      for (let x = 0; x < 7; x++) {
        const border = x === 0 || y === 0 || x === 6 || y === 6;
        const core = x >= 2 && x <= 4 && y >= 2 && y <= 4;
        if (border || core) pts.push({ x: fx + x, y: fy + y });
      }
    }
    return pts;
  };

  const finders = [finderPattern(0, 0), finderPattern(18, 0), finderPattern(0, 18)];

  return (
    <div style={{
      width: size, height: size, padding: 12,
      background: '#fff', borderRadius: 12,
      boxShadow: '0 0 30px rgba(34,211,238,0.25), inset 0 0 12px rgba(0,0,0,0.08)',
      position: 'relative', flexShrink: 0,
    }}>
      <svg width={size - 24} height={size - 24} style={{ display: 'block' }}>
        {/* 数据点 */}
        {cells.filter((c) => !isFinder(c.x, c.y)).map((c, i) => (
          <rect
            key={i}
            x={c.x * cell}
            y={c.y * cell}
            width={cell * 0.92}
            height={cell * 0.92}
            rx={1}
            fill={c.on ? '#0f172a' : 'transparent'}
          />
        ))}
        {/* 定位角标 */}
        {finders.map((pts, i) => pts.map((p, j) => (
          <rect
            key={`${i}-${j}`}
            x={p.x * cell}
            y={p.y * cell}
            width={cell * 0.95}
            height={cell * 0.95}
            rx={1}
            fill="#0f172a"
          />
        )))}
      </svg>
      {/* 中心 Logo */}
      <div style={{
        position: 'absolute', top: '50%', left: '50%',
        transform: 'translate(-50%, -50%)',
        width: 38, height: 38, borderRadius: 10,
        background: 'var(--gradient-main)',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        fontSize: 18, boxShadow: '0 2px 12px rgba(0,0,0,0.3)',
      }}>⚡</div>
    </div>
  );
}
