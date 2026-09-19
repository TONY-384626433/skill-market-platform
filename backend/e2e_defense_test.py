"""SkillHub 技能安全「三道防线」端到端测试

覆盖:
  第一道  代码级检测 (静态分析 + AST/能力语义分析)
          - 规则库已装载 AST-01 ~ AST-08
          - 语义样本命中 危险执行/网络外联/凭据读取/编码载荷/声明不一致
  第二道  动态沙箱验证 (隔离环境 + 全链路监控)
          - defense-status 报告沙箱模式与全链路监控通道
          - 行为样本命中动态规则 (若沙箱在线)
  第三道  AI 语义审计 (社工话术识别 + 意图深度分析)
          - 语义样本得出「意图偏离」(SEM-07)
          - 社工话术文本命中 SEM-01/02/04/05/06
          - 正常文本不误报
  回归    正常技能包结论仍为安全, 不产生 AST/SEM 误报
"""

import json
import os
import sys
import urllib.error
import urllib.request
import uuid

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), 'tools'))
import security_samples as samples  # noqa: E402

BASE = 'http://localhost:8080/api/v1'
PASSED, FAILED = [], []


def check(name, ok, detail=''):
    (PASSED if ok else FAILED).append(name)
    print(('[PASS] ' if ok else '[FAIL] ') + name + (' | ' + str(detail)[:200] if detail else ''))


def call(method, path, body=None, token=None, headers=None):
    req = urllib.request.Request(BASE + path, data=json.dumps(body).encode() if body is not None else None, method=method)
    req.add_header('Content-Type', 'application/json')
    if token:
        req.add_header('Authorization', 'Bearer ' + token)
    for k, v in (headers or {}).items():
        req.add_header(k, v)
    try:
        with urllib.request.urlopen(req, timeout=180) as r:
            return r.status, json.loads(r.read() or b'{}')
    except urllib.error.HTTPError as e:
        payload = e.read()
        try:
            return e.code, json.loads(payload or b'{}')
        except Exception:
            return e.code, {'raw': payload[:300].decode('utf8', 'replace')}


def upload(path, filename, content, auth=None):
    boundary = '----skillhub' + uuid.uuid4().hex
    body = b''.join([
        ('--' + boundary + '\r\nContent-Disposition: form-data; name="file"; filename="%s"\r\n' % filename).encode(),
        b'Content-Type: application/octet-stream\r\n\r\n', content,
        ('\r\n--' + boundary + '--\r\n').encode(),
    ])
    req = urllib.request.Request(BASE + path, data=body, method='POST')
    req.add_header('Content-Type', 'multipart/form-data; boundary=' + boundary)
    if auth:
        req.add_header('Authorization', 'Bearer ' + auth)
    try:
        with urllib.request.urlopen(req, timeout=180) as r:
            return r.status, json.loads(r.read() or b'{}')
    except urllib.error.HTTPError as e:
        payload = e.read()
        try:
            return e.code, json.loads(payload or b'{}')
        except Exception:
            return e.code, {'raw': payload[:200].decode('utf8', 'replace')}


def login(username, password):
    _, res = call('POST', '/auth/login', {'username': username, 'password': password})
    return res.get('token')


def rule_ids(findings):
    return {f.get('rule_id') for f in (findings or [])}


def main():
    admin = login('admin', 'demo')
    check('管理员登录', bool(admin))

    # ---------------- 规则库: 三道防线规则齐备 ----------------
    st, rules = call('GET', '/security/rules')
    ids = {r.get('rule_id') for r in (rules.get('data') or [])}
    ast_ok = all(('AST-%02d' % i) in ids for i in range(1, 9))
    sem_ok = all(('SEM-%02d' % i) in ids for i in range(1, 8))
    check('规则库含 AST-01~AST-08 (第一道/语义)', ast_ok, sorted(i for i in ids if i.startswith('AST')))
    check('规则库含 SEM-01~SEM-07 (第三道/语义审计)', sem_ok, sorted(i for i in ids if i.startswith('SEM')))

    # ---------------- 三道防线总览 ----------------
    st, defense = call('GET', '/admin/security/defense-status', token=admin)
    lines = {l.get('key'): l for l in (defense.get('lines') or [])}
    check('defense-status 返回三道防线', st == 200 and {'static', 'sandbox', 'semantic'} <= set(lines.keys()),
          list(lines.keys()))
    check('第一道防线(静态/AST)在线', lines.get('static', {}).get('mode') == 'active',
          lines.get('static', {}).get('engine'))
    sandbox_line = lines.get('sandbox', {})
    check('第二道防线(沙箱)上报隔离+全链路监控通道',
          bool(sandbox_line.get('channels')) and sandbox_line.get('mode') in ('active', 'unreachable', 'disabled'),
          '%s / %s' % (sandbox_line.get('mode'), sandbox_line.get('channels')))
    sem_line = lines.get('semantic', {})
    check('第三道防线(AI 语义审计)上报模式', sem_line.get('mode') in ('llm', 'heuristic'),
          '%s / %s' % (sem_line.get('engine'), sem_line.get('mode')))

    # ---------------- 第一道 + 第三道: 语义样本包扫描 ----------------
    st, res = upload('/admin/security/scan-package', 'report-helper.zip', samples.semantic_zip(), admin)
    scan = res.get('data') or {}
    found = rule_ids(scan.get('findings'))
    missing = [k for k in samples.EXPECTED_AST_RULES if k not in found]
    check('第一道防线(语义/AST)命中危险能力', not missing, '缺失: %s' % missing)
    facts = scan.get('semantic_facts') or {}
    caps = facts.get('capabilities') or {}
    check('语义能力画像产出', bool(caps) and facts.get('files_analyzed', 0) > 0, caps)
    check('声明一致性(AST-08)告警', 'AST-08' in found, facts.get('consistency'))
    sem = scan.get('semantic') or {}
    check('第三道防线(意图偏离 SEM-07)', sem.get('intent_divergence') is True and 'SEM-07' in rule_ids(sem.get('findings')),
          '%s -> %s' % (sem.get('declared_intent'), sem.get('inferred_intent')))
    check('语义样本结论为高危/可疑', scan.get('verdict') in ('malicious', 'suspicious'),
          '%s / %s' % (scan.get('verdict'), scan.get('risk_score')))

    # ---------------- 第三道: 社工话术文本即席审计 ----------------
    st, res = call('POST', '/admin/security/semantic-audit', {'text': samples.SOCIAL_ENGINEERING_TEXT}, token=admin)
    rep = res.get('data') or {}
    sem_found = rule_ids(rep.get('findings'))
    missing_sem = [k for k in samples.EXPECTED_SEM_RULES if k not in sem_found]
    check('社工话术识别 (SEM-01/02/04/05/06)', not missing_sem, '缺失: %s (命中: %s)' % (missing_sem, sorted(sem_found)))
    check('社工话术审计识别出话术手法', bool(rep.get('techniques')), rep.get('techniques'))
    check('社工话术审计输出风险分>0', (rep.get('risk_score') or 0) > 0, rep.get('risk_score'))
    check('社工审计引擎模式可见', rep.get('mode') in ('llm', 'heuristic'), rep.get('mode'))

    # ---------------- 第三道: 正常文本不误报 ----------------
    st, res = call('POST', '/admin/security/semantic-audit',
                   {'text': '本技能用于把两个数字相加并返回结果，不访问网络，不写入文件。'}, token=admin)
    rep_ok = res.get('data') or {}
    check('正常文本不被误报', len(rep_ok.get('findings') or []) == 0, rep_ok.get('findings'))

    # ---------------- 回归: 正常技能包仍安全 ----------------
    st, res = upload('/admin/security/scan-package', 'safe-report.zip', samples.benign_zip(), admin)
    scan_ok = res.get('data') or {}
    found_ok = rule_ids(scan_ok.get('findings'))
    check('正常技能包结论安全', scan_ok.get('verdict') == 'safe', '%s / %s' % (scan_ok.get('verdict'), scan_ok.get('risk_score')))
    check('正常技能包无 AST/SEM 误报',
          not [r for r in found_ok if r.startswith('AST-') or r.startswith('SEM-')], sorted(found_ok))

    # ---------------- 沙箱: 行为样本 (在线才验证) ----------------
    if sandbox_line.get('mode') == 'active':
        st, res = upload('/admin/security/scan-package', 'runtime-probe.zip', samples.runtime_probe_zip(), admin)
        probe = res.get('data') or {}
        sbx = probe.get('sandbox') or {}
        dyn = rule_ids(sbx.get('findings'))
        check('第二道防线(沙箱)观测到危险行为', len(dyn) > 0, sorted(dyn))
        check('沙箱全链路监控通道计数', bool(sbx.get('monitor')), sbx.get('monitor'))
    else:
        print('[SKIP] 动态沙箱未在线 (mode=%s), 跳行动为验证' % sandbox_line.get('mode'))

    print('\n========== 三道防线 e2e: %d 通过 / %d 失败 ==========' % (len(PASSED), len(FAILED)))
    if FAILED:
        print('失败项: ' + ', '.join(FAILED))
        sys.exit(1)


if __name__ == '__main__':
    main()
