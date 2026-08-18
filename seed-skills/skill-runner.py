# -*- coding: utf-8 -*-
"""
SKILL RUNNER — 私域 Skill 市场技能运行服务
==========================================
将 4 个 MCP 技能以 HTTP 服务形式常驻运行, 供网关转发调用。

路由: POST /<skill_key>/mcp   (JSON-RPC 2.0, MCP/2024-11-05)

技能:
  - db-inspection        数据库智能巡检助手
  - log-desensitization  日志敏感信息识别
  - alert-convergence    告警收敛分析
  - requirement-analysis  AI 需求分析助手

启动: python skill-runner.py [port]
Docker: docker compose up skill-runner
"""
import importlib.util
import json
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import urlparse

SKILL_KEYS = [
    "db-inspection",
    "log-desensitization",
    "alert-convergence",
    "requirement-analysis",
]

# 加载 4 个 MCP 技能模块 (server.py 的 main() 受 __main__ 保护, import 安全)
SKILL_MODULES = {}
for key in SKILL_KEYS:
    try:
        spec = importlib.util.spec_from_file_location(f"skill_{key}", f"{key}/server.py")
        mod = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(mod)
        SKILL_MODULES[key] = mod
        print(f"  ✓ 技能加载: {key}", file=sys.stderr)
    except Exception as e:
        print(f"  ✗ 技能加载失败: {key}: {e}", file=sys.stderr)


class SkillHTTPHandler(BaseHTTPRequestHandler):
    """HTTP 桥接: 接收 JSON-RPC 请求, 转发给对应技能的 handle_request"""

    def do_POST(self):
        parts = urlparse(self.path).path.strip("/").split("/")
        skill_key = parts[0] if parts else ""
        mod = SKILL_MODULES.get(skill_key)

        if not mod:
            return self._json_response(404, {
                "jsonrpc": "2.0", "id": 0,
                "error": {"code": -1, "message": f"未知技能: {skill_key}"},
            })

        try:
            length = int(self.headers.get("Content-Length", 0))
            body = self.rfile.read(length)
            request = json.loads(body.decode("utf-8"))
            response = mod.handle_request(request)
            return self._json_response(200, response)
        except json.JSONDecodeError as e:
            return self._json_response(400, {
                "jsonrpc": "2.0", "id": 0,
                "error": {"code": -32700, "message": f"JSON 解析错误: {e}"},
            })
        except Exception as e:
            return self._json_response(500, {
                "jsonrpc": "2.0", "id": 0,
                "error": {"code": -1, "message": f"内部错误: {e}"},
            })

    def _json_response(self, code, obj):
        payload = json.dumps(obj, ensure_ascii=False).encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, fmt, *args):
        sys.stderr.write(f"[skill-runner] {self.address_string()} {fmt % args}\n")


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8081
    print(f"🚀 SKILL RUNNER 启动: http://0.0.0.0:{port}", file=sys.stderr)
    print(f"   技能: {', '.join(SKILL_KEYS)}", file=sys.stderr)
    HTTPServer(("0.0.0.0", port), SkillHTTPHandler).serve_forever()
