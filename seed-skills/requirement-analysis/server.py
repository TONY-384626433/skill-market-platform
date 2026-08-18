"""
AI辅助需求分析助手 — MCP Server
===============================
功能: 对需求概述开展现状分析、价值及业务规则细化分析,
      输出规范化的需求文档
形态: MCP Server

工具:
  - analyze_requirement(title, description)  → 需求结构化分析
  - generate_prd(title, description)        → 生成规范化 PRD 文档
  - analyze_business_rules(scenario)         → 业务规则提取
"""

import json
import sys
from datetime import datetime

# ============================================================
# 需求分析模板
# ============================================================

REQUIREMENT_ANALYSIS_PROMPT = """
你是一位资深银行 IT 需求分析师。请对以下需求进行结构化分析:

需求标题: {title}
需求描述: {description}

请按以下维度输出分析结果:
1. 业务背景与现状
2. 核心价值 (对银行/对客户/对监管)
3. 关键业务规则
4. 涉及系统与接口
5. 风险点与边界条件
6. 优先级建议
"""


def analyze_requirement(title: str, description: str) -> dict:
    """基于规则 + 模板的需求分析"""

    # 关键词提取
    keywords = []
    bank_terms = ["贷款","存款","支付","转账","风控","征信","授信","审批",
                  "柜面","网银","手机银行","监管","合规","反洗钱","KYC","AML",
                  "对公","零售","同业","票据","信用证","外汇","利率","汇率"]
    for term in bank_terms:
        if term in title or term in description:
            keywords.append(term)

    # 涉及系统推断
    systems = []
    system_map = {
        "贷款": ["信贷管理系统", "核心银行系统", "风控系统", "征信查询系统"],
        "支付": ["支付网关", "核心银行系统", "反洗钱系统", "渠道接入平台"],
        "存款": ["核心银行系统", "利率管理系统", "柜面系统"],
        "风控": ["风控引擎", "实时计算平台", "规则管理平台"],
        "征信": ["征信查询系统", "人行征信接口", "数据仓库"],
        "审批": ["工作流引擎", "OA 系统", "影像平台"],
        "反洗钱": ["反洗钱系统", "交易监控平台", "可疑交易报告系统"],
    }
    for term, sys_list in system_map.items():
        if term in title or term in description:
            systems.extend(sys_list)

    # 业务规则推断
    business_rules = []
    if "贷款" in title or "贷款" in description:
        business_rules = [
            "借款人须满足 KYC 要求",
            "贷款金额不超过授信额度",
            "须通过风控模型评分 ≥ 阈值",
            "利率须在央行基准利率 ± 浮动范围内",
            "须满足监管合规要求 (LPR 定价)",
        ]
    elif "支付" in title or "支付" in description:
        business_rules = [
            "单笔限额: 个人 ≤ 5万, 企业 ≤ 100万 (可配置)",
            "日累计限额: 个人 ≤ 20万",
            "大额交易需实时上报反洗钱系统",
            "高风险交易需触发二次验证",
        ]
    elif "账户" in title or "账户" in description:
        business_rules = [
            "个人账户分 I/II/III 类, 权限逐级递减",
            "企业账户需双人复核",
            "账户状态变更需记录审计日志",
        ]

    # 风险分析
    risks = []
    if "贷款" in title or "贷款" in description:
        risks = [
            {"risk": "信用风险", "level": "high", "desc": "借款人违约导致资产损失"},
            {"risk": "操作风险", "level": "medium", "desc": "审批流程操作失误"},
            {"risk": "合规风险", "level": "medium", "desc": "监管政策变化影响产品合规性"},
        ]
    elif "支付" in title or "支付" in description:
        risks = [
            {"risk": "资金风险", "level": "high", "desc": "资金错误/重复划拨"},
            {"risk": "欺诈风险", "level": "high", "desc": "支付欺诈攻击"},
            {"risk": "系统风险", "level": "medium", "desc": "大流量下系统稳定性"},
        ]

    return {
        "title": title,
        "analysis_time": datetime.now().strftime('%Y-%m-%d %H:%M:%S'),
        "keywords": list(set(keywords)),
        "business_domain": keywords[0] if keywords else "通用",
        "related_systems": list(set(systems)),
        "business_rules": business_rules,
        "risks": risks,
        "complexity": "高" if len(keywords) >= 3 else "中" if len(keywords) >= 1 else "低",
        "suggested_priority": "P0" if any(k in ["监管","合规","反洗钱"] for k in keywords) else "P1",
    }


def generate_prd_content(title: str, description: str, analysis: dict) -> str:
    """生成规范化 PRD 文档"""
    now = datetime.now().strftime('%Y-%m-%d')

    prd = f"""# {title} — 产品需求文档 (PRD)

| 字段 | 内容 |
|------|------|
| 文档版本 | v1.0 |
| 创建日期 | {now} |
| 业务领域 | {analysis['business_domain']} |
| 优先级 | {analysis['suggested_priority']} |
| 复杂度 | {analysis['complexity']} |

---

## 1. 需求概述

### 1.1 背景
{description[:200]}...

### 1.2 核心价值

- **客户价值**: 提升用户体验, 简化操作流程
- **银行价值**: 提高运营效率, 降低人工成本, 控制风险
- **监管价值**: 满足合规要求, 提升数据质量

---

## 2. 业务规则

"""
    for i, rule in enumerate(analysis["business_rules"], 1):
        prd += f"{i}. {rule}\n"

    prd += f"""
---

## 3. 涉及系统

"""
    for sys_name in analysis["related_systems"]:
        prd += f"- **{sys_name}**: 接口对接 / 数据交互\n"

    prd += f"""
---

## 4. 风险分析

| 风险类型 | 风险等级 | 描述 | 应对措施 |
|----------|----------|------|----------|
"""
    for risk in analysis["risks"]:
        level_emoji = "🔴" if risk["level"] == "high" else "🟡" if risk["level"] == "medium" else "🟢"
        prd += f"| {risk['risk']} | {level_emoji} {risk['level']} | {risk['desc']} | 待制定 |\n"

    prd += f"""
---

## 5. 验收标准

- [ ] 功能完整性: 所有业务规则正确实现
- [ ] 性能要求: 响应时间 < 3s (P95)
- [ ] 安全要求: 敏感数据加密传输, 操作全程审计
- [ ] 兼容性: 支持 Chrome/Firefox/Edge 最新版本

---

## 6. 附录

- 关联需求: (待补充)
- 参考资料: (待补充)
- 术语表: {', '.join(analysis['keywords'])}

---

> 本文档由 AI 辅助需求分析助手自动生成, 供需求评审使用。
"""
    return prd


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
                "serverInfo": {"name": "requirement-analysis-assistant", "version": "1.0.0"},
                "capabilities": {"tools": {}}
            }
        }

    elif method == "tools/list":
        return {
            "jsonrpc": "2.0", "id": req_id,
            "result": {"tools": [
                {
                    "name": "analyze_requirement",
                    "description": "对需求概述进行结构化分析, 输出业务规则、涉及系统、风险点等",
                    "inputSchema": {
                        "type": "object",
                        "properties": {
                            "title": {"type": "string", "description": "需求标题"},
                            "description": {"type": "string", "description": "需求描述 (可包含业务背景、目标等)"}
                        },
                        "required": ["title", "description"]
                    }
                },
                {
                    "name": "generate_prd",
                    "description": "生成规范化的产品需求文档 (PRD)",
                    "inputSchema": {
                        "type": "object",
                        "properties": {
                            "title": {"type": "string"},
                            "description": {"type": "string"}
                        },
                        "required": ["title", "description"]
                    }
                },
                {
                    "name": "analyze_business_rules",
                    "description": "从业务场景描述中提取关键业务规则",
                    "inputSchema": {
                        "type": "object",
                        "properties": {
                            "scenario": {"type": "string", "description": "业务场景描述"}
                        },
                        "required": ["scenario"]
                    }
                },
            ]}
        }

    elif method == "tools/call":
        tool_name = params.get("name", "")
        args = params.get("arguments", {})

        if tool_name == "analyze_requirement":
            title = args.get("title", "")
            description = args.get("description", "")
            analysis = analyze_requirement(title, description)

            text = f"## 📋 需求分析结果\n\n"
            text += f"**需求**: {title}\n"
            text += f"**业务领域**: {analysis['business_domain']}\n"
            text += f"**复杂度**: {analysis['complexity']} | **建议优先级**: {analysis['suggested_priority']}\n\n"
            text += f"### 关键词\n{', '.join(analysis['keywords'])}\n\n"
            text += f"### 涉及系统\n" + "\n".join(f"- {s}" for s in analysis['related_systems']) + "\n\n"
            text += f"### 业务规则\n" + "\n".join(f"{i}. {r}" for i,r in enumerate(analysis['business_rules'],1))

            return {
                "jsonrpc": "2.0", "id": req_id,
                "result": {"content": [{"type": "text", "text": text}], "data": analysis}
            }

        elif tool_name == "generate_prd":
            title = args.get("title", "")
            description = args.get("description", "")
            analysis = analyze_requirement(title, description)
            prd = generate_prd_content(title, description, analysis)

            return {
                "jsonrpc": "2.0", "id": req_id,
                "result": {
                    "content": [{"type": "text", "text": prd}],
                    "data": {"prd": prd, "analysis": analysis}
                }
            }

        elif tool_name == "analyze_business_rules":
            scenario = args.get("scenario", "")
            analysis = analyze_requirement("场景分析", scenario)

            text = f"## 📐 业务规则提取\n\n"
            for i, rule in enumerate(analysis['business_rules'], 1):
                text += f"{i}. {rule}\n"
            text += f"\n### 风险点\n"
            for risk in analysis['risks']:
                text += f"- {risk['risk']} ({risk['level']}): {risk['desc']}\n"

            return {
                "jsonrpc": "2.0", "id": req_id,
                "result": {"content": [{"type": "text", "text": text}], "data": analysis}
            }

    return error_response(req_id, "未知方法")


def error_response(req_id, msg):
    return {"jsonrpc": "2.0", "id": req_id, "error": {"code": -1, "message": msg}}

def main():
    print("📝 AI 辅助需求分析助手 MCP Server v1.0.0 启动", file=sys.stderr)
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
