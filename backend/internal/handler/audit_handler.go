package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jjbank/skill-market/internal/service"
)

// AuditHandler 技能可用性审核 API
type AuditHandler struct {
	auditSvc *service.AuditService
	skillSvc *service.SkillService
}

func NewAuditHandler(auditSvc *service.AuditService, skillSvc *service.SkillService) *AuditHandler {
	return &AuditHandler{auditSvc: auditSvc, skillSvc: skillSvc}
}

// RunSkillAudit POST /api/v1/admin/skills/:id/audit — 管理员触发可用性审核
func (h *AuditHandler) RunSkillAudit(c *gin.Context) {
	h.run(c, c.Param("id"), "manual", true)
}

// SelfCheck POST /api/v1/skills/:id/self-check — 开发者自检 (仅限本人技能)
func (h *AuditHandler) SelfCheck(c *gin.Context) {
	skillID := c.Param("id")
	userID := c.GetString("user_id")
	role := c.GetString("role")

	skill, err := h.skillSvc.GetByID(skillID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if skill == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "技能不存在"})
		return
	}
	// 管理员可自检任意技能, 开发者仅限本人提交
	isAdmin := role == "admin"
	if !isAdmin && skill.AuthorID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅可对本人提交的技能发起自检"})
		return
	}
	h.run(c, skillID, "self_check", isAdmin)
}

func (h *AuditHandler) run(c *gin.Context, skillID, triggerType string, _ bool) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()

	audit, err := h.auditSvc.RunAudit(ctx, skillID, triggerType, c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	status := http.StatusOK
	if audit.Status != "passed" {
		status = http.StatusOK // 审核执行成功, 结论不合格由业务层展示
	}
	c.JSON(status, gin.H{
		"message": audit.Summary,
		"data":    audit,
	})
}

// GetAuditQueue GET /api/v1/admin/audit-queue — 审核队列 (可选 status 过滤)
func (h *AuditHandler) GetAuditQueue(c *gin.Context) {
	items, err := h.auditSvc.GetAuditQueue(c.Query("status"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items, "total": len(items)})
}

// GetAuditOverview GET /api/v1/admin/audit-overview — 审核概览
func (h *AuditHandler) GetAuditOverview(c *gin.Context) {
	overview, err := h.auditSvc.GetOverview()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": overview})
}

// GetRecentAudits GET /api/v1/admin/audits/recent — 最近审核记录
func (h *AuditHandler) GetRecentAudits(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
	audits, err := h.auditSvc.GetRecentAudits(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": audits, "total": len(audits)})
}

// GetSkillAudits GET /api/v1/admin/skills/:id/audits — 技能审核历史
func (h *AuditHandler) GetSkillAudits(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	audits, err := h.auditSvc.GetSkillAudits(c.Param("id"), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": audits, "total": len(audits)})
}

// GetAuditDetail GET /api/v1/admin/audits/:auditId — 单次审核报告详情
func (h *AuditHandler) GetAuditDetail(c *gin.Context) {
	audit, err := h.auditSvc.GetAudit(c.Param("auditId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if audit == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "审核记录不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": audit})
}

// GetAuditBadge GET /api/v1/skills/:id/audit-badge — 公开可用性徽章
func (h *AuditHandler) GetAuditBadge(c *gin.Context) {
	skill, err := h.skillSvc.GetByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if skill == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "技能不存在"})
		return
	}

	badge := gin.H{
		"skill_id":     skill.ID,
		"skill_key":    skill.SkillKey,
		"version":      skill.Version,
		"audit_status": "pending",
		"verified":     false,
		"engine":       "availability-audit",
	}
	status, score, critical, at, err := h.auditSvc.LatestAuditState(skill.ID)
	if err == nil && status != "" {
		badge["audit_status"] = status
		badge["audit_score"] = score
		if at != nil {
			badge["last_audit_at"] = at
		}
		badge["critical_failures"] = critical
		badge["verified"] = status == "passed" && critical == 0
	}
	// 附最近一次检查项, 便于前端展示"为什么通过/不通过"
	if audits, err := h.auditSvc.GetSkillAudits(skill.ID, 1); err == nil && len(audits) > 0 {
		badge["last_audit"] = audits[0]
	}
	c.JSON(http.StatusOK, gin.H{"data": badge})
}
