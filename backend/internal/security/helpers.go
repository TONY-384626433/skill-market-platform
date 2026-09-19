package security

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/jjbank/skill-market/internal/model"
)

// ============================================================
// 扫描辅助检测器
// ============================================================

var (
	reLongSecret  = regexp.MustCompile(`[A-Za-z0-9+/=_-]{24,}`)
	rePipInstall  = regexp.MustCompile(`(?i)\b(pip3?|python\s+-m\s+pip)\s+install\s+([^\n\r#]{1,200})`)
	reReqLine     = regexp.MustCompile(`^\s*([A-Za-z0-9_.\-]+)\s*(>=|~=|>|<=|<)?\s*([0-9][0-9A-Za-z._\-]*)?\s*$`)
	reNameField   = regexp.MustCompile(`(?im)^\s*name\s*:\s*\S+`)
	reDescField   = regexp.MustCompile(`(?im)^\s*description\s*:\s*\S+`)
	reFencedBlock = regexp.MustCompile("(?s)```.*?```")
	reHTMLComment = regexp.MustCompile(`(?s)<!--.*?-->`)
)

// firstMatch 返回首次命中的行号/片段
func firstMatch(re *regexp.Regexp, text string) (int, string, bool) {
	loc := re.FindStringIndex(text)
	if loc == nil {
		return 0, "", false
	}
	line := 1 + strings.Count(text[:loc[0]], "\n")
	snippet := text[loc[0]:loc[1]]
	if len(snippet) > 200 {
		snippet = snippet[:200]
	}
	return line, strings.ReplaceAll(snippet, "\n", " "), true
}

// longLine 检测超长行 (混淆特征)
func longLine(text string, limit int) (int, string, bool) {
	for i, line := range strings.Split(text, "\n") {
		if len(line) > limit {
			return i + 1, line, true
		}
	}
	return 0, "", false
}

// countZeroWidth 统计不可见字符数量
func countZeroWidth(text string) (int, rune) {
	n := 0
	var first rune
	for _, r := range text {
		for _, zw := range zeroWidthChars {
			if r == zw {
				if n == 0 {
					first = r
				}
				n++
			}
		}
	}
	return n, first
}

// unpinnedDependency 检测未固定版本的依赖
func unpinnedDependency(text string) (int, string, bool) {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if m := rePipInstall.FindStringSubmatch(trimmed); m != nil {
			spec := strings.TrimSpace(m[2])
			if spec == "" || strings.Contains(spec, "-r") || strings.Contains(spec, "-e") {
				continue
			}
			hasPin := false
			for _, item := range strings.Fields(spec) {
				if strings.Contains(item, "==") || strings.Contains(item, "===") {
					hasPin = true
				}
				if !strings.HasPrefix(item, "-") && !hasPin && !strings.Contains(item, "==") {
					return i + 1, "pip install " + item, true
				}
			}
		}
		// requirements.txt 风格: pkg 或 pkg>=1.0 但无 ==
		if m := reReqLine.FindStringSubmatch(trimmed); m != nil && m[3] == "" {
			name := m[1]
			lower := strings.ToLower(name)
			if lower == "name" || lower == "description" || lower == "version" || len(name) < 2 {
				continue
			}
			if strings.Contains(strings.ToLower(text), "requirements") {
				return i + 1, name, true
			}
		}
	}
	return 0, "", false
}

// metadataMissing 检查 SKILL.md 是否缺少标准元数据
func metadataMissing(text string) (bool, string) {
	head := text
	if idx := strings.Index(text, "\n---"); idx > 0 {
		head = text[:idx]
	}
	if len(head) > 4000 {
		head = head[:4000]
	}
	hasName := reNameField.MatchString(head)
	hasDesc := reDescField.MatchString(head)
	switch {
	case !hasName && !hasDesc:
		return true, "缺少 name / description 字段"
	case !hasName:
		return true, "缺少 name 字段"
	case !hasDesc:
		return true, "缺少 description 字段"
	}
	return false, ""
}

var (
	claimNoNetwork = regexp.MustCompile(`(?i)(无(任何)?(网络|联网|外联)|不(会|进行|发起|使用|访问)(任何)?(网络|联网|外联)|离线(运行|模式)|no\s+network|without\s+network|offline\s+(only|mode)|\bno\s+internet\b)`)
	claimReadOnly  = regexp.MustCompile(`(?i)(只读|不(会)?(写入|修改)文件|read[\s-]?only|does\s+not\s+(write|modify)\s+files?|no\s+file\s+write)`)
	codeNetworkUse = regexp.MustCompile(`(?i)(requests\.(get|post|put)|urllib\.request|httpx\.|aiohttp\.|axios\.|fetch\s*\(\s*['"]https?://|net\.http|http\.client|HttpClient|Invoke-WebRequest|Invoke-RestMethod|curl\s+https?://|wget\s+https?://|socket\.socket\()`)
	codeFileWrite  = regexp.MustCompile(`(?i)(open\s*\([^)]{0,80}['"][wa]\+?b?['"]|fs\.writeFile|os\.remove|shutil\.(rmtree|move|copy)|unlinkSync|removeSync|rmtree|os\.rename|Path\([^)]*\)\.write_text|with\s+open\([^)]*['"]w)`)
)

// declaredVsActual 一致性检查 (幻觉/隐瞒能力)
func declaredVsActual(files []model.SkillFile) (bool, string) {
	var docText, codeText strings.Builder
	for _, f := range files {
		if f.Skipped || f.Text == "" {
			continue
		}
		ext := strings.ToLower(f.Path[strings.LastIndex(f.Path, ".")+1:])
		switch ext {
		case "md", "markdown", "mdx", "txt":
			docText.WriteString(f.Text)
			docText.WriteString("\n")
		case "py", "js", "mjs", "cjs", "ts", "tsx", "go", "sh", "ps1", "rb", "java", "php":
			codeText.WriteString(f.Text)
			codeText.WriteString("\n")
		}
	}
	doc := docText.String()
	code := codeText.String()
	if doc == "" || code == "" {
		return false, ""
	}
	if claimNoNetwork.MatchString(doc) && codeNetworkUse.MatchString(code) {
		return true, "文档声称无网络访问, 但代码存在网络调用"
	}
	if claimReadOnly.MatchString(doc) && codeFileWrite.MatchString(code) {
		return true, "文档声称只读, 但代码存在文件写入/删除行为"
	}
	return false, ""
}

// maskEvidence 对证据片段脱敏 (长密钥样式串打码)
func maskEvidence(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	if len(s) > 220 {
		s = s[:220] + "..."
	}
	return reLongSecret.ReplaceAllStringFunc(s, func(m string) string {
		if len(m) < 24 {
			return m
		}
		if strings.Contains(m, "EICAR") || strings.Contains(m, "ANTIVIRUS") {
			return m[:16] + "***"
		}
		return m[:4] + "***(" + fmt.Sprint(len(m)) + ")"
	})
}

// safeRelPath 包内路径安全 (防 Zip Slip)
func safeRelPath(p string) bool {
	if p == "" {
		return false
	}
	clean := strings.ReplaceAll(p, "\\", "/")
	if strings.HasPrefix(clean, "/") || regexp.MustCompile(`^[A-Za-z]:`).MatchString(clean) {
		return false
	}
	for _, seg := range strings.Split(clean, "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}

func newScanID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "sec_" + fmt.Sprint(len(b))
	}
	return "sec_" + hex.EncodeToString(b)
}

// IsScannableFile 是否需要纳入静态扫描
func IsScannableFile(name string) bool {
	lower := strings.ToLower(name)
	for _, skip := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".woff", ".woff2", ".ttf", ".zip", ".gz", ".7z", ".pdf", ".mp4", ".mp3", ".wav", ".xlsx", ".docx"} {
		if strings.HasSuffix(lower, skip) {
			return false
		}
	}
	return true
}

// IsDocFile 是否为文档
func IsDocFile(name string) bool {
	lower := strings.ToLower(name)
	for ext := range docExt {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return strings.ToLower(name) == "skill.md"
}

// IsCodeFile 是否为代码/配置
func IsCodeFile(name string) bool {
	lower := strings.ToLower(name)
	for ext := range codeExt {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// LooksBinary 判断内容是否为二进制 (含 NUL 或大量不可打印字符)
func LooksBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return true
	}
	sample := data
	if len(sample) > 4096 {
		sample = sample[:4096]
	}
	bad := 0
	for _, b := range sample {
		if b == 9 || b == 10 || b == 13 {
			continue
		}
		if b < 32 || b == 0x7f {
			bad++
		}
	}
	return bad*100/len(sample) > 10
}

// StripMarkdown 去掉文档中的代码块与注释 (用于指纹规范化)
func StripMarkdown(s string) string {
	s = reFencedBlock.ReplaceAllString(s, " ")
	s = reHTMLComment.ReplaceAllString(s, " ")
	return s
}
