"""三道防线有效性实验 (对抗性验证)

思路: 每个探针只让「对应的那道防线」能拦住, 看是否真的拦得住、且正常包不误报。
  A 明文恶意      -> 第一道(静态正则) 即拦住
  B 混淆规避      -> 危险词全藏起来(源码无明文), 传统正则漏, 靠第一道「语义解码」抓回
  C 纯行为型      -> 静态干净, 运行期才作恶, 只有第二道(沙箱)看得见
  D 社工话术      -> 纯文档、无代码、无行为, 只有第三道(AI 语义审计)看得出
  E 正常包        -> 三道都不应误报
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

# 传统静态正则规则编号 (第一道里的字面量层); AST-* 是语义层
LEGACY_PREFIX = ('EXEC-', 'CRED-', 'NET-', 'OBF-', 'DEST-', 'MAL-', 'INJ-', 'FILE-', 'HALL-', 'SUP-', 'IP-')


def call(method, path, body=None, auth=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header('Content-Type', 'application/json')
    if auth:
        req.add_header('Authorization', 'Bearer ' + auth)
    try:
        with urllib.request.urlopen(req, timeout=180) as r:
            return r.status, json.loads(r.read() or b'{}')
    except urllib.error.HTTPError as e:
        try:
            return e.code, json.loads(e.read() or b'{}')
        except Exception:
            return e.code, {}


def scan_bytes(content, filename, auth):
    boundary = '----skillhub' + uuid.uuid4().hex
    body = b''.join([
        ('--' + boundary + '\r\nContent-Disposition: form-data; name="file"; filename="%s"\r\n' % filename).encode(),
        b'Content-Type: application/octet-stream\r\n\r\n', content,
        ('\r\n--' + boundary + '--\r\n').encode(),
    ])
    req = urllib.request.Request(BASE + '/admin/security/scan-package', data=body, method='POST')
    req.add_header('Content-Type', 'multipart/form-data; boundary=' + boundary)
    req.add_header('Authorization', 'Bearer ' + auth)
    with urllib.request.urlopen(req, timeout=180) as r:
        out = json.loads(r.read() or b'{}')
    return out.get('data', {})


def classify(scan):
    ids = {f['rule_id'] for f in (scan.get('findings') or [])}
    line1_sem = sorted(r for r in ids if r.startswith('AST-'))
    line1_legacy = sorted(r for r in ids if r.startswith(LEGACY_PREFIX))
    line2 = sorted(r for r in ids if r.startswith('DYN-'))
    line3 = sorted(r for r in ids if r.startswith('SEM-'))
    return line1_legacy, line1_sem, line2, line3


def check(name, ok, detail=''):
    (PASSED if ok else FAILED).append(name)
    print(('  [PASS] ' if ok else '  [FAIL] ') + name + (' | ' + str(detail)[:160] if detail else ''))


def main():
    st, res = call('POST', '/auth/login', {'username': 'admin', 'password': 'demo'})
    auth = res.get('token')
    if not auth:
        print('! 登录失败, 请确认后端已启动 (8080)')
        return 1

    print('=' * 92)
    print('三道防线有效性实验  (A 明文恶意 / B 混淆规避 / C 纯行为 / D 社工话术 / E 正常)')
    print('=' * 92)
    print('%-26s %-9s %-5s %-14s %-16s %-10s %s' % ('探针', '结论', '分值', '第一道-正则', '第一道-语义AST', '第二道-沙箱', '第三道-语义审计'))
    print('-' * 92)

    probes = [
        ('A 明文恶意', samples.malicious_zip(), 'bad-skill.zip'),
        ('B 混淆规避', samples.evasion_zip(), 'quiet-report.zip'),
        ('C 纯行为型', samples.runtime_probe_zip(), 'runtime-probe.zip'),
        ('D 社工话术', samples.semantic_zip(), 'report-helper.zip'),
        ('E 正常包', samples.benign_zip(), 'safe-report.zip'),
    ]
    results = {}
    for label, payload, filename in probes:
        scan = scan_bytes(payload, filename, auth)
        legacy, sem, sbx, sem_audit = classify(scan)
        results[label] = (scan, legacy, sem, sbx, sem_audit)
        print('%-26s %-9s %-5s %-14s %-16s %-10s %s' % (
            label, scan.get('verdict_cn') or scan.get('verdict'), scan.get('risk_score'),
            ','.join(r[:6] for r in legacy) or '-', ','.join(r[:6] for r in sem) or '-',
            ','.join(sbx) or '-', ','.join(sem_audit) or '-'))

    print('-' * 92)

    # A: 明文恶意 -> 第一道正则即拦
    scan_a, legacy_a, sem_a, _, _ = results['A 明文恶意']
    print('\n[A] 明文恶意: 传统正则直接命中')
    check('A 正则层命中 (非仅靠语义)', len(legacy_a) >= 4, sorted(legacy_a))
    check('A 结论为高危拦截', scan_a.get('verdict') == 'malicious', scan_a.get('verdict'))

    # B: 混淆规避 -> 正则看不到明文, 语义层解码后抓回
    scan_b, legacy_b, sem_b, _, _ = results['B 混淆规避']
    print('\n[B] 混淆规避: 源码里无明文字样, 全靠语义层解码')
    src = samples.evasion_zip()
    import zipfile, io as _io
    zf = zipfile.ZipFile(_io.BytesIO(src))
    server_src = zf.read('quiet-report/server.py').decode('utf8')
    literals = ['os.system(', 'id_rsa', 'urlopen', 'requests.get(', 'eval(', 'subprocess', '| sh']
    visible = [t for t in literals if t in server_src]
    print('    源码明文危险词: %s' % (visible or '无 (已全部隐藏)'))
    check('B 源码中不含明文危险词 (正则天然看不见)', not visible, visible)
    check('B 正则层几乎未命中 (证明确实被绕过)', len(legacy_b) <= 1, sorted(legacy_b))
    check('B 语义层仍抓出执行+外联+凭据+编码载荷', {'AST-01', 'AST-02', 'AST-03', 'AST-06'} <= set(sem_b), sorted(sem_b))
    decoded_ev = [f['evidence'] for f in scan_b.get('findings', [])
                  if f['rule_id'] in ('AST-01', 'AST-02', 'AST-03') and '解码' in (f.get('detail') or '')]
    print('    语义层证据(解码自载荷): %s' % (decoded_ev[:3] or '-'))
    check('B 能力来自 runtime 解码载荷 (原文不可见)', bool(decoded_ev))
    check('B 结论为高危/可疑', scan_b.get('verdict') in ('malicious', 'suspicious'), scan_b.get('verdict'))
    check('B 声明一致性 AST-08 触发 (声称只读无网络)', 'AST-08' in sem_b)

    # C: 纯行为型 -> 静态干净, 只有沙箱看得见
    scan_c, legacy_c, sem_c, sbx_c, _ = results['C 纯行为型']
    print('\n[C] 纯行为型: 静态看不出, 沙箱运行期抓行为')
    check('C 沙箱观测到危险行为', len(sbx_c) > 0, sorted(sbx_c))
    sb = scan_c.get('sandbox') or {}
    print('    沙箱全链路监控: %s' % (sb.get('monitor') or {}))
    check('C 结论为高危拦截(由沙箱行为驱动)', scan_c.get('verdict') == 'malicious', scan_c.get('verdict'))

    # D: 社工话术(代码层无问题) -> 第三道语义审计
    scan_d, legacy_d, sem_d_sem, sbx_d, sem_audit_d = results['D 社工话术']
    print('\n[D] 语义样本: 第三道得出「意图偏离」')
    sem = scan_d.get('semantic') or {}
    print('    宣称「%s」 -> 实际「%s」  偏离=%s' % (sem.get('declared_intent'), sem.get('inferred_intent'), sem.get('intent_divergence')))
    check('D 第三道给出意图偏离 (SEM-07)', sem.get('intent_divergence') is True and 'SEM-07' in (sem_audit_d or []), sem_audit_d)

    # 纯话术文本(无代码) -> 只有第三道能抓
    st, out = call('POST', '/admin/security/semantic-audit', {'text': samples.SOCIAL_ENGINEERING_TEXT}, auth=auth)
    rep = out.get('data') or {}
    sem_ids = sorted({f['rule_id'] for f in (rep.get('findings') or [])})
    print('    纯话术文本(无代码): 命中 %s, 手法: %s' % (sem_ids, '、'.join(rep.get('techniques') or [])))
    check('D 纯话术文本第三道命中 ≥4 类社工手法', len(sem_ids) >= 4, sem_ids)

    # E: 正常包 -> 三道均不误报
    scan_e, legacy_e, sem_e, sbx_e, sem_audit_e = results['E 正常包']
    print('\n[E] 正常包: 不得误报')
    check('E 结论安全', scan_e.get('verdict') == 'safe', scan_e.get('verdict'))
    check('E 无 语义/SEM/沙箱 误报', not (sem_e or sem_audit_e or [r for r in sbx_e]), (sem_e, sem_audit_e, sbx_e))

    print('\n' + '=' * 92)
    print('实验结论: %d 通过 / %d 失败' % (len(PASSED), len(FAILED)))
    if FAILED:
        print('失败项: ' + ', '.join(FAILED))
    print('=' * 92)
    return 1 if FAILED else 0


if __name__ == '__main__':
    sys.exit(main())
