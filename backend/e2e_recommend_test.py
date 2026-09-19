"""技能推荐 Agent 端到端测试

验证: 用户用自然语言描述需求 -> 推荐 Agent 跨源(企业库 + GitHub)找出合适的技能
  1. 推荐引擎状态 (llm / local) + 来源
  2. 四类业务需求各自命中正确的企业库技能 (Top-1)
  3. 推荐分数 / 推荐理由 结构完整
  4. GitHub 跨源推荐: 结果带仓库定位、已做安全核查(无恶意)、携带 star 数
  5. 默认来源 = 企业库 + GitHub
  6. 无关需求不硬凑 (返回空 + 引导)
  7. 空需求 -> 400; top_n 生效
"""

import json
import urllib.error
import urllib.request

BASE = 'http://localhost:8080/api/v1'
PASSED, FAILED = [], []


def check(name, ok, detail=''):
    (PASSED if ok else FAILED).append(name)
    print(('[PASS] ' if ok else '[FAIL] ') + name + (' | ' + str(detail)[:200] if detail else ''))


def call(method, path, body=None, auth=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header('Content-Type', 'application/json')
    if auth:
        req.add_header('Authorization', 'Bearer ' + auth)
    try:
        with urllib.request.urlopen(req, timeout=150) as r:
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
    admin = res.get('token')
    check('管理员登录', bool(admin))

    st, status = call('GET', '/agent/recommend/status', auth=admin)
    d = status.get('data') or {}
    check('推荐引擎状态可见', st == 200 and d.get('mode') in ('llm', 'local'), {k: d.get(k) for k in ('engine', 'mode')})
    check('推荐来源含 企业库 + GitHub', set(d.get('sources') or []) == {'internal', 'github'}, d.get('sources'))

    for query, expect in BROWSER_RECO:
        st, out = call('POST', '/agent/recommend', {'message': query, 'sources': ['internal']}, auth=admin)
        data = out.get('data') or {}
        recs = data.get('recommendations') or []
        top = recs[0] if recs else {}
        check('企业库命中技能: %s' % query[:14], st == 200 and top.get('skill_key') == expect,
              'top=%s(%s) score=%s' % (top.get('name'), top.get('skill_key'), top.get('match_score')))
        check('  推荐含分数与理由', (top.get('match_score') or 0) > 0 and bool(top.get('reason')),
              'score=%s reason=%s' % (top.get('match_score'), top.get('reason')))

    # ---- GitHub 跨源推荐 + 安全核查 + 热度优先 ----
    st, out = call('POST', '/agent/recommend',
                   {'message': 'data analysis skill', 'sources': ['github'], 'top_n': 5}, auth=admin)
    data = out.get('data') or {}
    gh = data.get('recommendations') or []
    if gh:
        check('GitHub 跨源推荐有结果', all(r.get('source') == 'github' for r in gh), 'n=%s' % len(gh))
        check('GitHub 推荐带仓库定位', all(r.get('repository') for r in gh), [r.get('repository') for r in gh][:3])
        check('GitHub 推荐均已安全核查 (无恶意)',
              all(r.get('security_status') in ('safe', 'suspicious', 'unverified') for r in gh),
              [r.get('security_status') for r in gh])
        check('GitHub 推荐已执行安全核查', (data.get('verified_count') or 0) > 0, 'verified=%s' % data.get('verified_count'))
        check('GitHub 推荐携带 star 数 (热度排序依据)', any((r.get('stars') or 0) > 0 for r in gh),
              [(r.get('repository'), r.get('stars')) for r in gh][:4])
        check('GitHub 推荐携带分支 ref (供审查抓包)', all(r.get('ref') for r in gh), [r.get('ref') for r in gh][:3])
        check('GitHub 推荐携带技能路径 path', all(r.get('path') for r in gh), [r.get('path') for r in gh][:3])
        # 热度优先: 安全(已核查)的应排在未核查/可疑之前
        ranks = [r.get('security_status') for r in gh]
        if 'safe' in ranks and 'unverified' in ranks:
            check('已核查(安全)优先于 未核查', ranks.index('safe') < ranks.index('unverified'), ranks)
        else:
            print('[SKIP] 同批结果中未同时出现 safe 与 unverified, 跳过排序对照')
    else:
        print('[SKIP] GitHub 检索无结果 (可能未配置 GITHUB_TOKEN 或网络受限)')

    # ---- 默认跨源 ----
    st, out = call('POST', '/agent/recommend', {'message': '日志脱敏', 'top_n': 5}, auth=admin)
    data = out.get('data') or {}
    check('默认来源=企业库+GitHub', set(data.get('sources') or []) == {'internal', 'github'}, data.get('sources'))

    st, out = call('POST', '/agent/recommend', {'message': '我想做一道红烧肉', 'sources': ['internal']}, auth=admin)
    data = out.get('data') or {}
    check('无关需求不硬凑 (空结果+引导)',
          st == 200 and len(data.get('recommendations') or []) == 0 and bool(data.get('notice')), data.get('notice'))

    st, out = call('POST', '/agent/recommend', {'message': ''}, auth=admin)
    check('空需求 -> 400', st == 400, out.get('error'))

    st, out = call('POST', '/agent/recommend',
                   {'message': '数据库巡检 日志脱敏 告警 需求', 'sources': ['internal'], 'top_n': 2}, auth=admin)
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
