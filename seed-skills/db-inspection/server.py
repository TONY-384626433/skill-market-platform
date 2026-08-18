"""
数据库智能巡检助手 — MCP Server
================================
功能: 对接 Zabbix 监控 + CMDB 配置库, 自动执行数据库健康检查
形态: MCP Server (Model Context Protocol)
安装: 注册到 Skill 市场后, 用户通过 MCP 协议调用

使用方式:
  python server.py

MCP 工具列表:
  - inspect_database(target_db, check_scope)  → 执行数据库健康巡检
  - get_slow_queries(target_db, limit)         → 获取慢查询列表
  - get_connection_stats(target_db)            → 获取连接数统计
"""

import json
import random
import sys
from datetime import datetime

# ============================================================
# 模拟数据 (实际部署对接 Zabbix API + CMDB API)
# ============================================================

MOCK_DATABASES = {
    "core-banking-db-01": {
        "host": "10.10.1.101",
        "port": 3306,
        "type": "MySQL",
        "version": "8.0.35",
        "cluster": "核心银行系统",
        "owner": "核心银行团队",
        "status": "running",
        "uptime_days": 123,
        "metrics": {
            "cpu_usage": 45.2,
            "memory_usage": 62.8,
            "disk_usage": 72.1,
            "connection_count": 156,
            "max_connections": 400,
            "slow_queries_today": 3,
            "qps": 2340,
            "replication_lag_sec": 0.5,
        }
    },
    "payment-db-02": {
        "host": "10.10.2.50",
        "port": 3306,
        "type": "MySQL",
        "version": "8.0.33",
        "cluster": "支付系统",
        "owner": "支付平台团队",
        "status": "running",
        "uptime_days": 45,
        "metrics": {
            "cpu_usage": 78.5,
            "memory_usage": 85.1,
            "disk_usage": 58.3,
            "connection_count": 289,
            "max_connections": 500,
            "slow_queries_today": 12,
            "qps": 5600,
            "replication_lag_sec": 2.3,
        }
    },
    "risk-control-db-01": {
        "host": "10.10.3.30",
        "port": 5432,
        "type": "PostgreSQL",
        "version": "15.4",
        "cluster": "风控系统",
        "owner": "风控平台团队",
        "status": "running",
        "uptime_days": 200,
        "metrics": {
            "cpu_usage": 32.1,
            "memory_usage": 45.6,
            "disk_usage": 35.2,
            "connection_count": 89,
            "max_connections": 300,
            "slow_queries_today": 0,
            "qps": 890,
            "replication_lag_sec": 0.1,
        }
    },
}

MOCK_SLOW_QUERIES = [
    {
        "sql": "SELECT * FROM transactions WHERE create_time < '2026-01-01' ORDER BY id",
        "duration_sec": 8.5,
        "rows_examined": 2500000,
        "database": "core_banking",
        "first_seen": "2026-09-15 02:15:30",
    },
    {
        "sql": "SELECT t.*, u.name FROM transactions t JOIN users u ON t.user_id = u.id WHERE t.status = 'PENDING'",
        "duration_sec": 5.2,
        "rows_examined": 1200000,
        "database": "payment",
        "first_seen": "2026-09-15 09:42:10",
    },
    {
        "sql": "SELECT COUNT(*) FROM audit_logs WHERE created_at BETWEEN '2026-01-01' AND '2026-12-31'",
        "duration_sec": 12.1,
        "rows_examined": 50000000,
        "database": "audit",
        "first_seen": "2026-09-15 14:00:05",
    },
]

# ============================================================
# MCP Server 实现
# ============================================================

def handle_request(request: dict) -> dict:
    """处理 MCP 协议请求"""
    method = request.get("method", "")
    req_id = request.get("id", 0)
    params = request.get("params", {})

    if method == "initialize":
        return {
            "jsonrpc": "2.0",
            "id": req_id,
            "result": {
                "protocolVersion": "2024-11-05",
                "serverInfo": {
                    "name": "db-inspection-assistant",
                    "version": "1.2.0"
                },
                "capabilities": {
                    "tools": {}
                }
            }
        }

    elif method == "tools/list":
        return {
            "jsonrpc": "2.0",
            "id": req_id,
            "result": {
                "tools": [
                    {
                        "name": "inspect_database",
                        "description": "执行数据库健康巡检, 检查 CPU/内存/磁盘/连接数/慢查询/复制延迟等指标, 生成巡检报告",
                        "inputSchema": {
                            "type": "object",
                            "properties": {
                                "target_db": {
                                    "type": "string",
                                    "description": "目标数据库实例名称, 如 core-banking-db-01"
                                },
                                "check_scope": {
                                    "type": "string",
                                    "enum": ["full", "performance", "security", "capacity"],
                                    "default": "full",
                                    "description": "巡检范围: full=全面检查, performance=性能, security=安全, capacity=容量"
                                }
                            },
                            "required": ["target_db"]
                        }
                    },
                    {
                        "name": "get_slow_queries",
                        "description": "获取数据库慢查询列表",
                        "inputSchema": {
                            "type": "object",
                            "properties": {
                                "target_db": {"type": "string"},
                                "limit": {"type": "integer", "default": 10}
                            },
                            "required": ["target_db"]
                        }
                    },
                    {
                        "name": "get_connection_stats",
                        "description": "获取数据库连接数统计信息",
                        "inputSchema": {
                            "type": "object",
                            "properties": {
                                "target_db": {"type": "string"}
                            },
                            "required": ["target_db"]
                        }
                    },
                ]
            }
        }

    elif method == "tools/call":
        tool_name = params.get("name", "")
        arguments = params.get("arguments", {})

        if tool_name == "inspect_database":
            return handle_inspect(arguments, req_id)
        elif tool_name == "get_slow_queries":
            return handle_slow_queries(arguments, req_id)
        elif tool_name == "get_connection_stats":
            return handle_connection_stats(arguments, req_id)
        else:
            return error_response(req_id, f"未知工具: {tool_name}")

    else:
        return error_response(req_id, f"未知方法: {method}")


def handle_inspect(args: dict, req_id) -> dict:
    target = args.get("target_db", "")
    scope = args.get("check_scope", "full")

    db = MOCK_DATABASES.get(target)
    if not db:
        return error_response(req_id, f"数据库实例不存在: {target}\n可用实例: {list(MOCK_DATABASES.keys())}")

    m = db["metrics"]
    issues = []
    score = 100

    # CPU 检查
    if m["cpu_usage"] > 80:
        issues.append({"severity": "critical", "component": "CPU", "detail": f"CPU 使用率 {m['cpu_usage']}%, 超过 80% 阈值", "suggestion": "建议分析高 CPU 查询并考虑扩容"})
        score -= 20
    elif m["cpu_usage"] > 60:
        issues.append({"severity": "warning", "component": "CPU", "detail": f"CPU 使用率 {m['cpu_usage']}%, 偏高", "suggestion": "关注 CPU 趋势"})
        score -= 5

    # 内存检查
    if m["memory_usage"] > 85:
        issues.append({"severity": "critical", "component": "内存", "detail": f"内存使用率 {m['memory_usage']}%, 即将耗尽", "suggestion": "检查内存泄漏, 考虑扩容"})
        score -= 25
    elif m["memory_usage"] > 70:
        issues.append({"severity": "warning", "component": "内存", "detail": f"内存使用率 {m['memory_usage']}%, 偏高"})
        score -= 10

    # 磁盘检查
    if m["disk_usage"] > 85:
        issues.append({"severity": "critical", "component": "磁盘", "detail": f"磁盘使用率 {m['disk_usage']}%, 严重不足"})
        score -= 25
    elif m["disk_usage"] > 70:
        issues.append({"severity": "warning", "component": "磁盘", "detail": f"磁盘使用率 {m['disk_usage']}%, 需要关注"})
        score -= 10

    # 连接数检查
    conn_ratio = m["connection_count"] / m["max_connections"] * 100
    if conn_ratio > 80:
        issues.append({"severity": "critical", "component": "连接池", "detail": f"连接数 {m['connection_count']}/{m['max_connections']} ({conn_ratio:.0f}%), 即将耗尽"})
        score -= 20
    elif conn_ratio > 60:
        issues.append({"severity": "warning", "component": "连接池", "detail": f"连接数使用率 {conn_ratio:.0f}%, 偏高"})
        score -= 5

    # 慢查询检查
    if m["slow_queries_today"] > 10:
        issues.append({"severity": "warning", "component": "慢查询", "detail": f"今日慢查询 {m['slow_queries_today']} 条, 建议优化"})
        score -= 15
    elif m["slow_queries_today"] > 0:
        issues.append({"severity": "info", "component": "慢查询", "detail": f"今日慢查询 {m['slow_queries_today']} 条"})

    # 复制延迟
    if m["replication_lag_sec"] > 5:
        issues.append({"severity": "critical", "component": "主从复制", "detail": f"复制延迟 {m['replication_lag_sec']}s, 严重滞后"})
        score -= 20
    elif m["replication_lag_sec"] > 1:
        issues.append({"severity": "warning", "component": "主从复制", "detail": f"复制延迟 {m['replication_lag_sec']}s, 偏高"})
        score -= 5

    # 生成报告
    report = f"""## 🏥 数据库巡检报告

**实例**: {target} ({db['type']} {db['version']})
**集群**: {db['cluster']}
**巡检时间**: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}
**巡检范围**: {scope}

---

### 📊 健康评分: **{max(0, score)}/100**

| 指标 | 当前值 | 状态 |
|------|--------|------|
| CPU 使用率 | {m['cpu_usage']}% | {'🟢 正常' if m['cpu_usage'] < 60 else '🟡 关注' if m['cpu_usage'] < 80 else '🔴 告警'} |
| 内存使用率 | {m['memory_usage']}% | {'🟢 正常' if m['memory_usage'] < 70 else '🟡 关注' if m['memory_usage'] < 85 else '🔴 告警'} |
| 磁盘使用率 | {m['disk_usage']}% | {'🟢 正常' if m['disk_usage'] < 70 else '🟡 关注' if m['disk_usage'] < 85 else '🔴 告警'} |
| 连接数 | {m['connection_count']}/{m['max_connections']} ({conn_ratio:.0f}%) | {'🟢 正常' if conn_ratio < 60 else '🟡 关注' if conn_ratio < 80 else '🔴 告警'} |
| QPS | {m['qps']} | - |
| 复制延迟 | {m['replication_lag_sec']}s | {'🟢 正常' if m['replication_lag_sec'] < 1 else '🟡 关注'} |
| 运行天数 | {db['uptime_days']} 天 | - |

"""

    if issues:
        report += "\n### ⚠️ 发现问题\n\n"
        for issue in sorted(issues, key=lambda x: {"critical": 0, "warning": 1, "info": 2}[x["severity"]]):
            emoji = "🔴" if issue["severity"] == "critical" else "🟡" if issue["severity"] == "warning" else "ℹ️"
            report += f"- {emoji} **{issue['component']}**: {issue['detail']}\n"
            report += f"  💡 {issue['suggestion']}\n\n"

    if not issues:
        report += "\n### ✅ 所有指标正常, 数据库运行健康!\n"

    return {
        "jsonrpc": "2.0",
        "id": req_id,
        "result": {
            "content": [{"type": "text", "text": report}],
            "data": {
                "score": max(0, score),
                "issues": issues,
                "metrics": m,
                "db_info": {k: v for k, v in db.items() if k != "metrics"}
            }
        }
    }


def handle_slow_queries(args: dict, req_id) -> dict:
    target = args.get("target_db", "")
    limit = args.get("limit", 10)

    db = MOCK_DATABASES.get(target)
    if not db:
        return error_response(req_id, f"数据库实例不存在: {target}")

    queries = MOCK_SLOW_QUERIES[:limit]

    text = f"## 🐢 慢查询列表 — {target}\n\n"
    for i, q in enumerate(queries, 1):
        text += f"### {i}. 耗时 {q['duration_sec']}s\n"
        text += f"```sql\n{q['sql']}\n```\n"
        text += f"- 扫描行数: {q['rows_examined']:,}\n"
        text += f"- 数据库: {q['database']}\n"
        text += f"- 首次发现: {q['first_seen']}\n\n"

    return {
        "jsonrpc": "2.0",
        "id": req_id,
        "result": {"content": [{"type": "text", "text": text}]}
    }


def handle_connection_stats(args: dict, req_id) -> dict:
    target = args.get("target_db", "")
    db = MOCK_DATABASES.get(target)
    if not db:
        return error_response(req_id, f"数据库实例不存在: {target}")

    m = db["metrics"]
    text = f"""## 🔌 连接数统计 — {target}

| 指标 | 值 |
|------|-----|
| 当前连接数 | {m['connection_count']} |
| 最大连接数 | {m['max_connections']} |
| 使用率 | {m['connection_count']/m['max_connections']*100:.1f}% |
| 活跃连接 | {int(m['connection_count']*0.6)} |
| 空闲连接 | {int(m['connection_count']*0.3)} |
| 等待连接 | {int(m['connection_count']*0.1)} |
"""

    return {
        "jsonrpc": "2.0",
        "id": req_id,
        "result": {"content": [{"type": "text", "text": text}]}
    }


def error_response(req_id, message: str) -> dict:
    return {
        "jsonrpc": "2.0",
        "id": req_id,
        "error": {"code": -1, "message": message}
    }


# ============================================================
# STDIO 模式主循环
# ============================================================

def main():
    """MCP Server STDIO 主循环"""
    print("🔌 数据库智能巡检助手 MCP Server v1.2.0 启动", file=sys.stderr)
    print(f"   可用实例: {list(MOCK_DATABASES.keys())}", file=sys.stderr)

    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            request = json.loads(line)
            response = handle_request(request)
            print(json.dumps(response, ensure_ascii=False))
            sys.stdout.flush()
        except json.JSONDecodeError as e:
            print(json.dumps(error_response(0, f"JSON 解析错误: {e}"), ensure_ascii=False))
            sys.stdout.flush()
        except Exception as e:
            print(json.dumps(error_response(0, f"内部错误: {e}"), ensure_ascii=False))
            sys.stdout.flush()


if __name__ == "__main__":
    main()
