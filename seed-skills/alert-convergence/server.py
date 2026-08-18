"""
系统运行告警收敛分析 — MCP Server
================================
功能: 当系统出现异常时, 对大量告警进行聚合收敛分析
      使用 AI 分析历史告警, 识别根因, 减少告警风暴
形态: MCP Server

工具:
  - analyze_alerts(host, time_range)     → 告警聚合分析
  - find_root_cause(alert_ids)           → 根因定位
  - generate_convergence_report(host)    → 生成收敛报告
"""

import json
import sys
from datetime import datetime, timedelta

# ============================================================
# 模拟告警数据
# ============================================================

MOCK_ALERTS = [
    {"id": "ALT-001", "host": "payment-node-01", "type": "CPU", "severity": "critical",
     "message": "CPU 使用率持续 > 95%, 持续 5 分钟", "timestamp": "2026-09-15 10:15:00", "service": "支付网关"},
    {"id": "ALT-002", "host": "payment-node-01", "type": "Memory", "severity": "warning",
     "message": "内存使用率 > 85%", "timestamp": "2026-09-15 10:15:30", "service": "支付网关"},
    {"id": "ALT-003", "host": "payment-node-01", "type": "DiskIO", "severity": "warning",
     "message": "磁盘 IO 等待 > 30%, 响应变慢", "timestamp": "2026-09-15 10:16:00", "service": "支付网关"},
    {"id": "ALT-004", "host": "payment-node-02", "type": "CPU", "severity": "warning",
     "message": "CPU 使用率 > 80%", "timestamp": "2026-09-15 10:16:10", "service": "支付网关"},
    {"id": "ALT-005", "host": "payment-node-03", "type": "CPU", "severity": "warning",
     "message": "CPU 使用率 > 80%", "timestamp": "2026-09-15 10:16:15", "service": "支付网关"},
    {"id": "ALT-006", "host": "payment-node-01", "type": "App", "severity": "critical",
     "message": "支付接口响应超时 > 5s, 错误率上升", "timestamp": "2026-09-15 10:18:00", "service": "支付网关"},
    {"id": "ALT-007", "host": "payment-node-01", "type": "Network", "severity": "warning",
     "message": "网络丢包率 > 1%", "timestamp": "2026-09-15 10:18:30", "service": "支付网关"},
    {"id": "ALT-008", "host": "core-db-01", "type": "DB", "severity": "critical",
     "message": "MySQL 连接数接近上限 (380/400)", "timestamp": "2026-09-15 10:17:00", "service": "核心数据库"},
    {"id": "ALT-009", "host": "core-db-01", "type": "DB", "severity": "critical",
     "message": "慢查询堆积 > 20 条, 最长 15s", "timestamp": "2026-09-15 10:17:30", "service": "核心数据库"},
    {"id": "ALT-010", "host": "core-db-01", "type": "DB", "severity": "warning",
     "message": "主从复制延迟 > 3s", "timestamp": "2026-09-15 10:18:00", "service": "核心数据库"},
    {"id": "ALT-011", "host": "redis-cluster-01", "type": "Cache", "severity": "critical",
     "message": "Redis 内存使用 > 95%", "timestamp": "2026-09-15 10:19:00", "service": "缓存集群"},
    {"id": "ALT-012", "host": "redis-cluster-01", "type": "Cache", "severity": "warning",
     "message": "Redis 响应延迟 > 100ms", "timestamp": "2026-09-15 10:19:30", "service": "缓存集群"},
    {"id": "ALT-013", "host": "app-server-05", "type": "App", "severity": "warning",
     "message": "GC 频繁 Full GC, 停顿 > 2s", "timestamp": "2026-09-15 08:30:00", "service": "贷款审批"},
    {"id": "ALT-014", "host": "app-server-05", "type": "Memory", "severity": "warning",
     "message": "堆内存使用 > 90%", "timestamp": "2026-09-15 08:30:30", "service": "贷款审批"},
    {"id": "ALT-015", "host": "network-switch-02", "type": "Network", "severity": "info",
     "message": "端口流量异常波动", "timestamp": "2026-09-15 09:00:00", "service": "网络设备"},
]


def analyze_and_converge(alerts: list) -> dict:
    """告警聚合分析: 按主机、类型、根因进行聚合收敛"""
    # 按主机分组
    by_host = {}
    for alert in alerts:
        host = alert["host"]
        if host not in by_host:
            by_host[host] = []
        by_host[host].append(alert)

    # 按类型分组
    by_type = {}
    for alert in alerts:
        t = alert["type"]
        if t not in by_type:
            by_type[t] = []
        by_type[t].append(alert)

    # 根因推断 (基于关联规则)
    root_causes = []

    # 规则: CPU + Memory + DiskIO + App 同时告警 → 流量冲击
    payment_cpu_mem = [a for a in alerts if a["host"].startswith("payment") and a["type"] in ["CPU","Memory"]]
    if len(payment_cpu_mem) >= 3:
        root_causes.append({
            "root_cause": "突发流量冲击",
            "confidence": 0.92,
            "affected_hosts": list(set(a["host"] for a in payment_cpu_mem)),
            "evidence": f"支付集群 {len(payment_cpu_mem)} 个节点同时出现 CPU/内存告警",
            "suggestion": "检查是否有促销活动导致流量暴增, 考虑临时扩容或限流"
        })

    # 规则: DB 连接数 + 慢查询 + 复制延迟 → 数据库性能瓶颈
    db_alerts = [a for a in alerts if a["host"].startswith("core-db")]
    if len(db_alerts) >= 2:
        root_causes.append({
            "root_cause": "数据库性能瓶颈 (连接池耗尽 → 慢查询 → 复制延迟)",
            "confidence": 0.88,
            "affected_hosts": list(set(a["host"] for a in db_alerts)),
            "evidence": f"核心数据库同时出现连接数/慢查询/复制延迟告警",
            "suggestion": "1. 紧急 kill 长时间运行的慢查询\n2. 分析慢查询 SQL 并添加索引\n3. 考虑读写分离或连接池扩容"
        })

    # 规则: Redis 内存 + 延迟 → 缓存雪崩风险
    redis_alerts = [a for a in alerts if a["host"].startswith("redis")]
    if len(redis_alerts) >= 2:
        root_causes.append({
            "root_cause": "缓存层压力过大, 可能引发缓存雪崩",
            "confidence": 0.85,
            "affected_hosts": list(set(a["host"] for a in redis_alerts)),
            "evidence": "Redis 内存接近上限且响应延迟增加",
            "suggestion": "1. 清理过期 Key\n2. 考虑缓存集群扩容\n3. 检查是否有大量 key 集中过期"
        })

    # 统计摘要
    severity_counts = {"critical": 0, "warning": 0, "info": 0}
    for alert in alerts:
        severity_counts[alert["severity"]] += 1

    return {
        "total_alerts": len(alerts),
        "unique_hosts": len(by_host),
        "alert_types": list(by_type.keys()),
        "severity_distribution": severity_counts,
        "by_host": {h: len(as_) for h, as_ in by_host.items()},
        "by_type": {t: len(as_) for t, as_ in by_type.items()},
        "root_causes": root_causes,
        "convergence_ratio": len(alerts) / max(len(root_causes), 1),  # 告警收敛比
    }


# ============================================================
# MCP Server
# ============================================================

def handle_request(request: dict) -> dict:
    method = request.get("method", "")
    req_id = request.get("id", 0)
    params = request.get("params", {})

    if method == "initialize":
        return {
            "jsonrpc": "2.0", "id": req_id,
            "result": {
                "protocolVersion": "2024-11-05",
                "serverInfo": {"name": "alert-convergence-analyzer", "version": "1.0.0"},
                "capabilities": {"tools": {}}
            }
        }

    elif method == "tools/list":
        return {
            "jsonrpc": "2.0", "id": req_id,
            "result": {"tools": [
                {
                    "name": "analyze_alerts",
                    "description": "分析指定主机和时间范围内的告警, 进行聚合收敛和根因推断",
                    "inputSchema": {
                        "type": "object",
                        "properties": {
                            "host": {"type": "string", "description": "主机名 (支持前缀匹配, 如 payment 匹配所有支付节点)"},
                            "time_range_minutes": {"type": "integer", "default": 60, "description": "时间范围 (分钟)"}
                        },
                        "required": ["host"]
                    }
                },
                {
                    "name": "find_root_cause",
                    "description": "对指定的告警 ID 列表进行根因分析",
                    "inputSchema": {
                        "type": "object",
                        "properties": {
                            "alert_ids": {"type": "array", "items": {"type": "string"}}
                        },
                        "required": ["alert_ids"]
                    }
                },
                {
                    "name": "generate_report",
                    "description": "生成告警收敛分析报告",
                    "inputSchema": {
                        "type": "object",
                        "properties": {
                            "host_filter": {"type": "string", "default": ""}
                        }
                    }
                },
            ]}
        }

    elif method == "tools/call":
        tool_name = params.get("name", "")
        args = params.get("arguments", {})

        if tool_name == "analyze_alerts":
            host = args.get("host", "")
            time_range = args.get("time_range_minutes", 60)
            # 过滤告警
            filtered = [a for a in MOCK_ALERTS if host in a["host"]]
            if not filtered:
                filtered = MOCK_ALERTS  # 回退: 返回全部
            result = analyze_and_converge(filtered)

            text = f"## 📊 告警收敛分析 — {host}\n\n"
            text += f"**时间范围**: 最近 {time_range} 分钟\n"
            text += f"**原始告警**: {result['total_alerts']} 条\n"
            text += f"**收敛后**: {len(result['root_causes'])} 个根因\n"
            text += f"**收敛比**: {result['convergence_ratio']:.1f}:1\n\n"

            text += "### 🎯 根因推断\n\n"
            for rc in result["root_causes"]:
                text += f"#### {rc['root_cause']} (置信度: {rc['confidence']:.0%})\n"
                text += f"- 影响主机: {', '.join(rc['affected_hosts'])}\n"
                text += f"- 证据: {rc['evidence']}\n"
                text += f"- 建议: {rc['suggestion']}\n\n"

            return {
                "jsonrpc": "2.0", "id": req_id,
                "result": {"content": [{"type": "text", "text": text}], "data": result}
            }

        elif tool_name == "find_root_cause":
            ids = args.get("alert_ids", [])
            matched = [a for a in MOCK_ALERTS if a["id"] in ids]
            result = analyze_and_converge(matched)

            text = f"## 🔍 根因分析结果\n\n"
            for rc in result["root_causes"]:
                text += f"### {rc['root_cause']}\n"
                text += f"**置信度**: {rc['confidence']:.0%} | **建议**: {rc['suggestion']}\n\n"

            return {
                "jsonrpc": "2.0", "id": req_id,
                "result": {"content": [{"type": "text", "text": text}], "data": result}
            }

        elif tool_name == "generate_report":
            host = args.get("host_filter", "")
            filtered = [a for a in MOCK_ALERTS if not host or host in a["host"]]
            result = analyze_and_converge(filtered)

            text = f"## 🏥 系统告警收敛报告\n\n"
            text += f"**生成时间**: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n\n"
            text += f"### 告警概览\n"
            text += f"| 级别 | 数量 |\n|------|------|\n"
            for sev, cnt in result["severity_distribution"].items():
                emoji = "🔴" if sev == "critical" else "🟡" if sev == "warning" else "ℹ️"
                text += f"| {emoji} {sev} | {cnt} |\n"

            text += f"\n### 受影响主机 TOP 5\n"
            for host_name, cnt in sorted(result["by_host"].items(), key=lambda x: x[1], reverse=True)[:5]:
                text += f"- {host_name}: {cnt} 条告警\n"

            text += f"\n### 根因分析 ({len(result['root_causes'])} 个)\n"
            for rc in result["root_causes"]:
                text += f"- **{rc['root_cause']}** → {rc['suggestion']}\n"

            return {
                "jsonrpc": "2.0", "id": req_id,
                "result": {"content": [{"type": "text", "text": text}], "data": result}
            }

    return error_response(req_id, "未知方法")


def error_response(req_id, msg):
    return {"jsonrpc": "2.0", "id": req_id, "error": {"code": -1, "message": msg}}

def main():
    print("🚨 告警收敛分析 MCP Server v1.0.0 启动", file=sys.stderr)
    for line in sys.stdin:
        line = line.strip()
        if not line: continue
        try:
            request = json.loads(line)
            response = handle_request(request)
            print(json.dumps(response, ensure_ascii=False))
            sys.stdout.flush()
        except Exception as e:
            print(json.dumps(error_response(0, str(e)), ensure_ascii=False))
            sys.stdout.flush()

if __name__ == "__main__":
    main()
