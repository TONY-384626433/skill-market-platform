# -*- coding: utf-8 -*-
"""把第一道防线(AST/语义)与第三道防线(AI 语义审计)的规则合并进规则库, 幂等。"""
import json, io, os

P = os.path.join(os.path.dirname(__file__), '..', 'rules', 'security-rules.json')
P = os.path.abspath(P)

with io.open(P, encoding='utf-8') as f:
    doc = json.load(f)

new_rules = [
    # ---------- 第一道防线: AST / 能力语义分析 ----------
    {"id": "AST-01", "category": "execution", "severity": "critical", "title": "语义分析：危险执行能力",
     "detail": "代码语义上可执行系统命令或动态求值代码 (eval/exec/subprocess/os.system/child_process 等), 后门/木马核心能力",
     "scope": "code", "pattern": None, "signature_encoded": False},
    {"id": "AST-02", "category": "network", "severity": "high", "title": "语义分析：网络外联能力",
     "detail": "代码语义上可发起外部网络请求 (requests/urllib/socket/fetch/Invoke-WebRequest 等), 存在数据外传风险",
     "scope": "code", "pattern": None, "signature_encoded": False},
    {"id": "AST-03", "category": "credential", "severity": "critical", "title": "语义分析：读取凭据/敏感文件",
     "detail": "语义上会读取密钥/密码/凭据文件或系统账号文件 (.ssh/.aws/.kube/shadow/浏览器 Cookies 等)",
     "scope": "code", "pattern": None, "signature_encoded": False},
    {"id": "AST-04", "category": "destructive", "severity": "high", "title": "语义分析：破坏性操作能力",
     "detail": "语义上会删除/清空/格式化文件或数据 (rmtree/os.remove/rm -rf/DROP TABLE 等)",
     "scope": "code", "pattern": None, "signature_encoded": False},
    {"id": "AST-05", "category": "persistence", "severity": "high", "title": "语义分析：持久化/越权能力",
     "detail": "语义上会写计划任务/开机自启/SSH 公钥/创建用户, 用于长期驻留或提权",
     "scope": "code", "pattern": None, "signature_encoded": False},
    {"id": "AST-06", "category": "obfuscation", "severity": "critical", "title": "语义分析：载荷编码后动态执行",
     "detail": "运行时解码 base64/hex/字符码后再求值执行 (eval(atob(...)) 等), 主动规避静态审查",
     "scope": "code", "pattern": None, "signature_encoded": False},
    {"id": "AST-07", "category": "evasion", "severity": "high", "title": "语义分析：反调试/反沙箱",
     "detail": "检测调试器/虚拟机/沙箱环境或长时间休眠以躲避动态分析",
     "scope": "code", "pattern": None, "signature_encoded": False},
    {"id": "AST-08", "category": "compliance", "severity": "high", "title": "语义一致性：声明与代码能力不符",
     "detail": "文档声明的能力(无网络/只读)与代码语义上具备的能力相矛盾, 疑似隐瞒真实行为或幻觉",
     "scope": "code", "pattern": None, "signature_encoded": False},
    # ---------- 第三道防线: AI 语义审计 (社工话术 / 意图深度分析) ----------
    {"id": "SEM-01", "category": "social_engineering", "severity": "critical", "title": "社工话术：冒充权威/系统指令",
     "detail": "冒充系统/管理员/官方口径下发指令, 诱导执行者越权操作 (权威伪装)",
     "scope": "markdown", "pattern": None, "signature_encoded": False},
    {"id": "SEM-02", "category": "social_engineering", "severity": "high", "title": "社工话术：制造紧迫/恐吓",
     "detail": "以「立即/紧急/否则后果自负/即将封禁」等话术压缩判断时间, 迫使命中目标仓促服从",
     "scope": "markdown", "pattern": None, "signature_encoded": False},
    {"id": "SEM-03", "category": "social_engineering", "severity": "high", "title": "社工话术：情感操纵/利益诱导",
     "detail": "通过共情、恭维、返利、红包、独家福利等诱导目标放下戒心配合操作",
     "scope": "markdown", "pattern": None, "signature_encoded": False},
    {"id": "SEM-04", "category": "social_engineering", "severity": "high", "title": "社工话术：隐瞒真实目的",
     "detail": "描述与实际行为不符, 刻意淡化/隐瞒数据外传、权限申请等真实用途 (欺骗性描述)",
     "scope": "markdown", "pattern": None, "signature_encoded": False},
    {"id": "SEM-05", "category": "social_engineering", "severity": "critical", "title": "社工话术：索取凭据/资金",
     "detail": "诱导提供账号密码、验证码、密钥、Token 或进行转账/代付等资金操作",
     "scope": "markdown", "pattern": None, "signature_encoded": False},
    {"id": "SEM-06", "category": "social_engineering", "severity": "high", "title": "社工话术：规避审查(语义变体)",
     "detail": "用同义改写/委婉表达绕过关键词审计, 实质仍要求忽略规则、隐瞒行为或绕过审批",
     "scope": "markdown", "pattern": None, "signature_encoded": False},
    {"id": "SEM-07", "category": "social_engineering", "severity": "critical", "title": "意图深度分析：隐藏目的",
     "detail": "综合文档话术与代码能力, 判定技能真实意图与宣称意图严重偏离 (存在未声明的隐藏目的)",
     "scope": "markdown", "pattern": None, "signature_encoded": False},
]

by_id = {r['id']: r for r in doc['rules']}
added = 0
for r in new_rules:
    if r['id'] not in by_id:
        doc['rules'].append(r)
        added += 1
    else:
        by_id[r['id']].update(r)

doc['engine'] = 'SEC-ENGINE 3.0.0'
doc['updated_at'] = '2026-09-19'
doc['description'] = ('技能安全静态扫描规则库 (签名库以数据文件形式独立存放, 可热更新与审计) '
                      '(含动态沙箱行为规则 DYN-* / 语义分析规则 AST-* / AI 语义审计规则 SEM-*)')

with io.open(P, 'w', encoding='utf-8') as f:
    json.dump(doc, f, ensure_ascii=False, indent=2)
    f.write('\n')

print('added', added, 'total', len(doc['rules']))
