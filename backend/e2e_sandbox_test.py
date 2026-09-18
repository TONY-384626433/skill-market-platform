"""SkillHub 动态沙箱验证 (DYN-01~08) 端到端测试

定位: 静态查毒之外的第二道验证 —— 把技能放进「无外网 / 只读根 / 非 root /
      cap-drop ALL / 限额」的隔离容器里真跑一次, 观测它到底做了什么。

覆盖:
  1. 沙箱引擎可用性 + 隔离自检 (无外网 / 只读根 / 非 root / capabilities 清空)
  2. 规则库与策略声明动态行为规则 DYN-01 ~ DYN-08
  3. 内部技能: 真跑一次 + MCP 握手, 无危险行为, 结论仍为 safe (不误伤)
  4. 上传正常可执行技能包: 沙箱内执行成功但无风险行为 -> 放行
  5. 上传行为型恶意样本: 命中 DYN-01/02/04/06/08 -> malicious 且阻断
  6. 沙箱报告随扫描记录持久化, 事后可回读取证 (隔离自检 + 行为事件 + 检查项)
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


def call(method, path, body=None, token=None):
    req = urllib.request.Request(BASE + path, data=json.dumps(body).encode() if body is not None else None, method=method)
    req.add_header('Content-Type', 'application/json')
    if token:
        req.add_header('Authorization', 'Bearer ' + token)
    try:
        with urllib.request.urlopen(req, timeout=300) as r:
            return r.status, json.loads(r.read() or b'{}'), dict(r.headers)
    except urllib.error.HTTPError as e:
        payload = e.read()
        try:
            return e.code, json.loads(payload or b'{}'), dict(e.headers)
        except Exception:
            return e.code, {'raw': payload[:200].decode('utf8', 'replace')}, dict(e.headers)


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
        with urllib.request.urlopen(req, timeout=300) as r:
            return r.status, json.loads(r.read() or b'{}')
    except urllib.error.HTTPError as e:
        payload = e.read()
        try:
            return e.code, json.loads(payload or b'{}')
        except Exception:
            return e.code, {'raw': payload[:200].decode('utf8', 'replace')}


def login(username, password):
    st, res, _ = call('POST', '/auth/login', {'username': username, 'password': password})
    return res.get('token')


def rule_ids(scan):
    return [f.get('rule_id') for f in (scan.get('findings') or [])]


def main():
    admin = login('admin', 'demo')
    check('管理员登录', bool(admin))

    # ---------- 1. 沙箱引擎与隔离自检 ----------
    st, rules, _ = call('GET', '/security/rules')
    engine = rules.get('engine', {})
    sandbox = engine.get('dynamic_sandbox') or {}
    check('沙箱引擎元信息对外可审计',
          st == 200 and sandbox.get('enabled') is True and sandbox.get('reachable') is True,
          'url=%s engine=%s' % (sandbox.get('url'), sandbox.get('engine')))
    iso = sandbox.get('isolated') or {}
    check('沙箱隔离自检: 无外网可达', iso.get('network_reachable') is False, iso)
    check('沙箱隔离自检: 非 root (uid 65534)', iso.get('uid') == 65534, iso.get('uid'))
    check('沙箱隔离自检: 根文件系统只读', iso.get('rootfs_read_only') is True, iso.get('rootfs_read_only'))
    check('沙箱隔离自检: capabilities 已清空', str(iso.get('cap_eff', '')).strip('0') == '', iso.get('cap_eff'))

    # ---------- 2. 规则与策略 ----------
    ids = {r['rule_id'] for r in rules.get('data', [])}
    dyn = {'DYN-0%d' % i for i in range(1, 9)}
    check('规则库含动态行为规则 DYN-01~08', dyn <= ids, sorted(dyn - ids))
    policy = rules.get('policy', {})
    check('安全策略声明动态沙箱 (规则+说明)',
          'DYN-01' in str(policy.get('dynamic_sandbox', {}).get('rules', ''))
          and policy.get('dynamic_sandbox', {}).get('mode') == 'behaviour_verification',
          policy.get('dynamic_sandbox', {}).get('rules'))

    # ---------- 3. 内部技能: 真跑一次, 不误伤 ----------
    st, scan, _ = call('POST', '/admin/skills/s-001/security-scan', token=admin)
    sb = scan.get('sandbox') or {}
    check('内部技能扫描携带沙箱验证报告', st == 200 and sb.get('status') == 'ok',
          'status=%s notice=%s' % (sb.get('status'), sb.get('notice')))
    check('内部技能在沙箱内真实执行 (MCP 入口)',
          sb.get('executed') is True and str(sb.get('entry', '')).endswith('server.py'),
          'entry=%s exit=%s' % (sb.get('entry'), sb.get('exit_code')))
    check('内部技能完成行为观测 (无危险行为)', not (dyn & set(rule_ids(scan))),
          'dyn_hits=%s' % sorted(dyn & set(rule_ids(scan))))
    check('内部技能结论仍为安全 (沙箱不误伤)', scan.get('verdict') == 'safe' and scan.get('critical_count') == 0,
          'verdict=%s risk=%s' % (scan.get('verdict'), scan.get('risk_score')))
    checks = sb.get('checks') or []
    check('沙箱输出检查项 (运行稳定性/汇总)',
          any(c.get('rule_id') == 'DYN-07' for c in checks) and any('DYN-01' in str(c.get('rule_id')) for c in checks),
          [c.get('rule_id') + ':' + c.get('status') for c in checks])
    check('沙箱报告含隔离上下文与引擎版本',
          (sb.get('isolated') or {}).get('network_reachable') is False and bool(sb.get('run_id')) and bool(sb.get('engine')),
          'run_id=%s engine=%s' % (sb.get('run_id'), sb.get('engine')))

    # ---------- 4. 上传正常可执行包 -> 沙箱内执行但放行 ----------
    st, out = upload('/admin/security/scan-package', 'runtime-ok.zip', samples.runtime_benign_zip(), admin)
    ok_scan = out.get('data', {})
    ok_sb = ok_scan.get('sandbox') or {}
    check('上传正常可执行技能包 -> 沙箱内执行成功', st == 200 and ok_sb.get('status') == 'ok' and ok_sb.get('executed') is True,
          'status=%s events=%s' % (ok_sb.get('status'), json.dumps(ok_sb.get('events'), ensure_ascii=False)))
    check('正常技能包无动态风险行为 -> 放行 (safe)',
          ok_scan.get('verdict') == 'safe' and not out.get('blocked') and not (dyn & set(rule_ids(ok_scan))),
          'verdict=%s risk=%s hits=%s' % (ok_scan.get('verdict'), ok_scan.get('risk_score'), rule_ids(ok_scan)))

    # ---------- 5. 上传行为型恶意样本 -> 动态命中并阻断 ----------
    st, out = upload('/admin/security/scan-package', 'runtime-probe.zip', samples.runtime_probe_zip(), admin)
    bad_scan = out.get('data', {})
    bad_sb = bad_scan.get('sandbox') or {}
    hits = set(rule_ids(bad_scan))
    check('行为型样本在沙箱内被执行并观测到行为',
          st == 200 and bad_sb.get('status') == 'ok' and bad_sb.get('executed') is True and (bad_sb.get('trace_lines') or 0) > 0,
          'trace=%s events=%s' % (bad_sb.get('trace_lines'), json.dumps(bad_sb.get('events'), ensure_ascii=False)))
    missing = sorted(set(samples.EXPECTED_DYN_RULES) - hits)
    check('动态规则命中 DYN-01/02/04/06/08', not missing, 'missing=%s hits=%s' % (missing, sorted(hits)))
    check('行为型样本结论为高危且阻断',
          bad_scan.get('verdict') == 'malicious' and out.get('blocked') is True and bad_scan.get('critical_count', 0) > 0,
          'verdict=%s risk=%s critical=%s' % (bad_scan.get('verdict'), bad_scan.get('risk_score'), bad_scan.get('critical_count')))
    dyn_findings = [f for f in (bad_scan.get('findings') or []) if str(f.get('rule_id', '')).startswith('DYN-')]
    check('动态发现标注为阻断级 (critical -> blocking)',
          dyn_findings and all(f.get('blocking') for f in dyn_findings if f.get('severity') == 'critical'),
          [(f.get('rule_id'), f.get('severity'), f.get('blocking')) for f in dyn_findings])

    # ---------- 6. 沙箱报告持久化, 可回读取证 ----------
    scan_id = bad_scan.get('id')
    st, detail, _ = call('GET', '/admin/security-scans/%s' % scan_id, token=admin)
    saved = detail.get('sandbox') if isinstance(detail, dict) and 'sandbox' in detail else (detail.get('data') or {}).get('sandbox')
    saved = saved or {}
    check('沙箱报告随扫描记录持久化 (事后可回读)',
          st == 200 and saved.get('status') == 'ok' and (saved.get('run_id') or '') == (bad_sb.get('run_id') or ''),
          'run_id=%s entries=%s' % (saved.get('run_id'), saved.get('entries')))
    check('回读报告保留行为事件与文件系统比对',
          bool(saved.get('events')) and 'filesystem_diff' in saved,
          json.dumps(saved.get('events'), ensure_ascii=False))

    print('\n===== 结果: %d 通过 / %d 失败 =====' % (len(PASSED), len(FAILED)))
    if FAILED:
        print('失败项:', FAILED)
    return 1 if FAILED else 0


if __name__ == '__main__':
    sys.exit(main())
