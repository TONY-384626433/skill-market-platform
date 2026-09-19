"""SkillHub 技能安全治理 端到端测试

覆盖:
  1. 规则库装载 (签名库外置, fail-closed)
  2. 内部技能静态查毒扫描 (safe / 徽章 / 溯源档案 / 完整性校验)
  3. GitHub 导入门禁: 无审查单 -> 拒绝下载; 提交审查 -> 自动审核 -> 放行
  4. 交付包加固: 签名清单 + 唯一水印 + 逐文件哈希校验 + 篡改检测
  5. 下载审计留痕 & 水印可追溯
  6. 高危审查单被阻断后禁止下载
"""

import hashlib
import hmac
import io
import json
import os
import subprocess
import sys
import urllib.error
import urllib.request
import uuid
import zipfile

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), 'tools'))
import security_samples as samples  # noqa: E402

BASE = 'http://localhost:8080/api/v1'
SIGNING_KEY = os.environ.get('SKILLHUB_SIGNING_KEY', 'skillhub-jjbank-2026-provenance-key')
PASSED, FAILED = [], []


def check(name, ok, detail=''):
    (PASSED if ok else FAILED).append(name)
    print(('[PASS] ' if ok else '[FAIL] ') + name + (' | ' + str(detail)[:180] if detail else ''))


def call(method, path, body=None, token=None, headers=None, raw=False):
    req = urllib.request.Request(BASE + path, data=json.dumps(body).encode() if body is not None else None, method=method)
    req.add_header('Content-Type', 'application/json')
    if token:
        req.add_header('Authorization', 'Bearer ' + token)
    for k, v in (headers or {}).items():
        req.add_header(k, v)
    try:
        with urllib.request.urlopen(req, timeout=180) as r:
            payload = r.read()
            return r.status, (payload if raw else json.loads(payload or b'{}')), dict(r.headers)
    except urllib.error.HTTPError as e:
        payload = e.read()
        try:
            body = json.loads(payload or b'{}')
        except Exception:
            body = {'raw': payload[:300].decode('utf8', 'replace')}
        return e.code, body, dict(e.headers)


def login(username, password):
    st, res, _ = call('POST', '/auth/login', {'username': username, 'password': password})
    return res.get('token')


def hget(headers, name):
    target = name.lower()
    for k, v in headers.items():
        if k.lower() == target:
            return v
    return ''


def manifest_signature(payload_parts, files):
    head = '|'.join(str(x) for x in payload_parts)
    items = sorted('%s:%s:%d' % (f['path'], f['sha256'], f['size']) for f in files)
    payload = head + '|' + '|'.join(items)
    digest = hmac.new(SIGNING_KEY.encode(), payload.encode(), hashlib.sha256).hexdigest()
    return 'hmac-sha256:' + digest


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


def psql(sql):
    out = subprocess.run(['docker', 'exec', 'skillhub-postgres', 'psql', '-U', 'skillhub', '-d', 'skillhub', '-t', '-A', '-c', sql],
                         capture_output=True, text=True, encoding='utf8', errors='replace')
    return (out.stdout or '').strip()


def main():
    admin = login('admin', 'demo')
    check('管理员登录', bool(admin))

    # ---------- 1. 规则库 ----------
    st, rules, _ = call('GET', '/security/rules')
    engine = rules.get('engine', {})
    check('规则库装载 (外置签名库)', st == 200 and engine.get('loaded') is True and engine.get('count', 0) >= 25,
          'rules=%s source=%s' % (engine.get('count'), engine.get('source')))
    cats = {r['category'] for r in rules.get('data', [])}
    check('规则覆盖查毒/危险行为/提示注入/供应链',
          {'malware', 'execution', 'injection', 'supply_chain', 'credential', 'network'} <= cats, sorted(cats))
    st, policy, _ = call('GET', '/security/rules')
    check('安全策略对外可审计', policy.get('policy', {}).get('github_import', {}).get('mode') == 'review_before_download')
    check('上传包策略: 先扫描后入库', policy.get('policy', {}).get('upload', {}).get('mode') == 'scan_before_accept',
          policy.get('policy', {}).get('upload', {}).get('api'))

    av = engine.get('av_engines') or []
    check('外部查毒引擎适配 (ClamAV/YARA) 状态可审计',
          len(av) == 2 and all(item.get('engine') and item.get('detail') for item in av),
          engine.get('av_summary'))
    key = engine.get('key', {})
    check('签名密钥指纹与来源公开 (不泄露密钥本体)', str(key.get('key_id', '')).startswith('kid_') and bool(key.get('algorithm')),
          'key_id=%s source=%s default=%s' % (key.get('key_id'), key.get('source'), key.get('is_default')))

    # ---------- 2. 内部技能扫描 ----------
    scans = {}
    for sid, key in (('s-001', 'db-inspection'), ('s-002', 'log-desensitization'),
                     ('s-003', 'alert-convergence'), ('s-004', 'requirement-analysis')):
        st, scan, _ = call('POST', '/admin/skills/%s/security-scan' % sid, token=admin)
        scans[sid] = scan
        check('扫描内部技能 %s -> %s' % (key, scan.get('verdict')),
              st == 200 and scan.get('verdict') == 'safe' and scan.get('critical_count') == 0,
              'risk=%s findings=%s %sms' % (scan.get('risk_score'), scan.get('finding_count'), scan.get('duration_ms')))

    st, badge, _ = call('GET', '/skills/s-001/security-badge')
    check('安全徽章 (公开)', st == 200 and badge.get('security_status') == 'safe' and badge.get('watermark_id', '').startswith('WM-'),
          badge.get('watermark_id'))

    st, prov, _ = call('GET', '/admin/skills/s-001/provenance', token=admin)
    data = prov.get('data', {})
    check('溯源档案 (水印+指纹+签名)', st == 200 and data.get('watermark_id') and data.get('content_hash') and data.get('signature'),
          'watermark=%s hash=%s' % (data.get('watermark_id'), (data.get('content_hash') or '')[:16]))

    st, ver, _ = call('POST', '/admin/skills/s-001/provenance/verify', token=admin)
    check('技能包完整性校验 -> valid', st == 200 and (ver.get('data') or {}).get('verify_status') == 'valid',
          (ver.get('data') or {}).get('verify_status'))

    # 篡改检测: 直接改文件内容后重新校验
    target = os.path.join('..', 'seed-skills', 'db-inspection', 'server.py')
    backup = None
    if os.path.exists(target):
        backup = open(target, 'rb').read()
        open(target, 'ab').write(b'\nZ_TAMPER_PROBE_987654321 = 1\n')
        st, tampered, _ = call('POST', '/admin/skills/s-001/provenance/verify', token=admin)
        check('篡改检测 (内容被改动 -> tampered)', (tampered.get('data') or {}).get('verify_status') == 'tampered',
              (tampered.get('data') or {}).get('verify_status'))
        open(target, 'wb').write(backup)
        call('POST', '/admin/skills/s-001/security-scan', token=admin)

    # ---------- 3. GitHub 导入门禁 ----------
    repo, ref, path = 'DenisSergeevitch/repo-task-proof-loop', 'main', 'SKILL.md'
    st, res, _ = call('GET', '/github/skills/download?repo=%s&ref=%s&path=%s' % (repo, ref, path), token=admin)
    check('未审核直接下载 -> 拒绝 (403 门禁)', st == 403 and res.get('gate') == 'security_review_required', res.get('error'))

    st, submit, _ = call('POST', '/github/import-requests',
                         {'repository': repo, 'ref': ref, 'path': path,
                          'skill_url': 'https://github.com/%s' % repo}, token=admin)
    req = submit.get('data', {}) if isinstance(submit.get('data'), dict) else submit
    req_id = req.get('id')
    check('提交 GitHub 技能安全审查', st == 200 and bool(req_id),
          'status=%s verdict=%s risk=%s' % (req.get('status'), req.get('verdict'), req.get('risk_score')))
    check('审查已完成静态查毒 (含文件数/结论)', req.get('scan_id') and req.get('verdict') in ('safe', 'suspicious', 'malicious'),
          'grade=%s critical=%s watermark=%s' % (req.get('grade'), req.get('critical_count'), req.get('watermark_id')))

    if req.get('status') == 'pending_review':
        st, dec, _ = call('POST', '/admin/import-requests/%s/decision' % req_id,
                          {'approve': True, 'note': '人工复核: 已确认来源与权限, 放行'}, token=admin)
        check('人工复核放行', st == 200 and dec.get('status') == 'approved', dec.get('status'))

    st, detail, _ = call('GET', '/admin/import-requests/%s' % req_id, token=admin)
    d = detail.get('data', detail)
    check('放行状态可下载', d.get('status') in ('approved', 'imported') and d.get('download_allowed') is True, d.get('status'))

    # ---------- 4. 交付包加固 ----------
    st, zip_bytes, headers = call('GET', '/github/skills/download?repo=%s&ref=%s&path=%s&request_id=%s' % (repo, ref, path, req_id),
                                  token=admin, raw=True)
    check('审核通过后可下载', st == 200 and zip_bytes[:2] == b'PK', 'size=%d' % len(zip_bytes))
    check('响应头带水印与签名', hget(headers, 'X-SkillHub-Watermark').startswith('DL-') and hget(headers, 'X-SkillHub-Signature').startswith('hmac-sha256:'),
          hget(headers, 'X-SkillHub-Watermark'))

    zf = zipfile.ZipFile(io.BytesIO(zip_bytes))
    names = zf.namelist()
    check('包内含签名清单与水印文件',
          'MANIFEST.skillhub.json' in names and '.skillhub-provenance.json' in names, names[:6])
    manifest = json.loads(zf.read('MANIFEST.skillhub.json').decode('utf8'))
    expected_sig = manifest_signature(
        [manifest['schema'], manifest['skill_key'], manifest.get('version', ''), manifest['owner_id'], manifest['watermark_id'],
         manifest['delivery_watermark'], manifest['issued_at'], manifest['source'], manifest['content_hash'],
         manifest['sim_hash'], manifest['risk_score'], manifest['scan_verdict'], manifest.get('key_id', '')], manifest['files'])
    check('清单 HMAC-SHA256 签名可复核', expected_sig == manifest['signature'], manifest['signature'][:32] + '...')

    bad = [f['path'] for f in manifest['files']
           if hashlib.sha256(zf.read(f['path'].split('/', 1)[-1] if f['path'] not in names else f['path'])).hexdigest() != f['sha256']]
    check('逐文件哈希与清单一致', not bad, bad[:3])

    tampered_manifest = dict(manifest)
    tampered_manifest['risk_score'] = 0
    tampered_manifest['scan_verdict'] = 'safe'
    check('清单被篡改则验签失败',
          manifest_signature([tampered_manifest['schema'], tampered_manifest['skill_key'], manifest.get('version', ''),
                              manifest['owner_id'], manifest['watermark_id'], manifest['delivery_watermark'],
                              manifest['issued_at'], manifest['source'], manifest['content_hash'],
                              manifest['sim_hash'], 0, 'safe', manifest.get('key_id', '')], manifest['files']) != manifest['signature'])

    check('清单携带签发密钥指纹 (可审计密钥版本)', bool(manifest.get('key_id', '').startswith('kid_')),
          manifest.get('key_id'))

    wt = json.loads(zf.read('.skillhub-provenance.json').decode('utf8'))
    check('水印文件绑定下载人可追溯', wt.get('watermark_id') == manifest['watermark_id'] and wt.get('delivery_watermark', '').startswith('DL-'),
          '%s / %s' % (wt.get('watermark_id'), wt.get('delivery_watermark')))

    # ---------- 5. 审计 ----------
    st, audits, _ = call('GET', '/admin/download-audits', token=admin)
    rows = audits.get('data', [])
    hit = [a for a in rows if a.get('watermark_id') == manifest['delivery_watermark']]
    check('下载审计留痕 (水印可反查)', bool(hit), 'records=%d' % len(rows))

    # ---------- 7. 上传技能包安全预检 (入库前的第一道闸门) ----------
    tk = admin
    st, out = upload('/admin/security/scan-package', 'safe-report.zip', samples.benign_zip(), tk)
    benign = out.get('data', {})
    check('上传正常技能包 -> 放行 (safe)', st == 200 and benign.get('verdict') == 'safe' and not out.get('blocked'),
          'risk=%s findings=%s' % (benign.get('risk_score'), benign.get('finding_count')))

    st, out = upload('/admin/security/scan-package', 'bad-skill.zip', samples.malicious_zip(), tk)
    evil = out.get('data', {})
    hits = [f['rule_id'] for f in (evil.get('findings') or [])]
    check('上传恶意技能包 -> 阻断 (malicious)',
          st == 200 and evil.get('verdict') == 'malicious' and out.get('blocked') is True and evil.get('critical_count', 0) > 0,
          'risk=%s critical=%s hits=%s' % (evil.get('risk_score'), evil.get('critical_count'), ','.join(hits[:8])))
    missing = [rid for rid in samples.EXPECTED_RULES if rid not in hits]
    check('恶意样本命中全部关键规则', not missing, 'missing=%s' % missing)

    st, out = upload('/admin/security/scan-package', 'slip.zip', samples.zip_slip_zip(), tk)
    slip = out.get('data', {})
    slip_hits = [f['rule_id'] for f in (slip.get('findings') or [])]
    check('路径穿越包被拦截 (Zip Slip)', 'FILE-01' in slip_hits and slip.get('verdict') == 'malicious',
          ','.join(slip_hits))

    # ---------- 6. 高危阻断 ----------
    st, submit2, _ = call('POST', '/github/import-requests',
                          {'repository': repo, 'ref': ref, 'path': path}, token=admin)
    blocked_id = (submit2.get('data') or {}).get('id') if isinstance(submit2.get('data'), dict) else None
    if blocked_id:
        psql("UPDATE github_import_requests SET status='blocked', verdict='malicious', risk_score=100, critical_count=3 WHERE id='%s'" % blocked_id)
        st, res2, _ = call('GET', '/github/skills/download?repo=%s&ref=%s&path=%s&request_id=%s' % (repo, ref, path, blocked_id),
                           token=admin)
        check('高危审查单禁止下载 (阻断)', st == 403 and '拦截' in json.dumps(res2, ensure_ascii=False), res2.get('error'))
        st, dec2, _ = call('POST', '/admin/import-requests/%s/decision' % blocked_id, {'approve': True}, token=admin)
        check('被阻断的审查单不允许人工放行', st == 400, dec2.get('error'))

    # ---------- 6b. 审查失败(未完成扫描) 不得冒充安全结论 ----------
    ghost_repo = 'skillhub-definitely-not-exists/repo-xyz'
    st, sub3, _ = call('POST', '/github/import-requests',
                       {'repository': ghost_repo, 'ref': 'main', 'path': 'SKILL.md'}, token=admin)
    failed = sub3.get('data') or {} if isinstance(sub3.get('data'), dict) else {}
    fid = failed.get('id')
    if fid:
        st, det3, _ = call('GET', '/admin/import-requests/%s' % fid, token=admin)
        d3 = det3.get('data', det3)
        check('审查失败 -> 状态 scan_failed (不冒充已阻断)', d3.get('status') == 'scan_failed', d3.get('status'))
        check('审查失败 -> 无扫描记录且风险归零', not d3.get('scan_id') and (d3.get('risk_score') or 0) == 0,
              'risk=%s scan_id=%s' % (d3.get('risk_score'), d3.get('scan_id')))
        st, res3, _ = call('GET', '/github/skills/download?repo=%s&ref=main&path=SKILL.md&request_id=%s' % (ghost_repo, fid),
                           token=admin)
        check('审查失败不可下载 (提示重试)', st == 403 and '审查' in json.dumps(res3, ensure_ascii=False), res3.get('error'))
        st, dec3, _ = call('POST', '/admin/import-requests/%s/decision' % fid, {'approve': True}, token=admin)
        check('审查失败不允许人工放行', st == 400, dec3.get('error'))
        st, re3, _ = call('POST', '/admin/import-requests/%s/scan' % fid, token=admin)
        check('审查失败可一键重试 (不卡死)', st == 200, (re3.get('status') if isinstance(re3, dict) else re3))
        # 清理本次测试产生的 ghost 审查单, 不污染演示队列
        psql("DELETE FROM github_import_requests WHERE repository='%s'" % ghost_repo)

    st, overview, _ = call('GET', '/admin/security-overview', token=admin)
    check('安全治理概览', st == 200 and overview.get('total_scans', 0) >= 4,
          'scanned=%s safe=%s blocked=%s imports=%s downloads=%s' % (overview.get('scanned'), overview.get('safe'),
                                                                    overview.get('blocked'), overview.get('import_approved'),
                                                                    overview.get('tracked_downloads')))

    print('\n===== 结果: %d 通过 / %d 失败 =====' % (len(PASSED), len(FAILED)))
    if FAILED:
        print('失败项:', FAILED)
    return 1 if FAILED else 0


if __name__ == '__main__':
    sys.exit(main())
