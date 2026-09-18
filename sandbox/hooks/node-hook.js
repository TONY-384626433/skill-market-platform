/**
 * SkillHub 沙箱行为审计钩子 (Node.js)
 *
 * 通过 NODE_OPTIONS="--require .../node-hook.js" 预加载, 包装:
 *   · child_process.exec/execSync/spawn/spawnSync/execFile  -> 命令执行
 *   · net.connect / http(s).request / fetch                 -> 网络外联
 *   · net.Server.listen                                     -> 监听端口
 *   · fs.writeFile*/appendFile*/unlinkSync/rm*              -> 文件写入与删除
 *
 * 只做观测记录 (JSONL 写入 $SANDBOX_TRACE), 不阻断行为。
 */
'use strict';
const fs = require('fs');

const tracePath = process.env.SANDBOX_TRACE;
if (tracePath) {
  // 先捕获原生写入函数, 避免包装后自我递归
  const rawAppend = fs.appendFileSync.bind(fs);
  let count = 0;
  const emit = (event, target, detail) => {
    if (count >= 4000) return;
    count += 1;
    try {
      rawAppend(tracePath, JSON.stringify({
        event,
        target: String(target === undefined ? '' : target).slice(0, 200),
        detail: String(detail === undefined ? '' : detail).slice(0, 200),
        pid: process.pid,
      }) + '\n');
    } catch (e) { /* 观测失败不影响运行 */ }
  };

  try {
    const cp = require('child_process');
    ['exec', 'execSync', 'spawn', 'spawnSync', 'execFile', 'execFileSync', 'fork'].forEach((name) => {
      const original = cp[name];
      if (typeof original !== 'function') return;
      cp[name] = function wrapped(...args) {
        emit('child_process', args[0], name);
        return original.apply(this, args);
      };
    });

    const net = require('net');
    const originalConnect = net.connect;
    const wrappedConnect = function wrapped(...args) {
      emit('net.connect', args[0], 'connect');
      return originalConnect.apply(this, args);
    };
    net.connect = wrappedConnect;
    net.createConnection = wrappedConnect;
    if (net.Server && net.Server.prototype && net.Server.prototype.listen) {
      const originalListen = net.Server.prototype.listen;
      net.Server.prototype.listen = function wrapped(...args) {
        emit('socket.listen', args[0], 'listen');
        return originalListen.apply(this, args);
      };
    }

    ['http', 'https'].forEach((mod) => {
      try {
        const m = require(mod);
        ['request', 'get'].forEach((name) => {
          const original = m[name];
          if (typeof original !== 'function') return;
          m[name] = function wrapped(...args) {
            emit('http.request', typeof args[0] === 'string' ? args[0] : (args[0] && args[0].host), mod);
            return original.apply(this, args);
          };
        });
      } catch (e) { /* 模块不存在则忽略 */ }
    });

    ['writeFileSync', 'appendFileSync', 'unlinkSync', 'rmdirSync', 'rmSync', 'writeFile', 'appendFile', 'unlink', 'rm'].forEach((name) => {
      const original = fs[name];
      if (typeof original !== 'function') return;
      fs[name] = function wrapped(...args) {
        emit(name.startsWith('unlink') || name === 'rm' || name === 'rmSync' ? 'fs.unlink' : 'fs.write', args[0], name);
        return original.apply(this, args);
      };
    });
  } catch (e) { /* 钩子失败不影响被测进程 */ }
}
