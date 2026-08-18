package model

import "time"

// GitHubSkillSearchRequest describes a public GitHub skill discovery query.
type GitHubSkillSearchRequest struct {
	Query    string `form:"query"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

// GitHubHostProfile is the local runtime profile used for compatibility checks.
type GitHubHostProfile struct {
	OS       string          `json:"os"`
	Arch     string          `json:"arch"`
	Runtimes map[string]bool `json:"runtimes"`
}

// GitHubCompatibility explains whether a discovered skill can run locally.
type GitHubCompatibility struct {
	Status       string   `json:"status"`
	Label        string   `json:"label"`
	Reasons      []string `json:"reasons"`
	Requirements []string `json:"requirements"`
}

// GitHubSkill is a normalized SKILL.md result from GitHub Code Search.
type GitHubSkill struct {
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	Description     string              `json:"description"`
	Category        string              `json:"category"`
	Tags            []string            `json:"tags"`
	Format          string              `json:"format"`
	Repository      string              `json:"repository"`
	RepositoryURL   string              `json:"repository_url"`
	SkillURL        string              `json:"skill_url"`
	Owner           string              `json:"owner"`
	Path            string              `json:"path"`
	Ref             string              `json:"ref"`
	SHA             string              `json:"sha"`
	Stars           int                 `json:"stars"`
	Language        string              `json:"language,omitempty"`
	License         string              `json:"license,omitempty"`
	UpdatedAt       time.Time           `json:"updated_at,omitempty"`
	Compatibility   GitHubCompatibility `json:"compatibility"`
	DownloadEnabled bool                `json:"download_enabled"`
}

// GitHubSkillPreview contains the source document used for a read-only review.
type GitHubSkillPreview struct {
	Repository            string              `json:"repository"`
	Ref                   string              `json:"ref"`
	Path                  string              `json:"path"`
	Name                  string              `json:"name"`
	Description           string              `json:"description"`
	License               string              `json:"license,omitempty"`
	DeclaredCompatibility string              `json:"declared_compatibility,omitempty"`
	Content               string              `json:"content"`
	Body                  string              `json:"body"`
	SizeBytes             int                 `json:"size_bytes"`
	LineCount             int                 `json:"line_count"`
	Compatibility         GitHubCompatibility `json:"compatibility"`
	SecurityNotice        string              `json:"security_notice"`
}

// GitHubRateLimit exposes enough quota information for the UI to be honest.
type GitHubRateLimit struct {
	Limit     int   `json:"limit"`
	Remaining int   `json:"remaining"`
	ResetAt   int64 `json:"reset_at"`
}

// GitHubSkillSearchResult is the normalized GitHub discovery response.
type GitHubSkillSearchResult struct {
	Data          []GitHubSkill     `json:"data"`
	Total         int               `json:"total"`
	Page          int               `json:"page"`
	PageSize      int               `json:"page_size"`
	SearchCap     int               `json:"search_cap"`
	Authenticated bool              `json:"authenticated"`
	Source        string            `json:"source"`
	Host          GitHubHostProfile `json:"host"`
	RateLimit     GitHubRateLimit   `json:"rate_limit"`
	Notice        string            `json:"notice"`
}
