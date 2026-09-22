# -*- coding: utf-8 -*-
"""
SkillHub · 多厂商大模型接入 一键演示
展示: 厂商列表与配置状态 · 当前激活厂商 · 真实对话(指定厂商) · 调用统计
用法: py demo_llm.py
"""
import json
import urllib.request

BASE = "http://localhost:8080/api/v1"


def call(method, path, body=None, auth=None, timeout=200):
    data = None
    if body is not None:
        data = json.dumps(body, ensure_ascii=False).encode("utf-8")
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header("Content-Type", "application/json; charset=utf-8")
    if auth:
        req.add_header("Authorization", "Bearer " + auth)
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        return json.loads(resp.read().decode("utf-8"))


def line(title):
    print("\n" + "=" * 60)
    print(title)
    print("=" * 60)


def main():
    auth = call("POST", "/auth/login", {"username": "admin", "password": "demo"})["token"]

    line("① 大模型厂商列表 (/agent/providers)")
    res = call("GET", "/agent/providers", None, auth)
    for p in res["data"]:
        flag = "[OK] configured" if p["configured"] else "[--] not configured"
        print("  %-9s %-22s %-10s %-26s %s" % (p["key"], p["label"], p["kind"], p["model"], flag))
    summary = res["summary"]
    active = summary.get("active") or {}
    print("\n  已配置: %s / %s ; 当前激活: %s (%s)" % (
        summary["configured_count"], summary["count"], active.get("label", "无"), active.get("model", "-")))

    if summary["configured_count"] == 0:
        print("\n  (未配置任何 Key → Agent 会降级为本地意图引擎; 设置 DEEPSEEK_API_KEY 等即可)")
        return

    line("② 真实对话 · 指定厂商")
    target = active.get("key")
    ans = call("POST", "/agent/chat", {"message": "帮我做一次数据库巡检", "provider": target}, auth)["data"]
    print("  提问: 帮我做一次数据库巡检")
    print("  引擎: %s / %s (fallback=%s)" % (ans["provider"], ans.get("active_provider"), ans["fallback"]))
    for s in ans.get("steps", []):
        print("    - 调用 %s/%s -> %s (%sms)" % (s.get("skill_key"), s.get("tool_name"), s.get("status"), s.get("duration_ms")))
    print("  回答(节选): " + ans["answer"][:180].replace("\n", " "))

    line("③ 调用统计 (/agent/status.provider_stats)")
    ps = call("GET", "/agent/status", None, auth)["data"]["provider_stats"]
    print("  总调用 %s 次, 失败 %s 次" % (ps["total_calls"], ps["total_failures"]))
    for p in ps["providers"]:
        print("    %-9s calls=%s ok=%s fail=%s rate=%s%% avg=%sms" % (
            p["provider"], p["calls"], p["success"], p["failures"], p["success_rate"], p["avg_ms"]))

    print("\n  提示: 任一厂商调用失败时会自动切换候选链中的下一家 (故障切换)。")


if __name__ == "__main__":
    main()
