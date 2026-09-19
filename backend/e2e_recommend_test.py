"""技能推荐 Agent 端到端测试

验证: 用户用自然语言描述需求 -> 推荐 Agent 找出合适的技能
  1. 推荐引擎状态 (llm / local)
  2. 四类业务需求各自命中正确的技能 (Top-1)
  3. 推荐分数 / 推荐理由 结构完整
  4. 无关需求不硬凑 (返回空 + 引导)
  5. 空需求 -> 400
  6. top_n 生效
"""

import json
import urllib.error
import urllib.request

BASE = 'http://localhost:8080/api/v1'
PASSED, FAILED = [], []


def check(name, ok, detail=''):
    (PASSED if ok else FAILED).append(name)
    print(('[PASS] ' if ok else '[FAIL] ') + name + (' | ' + str(detail)[:180] if detail else ''))


def call(method, path, body=None, token=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header('Content-Type', 'application/json')
    if token:
        req.add_header('Authorization', 'Bearer ' + token)
    try:
        with urllib.request.urlopen(req, timeout=90) as r:
            return r.status, json.loads(r.read() or b'{}')
    except urllib.error.HTTPError as e:
        try:
            return e.code, json.loads(e.read() or b'{}')
        except Exception:
            return e.code, {}


BROWSER_RECO = [
    ('把生产日志里的身份证和手机号脱敏', 'log-desensitization'),
    ('核心数据库跑不动了，想看看慢查询和健康状况', 'db-inspection'),
    ('把这段业务需求整理成 PRD 要点', 'requirement-analysis'),
    ('分析支付节点最近的告警噪声', 'alert-convergence'),
]


def main():
    st, res = call('POST', '/auth/login', {'username': 'admin', 'password': 'demo'})
    token = res.get('token')
    check('管理员登录', bool(token))

    st, status = call('GET', '/agent/recommend/status', token=token)
    d = status.get('data') or {}
    check('推荐引擎状态可见', st == 200 and d.get('mode') in ('llm', 'local'), d)

    for query, expect in BROWSER_RECO:
        st, out = call('POST', '/agent/recommend', {'message': query}, token=token)
        data = out.get('data') or {}
        recs = data.get('recommendations') or []
        top = recs[0] if recs else {}
        check('需求→命中技能: %s' % query[:14], st == 200 and top.get('skill_key') == expect,
              'top=%s(%s) score=%s' % (top.get('name'), top.get('skill_key'), top.get('match_score')))
        check('  推荐含分数与理由', (top.get('match_score') or 0) > 0 and bool(top.get('reason')),
              'score=%s reason=%s' % (top.get('match_score'), top.get('reason')))

    st, out = call('POST', '/agent/recommend', {'message': '我想做一道红烧肉'}, token=token)
    data = out.get('data') or {}
    check('无关需求不硬凑 (空结果+引导)',
          st == 200 and len(data.get('recommendations') or []) == 0 and bool(data.get('notice')),
          data.get('notice'))

    st, out = call('POST', '/agent/recommend', {'message': ''}, token=token)
    check('空需求 -> 400', st == 400, out.get('error'))

    st, out = call('POST', '/agent/recommend', {'message': '数据库巡检 日志脱敏 告警 需求', 'top_n': 2}, token=token)
    data = out.get('data') or {}
    check('top_n 生效 (最多返回 2 条)', st == 200 and len(data.get('recommendations') or []) <= 2,
          'n=%s' % len(data.get('recommendations') or []))

    print('\n===== 推荐 Agent e2e: %d 通过 / %d 失败 =====' % (len(PASSED), len(FAILED)))
    if FAILED:
        print('失败项:', FAILED)
        return 1
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
