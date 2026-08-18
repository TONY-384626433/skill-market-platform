package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	pathpkg "path"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jjbank/skill-market/internal/config"
	"github.com/jjbank/skill-market/internal/model"
	"gopkg.in/yaml.v3"
)

const (
	githubSearchCap       = 1000
	githubMaxArchiveSize  = 20 << 20
	githubMaxArchiveFiles = 160
)

var (
	githubRepoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	githubRefPattern  = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,160}$`)
	apiKeyPattern     = regexp.MustCompile(`\b[A-Z][A-Z0-9_]{2,}_(?:API_)?KEY\b`)
)

type githubCacheEntry struct {
	expires time.Time
	result  *model.GitHubSkillSearchResult
}

// GitHubService discovers public skills and packages a selected skill directory.
type GitHubService struct {
	client  *http.Client
	config  config.GitHubConfig
	host    model.GitHubHostProfile
	cacheMu sync.RWMutex
	cache   map[string]githubCacheEntry
}

type githubOwner struct {
	Login string `json:"login"`
}

type githubLicense struct {
	SPDXID string `json:"spdx_id"`
}

type githubRepository struct {
	Name            string        `json:"name"`
	FullName        string        `json:"full_name"`
	HTMLURL         string        `json:"html_url"`
	Description     string        `json:"description"`
	DefaultBranch   string        `json:"default_branch"`
	Language        string        `json:"language"`
	StargazersCount int           `json:"stargazers_count"`
	Topics          []string      `json:"topics"`
	UpdatedAt       time.Time     `json:"updated_at"`
	Owner           githubOwner   `json:"owner"`
	License         githubLicense `json:"license"`
}

type githubCodeItem struct {
	Name       string           `json:"name"`
	Path       string           `json:"path"`
	SHA        string           `json:"sha"`
	HTMLURL    string           `json:"html_url"`
	Repository githubRepository `json:"repository"`
}

type githubCodeSearchResponse struct {
	TotalCount int              `json:"total_count"`
	Items      []githubCodeItem `json:"items"`
}

type githubRepositorySearchResponse struct {
	TotalCount int                `json:"total_count"`
	Items      []githubRepository `json:"items"`
}

type githubTreeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
	Size int64  `json:"size"`
}

type githubTreeResponse struct {
	Tree      []githubTreeEntry `json:"tree"`
	Truncated bool              `json:"truncated"`
}

type skillFrontmatter struct {
	Name          string                 `yaml:"name"`
	Description   string                 `yaml:"description"`
	License       string                 `yaml:"license"`
	Compatibility string                 `yaml:"compatibility"`
	AllowedTools  interface{}            `yaml:"allowed-tools"`
	Metadata      map[string]interface{} `yaml:"metadata"`
}

func NewGitHubService(cfg config.GitHubConfig) *GitHubService {
	return &GitHubService{
		client: &http.Client{Timeout: 18 * time.Second},
		config: cfg,
		host:   detectHostProfile(),
		cache:  make(map[string]githubCacheEntry),
	}
}

func detectHostProfile() model.GitHubHostProfile {
	runtimes := map[string]bool{}
	for name, commands := range map[string][]string{
		"git":        {"git"},
		"github-cli": {"gh"},
		"node":       {"node"},
		"npm":        {"npm", "npm.cmd"},
		"python":     {"python", "python3"},
		"uv":         {"uv"},
		"docker":     {"docker"},
		"go":         {"go"},
		"cargo":      {"cargo"},
		"bun":        {"bun"},
		"powershell": {"pwsh", "powershell"},
		"bash":       {"bash"},
	} {
		for _, command := range commands {
			if _, err := exec.LookPath(command); err == nil {
				runtimes[name] = true
				break
			}
		}
		if _, ok := runtimes[name]; !ok {
			runtimes[name] = false
		}
	}
	return model.GitHubHostProfile{OS: runtime.GOOS, Arch: runtime.GOARCH, Runtimes: runtimes}
}

func (s *GitHubService) HostStatus() map[string]interface{} {
	return map[string]interface{}{
		"connected":     true,
		"authenticated": strings.TrimSpace(s.config.Token) != "",
		"host":          s.host,
		"source":        "GitHub Code Search",
		"search_cap":    githubSearchCap,
	}
}

func normalizeSearchRequest(req *model.GitHubSkillSearchRequest) {
	req.Query = strings.TrimSpace(req.Query)
	if len([]rune(req.Query)) > 100 {
		req.Query = string([]rune(req.Query)[:100])
	}
	if req.Page < 1 {
		req.Page = 1
	}
	if req.Page > 100 {
		req.Page = 100
	}
	if req.PageSize < 1 {
		req.PageSize = 12
	}
	if req.PageSize > 30 {
		req.PageSize = 30
	}
}

func (s *GitHubService) Search(ctx context.Context, req *model.GitHubSkillSearchRequest) (*model.GitHubSkillSearchResult, error) {
	normalizeSearchRequest(req)
	cacheKey := strings.ToLower(fmt.Sprintf("%s|%d|%d|%t", req.Query, req.Page, req.PageSize, s.config.Token != ""))
	s.cacheMu.RLock()
	entry, ok := s.cache[cacheKey]
	s.cacheMu.RUnlock()
	if ok && time.Now().Before(entry.expires) {
		return entry.result, nil
	}

	var result *model.GitHubSkillSearchResult
	var err error
	if strings.TrimSpace(s.config.Token) != "" {
		result, err = s.searchCode(ctx, req)
	} else {
		result, err = s.searchRepositories(ctx, req)
	}
	if err != nil {
		return nil, err
	}
	s.cacheMu.Lock()
	s.cache[cacheKey] = githubCacheEntry{expires: time.Now().Add(s.config.CacheTTL), result: result}
	s.cacheMu.Unlock()
	return result, nil
}

func plainSearchTerms(value string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r == '"' || r == '\'' || r == ':' || r == '(' || r == ')' {
			return ' '
		}
		return r
	}, value)
	parts := strings.Fields(cleaned)
	if len(parts) > 8 {
		parts = parts[:8]
	}
	return strings.Join(parts, " ")
}

func (s *GitHubService) searchCode(ctx context.Context, req *model.GitHubSkillSearchRequest) (*model.GitHubSkillSearchResult, error) {
	query := "filename:SKILL.md"
	if terms := plainSearchTerms(req.Query); terms != "" {
		query = terms + " " + query
	}
	params := url.Values{}
	params.Set("q", query)
	params.Set("sort", "indexed")
	params.Set("order", "desc")
	params.Set("page", strconv.Itoa(req.Page))
	params.Set("per_page", strconv.Itoa(req.PageSize))
	var response githubCodeSearchResponse
	headers, err := s.apiGetJSON(ctx, "/search/code?"+params.Encode(), &response)
	if err != nil {
		return nil, err
	}

	skills := make([]model.GitHubSkill, len(response.Items))
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 6)
	for index, item := range response.Items {
		wg.Add(1)
		go func(index int, item githubCodeItem) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			skills[index] = s.enrichCodeItem(ctx, item)
		}(index, item)
	}
	wg.Wait()

	return &model.GitHubSkillSearchResult{
		Data:          skills,
		Total:         response.TotalCount,
		Page:          req.Page,
		PageSize:      req.PageSize,
		SearchCap:     githubSearchCap,
		Authenticated: true,
		Source:        "GitHub Code Search",
		Host:          s.host,
		RateLimit:     rateLimitFromHeaders(headers),
		Notice:        "结果来自 GitHub 公开代码索引；GitHub 每个查询最多开放前 1000 条，可通过关键词继续缩小范围。",
	}, nil
}

func (s *GitHubService) searchRepositories(ctx context.Context, req *model.GitHubSkillSearchRequest) (*model.GitHubSkillSearchResult, error) {
	query := "topic:agent-skills"
	if terms := plainSearchTerms(req.Query); terms != "" {
		query = terms + " skill in:name,description,readme"
	}
	params := url.Values{}
	params.Set("q", query)
	params.Set("sort", "stars")
	params.Set("order", "desc")
	params.Set("page", strconv.Itoa(req.Page))
	params.Set("per_page", strconv.Itoa(req.PageSize))
	var response githubRepositorySearchResponse
	headers, err := s.apiGetJSON(ctx, "/search/repositories?"+params.Encode(), &response)
	if err != nil {
		return nil, err
	}

	skills := make([]model.GitHubSkill, 0, len(response.Items))
	for _, repository := range response.Items {
		content := repository.Name + " " + repository.Description + " " + strings.Join(repository.Topics, " ")
		compatibility := s.compatibilityFor(content, "")
		compatibility.Status = "needs_setup"
		compatibility.Label = "需要检查"
		compatibility.Reasons = append([]string{"未配置 GitHub Token，当前按仓库发现，需下载后确认 SKILL.md 路径。"}, compatibility.Reasons...)
		skills = append(skills, model.GitHubSkill{
			ID:              stableGitHubID(repository.FullName, repository.DefaultBranch, "SKILL.md"),
			Name:            repository.Name,
			Description:     fallbackDescription(repository.Description, "GitHub 开源技能仓库"),
			Category:        categorizeGitHubSkill(content),
			Tags:            uniqueStrings(append(repository.Topics, repository.Language)),
			Format:          "技能仓库",
			Repository:      repository.FullName,
			RepositoryURL:   repository.HTMLURL,
			SkillURL:        repository.HTMLURL,
			Owner:           repository.Owner.Login,
			Path:            "SKILL.md",
			Ref:             fallbackString(repository.DefaultBranch, "main"),
			Stars:           repository.StargazersCount,
			Language:        repository.Language,
			License:         repository.License.SPDXID,
			UpdatedAt:       repository.UpdatedAt,
			Compatibility:   compatibility,
			DownloadEnabled: true,
		})
	}

	return &model.GitHubSkillSearchResult{
		Data:          skills,
		Total:         response.TotalCount,
		Page:          req.Page,
		PageSize:      req.PageSize,
		SearchCap:     githubSearchCap,
		Authenticated: false,
		Source:        "GitHub Repository Search",
		Host:          s.host,
		RateLimit:     rateLimitFromHeaders(headers),
		Notice:        "未检测到 GITHUB_TOKEN，已使用公开仓库搜索降级模式；登录 GitHub CLI 后重启可展开每个 SKILL.md。",
	}, nil
}

func (s *GitHubService) enrichCodeItem(ctx context.Context, item githubCodeItem) model.GitHubSkill {
	repository := item.Repository
	ref := fallbackString(repository.DefaultBranch, "main")
	content, _ := s.fetchRaw(ctx, repository.FullName, ref, item.Path, 768<<10)
	frontmatter, body := parseSkillDocument(string(content))
	name := strings.TrimSpace(frontmatter.Name)
	if name == "" {
		directory := pathpkg.Base(pathpkg.Dir(item.Path))
		if directory == "." || directory == "/" {
			directory = repository.Name
		}
		name = strings.ReplaceAll(directory, "-", " ")
	}
	description := strings.TrimSpace(frontmatter.Description)
	if description == "" {
		description = firstProseParagraph(body)
	}
	description = fallbackDescription(description, fallbackDescription(repository.Description, "GitHub 开源 Skill"))
	combined := strings.Join([]string{name, description, frontmatter.Compatibility, body, repository.Language, strings.Join(repository.Topics, " ")}, " ")
	compatibility := s.compatibilityFor(combined, frontmatter.Compatibility)
	license := fallbackString(frontmatter.License, repository.License.SPDXID)
	format := "Agent Skill"
	if strings.Contains(strings.ToLower(combined), "mcp") {
		format = "MCP Skill"
	}

	return model.GitHubSkill{
		ID:              stableGitHubID(repository.FullName, ref, item.Path),
		Name:            titleCaseSkillName(name),
		Description:     truncateRunes(description, 220),
		Category:        categorizeGitHubSkill(combined),
		Tags:            buildGitHubTags(repository, compatibility.Requirements, format),
		Format:          format,
		Repository:      repository.FullName,
		RepositoryURL:   repository.HTMLURL,
		SkillURL:        fallbackString(item.HTMLURL, repository.HTMLURL+"/blob/"+ref+"/"+item.Path),
		Owner:           repository.Owner.Login,
		Path:            item.Path,
		Ref:             ref,
		SHA:             item.SHA,
		Stars:           repository.StargazersCount,
		Language:        repository.Language,
		License:         license,
		UpdatedAt:       repository.UpdatedAt,
		Compatibility:   compatibility,
		DownloadEnabled: true,
	}
}

func parseSkillDocument(content string) (skillFrontmatter, string) {
	var metadata skillFrontmatter
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "---") {
		return metadata, trimmed
	}
	parts := strings.SplitN(trimmed, "---", 3)
	if len(parts) < 3 {
		return metadata, trimmed
	}
	_ = yaml.Unmarshal([]byte(parts[1]), &metadata)
	return metadata, strings.TrimSpace(parts[2])
}

func firstProseParagraph(body string) string {
	for _, block := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n\n") {
		value := strings.TrimSpace(block)
		if value == "" || strings.HasPrefix(value, "#") || strings.HasPrefix(value, "```") || strings.HasPrefix(value, "<") {
			continue
		}
		value = strings.Join(strings.Fields(strings.TrimLeft(value, "-* ")), " ")
		if len([]rune(value)) >= 20 {
			return value
		}
	}
	return ""
}

func (s *GitHubService) compatibilityFor(content, declared string) model.GitHubCompatibility {
	lower := strings.ToLower(content)
	declaredLower := strings.ToLower(declared)
	requirements := make([]string, 0, 8)
	reasons := make([]string, 0, 4)
	missing := make([]string, 0, 4)

	checks := []struct {
		name     string
		patterns []string
	}{
		{"python", []string{"pip install", "python3", "requires python", "python >=", "python>="}},
		{"node", []string{"npm install", "npx ", "requires node", "node.js", "pnpm ", "yarn install"}},
		{"docker", []string{"docker compose", "docker run", "requires docker"}},
		{"go", []string{"go install", "go run", "requires go "}},
		{"cargo", []string{"cargo install", "cargo run", "requires rust"}},
		{"uv", []string{"uv sync", "uv run", "requires uv"}},
		{"bun", []string{"bun install", "bun run", "requires bun"}},
		{"bash", []string{"requires bash", "bash script", "#!/bin/bash"}},
	}
	for _, check := range checks {
		for _, pattern := range check.patterns {
			if strings.Contains(lower, pattern) {
				requirements = append(requirements, check.name)
				if !s.host.Runtimes[check.name] {
					missing = append(missing, check.name)
				}
				break
			}
		}
	}

	osMismatch := ""
	if (strings.Contains(declaredLower, "macos only") || strings.Contains(declaredLower, "requires macos")) && s.host.OS != "darwin" {
		osMismatch = "该 Skill 声明仅支持 macOS"
	}
	if (strings.Contains(declaredLower, "linux only") || strings.Contains(declaredLower, "requires linux")) && s.host.OS != "linux" {
		osMismatch = "该 Skill 声明仅支持 Linux"
	}
	if (strings.Contains(declaredLower, "windows only") || strings.Contains(declaredLower, "requires windows")) && s.host.OS != "windows" {
		osMismatch = "该 Skill 声明仅支持 Windows"
	}
	if osMismatch != "" {
		return model.GitHubCompatibility{Status: "incompatible", Label: "当前不兼容", Reasons: []string{osMismatch}, Requirements: uniqueStrings(requirements)}
	}

	if len(missing) > 0 {
		reasons = append(reasons, "本机缺少运行时: "+strings.Join(uniqueStrings(missing), ", "))
	}
	if apiKeyPattern.MatchString(content) {
		reasons = append(reasons, "运行前需要配置 API 凭据")
		requirements = append(requirements, "API Key")
	}
	if len(reasons) > 0 {
		return model.GitHubCompatibility{Status: "needs_setup", Label: "需要配置", Reasons: reasons, Requirements: uniqueStrings(requirements)}
	}
	return model.GitHubCompatibility{Status: "installable", Label: "本机可安装", Reasons: []string{"SKILL.md 格式有效，已满足检测到的本机运行时要求。"}, Requirements: uniqueStrings(requirements)}
}

func categorizeGitHubSkill(content string) string {
	lower := strings.ToLower(content)
	type categoryRule struct {
		name     string
		keywords []string
	}
	rules := []categoryRule{
		{"安全合规", []string{"security", "audit", "compliance", "vulnerability", "privacy", "安全", "审计"}},
		{"金融业务", []string{"finance", "stock", "trading", "investment", "banking", "financial", "股票", "投资"}},
		{"数据分析", []string{"data analysis", "analytics", "database", "sql", "spreadsheet", "csv", "数据", "数据库"}},
		{"开发工具", []string{"coding", "developer", "debug", "frontend", "backend", "react", "typescript", "python", "code review", "开发", "代码"}},
		{"自动化", []string{"automation", "workflow", "browser", "playwright", "selenium", "自动化", "工作流"}},
		{"内容创作", []string{"writing", "content", "document", "presentation", "image", "video", "design", "写作", "文档", "设计"}},
		{"研究检索", []string{"research", "academic", "search", "literature", "web research", "研究", "检索"}},
		{"MCP 服务", []string{"model context protocol", "mcp server", " mcp ", "mcp-"}},
		{"效率工具", []string{"productivity", "calendar", "email", "notion", "slack", "效率", "日程"}},
	}
	for _, rule := range rules {
		for _, keyword := range rule.keywords {
			if strings.Contains(lower, keyword) {
				return rule.name
			}
		}
	}
	return "通用智能"
}

func buildGitHubTags(repository githubRepository, requirements []string, format string) []string {
	values := make([]string, 0, 8)
	values = append(values, format)
	values = append(values, repository.Topics...)
	if repository.Language != "" {
		values = append(values, repository.Language)
	}
	values = append(values, requirements...)
	tags := uniqueStrings(values)
	return tags[:minInt(6, len(tags))]
}

func normalizeSkillLocator(repository, ref, skillPath string) (string, string, string, error) {
	repository = strings.TrimSpace(repository)
	ref = strings.TrimSpace(ref)
	skillPath = strings.TrimPrefix(pathpkg.Clean(strings.TrimSpace(skillPath)), "./")
	if !githubRepoPattern.MatchString(repository) {
		return "", "", "", errors.New("仓库地址格式无效")
	}
	if !githubRefPattern.MatchString(ref) || strings.Contains(ref, "..") {
		return "", "", "", errors.New("分支名称无效")
	}
	if skillPath == "." || strings.HasPrefix(skillPath, "/") || strings.HasPrefix(skillPath, "../") || !strings.EqualFold(pathpkg.Base(skillPath), "SKILL.md") {
		return "", "", "", errors.New("Skill 路径无效")
	}
	return repository, ref, skillPath, nil
}

// PreviewSkill retrieves a SKILL.md as inert text for review before download.
func (s *GitHubService) PreviewSkill(ctx context.Context, repository, ref, skillPath string) (*model.GitHubSkillPreview, error) {
	repository, ref, skillPath, err := normalizeSkillLocator(repository, ref, skillPath)
	if err != nil {
		return nil, err
	}
	content, err := s.fetchRaw(ctx, repository, ref, skillPath, 1<<20)
	if err != nil {
		return nil, err
	}
	frontmatter, body := parseSkillDocument(string(content))
	name := titleCaseSkillName(fallbackString(frontmatter.Name, pathpkg.Base(pathpkg.Dir(skillPath))))
	if name == "." || strings.TrimSpace(name) == "" {
		name = titleCaseSkillName(pathpkg.Base(repository))
	}
	description := fallbackDescription(frontmatter.Description, firstProseParagraph(body))
	compatibility := s.compatibilityFor(string(content), frontmatter.Compatibility)
	return &model.GitHubSkillPreview{
		Repository:            repository,
		Ref:                   ref,
		Path:                  skillPath,
		Name:                  name,
		Description:           description,
		License:               frontmatter.License,
		DeclaredCompatibility: frontmatter.Compatibility,
		Content:               string(content),
		Body:                  body,
		SizeBytes:             len(content),
		LineCount:             strings.Count(string(content), "\n") + 1,
		Compatibility:         compatibility,
		SecurityNotice:        "内容来自 GitHub 公开仓库，SkillHub 仅做静态预览与运行时兼容性提示，未执行其中脚本，也不代表通过安全审核。",
	}, nil
}

func (s *GitHubService) BuildArchive(ctx context.Context, repository, ref, skillPath string) (string, []byte, error) {
	var err error
	repository, ref, skillPath, err = normalizeSkillLocator(repository, ref, skillPath)
	if err != nil {
		return "", nil, err
	}

	var tree githubTreeResponse
	_, err = s.apiGetJSON(ctx, "/repos/"+repository+"/git/trees/"+url.PathEscape(ref)+"?recursive=1", &tree)
	if err != nil {
		return "", nil, err
	}
	if tree.Truncated {
		return "", nil, errors.New("仓库文件树过大，GitHub 返回了截断结果，请前往仓库下载")
	}

	directory := pathpkg.Dir(skillPath)
	if directory == "." {
		directory = ""
	}
	selected := make([]githubTreeEntry, 0, 24)
	foundSkill := false
	var skillEntry githubTreeEntry
	for _, entry := range tree.Tree {
		if entry.Type != "blob" {
			continue
		}
		cleanPath := pathpkg.Clean(entry.Path)
		if cleanPath == skillPath {
			foundSkill = true
			skillEntry = entry
		}
		if !archivePathSelected(cleanPath, directory) {
			continue
		}
		selected = append(selected, entry)
	}
	if !foundSkill {
		return "", nil, errors.New("仓库中未找到指定 SKILL.md")
	}
	if len(selected) == 0 {
		return "", nil, errors.New("没有找到可下载的 Skill 文件")
	}
	limitedArchive := len(selected) > githubMaxArchiveFiles
	if limitedArchive {
		selected = []githubTreeEntry{skillEntry}
	}
	var declaredSize int64
	for _, entry := range selected {
		declaredSize += entry.Size
	}
	if declaredSize > githubMaxArchiveSize {
		limitedArchive = true
		selected = []githubTreeEntry{skillEntry}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].Path < selected[j].Path })

	type downloadedFile struct {
		path string
		data []byte
		err  error
	}
	files := make([]downloadedFile, len(selected))
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 8)
	for index, entry := range selected {
		wg.Add(1)
		go func(index int, entry githubTreeEntry) {
			defer wg.Done()
			if entry.Size > 5<<20 {
				files[index].err = fmt.Errorf("文件 %s 超过单文件限制", entry.Path)
				return
			}
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			data, fetchErr := s.fetchRaw(ctx, repository, ref, entry.Path, 5<<20)
			files[index] = downloadedFile{path: entry.Path, data: data, err: fetchErr}
		}(index, entry)
	}
	wg.Wait()

	var totalSize int
	for _, file := range files {
		if file.err != nil {
			return "", nil, file.err
		}
		totalSize += len(file.data)
		if totalSize > githubMaxArchiveSize {
			return "", nil, errors.New("SKILL.md 超过 20 MB 安全下载限制")
		}
	}

	rootName := sanitizeArchiveName(pathpkg.Base(directory))
	if rootName == "" || rootName == "." {
		rootName = sanitizeArchiveName(pathpkg.Base(repository))
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, file := range files {
		relative := file.path
		if directory != "" {
			relative = strings.TrimPrefix(strings.TrimPrefix(file.path, directory), "/")
		}
		entryWriter, createErr := writer.Create(rootName + "/" + relative)
		if createErr != nil {
			return "", nil, createErr
		}
		if _, writeErr := entryWriter.Write(file.data); writeErr != nil {
			return "", nil, writeErr
		}
	}
	if limitedArchive {
		noticeWriter, createErr := writer.Create(rootName + "/SKILLHUB-DOWNLOAD-NOTICE.txt")
		if createErr != nil {
			return "", nil, createErr
		}
		_, _ = noticeWriter.Write([]byte("SkillHub packaged SKILL.md only because its repository directory exceeded the 160-file or 20-MB safety limit. Review the GitHub repository for optional assets and scripts.\n"))
	}
	if err := writer.Close(); err != nil {
		return "", nil, err
	}
	return rootName + ".zip", buffer.Bytes(), nil
}

func archivePathSelected(filePath, directory string) bool {
	if directory != "" {
		return filePath == directory || strings.HasPrefix(filePath, directory+"/")
	}
	lower := strings.ToLower(filePath)
	if !strings.Contains(filePath, "/") {
		return lower == "skill.md" || lower == "readme.md" || lower == "license" || lower == "license.md" || lower == "requirements.txt" || lower == "package.json" || lower == "pyproject.toml"
	}
	for _, prefix := range []string{"scripts/", "references/", "assets/", "templates/"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func (s *GitHubService) apiGetJSON(ctx context.Context, endpoint string, target interface{}) (http.Header, error) {
	base := strings.TrimRight(fallbackString(s.config.APIBase, "https://api.github.com"), "/")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "SkillHub-GitHub-Discovery/1.0")
	if token := strings.TrimSpace(s.config.Token); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("连接 GitHub 失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := struct {
			Message string `json:"message"`
		}{}
		_ = json.NewDecoder(io.LimitReader(response.Body, 128<<10)).Decode(&message)
		if message.Message == "" {
			message.Message = response.Status
		}
		return response.Header, fmt.Errorf("GitHub API 返回 %d: %s", response.StatusCode, message.Message)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(target); err != nil {
		return response.Header, fmt.Errorf("解析 GitHub 响应失败: %w", err)
	}
	return response.Header, nil
}

func (s *GitHubService) fetchRaw(ctx context.Context, repository, ref, filePath string, maxBytes int64) ([]byte, error) {
	rawURL := "https://raw.githubusercontent.com/" + escapePath(repository) + "/" + escapePath(ref) + "/" + escapePath(filePath)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "SkillHub-GitHub-Discovery/1.0")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("下载 %s 失败: %w", filePath, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载 %s 失败: GitHub 返回 %d", filePath, response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("文件 %s 超过下载限制", filePath)
	}
	return data, nil
}

func escapePath(value string) string {
	parts := strings.Split(strings.Trim(value, "/"), "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	return strings.Join(parts, "/")
}

func stableGitHubID(repository, ref, skillPath string) string {
	digest := sha256.Sum256([]byte(repository + "@" + ref + ":" + skillPath))
	return "gh_" + hex.EncodeToString(digest[:8])
}

func rateLimitFromHeaders(headers http.Header) model.GitHubRateLimit {
	limit, _ := strconv.Atoi(headers.Get("X-RateLimit-Limit"))
	remaining, _ := strconv.Atoi(headers.Get("X-RateLimit-Remaining"))
	reset, _ := strconv.ParseInt(headers.Get("X-RateLimit-Reset"), 10, 64)
	return model.GitHubRateLimit{Limit: limit, Remaining: remaining, ResetAt: reset}
}

func fallbackString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func fallbackDescription(value, fallback string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if value == "" {
		return fallback
	}
	return value
}

func truncateRunes(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit-1]) + "..."
}

func titleCaseSkillName(value string) string {
	value = strings.Join(strings.Fields(strings.ReplaceAll(strings.TrimSpace(value), "_", " ")), " ")
	if value == "" {
		return "Unnamed Skill"
	}
	return value
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func sanitizeArchiveName(value string) string {
	value = regexp.MustCompile(`[^A-Za-z0-9._-]+`).ReplaceAllString(value, "-")
	return strings.Trim(value, "-.")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
