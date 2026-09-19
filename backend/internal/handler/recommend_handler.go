package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jjbank/skill-market/internal/service"
)

// RecommendHandler 技能推荐 Agent API
type RecommendHandler struct {
	svc *service.RecommendService
}

func NewRecommendHandler(svc *service.RecommendService) *RecommendHandler {
	return &RecommendHandler{svc: svc}
}

// Status GET /api/v1/agent/recommend/status — 推荐引擎状态 (模式/模型)
func (h *RecommendHandler) Status(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"data": h.svc.Status()})
}

// Recommend POST /api/v1/agent/recommend — 按自然语言需求推荐技能
//   body: { "message": "把生产日志里的身份证手机号脱敏", "top_n": 5 }
func (h *RecommendHandler) Recommend(c *gin.Context) {
	var req struct {
		Message string   `json:"message"`
		Query   string   `json:"query"`
		TopN    int      `json:"top_n"`
		Sources []string `json:"sources"`
		Verify  *bool    `json:"verify_security"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	query := strings.TrimSpace(req.Message)
	if query == "" {
		query = strings.TrimSpace(req.Query)
	}
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请描述你的需求 (message 不能为空)"})
		return
	}
	opts := service.RecommendOptions{TopN: req.TopN, Sources: req.Sources, VerifyGitHub: true}
	if req.Verify != nil {
		opts.VerifyGitHub = *req.Verify
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()

	res, err := h.svc.Recommend(ctx, query, opts)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": res})
}
