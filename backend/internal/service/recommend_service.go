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
	"time"

	"github.com/jjbank/skill-market/internal/config"
)

// ============================================================
// 技能推荐 Agent (自然语言需求 → 匹配平台技能 → 推荐)
//
//   与「AI 智能体(编排执行)」互补: 这里只负责「找合适的技能并推荐」。
//   1) 候选集只取「已发布 + 公开 + 通过安全/未判恶意」的技能 (可放心推荐);
//   2) 配了 LLM_API_KEY 走大模型: 读懂需求 → 从候选里挑选并给出匹配理由;
//   3) 未配置时走本地检索打分 (中文二元组 + 字段加权 + 热度/评分加权), 演示环境永远可用。
// ============================================================

const recommendEngineVersion = "RECO-ENGINE 1.0.0"

// SkillCandidate 候选技能 (内部)
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

// SkillRecommendation 单条推荐
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
	SecurityStatus string   `json:"security_status,omitempty"`
	MatchScore     int      `json:"match_score"`
	Reason         string   `json:"reason,omitempty"`
}

// RecommendResult 推荐结果
type RecommendResult struct {
	Query            string                `json:"query"`
	Engine           string                `json:"engine"`
	Provider         string                `json:"provider"` // llm / local
	Model            string                `json:"model"`
	Interpretation   string                `json:"interpretation,omitempty"`
	Recommendations  []SkillRecommendation `json:"recommendations"`
	SuggestedQueries []string              `json:"suggested_queries,omitempty"`
	Notice           string                `json:"notice,omitempty"`
	DurationMs       int                   `json:"duration_ms"`
	CandidateCount   int                   `json:"candidate_count"`
}

// RecommendService 技能推荐
type RecommendService struct {
	db  *sql.DB
	cfg *config.Config
}

// NewRecommendService 创建服务
func NewRecommendService(db *sql.DB, cfg *config.Config) *RecommendService {
	return &RecommendService{db: db, cfg: cfg}
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
		"engine": recommendEngineVersion,
		"mode":   mode,
	}
	if mode == "llm" {
		out["model"] = s.cfg.LLM.Model
		out["api_base"] = s.cfg.LLM.APIBase
	} else {
		out["model"] = "local-retrieval-v1"
		out["notice"] = "未配置大模型 API Key, 当前使用本地检索推荐; 配置 LLM_API_KEY 后可启用大模型需求理解与推荐"
	}
	return out
}

// Recommend 根据自然语言需求推荐技能
func (s *RecommendService) Recommend(ctx context.Context, query string, topN int) (*RecommendResult, error) {
	start := time.Now()
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("请描述你的需求")
	}
	if len([]rune(query)) > 500 {
		return nil, fmt.Errorf("需求描述过长 (上限 500 字)")
	}
	if topN <= 0 || topN > 10 {
		topN = 5
	}
	candidates, err := s.candidates(ctx)
	if err != nil {
		return nil, err
	}

	result := &RecommendResult{
		Query: query, Engine: recommendEngineVersion, Provider: "local", Model: "local-retrieval-v1",
		Recommendations: []SkillRecommendation{}, CandidateCount: len(candidates),
	}
	if len(candidates) == 0 {
		result.Notice = "当前平台暂无可推荐的已发布技能"
		result.DurationMs = int(time.Since(start).Milliseconds())
		return result, nil
	}

	if s.llmEnabled() {
		result.Provider = "llm"
		result.Model = s.cfg.LLM.Model
		if items, interp, err := s.recommendWithLLM(ctx, query, candidates, topN); err == nil {
			result.Interpretation = interp
			result.Recommendations = items
		} else {
			// 大模型不可用 -> 降级本地检索, 留痕
			result.Provider = "local"
			result.Model = "local-retrieval-v1"
			result.Notice = "大模型推荐失败, 已降级为本地检索: " + truncate(err.Error(), 160)
			result.Recommendations = s.recommendLocal(query, candidates, topN)
		}
	} else {
		result.Recommendations = s.recommendLocal(query, candidates, topN)
	}

	if len(result.Recommendations) == 0 {
		result.Notice = "没有找到特别匹配的技能, 可以换个说法或补充场景细节"
		result.SuggestedQueries = defaultSuggestedQueries
	}
	result.DurationMs = int(time.Since(start).Milliseconds())
	return result, nil
}

// ---------- 候选集 ----------

func (s *RecommendService) candidates(ctx context.Context) ([]SkillCandidate, error) {
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

// parsePGTags 解析 Postgres 数组字面量 {a,b} (避免引入额外依赖)
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

// ---------- 本地检索推荐 ----------

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

// recommendLocal 本地检索打分: 字段加权命中 + 热度/评分微调
func (s *RecommendService) recommendLocal(query string, candidates []SkillCandidate, topN int) []SkillRecommendation {
	tokens := tokenizeQuery(query)
	if len(tokens) == 0 {
		return nil
	}
	type scored struct {
		cand    SkillCandidate
		raw     float64
		matched []string
	}
	all := make([]scored, 0, len(candidates))
	maxRaw := 0.0
	for _, c := range candidates {
		name := strings.ToLower(c.Name)
		category := strings.ToLower(c.Category + " " + c.SubCategory)
		summary := strings.ToLower(c.Summary)
		desc := strings.ToLower(truncate(c.Description, 1500))
		tagText := strings.ToLower(strings.Join(c.Tags, " "))
		skillType := strings.ToLower(c.SkillType)

		raw := 0.0
		matched := []string{}
		for _, t := range tokens {
			hit := 0.0
			if strings.Contains(name, t) {
				hit += 6
			}
			if strings.Contains(tagText, t) {
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
		if raw <= 0 {
			continue
		}
		// 热度/评分微调 (不喧宾夺主)
		raw += math.Log10(float64(c.InstallCount)+1) * 0.6
		raw += c.RatingAvg * 0.25
		if raw > maxRaw {
			maxRaw = raw
		}
		all = append(all, scored{cand: c, raw: raw, matched: matched})
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].raw > all[j].raw })
	out := []SkillRecommendation{}
	for i, sc := range all {
		if i >= topN {
			break
		}
		score := int(math.Round(sc.raw / maxRaw * 100))
		if score < 1 {
			score = 1
		}
		out = append(out, buildRecommendation(sc.cand, score, "匹配关键词："+strings.Join(sc.matched, "、")))
	}
	return out
}

func buildRecommendation(c SkillCandidate, score int, reason string) SkillRecommendation {
	return SkillRecommendation{
		SkillID: c.ID, SkillKey: c.SkillKey, Name: c.Name, Category: c.Category, Summary: c.Summary,
		Tags: c.Tags, SkillType: c.SkillType, InstallCount: c.InstallCount, RatingAvg: c.RatingAvg,
		SecurityStatus: c.SecurityStatus, MatchScore: score, Reason: reason,
	}
}

// ---------- 大模型推荐 ----------

type recommendLLMRequest struct {
	Model       string       `json:"model"`
	Messages    []llmMessage `json:"messages"`
	Temperature float64      `json:"temperature"`
}

func (s *RecommendService) recommendWithLLM(ctx context.Context, query string, candidates []SkillCandidate, topN int) ([]SkillRecommendation, string, error) {
	// 候选较多时先用本地检索预筛, 控制 prompt 体积
	send := candidates
	if len(send) > 60 {
		pre := s.recommendLocal(query, candidates, 60)
		keep := map[string]bool{}
		for _, r := range pre {
			keep[r.SkillID] = true
		}
		filtered := make([]SkillCandidate, 0, 60)
		for _, c := range candidates {
			if keep[c.ID] {
				filtered = append(filtered, c)
			}
		}
		if len(filtered) > 0 {
			send = filtered
		}
	}
	var sb strings.Builder
	for _, c := range send {
		tags := strings.Join(c.Tags, ",")
		fmt.Fprintf(&sb, "- id=%s | 名称=%s | 分类=%s | 类型=%s | 标签=%s | 简介=%s\n",
			c.ID, c.Name, c.Category, c.SkillType, tags, truncate(c.Summary, 120))
	}

	system := "你是九江银行 SkillHub 的技能推荐助手。用户会用自然语言描述需求 (可能是业务场景或问题), " +
		"你要从平台「已发布技能」里挑出最合适的并给出匹配理由。" +
		"严格只输出一个 JSON 对象, 结构为: " +
		`{"interpretation":"对用户需求的一句话理解","items":[{"skill_id":"...","score":0-100,"reason":"为什么适合"}]}` +
		"。要求: skill_id 必须来自给定清单; 最多返回 " + fmt.Sprint(topN) + " 条; 按 score 从高到低; 若没有合适的, items 返回空数组。不要编造清单之外的技能。"
	user := "用户需求：" + query + "\n\n可选技能清单：\n" + sb.String()

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
	byID := map[string]SkillCandidate{}
	for _, c := range candidates {
		byID[c.ID] = c
	}
	recs := []SkillRecommendation{}
	seen := map[string]bool{}
	for _, it := range parsed.Items {
		c, ok := byID[it.SkillID]
		if !ok || seen[it.SkillID] {
			continue
		}
		seen[it.SkillID] = true
		score := it.Score
		if score < 0 {
			score = 0
		}
		if score > 100 {
			score = 100
		}
		recs = append(recs, buildRecommendation(c, score, it.Reason))
		if len(recs) >= topN {
			break
		}
	}
	return recs, truncate(parsed.Interpretation, 300), nil
}

var defaultSuggestedQueries = []string{
	"把生产日志里的身份证和手机号脱敏",
	"巡检核心银行数据库的健康状况",
	"分析支付节点最近的告警并收敛",
	"把业务需求整理成 PRD 要点",
}
