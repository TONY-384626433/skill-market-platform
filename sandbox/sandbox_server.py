#!/usr/bin/env python3
"""SkillHub 动态沙箱验证服务 (Dynamic Sandbox Verification)

定位: 静态查毒之外的「第二道验证」——把待审技能放进一个
      无网络 / 只读根文件系统 / 非 root / 丢弃全部 capabilities / 限制内存与进程数
      的容器里真跑一次, 观察它到底做了什么:
        · 是否尝试外联 (DYN-01)
        · 是否执行系统命令/派生子进程 (DYN-02)
        · 是否越权写文件 (DYN-03)
        · 是否读取敏感凭据 (DYN-04)
        · 是否监听端口 (DYN-05)
        · 是否写持久化启动项 (DYN-06)
        · 运行是否稳定 (能完成 MCP 握手与工具调用) (DYN-07)

实现要点: 不依赖 strace/LD_PRELOAD (容器无网络装不了包), 改用
        · Python: sys.addaudithook (sitecustomize.py, 通过 PYTHONPATH 注入)
        · Node:   --require 预加载钩子, 包装 child_process/net/fs
        行为日志以 JSONL 落到 $SANDBOX_TRACE, 由本服务解析成结论。

接口:
  GET  /health   健康 + 隔离自检
  POST /verify   {skill_key, files:[{path,content_b64}], timeout} -> 动态验证报告
"""

import base64
import json
import os
import shutil
import stat
import socket
import subprocess
import sys
import threading
import time
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

ENGINE = "SANDBOX-ENGINE 1.0.0"
WORK_ROOT = Path(os.environ.get("SANDBOX_WORK", "/work"))
HOOK_DIR = os.environ.get("SANDBOX_HOOKS", "/opt/sandbox-hooks")
DEFAULT_TIMEOUT = int(os.environ.get("SANDBOX_TIMEOUT", "45"))
MAX_FILES = 200
MAX_FILE_BYTES = 4 << 20
MAX_TRACE_LINES = 4000

SENSITIVE_PATHS = [
    "/root/.ssh", "/root/.aws", "/root/.netrc", "/home", "/etc/shadow", "/etc/passwd",
    "/etc/sudoers", "/var/spool/cron", "/etc/cron.d", "/etc/systemd/system", "/usr/local/bin",
    "/root/.bashrc", "/root/.profile", "/opt", "/srv",
]
PERSIST_MARKERS = ["/etc/cron", "/var/spool/cron", "/etc/systemd", "rc.local", ".bashrc",
                   ".profile", ".zshrc", "crontab", "/etc/rc", "systemd/system"]
SENSITIVE_READ_MARKERS = [".ssh/id_", ".aws/credentials", ".netrc", "shadow", ".kube/config",
                          "keychain", "cookies.sqlite", "wallet.dat", ".git-credentials"]

ENTRY_CANDIDATES = ["server.py", "main.py", "app.py", "runner.py", "skill.py", "index.js", "install.sh", "setup.sh"]


# ------------------------------------------------------------------ 工具

def log(msg):
    print("[sandbox] " + str(msg), flush=True)


def snapshot_tree():
    """敏感目录快照 (路径 -> (size, mtime)); 无权限目录直接跳过, 不抛异常"""
    snap = {}
    for base in SENSITIVE_PATHS:
        try:
            st0 = os.stat(base)
        except OSError:
            continue
        if stat.S_ISREG(st0.st_mode):
            snap[base] = (st0.st_size, int(st0.st_mtime))
            continue
        for root, dirs, files in os.walk(base, onerror=lambda err: None):
            dirs[:] = [d for d in dirs if d not in ("proc", "sys", "dev")]
            for name in files:
                fp = os.path.join(root, name)
                try:
                    st = os.stat(fp)
                    snap[fp] = (st.st_size, int(st.st_mtime))
                except OSError:
                    continue
    return snap


def reading_listeners():
    """读取当前监听端口 (容器内无网络, 任何监听都值得关注)"""
    out = []
    for f in ("/proc/net/tcp", "/proc/net/tcp6"):
        try:
            lines = Path(f).read_text().splitlines()[1:]
        except OSError:
            continue
        for line in lines:
            parts = line.split()
            if len(parts) > 3 and parts[3] == "0A":  # 0A = LISTEN
                out.append(parts[1])
    return out


def isolation_selfcheck():
    """隔离自检: 网络是否真的不可达 / 是否非 root / capabilities"""
    reachable = False
    try:
        s = socket.socket()
        s.settimeout(1.5)
        s.connect(("1.1.1.1", 443))
        reachable = True
        s.close()
    except Exception:
        reachable = False
    caps = ""
    try:
        for line in Path("/proc/self/status").read_text().splitlines():
            if line.startswith("CapEff"):
                caps = line.split(":")[1].strip()
    except OSError:
        pass
    return {
        "network_reachable": reachable,
        "uid": os.getuid(),
        "cap_eff": caps,
        "rootfs_read_only": not os.access("/usr", os.W_OK),
        "engine": ENGINE,
    }


def parse_trace(path):
    events = []
    if not Path(path).exists():
        return events
    try:
        with open(path, encoding="utf8", errors="replace") as fh:
            for i, line in enumerate(fh):
                if i >= MAX_TRACE_LINES:
                    break
                line = line.strip()
                if not line:
                    continue
                try:
                    events.append(json.loads(line))
                except Exception:
                    continue
    except OSError:
        pass
    return events


def _is_own_scope(target, workdir):
    """写/删自己的沙箱工作目录、临时目录属于正常行为, 不算越权
    (银行技能落自己目录/临时文件是合理的, 只有动到系统目录或他处才告警)"""
    text = str(target or "").strip()
    if not text:
        return False
    if not text.startswith("/"):
        return True  # 相对路径 = 技能自己的工作目录
    safe = ["/tmp", "/var/tmp", "/run", "/dev/shm"]
    if workdir:
        safe.append(str(workdir))
    return any(text == p or text.startswith(p.rstrip("/") + "/") for p in safe)


def classify(events, workdir=None):
    """把行为事件归类"""
    buckets = {"network": [], "exec": [], "file_write": [], "sensitive_read": [],
               "listen": [], "persist": [], "delete": [], "benign": []}
    for ev in events:
        kind = str(ev.get("event", ""))
        detail = str(ev.get("detail", ""))
        target = str(ev.get("target", ""))
        lowered = (detail + " " + target).lower()
        if kind in ("socket.connect", "socket.getaddrinfo", "socket.gethostbyname", "net.connect", "http.request"):
            buckets["network"].append(ev)
        elif kind in ("subprocess.Popen", "os.system", "os.exec", "os.spawn", "child_process"):
            buckets["exec"].append(ev)
        elif kind in ("os.remove", "os.unlink", "shutil.rmtree", "fs.unlink", "fs.rm"):
            if _is_own_scope(target, workdir):
                buckets["benign"].append(ev)
            else:
                buckets["delete"].append(ev)
        elif kind in ("open.write", "fs.write", "fs.append"):
            if any(m in lowered for m in PERSIST_MARKERS):
                buckets["persist"].append(ev)
            elif _is_own_scope(target, workdir):
                buckets["benign"].append(ev)
            else:
                buckets["file_write"].append(ev)
        elif kind in ("socket.listen", "server.listen"):
            buckets["listen"].append(ev)
        if any(m in lowered for m in SENSITIVE_READ_MARKERS):
            buckets["sensitive_read"].append(ev)
    return buckets


def finding(rule_id, category, severity, title, detail, evidence="", blocking=None):
    return {
        "rule_id": rule_id, "category": category, "severity": severity,
        "title": title, "detail": detail,
        "evidence": evidence[:300],
        "blocking": (severity == "critical") if blocking is None else blocking,
    }


def mcp_probe(proc, timeout):
    """对 stdio MCP 技能做握手 + 工具调用 (尽力而为, 不阻塞)"""
    result = {"initialize": False, "tools": [], "tool_calls": 0, "tool_errors": 0, "responses": []}
    lock = threading.Lock()

    def reader():
        for raw in proc.stdout:
            line = raw.decode("utf8", "replace").strip()
            if not line:
                continue
            try:
                msg = json.loads(line)
            except Exception:
                continue
            with lock:
                if msg.get("id") == 1:
                    result["initialize"] = True
                if msg.get("id") == 2:
                    res = (msg.get("result") or {}).get("tools") or []
                    result["tools"] = [t.get("name") for t in res if isinstance(t, dict)]
                    result["tool_schemas"] = res
                if msg.get("id") and int(msg.get("id")) >= 100:
                    result["tool_calls"] += 1
                    if msg.get("error"):
                        result["tool_errors"] += 1
                    result["responses"].append(msg.get("result") or msg.get("error"))

    thread = threading.Thread(target=reader, daemon=True)
    thread.start()

    def send(obj):
        try:
            proc.stdin.write((json.dumps(obj) + "\n").encode())
            proc.stdin.flush()
            return True
        except Exception:
            return False

    send({"jsonrpc": "2.0", "id": 1, "method": "initialize",
          "params": {"protocolVersion": "2024-11-05", "capabilities": {},
                     "clientInfo": {"name": "skillhub-sandbox", "version": "1.0.0"}}})
    # 进程已退出则无需继续等待握手 (非 MCP 脚本很常见), 避免白等半个切片超时
    deadline = time.time() + timeout * 0.5
    while time.time() < deadline and not result["initialize"] and proc.poll() is None:
        time.sleep(0.2)
    send({"jsonrpc": "2.0", "method": "notifications/initialized"})
    send({"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": {}})
    deadline = time.time() + timeout * 0.3
    while time.time() < deadline and not result["tools"] and proc.poll() is None:
        time.sleep(0.2)
    for index, tool in enumerate((result.get("tool_schemas") or [])[:3]):
        args = {}
        schema = (tool.get("inputSchema") or {}).get("properties") or {}
        for key, spec in list(schema.items())[:4]:
            args[key] = "sandbox-probe" if spec.get("type") == "string" else 1
        send({"jsonrpc": "2.0", "id": 100 + index, "method": "tools/call",
              "params": {"name": tool.get("name"), "arguments": args}})
        time.sleep(0.6)
    return result


# ------------------------------------------------------------------ 核心

def verify(payload):
    started = time.time()
    skill_key = str(payload.get("skill_key") or "uploaded")[:80]
    timeout = int(payload.get("timeout") or DEFAULT_TIMEOUT)
    timeout = max(5, min(timeout, 120))
    files = payload.get("files") or []

    run_id = "run-" + uuid.uuid4().hex[:12]
    workdir = WORK_ROOT / run_id
    report = {"engine": ENGINE, "run_id": run_id, "skill_key": skill_key,
              "isolated": isolation_selfcheck(), "executed": False, "entry": None,
              "exit_code": None, "duration_ms": 0, "events": {}, "checks": [],
              "findings": [], "trace_lines": 0, "stdout_tail": "", "stderr_tail": "",
              "verdict_hint": "unknown"}

    # 1) 落盘 (拒绝路径穿越)
    workdir.mkdir(parents=True, exist_ok=True)
    written = 0
    for item in files[:MAX_FILES]:
        rel = str(item.get("path") or "")
        if not rel or rel.startswith("/") or ".." in rel.replace("\\", "/").split("/"):
            report["findings"].append(finding("FILE-01", "integrity", "critical", "路径穿越 (Zip Slip)",
                                              "包内文件使用越权路径, 沙箱拒绝解压", rel))
            continue
        try:
            content = base64.b64decode(item.get("content_b64") or "")
        except Exception:
            continue
        if len(content) > MAX_FILE_BYTES:
            continue
        target = workdir / rel
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(content)
        written += 1
    if written == 0:
        report["checks"].append({"rule_id": "DYN-00", "name": "可执行内容", "status": "skipped",
                                "detail": "包内无可执行文件, 跳过动态验证"})
        report["verdict_hint"] = "skipped"
        shutil.rmtree(workdir, ignore_errors=True)
        return report
    os.chmod(workdir, 0o777)

    # 2) 选择入口 (递归查找, 同名取最浅的一个; 最多跑 3 个, 覆盖面更全)

    def _find_entry(name):
        hits = []
        for root, dirs, files in os.walk(workdir):
            dirs[:] = [d for d in dirs if d not in (".git", "node_modules", "__pycache__")]
            if name in files:
                rel = os.path.relpath(os.path.join(root, name), workdir)
                hits.append(rel.replace(os.sep, "/"))
        hits.sort(key=lambda r: (r.count("/"), r))
        return hits[0] if hits else None

    entries = []
    for candidate in ENTRY_CANDIDATES:
        hit = _find_entry(candidate)
        if hit:
            entries.append(hit)
        if len(entries) >= 3:
            break
    entry = entries[0] if entries else None
    report["entry"] = entry
    report["entries"] = entries

    before = snapshot_tree()
    trace_path = workdir / "trace.jsonl"
    env = dict(os.environ)
    env.update({
        "PYTHONPATH": HOOK_DIR + os.pathsep + env.get("PYTHONPATH", ""),
        "SANDBOX_TRACE": str(trace_path),
        "PYTHONUNBUFFERED": "1",
        "NODE_OPTIONS": "--require " + str(Path(HOOK_DIR) / "node-hook.js"),
        "HOME": str(workdir),
        "TMPDIR": str(workdir / ".tmp"),
        "SKILLHUB_SANDBOX": "1",
    })
    (workdir / ".tmp").mkdir(exist_ok=True)

    proc = None
    mcp = {"initialize": False, "tools": [], "tool_calls": 0, "tool_errors": 0}
    stdout_all, stderr_all = [], []
    timed_out = False
    slice_timeout = max(6, int(timeout / max(1, len(entries))))
    try:
        for entry in entries:
            if entry.endswith(".js"):
                if not shutil.which("node"):
                    report["checks"].append({"rule_id": "DYN-07", "name": "运行环境", "status": "skipped",
                                             "detail": "容器内无 node 运行时, 跳过 " + entry})
                    continue
                cmd = ["node", str(workdir / entry)]
            elif entry.endswith(".py"):
                cmd = [sys.executable, "-u", str(workdir / entry)]
            else:
                cmd = ["bash", str(workdir / entry)]

            report["executed"] = True
            proc = subprocess.Popen(cmd, cwd=str(workdir), env=env, stdin=subprocess.PIPE,
                                    stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                    preexec_fn=os.setsid)
            current = {"initialize": False, "tools": [], "tool_calls": 0, "tool_errors": 0}
            if entry.endswith(".py"):
                current = mcp_probe(proc, slice_timeout)
            try:
                out, err = proc.communicate(timeout=slice_timeout)
            except subprocess.TimeoutExpired:
                os.killpg(os.getpgid(proc.pid), 9)
                out, err = proc.communicate()
                timed_out = True
            report.setdefault("runs", []).append({
                "entry": entry, "exit_code": proc.returncode,
                "mcp_initialize": current.get("initialize"),
                "tools": len(current.get("tools") or []),
            })
            if current.get("initialize") and not mcp.get("initialize"):
                mcp = current
            elif current.get("tools"):
                mcp["tools"] = mcp.get("tools") or current.get("tools")
            mcp["tool_calls"] += current.get("tool_calls", 0)
            mcp["tool_errors"] += current.get("tool_errors", 0)
            stdout_all.append((out or b"").decode("utf8", "replace"))
            stderr_all.append((err or b"").decode("utf8", "replace"))
            report["exit_code"] = proc.returncode

        if timed_out:
            report["findings"].append(finding("DYN-07", "compliance", "medium", "运行超时",
                                              f"沙箱内运行超过 {slice_timeout}s 被强制终止, 需人工确认是否存在死循环/常驻行为"))
        report["stdout_tail"] = "".join(stdout_all)[-1500:]
        report["stderr_tail"] = "".join(stderr_all)[-1500:]
    except Exception as exc:  # noqa: BLE001
        report["findings"].append(finding("DYN-07", "compliance", "medium", "运行异常",
                                          f"沙箱执行异常: {exc}"))
    finally:
        if proc and proc.poll() is None:
            try:
                os.killpg(os.getpgid(proc.pid), 9)
            except Exception:
                pass

    # 3) 行为归类
    events = parse_trace(trace_path)
    buckets = classify(events, workdir)
    report["trace_lines"] = len(events)
    report["events"] = {k: len(v) for k, v in buckets.items()}

    def ev_sample(items, n=3):
        return " | ".join(str(i.get("event")) + ":" + str(i.get("target") or i.get("detail"))[:70] for i in items[:n])

    if buckets["network"]:
        report["findings"].append(finding("DYN-01", "network", "critical", "运行期尝试外联",
                                          "运行过程中发起网络连接/域名解析; 银行内网技能不允许未声明的外联行为",
                                          ev_sample(buckets["network"])))
    if buckets["exec"]:
        report["findings"].append(finding("DYN-02", "execution", "critical", "运行期执行系统命令",
                                          "运行过程中派生子进程/调用系统命令, 存在命令执行与提权风险",
                                          ev_sample(buckets["exec"])))
    if buckets["persist"]:
        report["findings"].append(finding("DYN-06", "persistence", "critical", "运行期写持久化启动项",
                                          "运行过程中尝试写入 cron/systemd/启动项, 属于持久化后门行为",
                                          ev_sample(buckets["persist"])))
    if buckets["sensitive_read"]:
        report["findings"].append(finding("DYN-04", "credential", "critical", "运行期读取敏感凭据",
                                          "运行过程中访问密钥/凭据类文件, 属于窃密行为",
                                          ev_sample(buckets["sensitive_read"])))
    if buckets["file_write"]:
        report["findings"].append(finding("DYN-03", "integrity", "high", "运行期越权写文件",
                                          "运行过程中向工作目录之外写入文件",
                                          ev_sample(buckets["file_write"])))
    if buckets["listen"]:
        report["findings"].append(finding("DYN-05", "network", "high", "运行期监听端口",
                                          "运行过程中开放监听端口, 需确认是否为声明内的服务",
                                          ev_sample(buckets["listen"])))
    if buckets["delete"]:
        report["findings"].append(finding("DYN-08", "destructive", "high", "运行期删除文件",
                                          "运行过程中执行删除动作", ev_sample(buckets["delete"])))

    # 4) 文件系统比对 (容器根只读, 越权写入会直接失败并被记录)
    after = snapshot_tree()
    new_files = [p for p in after if p not in before]
    changed = [p for p in after if p in before and after[p] != before[p]]
    if new_files or changed:
        evidence = ",".join((new_files + changed)[:4])
        report["findings"].append(finding("DYN-03", "integrity", "critical", "文件系统被越权修改",
                                          "沙箱快照对比发现工作目录之外的文件新增/变更",
                                          evidence))
    report["filesystem_diff"] = {"new": new_files[:10], "changed": changed[:10]}

    # 5) 运行稳定性 (MCP 握手)
    mcp_summary = {"initialize": mcp.get("initialize"), "tools": mcp.get("tools"),
                   "tool_calls": mcp.get("tool_calls"), "tool_errors": mcp.get("tool_errors")}
    report["mcp"] = mcp_summary
    if entries:
        if mcp.get("initialize"):
            detail = f"MCP 握手成功, 发现工具 {len(mcp.get('tools') or [])} 个, 调用 {mcp.get('tool_calls')} 次 (失败 {mcp.get('tool_errors')})"
            report["checks"].append({"rule_id": "DYN-07", "name": "运行稳定性", "status": "passed", "detail": detail})
        elif report["exit_code"] == 0:
            report["checks"].append({"rule_id": "DYN-07", "name": "运行稳定性", "status": "passed",
                                     "detail": f"非 MCP 脚本, 正常退出 (exit={report['exit_code']})"})
        else:
            report["checks"].append({"rule_id": "DYN-07", "name": "运行稳定性", "status": "warning",
                                     "detail": f"运行未完成握手或异常退出 (exit={report['exit_code']})"})

    report["duration_ms"] = int((time.time() - started) * 1000)
    critical = [f for f in report["findings"] if f["severity"] == "critical"]
    high = [f for f in report["findings"] if f["severity"] == "high"]
    report["critical_count"] = len(critical)
    report["high_count"] = len(high)
    report["checks"].append({
        "rule_id": "DYN-01~08", "name": "行为观测汇总",
        "status": "failed" if critical else ("warning" if high else "passed"),
        "detail": "行为事件: " + json.dumps(report["events"], ensure_ascii=False),
    })
    report["verdict_hint"] = "malicious" if critical else ("suspicious" if (high or report["findings"]) else "safe")
    shutil.rmtree(workdir, ignore_errors=True)
    return report


# ------------------------------------------------------------------ HTTP

class Handler(BaseHTTPRequestHandler):
    server_version = "SkillHubSandbox/1.0"

    def _json(self, code, payload):
        body = json.dumps(payload, ensure_ascii=False).encode("utf8")
        self.send_response(code)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):  # noqa: N802
        if self.path.startswith("/health"):
            self._json(200, {"status": "ok", "service": "skillhub-sandbox", "engine": ENGINE,
                             "isolated": isolation_selfcheck()})
            return
        self._json(404, {"error": "not found"})

    def do_POST(self):  # noqa: N802
        if not self.path.startswith("/verify"):
            self._json(404, {"error": "not found"})
            return
        try:
            length = int(self.headers.get("Content-Length") or 0)
            if length <= 0 or length > 64 << 20:
                self._json(413, {"error": "请求体过大或为空"})
                return
            payload = json.loads(self.rfile.read(length) or b"{}")
        except Exception as exc:  # noqa: BLE001
            self._json(400, {"error": f"请求解析失败: {exc}"})
            return
        try:
            report = verify(payload)
        except Exception as exc:  # noqa: BLE001
            log("verify failed: %s" % exc)
            self._json(500, {"error": f"沙箱验证失败: {exc}"})
            return
        self._json(200, report)

    def log_message(self, fmt, *args):  # 保持输出干净
        return


def main():
    port = int(os.environ.get("SANDBOX_PORT", "8090"))
    WORK_ROOT.mkdir(parents=True, exist_ok=True)
    log(f"{ENGINE} 启动, 监听 :{port}, 工作目录 {WORK_ROOT}")
    log("隔离自检: " + json.dumps(isolation_selfcheck(), ensure_ascii=False))
    ThreadingHTTPServer(("0.0.0.0", port), Handler).serve_forever()


if __name__ == "__main__":
    main()
