"""
日志敏感信息识别 — MCP Server
==============================
功能: 面向应用日志的敏感信息智能识别与脱敏处理
      融合正则匹配 + 上下文语义推理, 精准识别人名、身份证号、手机号、银行卡号等
形态: MCP Server (Model Context Protocol)

工具:
  - scan_log(log_content)    → 扫描日志中的敏感信息
  - desensitize(log_content) → 脱敏处理 (返回脱敏后文本)
  - detect_pii(text)         → 检测文本中的 PII 实体
"""

import json
import re
import sys
from datetime import datetime

# ============================================================
# 敏感信息检测规则 (正则 + 规则引擎)
# ============================================================

SENSITIVE_PATTERNS = [
    # (名称, 正则, 脱敏函数)
    ("身份证号", r"(?<![0-9])[1-9]\d{5}(?:19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[\dXx](?![0-9])",
     lambda m: m[0][:6] + "********" + m[0][-4:]),
    ("手机号", r"(?<![0-9])1[3-9]\d{9}(?![0-9])",
     lambda m: m[0][:3] + "****" + m[0][-4:]),
    ("银行卡号", r"(?<![0-9])(?:62|60|99|95)\d{14,17}(?![0-9])",
     lambda m: m[0][:4] + " **** **** " + m[0][-4:]),
    ("邮箱", r"(?<![0-9A-Za-z])[\w.-]+@[\w.-]+\.\w{2,}(?![0-9A-Za-z])",
     lambda m: m[0][0] + "***@" + m[0].split("@")[1]),
    ("IP地址(内网)", r"(?<![0-9])(?:10\.\d{1,3}|172\.(?:1[6-9]|2\d|3[01])|192\.168)\.\d{1,3}\.\d{1,3}(?![0-9])",
     lambda m: "***.***.***.***"),
    ("座机号", r"(?<![0-9])0\d{2,3}[-\s]?\d{7,8}(?![0-9])",
     lambda m: m[0][:4] + "****" + m[0][-2:]),
    ("金额(万元)", r"(?<![0-9])\d+(?:\.\d+)?\s*万元(?![0-9A-Za-z])",
     lambda m: "***万元"),
    ("密码", r"(?:password|passwd|pwd|密码)[\s:=]+[\S]+",
     lambda m: m[0].split(":")[0] + ": ****"),
    ("Token/Key", r"(?:token|api_key|secret|access_key)[\s:=]+[\w.-]+",
     lambda m: m[0].split(":")[0] + ": ****"),
]

# 中文姓名特征 (姓氏 + 名字)
CN_SURNAMES = ["王","李","张","刘","陈","杨","黄","赵","吴","周","徐","孙","马","朱","胡","郭","何","高","林",
               "郑","罗","梁","谢","宋","唐","许","邓","韩","冯","曹","彭","曾","肖","田","董","袁","潘"]

# 银行相关敏感关键词
BANK_SENSITIVE_KEYWORDS = [
    "客户姓名", "身份证", "手机号", "银行卡", "密码", "交易密码",
    "账户余额", "授信额度", "信用评级", "征信", "反洗钱",
]

def scan_all_patterns(text: str) -> list:
    """扫描所有正则模式, 返回匹配列表"""
    findings = []
    for name, pattern, mask_fn in SENSITIVE_PATTERNS:
        for match in re.finditer(pattern, text):
            findings.append({
                "type": name,
                "value_preview": match.group()[:30],
                "position": match.span(),
                "severity": "high" if name in ["身份证号","银行卡号","密码"] else "medium"
            })
    return findings

def detect_chinese_names(text: str) -> list:
    """基于姓氏 + 上下文模式检测中文姓名"""
    findings = []
    # 简化的姓名检测: 姓氏 + 1-2个字 + 常见上下文关键词
    for surname in CN_SURNAMES:
        for match in re.finditer(re.escape(surname) + r"[一-龥]{1,2}(?=[\s，。、,.:;!?\)\]])", text):
            name = match.group()
            before = text[max(0,match.start()-10):match.start()]
            after = text[match.end():match.end()+10]
            # 检查上下文是否暗示这是姓名
            context_clues = ["先生","女士","同志","客户","用户","员工","经理","主任","行长","联系人","申请人"]
            if any(clue in before or clue in after for clue in context_clues):
                findings.append({
                    "type": "中文姓名",
                    "value_preview": name,
                    "position": match.span(),
                    "severity": "medium"
                })
    return findings

def desensitize_text(text: str, findings: list) -> str:
    """根据检测结果进行脱敏"""
    result = text
    # 从后往前替换, 避免位置偏移
    for finding in sorted(findings, key=lambda f: f["position"][0], reverse=True):
        start, end = finding["position"]
        original = text[start:end]
        if finding["type"] == "中文姓名":
            masked = original[0] + "*" * (len(original) - 1)
        else:
            masked = "***"
        result = result[:start] + masked + result[end:]
    return result


# ============================================================
# MCP Server 实现
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
                "serverInfo": {"name": "log-desensitization", "version": "1.0.0"},
                "capabilities": {"tools": {}}
            }
        }

    elif method == "tools/list":
        return {
            "jsonrpc": "2.0", "id": req_id,
            "result": {"tools": [
                {
                    "name": "scan_log",
                    "description": "扫描日志内容, 识别所有敏感信息并返回发现列表",
                    "inputSchema": {
                        "type": "object",
                        "properties": {
                            "log_content": {"type": "string", "description": "待扫描的日志内容"}
                        },
                        "required": ["log_content"]
                    }
                },
                {
                    "name": "desensitize",
                    "description": "对日志内容进行脱敏处理, 返回脱敏后的文本",
                    "inputSchema": {
                        "type": "object",
                        "properties": {
                            "log_content": {"type": "string"}
                        },
                        "required": ["log_content"]
                    }
                },
                {
                    "name": "detect_pii",
                    "description": "检测文本中的个人身份信息(PII)实体类型和数量",
                    "inputSchema": {
                        "type": "object",
                        "properties": {
                            "text": {"type": "string"}
                        },
                        "required": ["text"]
                    }
                },
            ]}
        }

    elif method == "tools/call":
        tool_name = params.get("name", "")
        args = params.get("arguments", {})

        if tool_name == "scan_log":
            return handle_scan(args, req_id)
        elif tool_name == "desensitize":
            return handle_desensitize(args, req_id)
        elif tool_name == "detect_pii":
            return handle_detect_pii(args, req_id)
        else:
            return error_response(req_id, f"未知工具: {tool_name}")

    return error_response(req_id, f"未知方法: {method}")


def handle_scan(args: dict, req_id) -> dict:
    content = args.get("log_content", "")
    if not content:
        return error_response(req_id, "log_content 不能为空")

    regex_findings = scan_all_patterns(content)
    name_findings = detect_chinese_names(content)
    all_findings = regex_findings + name_findings

    # 统计
    by_type = {}
    for f in all_findings:
        by_type[f["type"]] = by_type.get(f["type"], 0) + 1

    text = f"## 🔍 敏感信息扫描报告\n\n"
    text += f"**扫描时间**: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n"
    text += f"**扫描字符数**: {len(content)}\n"
    text += f"**发现敏感信息**: {len(all_findings)} 条\n\n"

    text += "### 📊 类型分布\n\n| 类型 | 数量 |\n|------|------|\n"
    for t, c in sorted(by_type.items(), key=lambda x: x[1], reverse=True):
        text += f"| {t} | {c} |\n"

    if all_findings:
        text += f"\n### 📋 详细信息 (前 20 条)\n\n"
        for f in all_findings[:20]:
            text += f"- [{f['severity'].upper()}] **{f['type']}**: `{f['value_preview']}`\n"

    return {
        "jsonrpc": "2.0", "id": req_id,
        "result": {
            "content": [{"type": "text", "text": text}],
            "data": {
                "total_findings": len(all_findings),
                "by_type": by_type,
                "findings": all_findings
            }
        }
    }


def handle_desensitize(args: dict, req_id) -> dict:
    content = args.get("log_content", "")
    regex_findings = scan_all_patterns(content)
    name_findings = detect_chinese_names(content)
    all_findings = regex_findings + name_findings

    masked = desensitize_text(content, all_findings)

    text = f"## ✅ 脱敏处理完成\n\n"
    text += f"**原始字符数**: {len(content)}\n"
    text += f"**脱敏字符数**: {len(masked)}\n"
    text += f"**处理敏感信息**: {len(all_findings)} 条\n\n"
    text += f"### 脱敏后文本 (前 500 字符):\n\n```\n{masked[:500]}\n```"

    return {
        "jsonrpc": "2.0", "id": req_id,
        "result": {
            "content": [{"type": "text", "text": text}],
            "data": {"masked_content": masked, "findings_count": len(all_findings)}
        }
    }


def handle_detect_pii(args: dict, req_id) -> dict:
    text = args.get("text", "")
    all_findings = scan_all_patterns(text) + detect_chinese_names(text)

    by_type = {}
    for f in all_findings:
        by_type[f["type"]] = by_type.get(f["type"], 0) + 1

    return {
        "jsonrpc": "2.0", "id": req_id,
        "result": {
            "content": [{"type": "text", "text": json.dumps(by_type, ensure_ascii=False, indent=2)}],
            "data": {"pii_types": by_type, "total": len(all_findings)}
        }
    }


def error_response(req_id, msg):
    return {"jsonrpc": "2.0", "id": req_id, "error": {"code": -1, "message": msg}}


def main():
    print("🔒 日志敏感信息识别 MCP Server v1.0.0 启动", file=sys.stderr)
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
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
