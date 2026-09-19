package service

import (
	"strings"
	"testing"
)

func TestTokenizeQueryChineseBigrams(t *testing.T) {
	toks := tokenizeQuery("把生产日志里的手机号脱敏")
	set := map[string]bool{}
	for _, x := range toks {
		set[x] = true
	}
	for _, want := range []string{"脱敏", "日志", "手机"} {
		if !set[want] {
			t.Errorf("expected token %q in %v", want, toks)
		}
	}
	set2 := map[string]bool{}
	for _, x := range tokenizeQuery("生成 PRD 文档") {
		set2[x] = true
	}
	if !set2["prd"] {
		t.Errorf("expected ascii token prd")
	}
}

type demoSkill struct {
	key, name, tags, category, summary, desc, stype string
}

var demoSkills = []demoSkill{
	{"log-desensitization", "日志敏感信息识别", "脱敏,日志,敏感,PII", "安全合规", "识别并脱敏日志中的身份证、手机号等敏感信息", "", "mcp"},
	{"db-inspection", "数据库智能巡检助手", "巡检,数据库,慢查询,健康", "智能运维", "对数据库做健康巡检与慢查询分析", "", "mcp"},
	{"requirement-analysis", "AI 需求分析助手", "需求,PRD,业务规则", "研发效能", "把业务需求整理成 PRD 要点", "", "mcp"},
	{"alert-convergence", "告警收敛分析", "告警,收敛,根因", "智能运维", "对海量告警做收敛与根因分析", "", "mcp"},
}

func topByRelevance(query string) string {
	bestKey, best := "", 0.0
	for _, s := range demoSkills {
		raw, _ := relevanceScore(query, s.name, s.tags, s.category, s.summary, s.desc, s.stype)
		if raw > best {
			best, bestKey = raw, s.key
		}
	}
	return bestKey
}

func TestRelevancePicksRightSkill(t *testing.T) {
	for _, c := range []struct{ q, want string }{
		{"帮我把生产日志里的身份证和手机号脱敏", "log-desensitization"},
		{"核心数据库跑不动了 想看看慢查询和健康状况", "db-inspection"},
		{"把这段业务需求整理成 PRD", "requirement-analysis"},
		{"分析支付节点的告警噪声", "alert-convergence"},
	} {
		if got := topByRelevance(c.q); got != c.want {
			t.Errorf("query %q: expected %s, got %s", c.q, c.want, got)
		}
	}
}

func TestRelevanceNoMatch(t *testing.T) {
	if got := topByRelevance("我想做一道红烧肉"); got != "" {
		t.Errorf("expected no match, got %s", got)
	}
}

func TestBuildGitHubQueryMapsChinese(t *testing.T) {
	q := buildGitHubQuery("把生产日志里的身份证和手机号脱敏")
	if !strings.Contains(q, "desensitize") || !strings.Contains(q, "log") {
		t.Errorf("expected github query %q to contain desensitize/log", q)
	}
	q2 := buildGitHubQuery("数据库巡检")
	if !strings.Contains(q2, "database") || !strings.Contains(q2, "inspection") {
		t.Errorf("expected github query %q to contain database/inspection", q2)
	}
}

func TestNormalizeSources(t *testing.T) {
	if got := normalizeSources(nil); len(got) != 2 {
		t.Errorf("expected default both sources, got %v", got)
	}
	if got := normalizeSources([]string{"github", "github", "x"}); len(got) != 1 || got[0] != "github" {
		t.Errorf("expected [github], got %v", got)
	}
}
