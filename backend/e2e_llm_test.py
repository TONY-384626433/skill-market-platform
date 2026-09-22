# -*- coding: utf-8 -*-
"""
SkillHub · 多厂商大模型接入 e2e 测试
覆盖: /agent/providers 厂商列表与配置状态 · /agent/status 统计 · /agent/chat 指定/缺省厂商
用法: py e2e_llm_test.py   (需后端 8080 在跑)
"""
import json
import sys
import urllib.error
import urllib.request

BASE = "http://localhost:8080/api/v1"
PASS = 0
FAIL = 0


def check(name, cond, detail=""):
    global PASS, FAIL
    if cond:
        PASS += 1
        print("[PASS] %s | %s" % (name, detail))
    else:
        FAIL += 1
        print("[FAIL] %s | %s" % (name, detail))


def call(method, path, body=None, auth=None, timeout=200):
    data = None
    if body is not None:
        data = json.dumps(body, ensure_ascii=False).encode("utf-8")
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header("Content-Type", "application/json; charset=utf-8")
    if auth:
        req.add_header("Authorization", "Bearer " + auth)
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        return resp.status, json.loads(resp.read().decode("utf-8"))


def call_status(method, path, body=None, auth=None, timeout=60):
    """返回 (status_code, json_or_text) 不抛异常"""
    data = None
    if body is not None:
        data = json.dumps(body, ensure_ascii=False).encode("utf-8")
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header("Content-Type", "application/json; charset=utf-8")
    if auth:
        req.add_header("Authorization", "Bearer " + auth)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8")
            try:
                return resp.status, json.loads(raw)
            except Exception:
                return resp.status, raw
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8")
        try:
            return e.code, json.loads(raw)
        except Exception:
            return e.code, raw


def main():
    # --- 登录 ---
    _s, login = call("POST", "/auth/login", {"username": "admin", "password": "demo"})
    auth = login["token"]
    check("登录 admin/demo 拿到 token", bool(auth))

    # --- 厂商列表 ---
    s, res = call("GET", "/agent/providers", None, auth)
    data = res.get("data", [])
    keys = {p["key"] for p in data}
    expect = {"custom", "deepseek", "kimi", "openai", "claude", "ark", "qwen", "zhipu"}
    check("厂商列表返回 8 家", len(keys) >= 8 and expect.issubset(keys), "keys=%s" % sorted(keys))
    claude = next((p for p in data if p["key"] == "claude"), None)
    check("Claude 走 anthropic 协议", claude and claude["kind"] == "anthropic", "kind=%s" % (claude and claude["kind"]))
    deepseek = next((p for p in data if p["key"] == "deepseek"), None)
    check("DeepSeek 走 openai 兼容", deepseek and deepseek["kind"] == "openai")
    check("至少有一家已配置", any(p["configured"] for p in data),
          "configured=%s" % [p["key"] for p in data if p["configured"]])

    # --- 状态 / 统计 ---
    s, st = call("GET", "/agent/status", None, auth)
    d = st.get("data", {})
    check("status 有 provider_stats", "provider_stats" in d)
    check("status 有 providers 明细", "providers" in d and len(d["providers"]) >= 8)
    if any(p["configured"] for p in data):
        check("已配置厂商 → mode=llm", d.get("mode") == "llm", "mode=%s active=%s" % (d.get("mode"), d.get("active_provider")))

    # --- 对话: 指定厂商 ---
    configured = [p for p in data if p["configured"]]
    if configured:
        target = configured[0]["key"]
        s, r = call("POST", "/agent/chat", {"message": "帮我做一次数据库巡检", "provider": target}, auth)
        a = r.get("data", {})
        check("指定厂商对话返回 llm", a.get("provider") == "llm", "provider=%s active=%s fallback=%s" % (a.get("provider"), a.get("active_provider"), a.get("fallback")))
        check("返回 answer 非空", bool(a.get("answer")))
        check("active_provider 已回填", bool(a.get("active_provider")), "active=%s" % a.get("active_provider"))

        # --- 对话: 缺省厂商(自动择优) ---
        s, r2 = call("POST", "/agent/chat", {"message": "把这段日志脱敏"}, auth)
        a2 = r2.get("data", {})
        check("缺省对话也能成功", a2.get("provider") in ("llm", "local-intent"), "provider=%s" % a2.get("provider"))

        # --- 对话: 不存在的厂商 → 应回退到已配置的, 不报错 ---
        s, r3 = call("POST", "/agent/chat", {"message": "你好", "provider": "no-such-provider"}, auth)
        a3 = r3.get("data", {})
        check("未知厂商自动回退(不报错)", s == 200 and a3.get("provider") in ("llm", "local-intent"),
              "status=%s provider=%s" % (s, a3.get("provider")))

        # --- 统计累加 ---
        s, st2 = call("GET", "/agent/status", None, auth)
        ps = st2["data"]["provider_stats"]
        check("provider_stats 有调用记录", ps["total_calls"] >= 1,
              "total_calls=%s failures=%s" % (ps["total_calls"], ps["total_failures"]))
        top = ps["providers"][0] if ps["providers"] else {}
        check("统计含成功率与耗时", "success_rate" in top and "avg_ms" in top,
              "%s rate=%s%% avg=%sms" % (top.get("provider"), top.get("success_rate"), top.get("avg_ms")))
    else:
        print("[SKIP] 无已配置厂商, 跳过对话类断言 (设置 DEEPSEEK_API_KEY 等即可)")

    print("")
    print("========== 多厂商 e2e: %d 通过 / %d 失败 ==========" % (PASS, FAIL))
    sys.exit(1 if FAIL else 0)


if __name__ == "__main__":
    main()
