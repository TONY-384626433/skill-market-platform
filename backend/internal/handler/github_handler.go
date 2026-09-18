package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jjbank/skill-market/internal/model"
	sec "github.com/jjbank/skill-market/internal/security"
	"github.com/jjbank/skill-market/internal/service"
)

// GitHubHandler exposes public, read-only discovery and archive endpoints.
type GitHubHandler struct {
	service  *service.GitHubService
	security *service.SecurityService
}

func NewGitHubHandler(githubService *service.GitHubService) *GitHubHandler {
	return &GitHubHandler{service: githubService}
}

// AttachSecurity 注入安全治理服务 (下载门禁 + 水印签名)
func (h *GitHubHandler) AttachSecurity(s *service.SecurityService) {
	h.security = s
}

func (h *GitHubHandler) Status(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.HostStatus())
}

func (h *GitHubHandler) SearchSkills(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "12"))
	request := model.GitHubSkillSearchRequest{Query: c.Query("query"), Page: page, PageSize: pageSize}
	result, err := h.service.Search(c.Request.Context(), &request)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *GitHubHandler) PreviewSkill(c *gin.Context) {
	preview, err := h.service.PreviewSkill(c.Request.Context(), c.Query("repo"), c.Query("ref"), c.Query("path"))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.Header("Cache-Control", "private, max-age=300")
	c.JSON(http.StatusOK, preview)
}

// DownloadSkill 下载技能包。
//
// 安全门禁: GitHub 属于外部不可信来源, 必须先提交安全审查并审核通过
// (携带 request_id), 否则一律拒绝下载 —— 防止病毒/后门/提示注入技能入库。
func (h *GitHubHandler) DownloadSkill(c *gin.Context) {
	repository, ref, skillPath := c.Query("repo"), c.Query("ref"), c.Query("path")
	requestID := c.Query("request_id")
	if h.security == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "安全治理服务未就绪, 下载已阻断"})
		return
	}

	req, err := h.security.DownloadGate(c.Request.Context(), requestID, repository, ref, skillPath)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"error":  err.Error(),
			"gate":   "security_review_required",
			"policy": "外部技能先审核、后下载: GitHub 来源的 SKILL.md 必须通过静态安全审查(查毒/危险行为/提示注入/供应链)才允许下载",
			"hint":   "POST /api/v1/github/import-requests {repository, ref, path} 提交审查",
		})
		return
	}

	filename, archive, err := h.service.BuildArchive(c.Request.Context(), repository, ref, skillPath)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	// 交付包加固: 注入签名清单 + 唯一水印 (泄漏可追溯到下载人)
	userID := c.GetString("user_id")
	delivery := sec.NewDeliveryWatermark(req.SkillPath, userID+req.ID, time.Now().UTC().Format(time.RFC3339))
	signed, manifest, signErr := h.security.SignArchive(archive, service.ArchiveMeta{
		SkillKey: req.SkillPath, SkillName: req.SkillName, OwnerID: req.RequestedBy,
		License: "reviewed-external", Watermark: req.WatermarkID, Delivery: delivery,
		Source: "github:" + repository, Verdict: req.Verdict, RiskScore: req.RiskScore,
		Channel: "github_import", SkillID: req.ID, UserID: userID,
	})
	if signErr == nil {
		archive = signed
	}
	_ = h.security.RecordDownload(c.Request.Context(), req.ID, req.SkillPath, userID, delivery,
		manifest.Signature, c.ClientIP(), c.GetHeader("User-Agent"), "github_import")

	filename = strings.ReplaceAll(filename, `"`, "")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("Cache-Control", "private, max-age=300")
	c.Header("X-SkillHub-Import-Request", req.ID)
	c.Header("X-SkillHub-Watermark", delivery)
	c.Header("X-SkillHub-Signature", manifest.Signature)
	c.Data(http.StatusOK, "application/zip", archive)
}
