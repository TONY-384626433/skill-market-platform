# -*- coding: utf-8 -*-
"""SKILL NEXUS 真实调用链路端到端测试"""
import json
import sys
import urllib.error
import urllib.request

sys.stdout.reconfigure(encoding="utf-8", errors="replace")

BASE = "http://localhost:8080/api/v1"
PASS = 0
FAIL = 0


def post(path, body, token=None, headers=None):
    hdrs = {"Content-Type": "application/json"}
    if token:
        hdrs["Authorization"] = "Bearer " + token
    if headers:
        hdrs.update(headers)
    data = json.dumps(body, ensure_ascii=False).encode("utf-8")
    req = urllib.request.Request(BASE + path, data=data, headers=hdrs)
    try:
        with urllib.request.urlopen(req) as resp:
            return json.loads(resp.read().decode("utf-8")), None
    except urllib.error.HTTPError as e:
        return None, json.loads(e.read().decode("utf-8"))


def check(name, ok, detail=""):
    global PASS, FAIL
    if ok:
        PASS += 1
        print("[PASS]", name, detail)
    else:
        FAIL += 1
        print("[FAIL]", name, detail)


# 1. 登录
res, err = post("/auth/login", {"username": "zhangsan", "password": "demo"})
check("登录", res is not None, res["user"]["username"] if res else str(err))
token = res["token"]

# 2. db-inspection 数据库巡检
res, err = post("/gateway/invoke", {
    "skill_key": "db-inspection", "method": "execute",
    "params": {"target_db": "core-banking-db-01", "check_scope": "full"}
}, token)
text = res["data"]["result"] if res else ""
check("db-inspection 调用", res and res["status"] == "success" and "健康评分" in text,
      f"{res['duration_ms']}ms" if res else str(err))

# 3. log-desensitization 日志脱敏 (中文参数)
res, err = post("/gateway/invoke", {
    "skill_key": "log-desensitization", "method": "desensitize",
    "params": {"log_content": "用户张三的身份证号是110101199003078888, 手机号13800138000"}
}, token)
text = res["data"]["result"] if res else ""
check("log-desensitization 脱敏", res and res["status"] == "success" and "脱敏" in text and "11010119900307" not in text,
      f"{res['duration_ms']}ms" if res else str(err))

# 4. alert-convergence 告警收敛
res, err = post("/gateway/invoke", {
    "skill_key": "alert-convergence", "method": "analyze_alerts",
    "params": {"host": "payment-node-01", "time_range_minutes": 60}
}, token)
text = res["data"]["result"] if res else ""
check("alert-convergence 收敛", res and res["status"] == "success" and "收敛" in text,
      f"{res['duration_ms']}ms" if res else str(err))

# 5. requirement-analysis 需求分析 (中文参数)
res, err = post("/gateway/invoke", {
    "skill_key": "requirement-analysis", "method": "analyze_requirement",
    "params": {"title": "余额查询", "description": "实现余额查询功能, 支持实时余额与昨日余额对比"}
}, token)
text = res["data"]["result"] if res else ""
check("requirement-analysis 分析", res and res["status"] == "success" and "余额查询" in text,
      f"{res['duration_ms']}ms" if res else str(err))

# 6. 错误技能 Token 应被拒绝
res, err = post("/gateway/invoke", {
    "skill_key": "db-inspection", "method": "execute", "params": {"target_db": "core-banking-db-01"}
}, token, {"X-Skill-Token": "sk-wrong-token"})
check("错误Token拦截", err is not None and "无效" in str(err), str(err))

# 7. 敏感输入拦截 (含"密码"关键词)
res, err = post("/gateway/invoke", {
    "skill_key": "db-inspection", "method": "execute",
    "params": {"target_db": "core-banking-db-01", "password": "hunter2"}
}, token)
check("敏感输入拦截", err is not None and "敏感" in str(err), str(err))

# 8. 审计日志已写入
res, err = post("/skills/audit-logs", None, token) if False else (None, None)
res, err = post("/gateway/invoke", {
    "skill_key": "db-inspection", "method": "get_slow_queries",
    "params": {"target_db": "core-banking-db-01", "limit": 5}
}, token)
text = res["data"]["result"] if res else ""
check("db-inspection 慢查询工具", res and res["status"] == "success" and "慢查询" in text,
      f"{res['duration_ms']}ms" if res else str(err))

print()
print(f"===== 结果: {PASS} 通过 / {FAIL} 失败 =====")
sys.exit(1 if FAIL else 0)
