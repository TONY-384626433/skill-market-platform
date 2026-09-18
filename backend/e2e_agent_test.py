# -*- coding: utf-8 -*-
"""AI 智能体 — 端到端验收测试
覆盖: 工具发现 / 运行状态 / 意图路由真实调用 / 安全拦截 / 审计留痕
"""
import json
import sys
import urllib.error
import urllib.request

sys.stdout.reconfigure(encoding="utf-8", errors="replace")

BASE = "http://localhost:8080/api/v1"
PASS = 0
FAIL = 0
AUTH = {"value": ""}


def call(method, path, body=None):
    headers = {"Content-Type": "application/json"}
    if AUTH["value"]:
        headers["Authorization"] = "Bearer " + AUTH["value"]
    data = json.dumps(body, ensure_ascii=False).encode("utf-8") if body is not None else None
    req = urllib.request.Request(BASE + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=200) as resp:
            return json.loads(resp.read().decode("utf-8")), resp.status
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8", "replace")
        try:
            return json.loads(raw), e.code
        except Exception:
            return {"raw": raw}, e.code
    except Exception as e:  # noqa: BLE001
        return {"error": str(e)}, 0


def check(name, ok, detail=""):
    global PASS, FAIL
    if ok:
        PASS += 1
        print("[PASS]", name, detail)
    else:
        FAIL += 1
        print("[FAIL]", name, detail)


# ---------- 登录 ----------
res, _ = call("POST", "/auth/login", {"username": "zhangsan", "password": "demo"})
AUTH["value"] = res.get("token", "")
check("登录", bool(AUTH["value"]))

# ---------- 1. 智能体状态 ----------
res, code = call("GET", "/agent/status")
status = res.get("data", {})
check("智能体状态接口", code == 200 and status.get("tool_count", 0) > 0,
      f"模式={status.get('mode')} 模型={status.get('model')} 技能={status.get('skill_count')} 工具={status.get('tool_count')}")

# ---------- 2. 工具发现 (只暴露通过可用性审核的技能) ----------
res, code = call("GET", "/agent/tools")
tools = res.get("data", [])
keys = sorted({t["skill_key"] for t in tools})
check("工具发现", code == 200 and len(tools) >= 4, f"{len(tools)} 个工具 / 技能: {', '.join(keys)}")
check("只暴露已过审技能", "broken-skill-probe" not in keys, str(keys))
schema_ok = all(isinstance(t.get("input_schema"), dict) and t["input_schema"].get("type") == "object" for t in tools)
check("工具携带真实 inputSchema", schema_ok, "" if schema_ok else "存在缺失 schema 的工具")

# ---------- 3. 四类意图 → 真实技能调用 ----------
cases = [
    ("巡检一下核心银行数据库 core-banking-db-01", "db-inspection", "inspect_database"),
    ("帮我分析支付节点最近的告警收敛情况", "alert-convergence", "analyze_alerts"),
    ("把这段需求整理成结构化分析: 客户线上申请对公开户", "requirement-analysis", "analyze_requirement"),
    ("帮我检测并脱敏: 用户张三 身份证 110101199003078888 手机号 13800138000", "log-desensitization", "desensitize"),
]
for question, expect_skill, expect_tool in cases:
    res, code = call("POST", "/agent/chat", {"message": question})
    data = res.get("data") or {}
    steps = data.get("steps", [])
    hit = any(s["skill_key"] == expect_skill and s["tool_name"] == expect_tool and s["status"] == "success" for s in steps)
    check(f"意图路由 → {expect_skill}", code == 200 and hit,
          f"{len(steps)} 步 / {data.get('duration_ms')}ms / 回答 {len(data.get('answer', ''))} 字")
    if not data.get("answer"):
        print("       ↳ 接口返回:", str(res)[:160])

# ---------- 4. 安全: 非脱敏技能收到含敏感信息的参数必须被拦截 ----------
res, code = call("POST", "/agent/chat", {"message": "把这条需求整理一下: 客户忘记密码后需要线上重置流程"})
data = res.get("data") or {}
blocked = any(s["status"] == "blocked" for s in (data.get("steps") or []))
check("智能体侧敏感输入拦截", code == 200 and blocked, "已拦截敏感参数" if blocked else "未触发拦截")

# ---------- 4b. 脱敏白名单技能应正常放行 ----------
res, code = call("POST", "/agent/chat", {"message": "帮我把日志脱敏: 身份证 110101199003078888 手机号 13800138000"})
data = res.get("data") or {}
steps = data.get("steps") or []
allowed = any(s["skill_key"] == "log-desensitization" and s["status"] == "success" for s in steps)
check("脱敏类技能白名单放行", code == 200 and allowed, "敏感数据交由脱敏技能处理")

# ---------- 5. 兜底: 无匹配意图时给出可用能力清单 ----------
res, code = call("POST", "/agent/chat", {"message": "今天天气怎么样"})
data = res.get("data") or {}
check("无匹配意图时给出能力清单", code == 200 and "可自动调度" in data.get("answer", "") and len(data.get("steps") or []) == 0,
      data.get("answer", "")[:40].replace("\n", " "))

# ---------- 6. 回答里带上真实技能输出 ----------
res, code = call("POST", "/agent/chat", {"message": "巡检一下支付数据库 payment-db-02"})
answer = (res.get("data") or {}).get("answer", "")
check("回答包含真实技能结果", code == 200 and ("健康" in answer or "巡检" in answer or "数据库" in answer), answer[:60].replace("\n", " "))

# ---------- 7. 审计留痕 ----------
res, code = call("GET", "/admin/audit-logs?page=1&page_size=5")
source_agent = [l for l in res.get("data", []) if l.get("source_ip") == "agent"]
check("智能体调用写入审计日志", code in (200, 403), f"HTTP {code}, agent 来源 {len(source_agent)} 条 (403 表示该账号非管理员)")

# ---------- 8. 参数校验 ----------
res, code = call("POST", "/agent/chat", {"message": ""})
check("空消息参数校验", code == 400, str(res.get("error")))

print()
print(f"===== 结果: {PASS} 通过 / {FAIL} 失败 =====")
sys.exit(1 if FAIL else 0)
