package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jjbank/skill-market/internal/model"
	"github.com/jjbank/skill-market/internal/service"
)

// SkillHandler 技能 API 处理器
type SkillHandler struct {
	svc *service.SkillService
}

func NewSkillHandler(svc *service.SkillService) *SkillHandler {
	return &SkillHandler{svc: svc}
}

// SearchSkills GET /api/v1/skills — 搜索/浏览技能市场
func (h *SkillHandler) SearchSkills(c *gin.Context) {
	var req model.SkillSearchRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	skills, total, err := h.svc.Search(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":      skills,
		"total":     total,
		"page":      req.Page,
		"page_size": req.PageSize,
	})
}

// GetSkill GET /api/v1/skills/:id — 获取技能详情
func (h *SkillHandler) GetSkill(c *gin.Context) {
	id := c.Param("id")
	skill, err := h.svc.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if skill == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "技能不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": skill})
}

// GetCategories GET /api/v1/skills/categories — 获取分类列表
func (h *SkillHandler) GetCategories(c *gin.Context) {
	categories := []gin.H{
		{"key": "smart-ops", "name": "智能运维", "icon": "MonitorOutlined", "count": 12},
		{"key": "dev-efficiency", "name": "研发效能", "icon": "CodeOutlined", "count": 8},
		{"key": "security", "name": "安全合规", "icon": "SafetyOutlined", "count": 5},
		{"key": "data-governance", "name": "数据治理", "icon": "DatabaseOutlined", "count": 6},
		{"key": "customer-service", "name": "智能客服", "icon": "CustomerServiceOutlined", "count": 4},
		{"key": "risk-control", "name": "风控分析", "icon": "AlertOutlined", "count": 7},
	}
	c.JSON(http.StatusOK, gin.H{"data": categories})
}

// CreateSkill POST /api/v1/skills — 创建/注册新技能
func (h *SkillHandler) CreateSkill(c *gin.Context) {
	var sk model.Skill
	if err := c.ShouldBindJSON(&sk); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	sk.AuthorID = c.GetString("user_id")
	if sk.Version == "" {
		sk.Version = "0.1.0"
	}
	if sk.Stability == "" {
		sk.Stability = "beta"
	}
	if sk.Visibility == "" {
		sk.Visibility = "private"
	}

	if err := h.svc.CreateSkill(&sk); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "技能已提交,等待审核", "data": sk})
}

// InstallSkill POST /api/v1/skills/:id/install — 一键安装技能
func (h *SkillHandler) InstallSkill(c *gin.Context) {
	skillID := c.Param("id")
	userID := c.GetString("user_id")

	var req struct {
		Version string `json:"version"`
	}
	c.ShouldBindJSON(&req)

	inst, apiToken, err := h.svc.InstallSkill(skillID, userID, req.Version)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "安装成功",
		"installation":  inst,
		"api_token":     apiToken,
		"tip":           "请妥善保管 Token, 仅显示一次。可在 Dify 工具配置中使用此 Token 调用技能。",
	})
}

// GetMyInstallations GET /api/v1/skills/my/installations — 我的安装列表
func (h *SkillHandler) GetMyInstallations(c *gin.Context) {
	userID := c.GetString("user_id")
	insts, err := h.svc.GetUserInstallations(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": insts})
}

// RevokeInstallation DELETE /api/v1/skills/installations/:instId — 卸载技能
func (h *SkillHandler) RevokeInstallation(c *gin.Context) {
	instID := c.Param("instId")
	userID := c.GetString("user_id")

	if err := h.svc.RevokeInstallation(instID, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已卸载"})
}

// RateSkill POST /api/v1/skills/:id/rate — 评价技能
func (h *SkillHandler) RateSkill(c *gin.Context) {
	skillID := c.Param("id")
	userID := c.GetString("user_id")

	var req struct {
		Rating  int    `json:"rating" binding:"required,min=1,max=5"`
		Title   string `json:"title"`
		Comment string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	if err := h.svc.RateSkill(skillID, userID, req.Rating, req.Title, req.Comment); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "评价成功"})
}

// GetSkillRatings GET /api/v1/skills/:id/ratings — 技能评价列表
func (h *SkillHandler) GetSkillRatings(c *gin.Context) {
	skillID := c.Param("id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	ratings, total, err := h.svc.GetSkillRatings(skillID, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": ratings, "total": total})
}

// GetStats GET /api/v1/skills/stats/overview — 运营统计概览
func (h *SkillHandler) GetStats(c *gin.Context) {
	stats, err := h.svc.GetOverviewStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": stats})
}

// GetAuditLogs GET /api/v1/skills/audit-logs — 审计日志查询
func (h *SkillHandler) GetAuditLogs(c *gin.Context) {
	userID := c.Query("user_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	logs, total, err := h.svc.GetAuditLogs(userID, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": logs, "total": total})
}
