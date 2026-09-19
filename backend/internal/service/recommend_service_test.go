package service

import "testing"

func TestTokenizeQueryChineseBigrams(t *testing.T) {
	toks := tokenizeQuery("把生产日志里的手机号脱敏")
	set := map[string]bool{}
	for _, x := range toks {
		set[x] = true
	}
	// 应含中文二元组与整段
	for _, want := range []string{"脱敏", "日志", "手机"} {
		if !set[want] {
			t.Errorf("expected token %q in %v", want, toks)
		}
	}
	// 英文词
	toks2 := tokenizeQuery("生成 PRD 文档")
	set2 := map[string]bool{}
	for _, x := range toks2 {
		set2[x] = true
	}
	if !set2["prd"] {
		t.Errorf("expected ascii token prd in %v", toks2)
	}
}

func recoCandidates() []SkillCandidate {
	return []SkillCandidate{
		{ID: "s-002", SkillKey: "log-desensitization", Name: "日志敏感信息识别", Category: "安全合规",
			Tags: []string{"脱敏", "日志", "敏感", "PII"}, Summary: "识别并脱敏日志中的身份证、手机号等敏感信息", InstallCount: 40, RatingAvg: 4.8},
		{ID: "s-001", SkillKey: "db-inspection", Name: "数据库智能巡检助手", Category: "智能运维",
			Tags: []string{"巡检", "数据库", "慢查询", "健康"}, Summary: "对数据库做健康巡检与慢查询分析", InstallCount: 55, RatingAvg: 4.9},
		{ID: "s-004", SkillKey: "requirement-analysis", Name: "AI 需求分析助手", Category: "研发效能",
			Tags: []string{"需求", "PRD", "业务规则"}, Summary: "把业务需求整理成 PRD 要点", InstallCount: 22, RatingAvg: 4.5},
		{ID: "s-003", SkillKey: "alert-convergence", Name: "告警收敛分析", Category: "智能运维",
			Tags: []string{"告警", "收敛", "根因"}, Summary: "对海量告警做收敛与根因分析", InstallCount: 31, RatingAvg: 4.6},
	}
}

func TestRecommendLocalPicksRightSkill(t *testing.T) {
	svc := &RecommendService{}
	cases := []struct {
		query string
		want  string
	}{
		{"帮我把生产日志里的身份证和手机号脱敏", "log-desensitization"},
		{"核心数据库跑不动了 想看看慢查询和健康状况", "db-inspection"},
		{"把这段业务需求整理成 PRD", "requirement-analysis"},
		{"分析支付节点的告警噪声", "alert-convergence"},
	}
	for _, c := range cases {
		recs := svc.recommendLocal(c.query, recoCandidates(), 5)
		if len(recs) == 0 {
			t.Errorf("query %q: expected recommendations", c.query)
			continue
		}
		if recs[0].SkillKey != c.want {
			t.Errorf("query %q: expected top=%s, got=%s (score=%d)", c.query, c.want, recs[0].SkillKey, recs[0].MatchScore)
		}
		if recs[0].MatchScore <= 0 || recs[0].MatchScore > 100 {
			t.Errorf("query %q: score out of range: %d", c.query, recs[0].MatchScore)
		}
	}
}

func TestRecommendLocalNoMatch(t *testing.T) {
	svc := &RecommendService{}
	recs := svc.recommendLocal("我想做一道红烧肉", recoCandidates(), 5)
	if len(recs) != 0 {
		t.Errorf("expected no recommendations for unrelated query, got %+v", recs)
	}
}
