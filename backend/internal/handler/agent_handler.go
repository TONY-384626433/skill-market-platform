package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jjbank/skill-market/internal/model"
	"github.com/jjbank/skill-market/internal/service"
)

// AgentHandler AI 智能体 API
type AgentHandler struct {
	svc *service.AgentService
}

func NewAgentHandler(svc *service.AgentService) *AgentHandler {
	return &AgentHandler{svc: svc}
}

// Status GET /api/v1/agent/status — 智能体运行状态 (模式/模型/可用技能数)
func (h *AgentHandler) Status(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	c.JSON(http.StatusOK, gin.H{"data": h.svc.Status(ctx)})
}

// ListTools GET /api/v1/agent/tools — 智能体可调度的工具清单
func (h *AgentHandler) ListTools(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	tools, err := h.svc.ListTools(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": tools, "total": len(tools)})
}

// Chat POST /api/v1/agent/chat — 自然语言对话并自动编排技能
func (h *AgentHandler) Chat(c *gin.Context) {
	var req struct {
		Message string               `json:"message" binding:"required"`
		History []model.AgentMessage `json:"history"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: message 不能为空"})
		return
	}
	// 只保留最近 6 轮, 避免上下文过长
	if len(req.History) > 12 {
		req.History = req.History[len(req.History)-12:]
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 180*time.Second)
	defer cancel()

	answer, err := h.svc.Chat(ctx, c.GetString("user_id"), req.Message, req.History)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": answer})
}
