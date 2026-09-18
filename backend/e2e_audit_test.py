# -*- coding: utf-8 -*-
"""技能可用性审核 — 端到端验收测试
覆盖: 审核引擎 / 6 类检查项 / 审核队列 / 报告 / 发布门禁 / 安装门禁 / 坏技能识别
"""
import json
import sys
import urllib.error
import urllib.request

sys.stdout.reconfigure(encoding="utf-8", errors="replace")

BASE = "http://localhost:8080/api/v1"
PASS = 0
FAIL = 0


def call(method, path, body=None, token=None):
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    data = json.dumps(body, ensure_ascii=False).encode("utf-8") if body is not None else None
    req = urllib.request.Request(BASE + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=120) as resp:
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


# ---------- 1. 登录 ----------
res, _ = call("POST", "/auth/login", {"username": "admin", "password": "demo"})
admin_token = res.get("token")
check("管理员登录", bool(admin_token), res.get("user", {}).get("username", ""))

res, _ = call("POST", "/auth/login", {"username": "zhangsan", "password": "demo"})
dev_token = res.get("token")
check("开发者登录", bool(dev_token))

# ---------- 2. 审核概览 / 队列 ----------
res, code = call("GET", "/admin/audit-overview", token=admin_token)
overview = res.get("data", {})
check("审核概览接口", code == 200 and "total_skills" in overview, f"技能 {overview.get('total_skills')} 个 / 引擎 v{overview.get('engine_version')}")

res, code = call("GET", "/admin/audit-queue", token=admin_token)
queue = res.get("data", [])
check("审核队列接口", code == 200 and len(queue) >= 4, f"{len(queue)} 项")

# ---------- 3. 真实技能审核 (4 个种子技能) ----------
seed_ids = {"s-001": "db-inspection", "s-002": "log-desensitization"}
# 其余技能 ID 从队列里取 (跳过故意构造的坏技能)
for item in queue:
    if item["skill_key"] == "broken-skill-probe":
        continue
    seed_ids.setdefault(item["id"], item["skill_key"])

audited = {}
for sid, key in seed_ids.items():
    res, code = call("POST", f"/admin/skills/{sid}/audit", {}, admin_token)
    audit = res.get("data") or {}
    audited[key] = audit
    checks = audit.get("checks", [])
    check(f"审核 {key}", code == 200 and audit.get("status") == "passed",
          f"{audit.get('score')} 分 / {audit.get('grade')} 级 / {len(checks)} 项检查 / {audit.get('duration_ms')}ms")
    if audit.get("status") != "passed":
        for c in checks:
            if c.get("status") == "failed":
                print("       ↳ 失败项:", c["code"], c["name"], c.get("detail"))

# 检查项完整性 (以 db-inspection 为例)
db_audit = audited.get("db-inspection", {})
codes = [c["code"] for c in db_audit.get("checks", [])]
check("六类检查项齐全", codes == ["AVAIL-01", "AVAIL-02", "AVAIL-03", "AVAIL-04", "AVAIL-05", "AVAIL-06"], str(codes))
upstream = [c for c in db_audit.get("checks", []) if c["code"] == "AVAIL-03"]
check("协议握手检查有真实证据", bool(upstream and "tools=" in (upstream[0].get("evidence") or "")),
      (upstream[0].get("detail", "") if upstream else ""))
func = [c for c in db_audit.get("checks", []) if c["code"] == "AVAIL-04"]
check("功能调用检查有真实输出", bool(func and "output=" in (func[0].get("evidence") or "")),
      (func[0].get("detail", "")[:60] if func else ""))
perf = [c for c in db_audit.get("checks", []) if c["code"] == "AVAIL-05"]
check("性能基线采集到数据", bool(perf and "avg=" in (perf[0].get("evidence") or "")),
      (perf[0].get("evidence", "") if perf else ""))

# ---------- 4. 坏技能必须被判不合格 ----------
bad_key = "broken-skill-probe"
res, code = call("POST", "/skills", {
    "skill_key": bad_key,
    "name": "探针: 无法连接的坏技能",
    "version": "0.1.0",
    "category": "测试",
    "summary": "用于验证审核引擎能识别不可用技能",
    "skill_type": "mcp",
    "endpoint_url": "http://skill-runner:8081/broken-skill-probe/mcp",
    "tags": ["测试"],
    "manifest": "{}",
    "permissions": "[]",
}, dev_token)
bad_id = (res.get("data") or {}).get("id")
if not bad_id:
    # 已存在同 key 技能时从我的提交列表回查
    subs, _ = call("GET", "/skills/my/submissions", token=dev_token)
    for item in subs.get("data", []):
        if item.get("skill_key") == bad_key:
            bad_id = item["id"]
            break
check("创建/复用待审坏技能", bool(bad_id), f"id={bad_id}")

res, code = call("POST", f"/admin/skills/{bad_id}/audit", {}, admin_token)
bad_audit = res.get("data") or {}
bad_checks = {c["code"]: c for c in bad_audit.get("checks", [])}
check("坏技能审核判定为不合格", bad_audit.get("status") == "failed",
      f"{bad_audit.get('score')} 分 / 关键失败 {bad_audit.get('critical_failures')} 项")
check("识别出服务不可达", bad_checks.get("AVAIL-03", {}).get("status") == "failed",
      bad_checks.get("AVAIL-03", {}).get("detail", "")[:70])
check("识别出接口契约缺失", bad_checks.get("AVAIL-02", {}).get("status") in ("failed", "warning"),
      bad_checks.get("AVAIL-02", {}).get("detail", "")[:70])
check("识别出权限声明缺失", bad_checks.get("AVAIL-06", {}).get("status") in ("failed", "warning"),
      bad_checks.get("AVAIL-06", {}).get("detail", "")[:70])

# ---------- 5. 发布门禁: 未通过审核不允许发布 ----------
res, code = call("POST", f"/admin/skills/{bad_id}/review", {"verdict": "approve", "comment": "试图绕过审核"}, admin_token)
check("发布门禁拦截未过审技能", code == 409 and res.get("gate") == "availability_audit", str(res.get("error"))[:60])

# ---------- 6. 报告查询 ----------
res, code = call("GET", f"/admin/skills/{bad_id}/audits", token=admin_token)
check("技能审核历史", code == 200 and len(res.get("data", [])) >= 1, f"{len(res.get('data', []))} 条")

res, code = call("GET", "/admin/audits/recent?limit=10", token=admin_token)
check("最近审核记录", code == 200 and len(res.get("data", [])) >= 4, f"{len(res.get('data', []))} 条")

audit_id = bad_audit.get("id")
res, code = call("GET", f"/admin/audits/{audit_id}", token=admin_token)
check("单次审核报告详情", code == 200 and res.get("data", {}).get("id") == audit_id)

# ---------- 7. 公开徽章 ----------
res, code = call("GET", "/skills/s-001/audit-badge")
badge = res.get("data", {})
check("可用性徽章 (公开)", code == 200 and badge.get("verified") is True,
      f"状态={badge.get('audit_status')} 分数={badge.get('audit_score')}")
res, code = call("GET", f"/skills/{bad_id}/audit-badge")
check("坏技能徽章不可信", res.get("data", {}).get("verified") is False,
      f"状态={res.get('data', {}).get('audit_status')}")

# ---------- 8. 安装门禁 ----------
res, code = call("POST", "/skills/s-001/install", {"version": ""}, dev_token)
install_ok = code == 200 and bool(res.get("api_token"))
if not install_ok and "已安装" in str(res.get("error", "")):
    install_ok = True  # 重复运行时已安装视为正常
check("已过审技能可安装", install_ok, res.get("error", "拿到安装 Token"))

# ---------- 9. 开发者自检 ----------
res, code = call("POST", f"/skills/{bad_id}/self-check", {}, dev_token)
check("开发者自检", code == 200 and res.get("data", {}).get("trigger_type") == "self_check",
      res.get("data", {}).get("summary", "")[:50])

res, code = call("POST", "/skills/s-001/self-check", {}, dev_token)
check("自检越权拦截 (非本人技能)", code == 403, str(res.get("error"))[:50])

# ---------- 10. 概览复核 ----------
res, _ = call("GET", "/admin/audit-overview", token=admin_token)
ov = res.get("data", {})
check("审核概览复核", ov.get("passed", 0) >= 4 and ov.get("failed", 0) >= 1,
      f"通过 {ov.get('passed')} / 不合格 {ov.get('failed')} / 待检测 {ov.get('pending')} / 平均分 {ov.get('avg_score')} / 通过率 {ov.get('pass_rate')}%")

print()
print(f"===== 结果: {PASS} 通过 / {FAIL} 失败 =====")
sys.exit(1 if FAIL else 0)
