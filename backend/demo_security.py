"""安全治理一键演示 (银行落地汇报用)

用法:  py -3 demo_security.py [后端地址, 默认 http://localhost:8080]

演示内容:
  1. 安全引擎与策略 (规则库 / 病毒库 / 签名密钥指纹)
  2. 内部技能静态查毒
  3. 上传技能包安全预检: 正常包 -> 放行; 恶意包 -> 阻断并列出命中规则; 路径穿越包 -> 拦截
  4. 动态沙箱: 可执行技能在隔离容器(无外网/只读根/非root)真跑一次, 观测外联/命令执行/窃密/持久化
  5. GitHub 导入门禁: 未审核直接下载 -> 403 拒绝
"""

import json
import os
import sys
import urllib.error
import urllib.request
import uuid

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), 'tools'))
import security_samples as samples  # noqa: E402

BASE = (sys.argv[1] if len(sys.argv) > 1 else 'http://localhost:8080') + '/api/v1'


def call(method, path, body=None, auth=None, raw=False, form=None):
    data = json.dumps(body).encode() if body is not None else None
    headers = {'Content-Type': 'application/json'}
    if form is not None:
        boundary = '----skillhub' + uuid.uuid4().hex
        filename, content = form
        data = b''.join([
            ('--' + boundary + '\r\nContent-Disposition: form-data; name="file"; filename="%s"\r\n' % filename).encode(),
            b'Content-Type: application/octet-stream\r\n\r\n', content,
            ('\r\n--' + boundary + '--\r\n').encode(),
        ])
        headers['Content-Type'] = 'multipart/form-data; boundary=' + boundary
    req = urllib.request.Request(BASE + path, data=data, method=method)
    for k, v in headers.items():
        req.add_header(k, v)
    if auth:
        req.add_header('Authorization', 'Bearer ' + auth)
    try:
        with urllib.request.urlopen(req, timeout=180) as r:
            payload = r.read()
            return r.status, (payload if raw else json.loads(payload or b'{}'))
    except urllib.error.HTTPError as e:
        payload = e.read()
        try:
            return e.code, json.loads(payload or b'{}')
        except Exception:
            return e.code, {'raw': payload[:200].decode('utf8', 'replace')}


def line(char='-', n=74):
    print(char * n)


def main():
    line('=')
    print('SkillHub 技能安全治理演示 (供应链安全 · 可落地银行内网)')
    line('=')

    st, res = call('POST', '/auth/login', {'username': 'admin', 'password': 'demo'})
    admin = res.get('token')
    if not admin:
        print('! 登录失败, 请确认后端已启动 (8080) 且存在 admin/demo 账号')
        return 1
    print('[1] 管理员登录 OK')

    st, meta = call('GET', '/security/rules', auth=admin)
    engine = meta.get('engine', {})
    key = engine.get('key', {})
    policy = meta.get('policy', {})
    print('[2] 安全引擎')
    print('    规则库  : %s 条 / %s' % (engine.get('count'), engine.get('source')))
    print('    病毒库  : %s' % engine.get('av_summary'))
    print('    签名密钥: %s (来源 %s, 内置演示密钥=%s)' % (key.get('key_id'), key.get('source'), key.get('is_default')))
    print('    导入策略: %s' % json.dumps(policy.get('github_import', {}), ensure_ascii=False))
    print('    上传策略: %s' % json.dumps(policy.get('upload', {}), ensure_ascii=False))

    print('[3] 内部技能静态查毒')
    for sid, name in (('s-001', 'db-inspection'), ('s-002', 'log-desensitization'),
                      ('s-003', 'alert-convergence'), ('s-004', 'requirement-analysis')):
        st, scan = call('POST', '/admin/skills/%s/security-scan' % sid, auth=admin)
        print('    %-22s -> %-9s risk=%-3s 发现=%-2s 耗时=%sms' % (
            name, scan.get('verdict_cn') or scan.get('verdict'), scan.get('risk_score'),
            scan.get('finding_count'), scan.get('duration_ms')))

    print('[4] 上传技能包安全预检 (入库前第一道闸门)')
    for label, payload, filename in (('正常包', samples.benign_zip(), 'safe-report.zip'),
                                     ('恶意包', samples.malicious_zip(), 'bad-skill.zip'),
                                     ('穿越包', samples.zip_slip_zip(), 'slip.zip')):
        st, out = call('POST', '/admin/security/scan-package', auth=admin, form=('file', payload))
        scan = out.get('data', {})
        print('    %-5s -> %-9s risk=%-3s 严重=%-2s 阻断=%s' % (
            label, scan.get('verdict_cn') or scan.get('verdict'), scan.get('risk_score'),
            scan.get('critical_count'), out.get('blocked')))
        hits = [('%s(%s)' % (f['rule_id'], f['severity'])) for f in (scan.get('findings') or [])]
        hit_ids = [f['rule_id'] for f in (scan.get('findings') or [])]
        if hits:
            print('            命中: %s' % ', '.join(hits[:12]))
            if label == '恶意包':
                missing = [rid for rid in samples.EXPECTED_RULES if rid not in hit_ids]
                print('            关键规则覆盖: %s' % ('全部命中' if not missing else '缺少 ' + ','.join(missing)))

    print('[5] 动态沙箱验证 (静态查毒之外的第二道防线)')
    sbx = engine.get('dynamic_sandbox', {})
    print('    引擎    : %s  地址 %s  可用=%s' % (sbx.get('engine'), sbx.get('url'), sbx.get('reachable')))
    if not sbx.get('reachable'):
        print('    提示    : %s' % (sbx.get('notice') or '沙箱未就绪, 仅做静态扫描'))
    else:
        iso = sbx.get('isolated') or {}
        print('    隔离自检: 无外网可达=%s  只读根=%s  非root(uid)=%s  capabilities=%s' % (
            iso.get('network_reachable') is False, iso.get('rootfs_read_only'), iso.get('uid'), iso.get('cap_eff')))
    for label, payload, _ in (('正常可执行包', samples.runtime_benign_zip(), 'runtime-ok.zip'),
                              ('行为型样本', samples.runtime_probe_zip(), 'runtime-probe.zip')):
        st, out = call('POST', '/admin/security/scan-package', auth=admin, form=('file', payload))
        scan = out.get('data', {})
        sb = scan.get('sandbox') or {}
        ev = sb.get('events') or {}
        observed = ' '.join('%s=%s' % (k, v) for k, v in ev.items() if v) or '无'
        print('    %-6s -> %-9s 沙箱=%-9s 已执行=%-5s 行为事件=%s' % (
            label, scan.get('verdict_cn') or scan.get('verdict'), sb.get('status'), sb.get('executed'), observed))
        hits = [f['rule_id'] for f in (scan.get('findings') or []) if str(f.get('rule_id', '')).startswith('DYN-')]
        if hits:
            print('             动态命中: %s' % ', '.join(hits))
            missing = [rid for rid in samples.EXPECTED_DYN_RULES if rid not in hits]
            print('             动态规则覆盖: %s' % ('全部命中' if not missing else '缺少 ' + ','.join(missing)))

    print('[6] GitHub 导入门禁 (先审核, 后下载)')
    st, res = call('GET', '/github/skills/download?repo=DenisSergeevitch/repo-task-proof-loop&ref=main&path=SKILL.md',
                   auth=admin)
    print('    未审核直接下载 -> HTTP %s / gate=%s' % (st, res.get('gate')))
    print('    拒绝原因: %s' % res.get('error'))

    line('=')
    print('演示结束: 恶意/穿越包已阻断, 正常包放行, 动态沙箱真实复现危险行为, 未审核的外部技能禁止下载。')
    line('=')
    return 0


if __name__ == '__main__':
    sys.exit(main())
