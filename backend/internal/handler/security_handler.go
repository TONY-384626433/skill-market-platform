package handler

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jjbank/skill-market/internal/service"
)

// SecurityHandler 技能安全治理 (查毒 / 防盗用溯源 / GitHub 导入门禁)
type SecurityHandler struct {
	svc    *service.SecurityService
	github *service.GitHubService
}

func NewSecurityHandler(svc *service.SecurityService, github *service.GitHubService) *SecurityHandler {
	return &SecurityHandler{svc: svc, github: github}
}

// ---------- 公开 ----------

// DefenseStatus GET /api/v1/admin/security/defense-status
// 三道防线总体状态 (代码级检测 / 动态沙箱 / AI 语义审计)
func (h *SecurityHandler) DefenseStatus(c *gin.Context) {
	c.JSON(http.StatusOK, h.svc.DefenseStatus())
}

// SemanticAudit POST /api/v1/admin/security/semantic-audit
// 第三道防线: 对指定技能或一段文本跑 AI 语义审计 (社工话术 + 意图深度分析)
func (h *SecurityHandler) SemanticAudit(c *gin.Context) {
	var req struct {
		SkillID string `json:"skill_id"`
		Text    string `json:"text"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	var report interface{}
	var err error
	if strings.TrimSpace(req.SkillID) != "" {
		report, err = h.svc.SemanticAuditSkill(c.Request.Context(), req.SkillID)
	} else {
		report, err = h.svc.SemanticAuditText(c.Request.Context(), req.Text)
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": report})
}

// SecurityRules 规则库说明 (透明可审计)
func (h *SecurityHandler) SecurityRules(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"data":   service.SecurityRuleDocs(),
		"engine": service.SecurityEngineMeta(),
		"policy": service.SecurityPolicy(),
	})
}

// SkillSecurityBadge GET /api/v1/skills/:id/security-badge
func (h *SecurityHandler) SkillSecurityBadge(c *gin.Context) {
	badge, err := h.svc.Badge(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.Header("Cache-Control", "private, max-age=60")
	c.JSON(http.StatusOK, badge)
}

// ---------- 登录用户: GitHub 导入审查 ----------

// SubmitImportRequest POST /api/v1/github/import-requests
func (h *SecurityHandler) SubmitImportRequest(c *gin.Context) {
	var req struct {
		Repository string `json:"repository" binding:"required"`
		Ref        string `json:"ref"`
		Path       string `json:"path" binding:"required"`
		SkillURL   string `json:"skill_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: repository 与 path 必填"})
		return
	}
	userID := c.GetString("user_id")
	userName := c.GetString("username")
	item, err := h.svc.CreateImportRequest(c.Request.Context(), req.Repository, req.Ref, req.Path, req.SkillURL, userID, userName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// 提交即自动送审 (先审核, 后下载)
	scanned, err := h.svc.ScanImportRequest(c.Request.Context(), item.ID, h.github)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"data": item, "notice": "审查单已创建, 但自动审查未完成: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": scanned})
}

// GetImportRequest GET /api/v1/github/import-requests/:reqId
func (h *SecurityHandler) GetImportRequest(c *gin.Context) {
	item, err := h.svc.ImportRequest(c.Request.Context(), c.Param("reqId"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "审查单不存在"})
		return
	}
	c.JSON(http.StatusOK, item)
}

// ---------- 管理端 ----------

// Overview GET /api/v1/admin/security-overview
func (h *SecurityHandler) Overview(c *gin.Context) {
	data, err := h.svc.Overview(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

// Queue GET /api/v1/admin/security-queue
func (h *SecurityHandler) Queue(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	data, err := h.svc.Queue(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// RecentScans GET /api/v1/admin/security-scans
func (h *SecurityHandler) RecentScans(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	data, err := h.svc.RecentScans(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// ScanDetail GET /api/v1/admin/security-scans/:scanId
func (h *SecurityHandler) ScanDetail(c *gin.Context) {
	scan, err := h.svc.ScanByID(c.Request.Context(), c.Param("scanId"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "扫描记录不存在"})
		return
	}
	c.JSON(http.StatusOK, scan)
}

// RunSkillScan POST /api/v1/admin/skills/:id/security-scan
func (h *SecurityHandler) RunSkillScan(c *gin.Context) {
	scan, err := h.svc.ScanSkill(c.Request.Context(), c.Param("id"), "manual", c.GetString("user_id"), c.GetString("username"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, scan)
}

// SkillScans GET /api/v1/admin/skills/:id/security-scans
func (h *SecurityHandler) SkillScans(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	data, err := h.svc.ScansForSkill(c.Request.Context(), c.Param("id"), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data, "rules": service.SecurityRuleDocs()})
}

// Provenance GET /api/v1/admin/skills/:id/provenance
func (h *SecurityHandler) Provenance(c *gin.Context) {
	data, err := h.svc.Provenance(c.Request.Context(), c.Param("id"))
	if err != nil {
		// 未建立溯源档案时按需生成
		if scan, scanErr := h.svc.EnsureProvenanceByScan(c.Request.Context(), c.Param("id")); scanErr == nil && scan != nil {
			c.JSON(http.StatusOK, gin.H{"data": scan, "created": true})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "该技能尚未建立溯源档案, 请先运行安全扫描"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// VerifyProvenance POST /api/v1/admin/skills/:id/provenance/verify
func (h *SecurityHandler) VerifyProvenance(c *gin.Context) {
	data, err := h.svc.VerifyProvenance(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// DownloadAudits GET /api/v1/admin/download-audits
func (h *SecurityHandler) DownloadAudits(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	data, err := h.svc.Downloads(c.Request.Context(), c.Query("skill_id"), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// ImportRequests GET /api/v1/admin/import-requests
func (h *SecurityHandler) ImportRequests(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	data, err := h.svc.ListImportRequests(c.Request.Context(), c.Query("status"), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// RunImportScan POST /api/v1/admin/import-requests/:reqId/scan
func (h *SecurityHandler) RunImportScan(c *gin.Context) {
	item, err := h.svc.ScanImportRequest(c.Request.Context(), c.Param("reqId"), h.github)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

// DecideImport POST /api/v1/admin/import-requests/:reqId/decision
func (h *SecurityHandler) DecideImport(c *gin.Context) {
	var req struct {
		Approve bool   `json:"approve"`
		Note    string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	item, err := h.svc.DecideImport(c.Request.Context(), c.Param("reqId"), req.Approve, req.Note, c.GetString("user_id"), c.GetString("username"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

// ExportRules GET /api/v1/admin/security-rules/export (规则库审计导出)
func (h *SecurityHandler) ExportRules(c *gin.Context) {
	data, err := service.SecurityRulesExport()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Header("Content-Disposition", `attachment; filename="skillhub-security-rules.json"`)
	c.Data(http.StatusOK, "application/json", data)
}

// ScanPackage POST /api/v1/admin/security/scan-package
// 上传技能包安全预检 (zip 或单文件): 入库/上架前先查毒 + 查重, 通过才允许入库
func (h *SecurityHandler) ScanPackage(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请以 multipart/form-data 上传文件字段 file"})
		return
	}
	handle, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "读取上传文件失败"})
		return
	}
	defer handle.Close()
	data, err := io.ReadAll(io.LimitReader(handle, 25<<20))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "读取上传内容失败"})
		return
	}
	scan, err := h.svc.ScanUploadedPackage(c.Request.Context(), fileHeader.Filename, data, c.GetString("user_id"), c.GetString("username"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": scan, "blocked": scan.CriticalCount > 0})
}
