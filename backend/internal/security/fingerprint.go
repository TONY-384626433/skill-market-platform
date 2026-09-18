package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jjbank/skill-market/internal/model"
)

// ============================================================
// 内容指纹 / 水印 / 包签名
//   防盗用三件套:
//     1. 指纹查重 —— 别人把你上传的技能改个名字再传, 相似度会命中
//     2. 水印     —— 每个技能/每次交付都带唯一溯源号, 泄漏可追责
//     3. 包签名   —— HMAC-SHA256 签名清单, 装包前验签, 防篡改/防调包
// ============================================================

const (
	signingKeyEnv  = "SKILLHUB_SIGNING_KEY"
	defaultKey     = "skillhub-jjbank-2026-provenance-key"
	manifestName   = "MANIFEST.skillhub.json"
	watermarkFile  = ".skillhub-provenance.json"
	Algorithm      = "HMAC-SHA256"
	ManifestSchema = "skillhub.provenance/v1"
)

// SigningKey 签名密钥 (解析顺序: 挂载文件 > 环境变量 > 内置演示密钥)
func SigningKey() string {
	keyOnce.Do(loadKeyOnce)
	return keyValue
}

// NormalizeText 文本规范化 (去格式/注释/多余空白, 用于指纹)
func NormalizeText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	var b strings.Builder
	for i := 0; i < len(s); {
		c := s[i]
		// 去行注释
		if c == '#' || (c == '/' && i+1 < len(s) && s[i+1] == '/') {
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		b.WriteByte(c)
		i++
	}
	text := strings.ToLower(b.String())
	text = strings.Join(strings.Fields(text), " ")
	return text
}

// Normalize 对整包做规范化并产出稳定文本 (按路径排序, 保证可复现)
func Normalize(files []model.SkillFile) string {
	paths := make([]string, 0, len(files))
	byPath := map[string]model.SkillFile{}
	for _, f := range files {
		if f.Skipped {
			continue
		}
		paths = append(paths, f.Path)
		byPath[f.Path] = f
	}
	sort.Strings(paths)
	var b strings.Builder
	for _, p := range paths {
		f := byPath[p]
		content := f.Text
		if IsDocFile(p) {
			content = StripMarkdown(content)
		}
		b.WriteString(p)
		b.WriteString("::")
		b.WriteString(NormalizeText(content))
		b.WriteString("\n")
	}
	return b.String()
}

// ContentHash 整包内容哈希
func ContentHash(files []model.SkillFile) string {
	sum := sha256.Sum256([]byte(Normalize(files)))
	return hex.EncodeToString(sum[:])
}

// FileSHA256 单文件哈希
func FileSHA256(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// SimHash 64 位指纹 (用于相似度比对)
func SimHash(files []model.SkillFile) uint64 {
	tokens := shingles(Normalize(files))
	var v [64]int
	for token := range tokens {
		h := fnv64(token)
		for i := 0; i < 64; i++ {
			if h&(1<<uint(i)) != 0 {
				v[i]++
			} else {
				v[i]--
			}
		}
	}
	var fp uint64
	for i := 0; i < 64; i++ {
		if v[i] > 0 {
			fp |= 1 << uint(i)
		}
	}
	return fp
}

// SimHashHex 十六进制表示
func SimHashHex(files []model.SkillFile) string {
	return fmt.Sprintf("%016x", SimHash(files))
}

// Similarity 相似度 0-1 (Shingle Jaccard + SimHash 距离加权)
func Similarity(filesA, filesB []model.SkillFile) float64 {
	a := shingles(Normalize(filesA))
	b := shingles(Normalize(filesB))
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for k := range a {
		if _, ok := b[k]; ok {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	jaccard := float64(inter) / float64(union)
	ha, hb := SimHash(filesA), SimHash(filesB)
	sim := 1 - float64(popcount(ha^hb))/64.0
	if sim > jaccard {
		return sim
	}
	return jaccard
}

// SimilarityFromHex 用已存指纹计算相似度 (免重新读取文件)
func SimilarityFromHex(simHexA string, shinglesA map[string]struct{}, filesB []model.SkillFile) float64 {
	b := shingles(Normalize(filesB))
	if shinglesA != nil && len(shinglesA) > 0 {
		inter := 0
		for k := range shinglesA {
			if _, ok := b[k]; ok {
				inter++
			}
		}
		union := len(shinglesA) + len(b) - inter
		if union > 0 {
			return float64(inter) / float64(union)
		}
	}
	var ha uint64
	fmt.Sscanf(simHexA, "%016x", &ha)
	hb := SimHash(filesB)
	return 1 - float64(popcount(ha^hb))/64.0
}

func shingles(text string) map[string]struct{} {
	words := strings.Fields(text)
	out := make(map[string]struct{})
	if len(words) == 0 {
		return out
	}
	const n = 3
	if len(words) < n {
		out[strings.Join(words, " ")] = struct{}{}
		return out
	}
	for i := 0; i+n <= len(words); i++ {
		out[strings.Join(words[i:i+n], " ")] = struct{}{}
	}
	return out
}

func fnv64(s string) uint64 {
	const (
		offset = 14695981039346656037
		prime  = 1099511628211
	)
	var h uint64 = offset
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime
	}
	return h
}

func popcount(x uint64) int {
	n := 0
	for x != 0 {
		x &= x - 1
		n++
	}
	return n
}

// ---------- 水印 ----------

// NewWatermark 生成技能级溯源水印号
func NewWatermark(skillKey, ownerID, issued string) string {
	key := SigningKey()
	if len(key) > 8 {
		key = key[:8]
	}
	sum := sha256.Sum256([]byte("wm|" + skillKey + "|" + ownerID + "|" + issued + "|" + key))
	return "WM-" + strings.ToUpper(hex.EncodeToString(sum[:6]))
}

// NewDeliveryWatermark 生成单次交付水印 (绑定下载人, 泄漏可定位到人)
func NewDeliveryWatermark(skillKey, userID, issued string) string {
	sum := sha256.Sum256([]byte("dl|" + skillKey + "|" + userID + "|" + issued))
	return "DL-" + strings.ToUpper(hex.EncodeToString(sum[:5]))
}

// ---------- 包签名 ----------

// PackageManifest 交付包签名清单
type PackageManifest struct {
	Schema       string         `json:"schema"`
	SkillKey     string         `json:"skill_key"`
	SkillName    string         `json:"skill_name,omitempty"`
	Version      string         `json:"version,omitempty"`
	OwnerID      string         `json:"owner_id,omitempty"`
	License      string         `json:"license,omitempty"`
	WatermarkID  string         `json:"watermark_id"`
	DeliveryMark string         `json:"delivery_watermark,omitempty"`
	IssuedAt     string         `json:"issued_at"`
	Source       string         `json:"source,omitempty"`
	ContentHash  string         `json:"content_hash"`
	SimHash      string         `json:"sim_hash"`
	Engine       string         `json:"scan_engine"`
	ScanVerdict  string         `json:"scan_verdict"`
	RiskScore    int            `json:"risk_score"`
	KeyID        string         `json:"key_id"` // 签发密钥指纹 (证明密钥版本, 不泄露密钥)
	Files        []ManifestFile `json:"files"`
	Signature    string         `json:"signature"` // 对 payload 的 HMAC-SHA256
}

// ManifestFile 单文件条目
type ManifestFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// ManifestPayload 参与签名的规范化载荷
func ManifestPayload(m PackageManifest) string {
	items := make([]string, 0, len(m.Files))
	for _, f := range m.Files {
		items = append(items, fmt.Sprintf("%s:%s:%d", f.Path, f.SHA256, f.Size))
	}
	sort.Strings(items)
	head := strings.Join([]string{
		m.Schema, m.SkillKey, m.Version, m.OwnerID, m.WatermarkID, m.DeliveryMark,
		m.IssuedAt, m.Source, m.ContentHash, m.SimHash, fmt.Sprint(m.RiskScore), m.ScanVerdict, m.KeyID,
	}, "|")
	return head + "|" + strings.Join(items, "|")
}

// SignManifest 计算签名
func SignManifest(m PackageManifest, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(ManifestPayload(m)))
	return "hmac-sha256:" + hex.EncodeToString(mac.Sum(nil))
}

// VerifyManifest 验签: 是否被篡改
func VerifyManifest(m PackageManifest, files []model.SkillFile, key string) (bool, string) {
	if m.Signature == "" {
		return false, "包未签名"
	}
	if m.Signature != SignManifest(m, key) {
		return false, "签名校验失败: 清单被篡改或来源不明"
	}
	if m.ContentHash != "" && m.ContentHash != ContentHash(files) {
		return false, "内容哈希与签名清单不一致: 包内容被改动"
	}
	index := map[string]model.SkillFile{}
	for _, f := range files {
		index[f.Path] = f
	}
	for _, entry := range m.Files {
		f, ok := index[entry.Path]
		if !ok {
			return false, "缺失文件: " + entry.Path
		}
		if FileSHA256(f.Content) != entry.SHA256 {
			return false, "文件被篡改: " + entry.Path
		}
	}
	return true, "签名有效, 内容完整"
}

// BuildManifest 生成交付清单 (含签名 + 水印)
func BuildManifest(skillKey, skillName, version, ownerID, license, watermark, delivery, issued, source, verdict string, risk int, files []model.SkillFile) PackageManifest {
	sorted := append([]model.SkillFile(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	entries := make([]ManifestFile, 0, len(sorted))
	for _, f := range sorted {
		if f.Size == 0 && len(f.Content) > 0 {
			f.Size = int64(len(f.Content))
		}
		entries = append(entries, ManifestFile{Path: f.Path, Size: f.Size, SHA256: FileSHA256(f.Content)})
	}
	m := PackageManifest{
		Schema: ManifestSchema, SkillKey: skillKey, SkillName: skillName, Version: version,
		OwnerID: ownerID, License: license, WatermarkID: watermark, DeliveryMark: delivery,
		IssuedAt: issued, Source: source, ContentHash: ContentHash(files), SimHash: SimHashHex(files),
		Engine: engineVersion, ScanVerdict: verdict, RiskScore: risk, KeyID: KeyID(), Files: entries,
	}
	m.Signature = SignManifest(m, SigningKey())
	return m
}

// MarshalManifest JSON 序列化
func MarshalManifest(m PackageManifest) []byte {
	b, _ := json.MarshalIndent(m, "", "  ")
	return b
}

// WatermarkDocument 写入包内的溯源文件 (含人可读 + 机读水印)
func WatermarkDocument(m PackageManifest) []byte {
	doc := map[string]interface{}{
		"watermark_id":       m.WatermarkID,
		"delivery_watermark": m.DeliveryMark,
		"skill_key":          m.SkillKey,
		"owner_id":           m.OwnerID,
		"license":            m.License,
		"issued_at":          m.IssuedAt,
		"content_hash":       m.ContentHash,
		"sim_hash":           m.SimHash,
		"scan_verdict":       m.ScanVerdict,
		"risk_score":         m.RiskScore,
		"key_id":             m.KeyID,
		"signature":          m.Signature,
		"notice":             "本文件用于来源追溯: 该技能包由 SkillHub 签发, 水印号唯一绑定下载人, 未经授权再分发将被指纹查重与审计追踪",
	}
	b, _ := json.MarshalIndent(doc, "", "  ")
	return b
}

// ManifestFileName 清单文件名
func ManifestFileName() string { return manifestName }

// WatermarkFileName 水印文件名
func WatermarkFileName() string { return watermarkFile }
