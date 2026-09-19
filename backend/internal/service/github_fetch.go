package service

import (
	"context"
	"fmt"
	"path"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/jjbank/skill-market/internal/model"
)

// ============================================================
// 技能包文件抓取 (用于「先审核, 后下载」的安全门禁)
// ============================================================

const (
	importMaxFiles = 120
	importMaxBytes = 8 << 20 // 单包最多 8MB 参与安全审查
	importMaxFile  = 2 << 20 // 单文件 2MB
)

// FetchSkillFiles 拉取技能目录下的全部文件到内存, 供安全扫描/指纹/签名使用
func (s *GitHubService) FetchSkillFiles(ctx context.Context, repository, ref, skillPath string) ([]model.SkillFile, error) {
	repo, cleanRef, cleanPath, err := normalizeSkillLocator(repository, ref, skillPath)
	if err != nil {
		return nil, err
	}
	directory := path.Dir(cleanPath)
	if directory == "." || strings.TrimSpace(directory) == "" {
		directory = ""
	}
	var tree githubTreeResponse
	endpoint := fmt.Sprintf("/repos/%s/git/trees/%s?recursive=1", repo, escapePath(cleanRef))
	if _, err := s.apiGetJSON(ctx, endpoint, &tree); err != nil {
		// 分支名可能不对 (例如默认分支是 master 但传了 main): 回退到仓库真实默认分支再试一次
		if def := s.defaultBranch(ctx, repo); def != "" && !strings.EqualFold(def, cleanRef) {
			endpoint = fmt.Sprintf("/repos/%s/git/trees/%s?recursive=1", repo, escapePath(def))
			if _, err2 := s.apiGetJSON(ctx, endpoint, &tree); err2 != nil {
				return nil, err
			}
			cleanRef = def
		} else {
			return nil, err
		}
	}
	type candidate struct {
		path string
		size int64
	}
	selected := make([]candidate, 0, 16)
	for _, entry := range tree.Tree {
		if entry.Type != "blob" {
			continue
		}
		if directory != "" && !strings.HasPrefix(entry.Path, directory+"/") {
			continue
		}
		if strings.Contains(entry.Path, "/.git/") {
			continue
		}
		if entry.Size > importMaxFile {
			continue
		}
		selected = append(selected, candidate{path: entry.Path, size: entry.Size})
		if len(selected) >= importMaxFiles {
			break
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("技能目录未找到可审查文件: %s", cleanPath)
	}
	// 保证 SKILL.md 一定被包含
	hasSkill := false
	for _, c := range selected {
		if strings.EqualFold(path.Base(c.path), "SKILL.md") {
			hasSkill = true
			break
		}
	}
	if !hasSkill {
		if data, err := s.fetchRaw(ctx, repo, cleanRef, cleanPath, importMaxFile); err == nil {
			selected = append([]candidate{{path: cleanPath, size: int64(len(data))}}, selected...)
			_ = data
		}
	}

	files := make([]model.SkillFile, len(selected))
	var total int64
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 6)
	for i, c := range selected {
		wg.Add(1)
		go func(i int, c candidate) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			data, err := s.fetchRaw(ctx, repo, cleanRef, c.path, importMaxFile)
			if err != nil {
				files[i] = model.SkillFile{Path: c.path, Size: c.size, Skipped: true}
				return
			}
			f := model.SkillFile{Path: c.path, Size: int64(len(data)), Content: data}
			if utf8.Valid(data) {
				f.Text = string(data)
			} else {
				f.Skipped = true
			}
			files[i] = f
		}(i, c)
	}
	wg.Wait()

	kept := make([]model.SkillFile, 0, len(files))
	for _, f := range files {
		total += f.Size
		if total > importMaxBytes {
			break
		}
		// 包内相对路径 (去掉技能目录前缀), 便于落库与清单展示
		relative := strings.TrimPrefix(f.Path, directory+"/")
		if relative == "" {
			relative = f.Path
		}
		f.Path = relative
		kept = append(kept, f)
	}
	if len(kept) == 0 {
		return nil, fmt.Errorf("技能目录为空或全部超出限制")
	}
	return kept, nil
}
