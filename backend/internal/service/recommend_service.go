package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jjbank/skill-market/internal/config"
	"github.com/jjbank/skill-market/internal/model"
	sec "github.com/jjbank/skill-market/internal/security"
)

// ============================================================
// 技能推荐 Agent (自然语言需求 → 跨源匹配 → 推荐)
//
//   来源: 企业技能库 (内部) + GitHub 开源技能。
//   规则:
//     1) GitHub 技能在推荐前先做「安全核查」(静态查毒 + AST/语义 + AI 语义审计),
//        判定 malicious 的直接剔除, suspicious 降权并标注;
//     2) 「收藏(star)多 / 下载(安装)多」的技能在同等相关度下优先;
//     3) 配 LLM_API_KEY 走大模型读懂需求并从(已核查的)候选里挑选, 否则本地检索。
// ============================================================

const recommendEngineVersion = "RECO-ENGINE 2.0.0"

const (
	sourceInternal = "internal"
	sourceGitHub   = "github"
)

// SkillCandidate 内部候选技能
type SkillCandidate struct {
	ID             string
	SkillKey       string
	Name           string
	Category       string
	SubCategory    string
	Tags           []string
	Summary        string
	Description    string
	SkillType      string
	InstallCount   int64
	RatingAvg      float64
	RatingCount    int
	SecurityStatus string
	AuditStatus    string
}

// SkillRecommendation 单条推荐 (跨源)
type SkillRecommendation struct {
	SkillID        string   `json:"skill_id"`
	SkillKey       string   `json:"skill_key,omitempty"`
	Name           string   `json:"name"`
	Category       string   `json:"category"`
	Summary        string   `json:"summary,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	SkillType      string   `json:"skill_type,omitempty"`
	InstallCount   int64    `json:"install_count"`
	RatingAvg      float64  `json:"rating_avg"`
	MatchScore     int      `json:"match_score"`
	Reason         string   `json:"reason,omitempty"`

	// 来源
	Source        string `json:"source"` // internal / github
	Repository    string `json:"repository,omitempty"`
	RepositoryURL string `json:"repository_url,omitempty"`
	Path          string `json:"path,omitempty"`
	Ref           string `json:"ref,omitempty"`
	Stars         int    `json:"stars,omitempty"`

	// 安全核查 (GitHub 技能推荐前必做)
	SecurityStatus   string `json:"security_status,omitempty"` // safe / suspicious / malicious / unverified
	SecurityVerified bool   `json:"security_verified"`
	SecuritySummary  string `json:"security_summary,omitempty"`
	RiskScore        int    `json:"risk_score,omitempty"`
}

// RecommendOptions 推荐参数
type RecommendOptions struct {
	TopN         int
	Sources      []string
	VerifyGitHub bool
}

// RecommendResult 推荐结果
type RecommendResult struct {
	Query           string                `json:"query"`
	Engine          string                `json:"engine"`
	Provider        string                `json:"provider"`
	Model           string                `json:"model"`
	Sources         []string              `json:"sources"`
	Interpretation  string                `json:"interpretation,omitempty"`
	Recommendations []SkillRecommendation `json:"recommendations"`
	SuggestedQueries []string             `json:"suggested_queries,omitempty"`
	Notice          string                `json:"notice,omitempty"`
	GitHubNotice    string                `json:"github_notice,omitempty"`
	DurationMs      int                   `json:"duration_ms"`
	CandidateCount  int                   `json:"candidate_count"`
	VerifiedCount   int                   `json:"verified_count"`
}

// RecommendService 技能推荐
type RecommendService struct {
	db       *sql.DB
	cfg      *config.Config
	github   *GitHubService
	security *SecurityService
}

// recoCandidate 打分中间态
type recoCandidate struct {
	rec       SkillRecommendation
	relRaw    float64
	popMetric float64
}

// NewRecommendService 创建服务
func NewRecommendService(db *sql.DB, cfg *config.Config, github *GitHubService, security *SecurityService) *RecommendService {
	return &RecommendService{db: db, cfg: cfg, github: github, security: security}
}

// Mode 当前模式
func (s *RecommendService) Mode() string {
	if s.llmEnabled() {
		return "llm"
	}
	return "local"
}

func (s *RecommendService) llmEnabled() bool {
	return s.cfg != nil && strings.TrimSpace(s.cfg.LLM.APIKey) != "" && strings.TrimSpace(s.cfg.LLM.APIBase) != ""
}

// Status 推荐引擎状态
func (s *RecommendService) Status() map[string]interface{} {
	mode := s.Mode()
	out := map[string]interface{}{
		"engine":  recommendEngineVersion,
		"mode":    mode,
		"sources": []string{"internal", "github"},
	}
	if mode == "llm" {
		out["model"] = s.cfg.LLM.Model
		out["api_base"] = s.cfg.LLM.APIBase
	} else {
		out["model"] = "local-retrieval-v2"
		out["notice"] = "未配置大模型 API Key, 当前使用本地检索推荐; 配置 LLM_API_KEY 后可启用大模型需求理解与推荐"
	}
	if s.github != nil {
		out["github"] = s.github.HostStatus()
	}
	return out
}

// Recommend 根据自然语言需求跨源推荐技能
func (s *RecommendService) Recommend(ctx context.Context, query string, opts RecommendOptions) (*RecommendResult, error) {
	start := time.Now()
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("请描述你的需求")
	}
	if len([]rune(query)) > 500 {
		return nil, fmt.Errorf("需求描述过长 (上限 500 字)")
	}
	if opts.TopN <= 0 || opts.TopN > 10 {
		opts.TopN = 5
	}
	sources := normalizeSources(opts.Sources)

	result := &RecommendResult{
		Query: query, Engine: recommendEngineVersion, Provider: "local", Model: "local-retrieval-v2",
		Sources: sources, Recommendations: []SkillRecommendation{},
	}

	type cand = recoCandidate
	var pool []cand

	// ---- 企业库候选 ----
	if contains(sources, sourceInternal) {
		internal, err := s.internalCandidates(ctx)
		if err == nil {
			for _, c := range internal {
				rel, matched := relevanceScore(query, c.Name, strings.Join(c.Tags, " "), c.Category+" "+c.SubCategory,
					c.Summary, truncate(c.Description, 1500), c.SkillType)
				if rel <= 0 {
					continue
				}
				rec := SkillRecommendation{
					SkillID: c.ID, SkillKey: c.SkillKey, Name: c.Name, Category: c.Category, Summary: c.Summary,
					Tags: c.Tags, SkillType: c.SkillType, InstallCount: c.InstallCount, RatingAvg: c.RatingAvg,
					Source: sourceInternal, SecurityStatus: c.SecurityStatus,
					SecurityVerified: c.SecurityStatus == "safe" || c.SecurityStatus == "suspicious",
					Reason:           "匹配关键词：" + strings.Join(matched, "、"),
				}
				pool = append(pool, cand{rec: rec, relRaw: rel, popMetric: float64(c.InstallCount)})
			}
		}
	}

	// ---- GitHub 候选 (先检索, 再核查安全性) ----
	var ghNotice string
	if contains(sources, sourceGitHub) && s.github != nil {
		ghSkills, notice, err := s.gitHubCandidates(ctx, query)
		ghNotice = notice
		if err == nil {
			for _, g := range ghSkills {
				rel, matched := relevanceScore(query, g.Name, strings.Join(g.Tags, " "), g.Category, g.Description, "", "")
				if rel <= 0 {
					continue
				}
				rec := SkillRecommendation{
					SkillID: g.ID, Name: g.Name, Category: g.Category, Summary: g.Description, Tags: g.Tags,
					SkillType: g.Format, Source: sourceGitHub, Repository: g.Repository, RepositoryURL: g.RepositoryURL,
					Path: g.Path, Ref: g.Ref, Stars: g.Stars,
					Reason: "匹配关键词：" + strings.Join(matched, "、"),
				}
				pool = append(pool, cand{rec: rec, relRaw: rel, popMetric: float64(g.Stars)})
			}
		} else if ghNotice == "" {
			ghNotice = "GitHub 检索失败: " + truncate(err.Error(), 120)
		}
	}

	result.CandidateCount = len(pool)
	result.GitHubNotice = ghNotice
	if len(pool) == 0 {
		result.Provider = s.providerLabel()
		if s.llmEnabled() {
			result.Model = s.cfg.LLM.Model
		}
		result.Notice = "没有找到特别匹配的技能, 可以换个说法或补充场景细节"
		result.SuggestedQueries = defaultSuggestedQueries
		result.DurationMs = int(time.Since(start).Milliseconds())
		return result, nil
	}

	// ---- 相关度阈值过滤 (剔除“蹭热度”的不相关仓库) ----
	maxRel := 0.0
	for _, c := range pool {
		if c.relRaw > maxRel {
			maxRel = c.relRaw
		}
	}
	if maxRel > 0 {
		kept := make([]cand, 0, len(pool))
		for _, c := range pool {
			if c.relRaw >= 0.35*maxRel {
				kept = append(kept, c)
			}
		}
		pool = kept
	}

	// ---- 相关度 + 热度(收藏/下载) 归一化打分 ----
	maxPop := 0.0
	for _, c := range pool {
		if l := math.Log10(c.popMetric + 1); l > maxPop {
			maxPop = l
		}
	}
	for i := range pool {
		rel := 0.0
		if maxRel > 0 {
			rel = pool[i].relRaw / maxRel
		}
		pop := 0.0
		if maxPop > 0 {
			pop = math.Log10(pool[i].popMetric+1) / maxPop
		}
		// 相关度为主, 热度(收藏/下载)显著加权 -> 同等相关时优先热门
		pool[i].rec.MatchScore = int(math.Round((0.6*rel + 0.4*pop) * 100))
	}
	sort.SliceStable(pool, func(i, j int) bool { return pool[i].rec.MatchScore > pool[j].rec.MatchScore })

	// ---- 对入选的 GitHub 候选做安全核查 (推荐前必做) ----
	shortlist := pool
	if len(shortlist) > 12 {
		shortlist = shortlist[:12]
	}
	if opts.VerifyGitHub && contains(sources, sourceGitHub) && s.security != nil && s.github != nil {
		result.VerifiedCount = s.verifyGitHubShortlist(ctx, shortlist)
	}

	// 安全结论参与排序: 可疑降权, 未核查(未能抓包)明显降权; 恶意剔除
	filtered := make([]cand, 0, len(shortlist))
	for _, c := range shortlist {
		if c.rec.Source == sourceGitHub {
			switch c.rec.SecurityStatus {
			case "malicious":
				continue
			case "suspicious":
				c.rec.MatchScore = int(float64(c.rec.MatchScore) * 0.75)
			case "unverified":
				c.rec.MatchScore = int(float64(c.rec.MatchScore) * 0.6)
			}
		}
		filtered = append(filtered, c)
	}
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].rec.MatchScore > filtered[j].rec.MatchScore })

	var final []SkillRecommendation
	if s.llmEnabled() {
		result.Provider = "llm"
		result.Model = s.cfg.LLM.Model
		if items, interp, err := s.rankWithLLM(ctx, query, filtered, opts.TopN); err == nil && len(items) > 0 {
			result.Interpretation = interp
			final = items
		} else {
			result.Provider = "local"
			result.Model = "local-retrieval-v2"
			if err != nil {
				result.Notice = "大模型推荐失败, 已降级为本地检索: " + truncate(err.Error(), 140)
			}
		}
	} else {
		result.Provider = "local"
	}
	if len(final) == 0 {
		final = topOf(filtered, opts.TopN)
	}

	result.Recommendations = final
	if len(final) == 0 {
		result.Notice = "没有找到特别匹配的技能, 可以换个说法或补充场景细节"
		result.SuggestedQueries = defaultSuggestedQueries
	}
	result.DurationMs = int(time.Since(start).Milliseconds())
	return result, nil
}

func (s *RecommendService) providerLabel() string {
	if s.llmEnabled() {
		return "llm"
	}
	return "local"
}

// verifyGitHubShortlist 对 GitHub 候选逐个抓包并快速安全核查, 回填结果; 返回核查成功数
func (s *RecommendService) verifyGitHubShortlist(ctx context.Context, items []recoCandidate) int {
	verified := 0
	budget := 0
	for i := range items {
		if items[i].rec.Source != sourceGitHub {
			continue
		}
		if budget >= 8 { // 控制耗时/配额: 最多核查 8 个
			items[i].rec.SecurityStatus = "unverified"
			continue
		}
		budget++
		repo, ref, path := items[i].rec.Repository, items[i].rec.Ref, items[i].rec.Path
		if repo == "" {
			items[i].rec.SecurityStatus = "unverified"
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, 25*time.Second)
		files, err := s.github.FetchSkillFiles(cctx, repo, ref, path)
		if err != nil || len(files) == 0 {
			cancel()
			items[i].rec.SecurityStatus = "unverified"
			items[i].rec.SecuritySummary = "未能抓取技能包, 未完成安全核查"
			continue
		}
		subject := sec.ScanSubject{Type: "recommend", SkillKey: repo + ":" + path, SkillName: items[i].rec.Name,
			Target: "github:" + repo, Trigger: "recommend"}
		scan := s.security.QuickScan(cctx, subject, files)
		cancel()
		verified++
		items[i].rec.SecurityStatus = scan.Verdict
		items[i].rec.SecurityVerified = true
		items[i].rec.RiskScore = scan.RiskScore
		items[i].rec.SecuritySummary = scan.Summary
	}
	return verified
}

func topOf(items []recoCandidate, n int) []SkillRecommendation {
	out := []SkillRecommendation{}
	for i := 0; i < len(items) && len(out) < n; i++ {
		out = append(out, items[i].rec)
	}
	return out
}

// ---------- 内部候选 ----------

func (s *RecommendService) internalCandidates(ctx context.Context) ([]SkillCandidate, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, s.skill_key, s.name, COALESCE(s.category,''), COALESCE(s.sub_category,''),
		       COALESCE(s.tags, '{}'), COALESCE(s.summary,''), COALESCE(s.description,''), COALESCE(s.skill_type,''),
		       COALESCE(s.install_count,0), COALESCE(s.rating_avg,0), COALESCE(s.rating_count,0),
		       COALESCE(s.security_status,'unscanned'), COALESCE(s.audit_status,'pending')
		FROM skills s
		WHERE s.status = 'published' AND s.visibility = 'public'
		  AND COALESCE(s.quarantined,FALSE) = FALSE
		  AND COALESCE(s.security_status,'') <> 'malicious'
		ORDER BY s.install_count DESC
		LIMIT 200`)
	if err != nil {
		return nil, fmt.Errorf("加载候选技能失败: %w", err)
	}
	defer rows.Close()
	out := []SkillCandidate{}
	for rows.Next() {
		var c SkillCandidate
		var tags string
		if err := rows.Scan(&c.ID, &c.SkillKey, &c.Name, &c.Category, &c.SubCategory, &tags, &c.Summary,
			&c.Description, &c.SkillType, &c.InstallCount, &c.RatingAvg, &c.RatingCount,
			&c.SecurityStatus, &c.AuditStatus); err != nil {
			return nil, err
		}
		c.Tags = parsePGTags(tags)
		out = append(out, c)
	}
	return out, rows.Err()
}

func parsePGTags(raw string) []string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "{}")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.Trim(strings.TrimSpace(p), `"`)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ---------- GitHub 候选 ----------

func (s *RecommendService) gitHubCandidates(ctx context.Context, query string) ([]model.GitHubSkill, string, error) {
	gq := buildGitHubQuery(query)
	req := &model.GitHubSkillSearchRequest{Query: gq, Page: 1, PageSize: 25}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	res, err := s.github.Search(cctx, req)
	if err != nil {
		return nil, "", err
	}
	if res == nil {
		return nil, "", fmt.Errorf("GitHub 返回为空")
	}
	// 去重: 同一仓库只保留一条 (避免同仓库多个 SKILL.md 刷屏)
	seen := map[string]bool{}
	uniq := make([]model.GitHubSkill, 0, len(res.Data))
	for _, g := range res.Data {
		key := strings.ToLower(g.Repository)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		uniq = append(uniq, g)
	}
	if len(uniq) > 25 {
		uniq = uniq[:25]
	}
	// 补齐 star (代码搜索不返回), 供「收藏多优先」排序; 并发限流
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i := range uniq {
		if uniq[i].Stars > 0 || uniq[i].Repository == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()
			sc, cancel := context.WithTimeout(ctx, 12*time.Second)
			defer cancel()
			stars, _ := s.github.RepositoryStats(sc, uniq[idx].Repository)
			uniq[idx].Stars = stars
		}(i)
	}
	wg.Wait()
	return uniq, res.Notice, nil
}

// zhHints 中文关键词 → 英文检索提示 (按优先级排序, 提升 GitHub 代码搜索命中率)
var zhHints = []struct{ zh, en string }{
	{"脱敏", "desensitize"}, {"日志", "log"}, {"敏感", "sensitive"}, {"身份证", "pii"}, {"手机号", "phone"},
	{"数据库", "database"}, {"慢查询", "slow query"}, {"巡检", "inspection"}, {"告警", "alert"}, {"收敛", "convergence"},
	{"需求", "requirement"}, {"分析", "analysis"}, {"安全", "security"}, {"审计", "audit"},
	{"报告", "report"}, {"报表", "report"}, {"文档", "document"}, {"监控", "monitor"}, {"运维", "ops"},
	{"爬虫", "crawler"}, {"数据", "data"}, {"清洗", "clean"}, {"转换", "convert"}, {"翻译", "translate"},
	{"总结", "summarize"}, {"检索", "search"}, {"知识库", "knowledge base"}, {"营销", "marketing"},
}

func buildGitHubQuery(query string) string {
	lower := strings.ToLower(query)
	seen := map[string]bool{}
	parts := []string{}
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		parts = append(parts, v)
	}
	// 英文关键词优先 (GitHub 命中率高); 少量词保证召回
	for _, w := range reASCIIWord.FindAllString(lower, -1) {
		if len(parts) >= 2 {
			break
		}
		add(w)
	}
	// 中文 → 英文提示 (按优先级), 控制词数以提升召回
	for _, h := range zhHints {
		if len(parts) >= 4 {
			break
		}
		if strings.Contains(lower, h.zh) {
			add(h.en)
		}
	}
	// 退而求其次: 中文原词
	if len(parts) == 0 {
		for _, run := range reHanRun.FindAllString(query, -1) {
			if len([]rune(run)) >= 2 {
				add(run)
			}
			if len(parts) >= 3 {
				break
			}
		}
	}
	if len(parts) == 0 {
		return "agent skill"
	}
	if len(parts) > 4 {
		parts = parts[:4]
	}
	return strings.Join(parts, " ")
}

// ---------- 相关度打分 ----------

var (
	reASCIIWord = regexp.MustCompile(`[a-z0-9_+\-]{2,}`)
	reHanRun    = regexp.MustCompile(`\p{Han}+`)
)

func tokenizeQuery(q string) []string {
	q = strings.ToLower(q)
	set := map[string]bool{}
	for _, w := range reASCIIWord.FindAllString(q, -1) {
		set[w] = true
	}
	for _, run := range reHanRun.FindAllString(q, -1) {
		rs := []rune(run)
		if len(rs) == 1 {
			set[string(rs)] = true
			continue
		}
		set[run] = true
		for i := 0; i+1 < len(rs); i++ {
			set[string(rs[i:i+2])] = true
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}

// relevanceScore 字段加权命中: name 6 / tags 4 / category 3 / summary 2 / desc 1 / type 2
func relevanceScore(query, name, tags, category, summary, desc, skillType string) (float64, []string) {
	tokens := tokenizeQuery(query)
	if len(tokens) == 0 {
		return 0, nil
	}
	name, tags, category, summary, desc, skillType = strings.ToLower(name), strings.ToLower(tags),
		strings.ToLower(category), strings.ToLower(summary), strings.ToLower(desc), strings.ToLower(skillType)
	raw := 0.0
	matched := []string{}
	for _, t := range tokens {
		hit := 0.0
		if strings.Contains(name, t) {
			hit += 6
		}
		if strings.Contains(tags, t) {
			hit += 4
		}
		if strings.Contains(category, t) {
			hit += 3
		}
		if strings.Contains(skillType, t) {
			hit += 2
		}
		if strings.Contains(summary, t) {
			hit += 2
		}
		if strings.Contains(desc, t) {
			hit += 1
		}
		if hit > 0 {
			raw += hit
			if len(matched) < 5 {
				matched = append(matched, t)
			}
		}
	}
	return raw, matched
}

// ---------- 大模型排序 ----------

type recommendLLMRequest struct {
	Model       string       `json:"model"`
	Messages    []llmMessage `json:"messages"`
	Temperature float64      `json:"temperature"`
}

func (s *RecommendService) rankWithLLM(ctx context.Context, query string, items []recoCandidate, topN int) ([]SkillRecommendation, string, error) {
	if len(items) == 0 {
		return nil, "", nil
	}
	var sb strings.Builder
	byID := map[string]SkillRecommendation{}
	for _, c := range items {
		r := c.rec
		byID[r.SkillID] = r
		pop := ""
		if r.Source == sourceGitHub {
			pop = fmt.Sprintf("star=%d", r.Stars)
		} else {
			pop = fmt.Sprintf("安装=%d", r.InstallCount)
		}
		fmt.Fprintf(&sb, "- id=%s | 来源=%s | 名称=%s | 分类=%s | %s | 安全=%s | 简介=%s\n",
			r.SkillID, r.Source, r.Name, r.Category, pop,
			fallbackStr(r.SecurityStatus, "未核查"), truncate(r.Summary, 120))
	}
	system := "你是九江银行 SkillHub 的技能推荐助手。用户用自然语言描述需求, 你要从候选(含企业库与 GitHub 开源技能)中" +
		"挑出最合适的并给出理由。同等相关时优先 star/安装量高的; GitHub 技能若安全结论为 suspicious 要说明并谨慎推荐。" +
		"严格只输出一个 JSON 对象: " +
		`{"interpretation":"对用户需求的一句话理解","items":[{"skill_id":"...","score":0-100,"reason":"为什么适合(含安全/热度说明)"}]}` +
		"；skill_id 必须来自清单; 最多 " + fmt.Sprint(topN) + " 条; 按 score 降序; 没有合适的则 items 为空。"
	user := "用户需求：" + query + "\n\n候选技能：\n" + sb.String()

	body, _ := json.Marshal(recommendLLMRequest{
		Model: s.cfg.LLM.Model, Temperature: 0.2,
		Messages: []llmMessage{{Role: "system", Content: system}, {Role: "user", Content: user}},
	})
	url := strings.TrimRight(s.cfg.LLM.APIBase, "/") + "/chat/completions"
	reqCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.cfg.LLM.APIKey)
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, "", fmt.Errorf("大模型不可达: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("大模型返回 HTTP %d: %s", resp.StatusCode, truncate(string(raw), 160))
	}
	var out struct {
		Choices []struct {
			Message llmMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Choices) == 0 {
		return nil, "", fmt.Errorf("大模型响应解析失败")
	}
	payload := extractJSONObject(contentText(out.Choices[0].Message.Content))
	if payload == "" {
		return nil, "", fmt.Errorf("大模型未返回结构化结论")
	}
	var parsed struct {
		Interpretation string `json:"interpretation"`
		Items          []struct {
			SkillID string `json:"skill_id"`
			Score   int    `json:"score"`
			Reason  string `json:"reason"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return nil, "", fmt.Errorf("推荐结论解析失败: %v", err)
	}
	recs := []SkillRecommendation{}
	seen := map[string]bool{}
	for _, it := range parsed.Items {
		r, ok := byID[it.SkillID]
		if !ok || seen[it.SkillID] {
			continue
		}
		seen[it.SkillID] = true
		if it.Score > 0 {
			r.MatchScore = it.Score
		}
		if it.Reason != "" {
			r.Reason = it.Reason
		}
		recs = append(recs, r)
		if len(recs) >= topN {
			break
		}
	}
	return recs, truncate(parsed.Interpretation, 300), nil
}

// ---------- 小工具 ----------

func normalizeSources(raw []string) []string {
	out := []string{}
	for _, s := range raw {
		v := strings.ToLower(strings.TrimSpace(s))
		if (v == sourceInternal || v == sourceGitHub) && !contains(out, v) {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		out = []string{sourceInternal, sourceGitHub}
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func fallbackStr(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

var defaultSuggestedQueries = []string{
	"把生产日志里的身份证和手机号脱敏",
	"巡检核心银行数据库的健康状况",
	"分析支付节点最近的告警并收敛",
	"把业务需求整理成 PRD 要点",
}
