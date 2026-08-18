package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jjbank/skill-market/internal/config"
	"github.com/jjbank/skill-market/internal/model"
	"github.com/jjbank/skill-market/internal/service"
)

// GatewayHandler 技能调用网关
type GatewayHandler struct {
	svc *service.SkillService
	cfg *config.Config
}

func NewGatewayHandler(svc *service.SkillService, cfg *config.Config) *GatewayHandler {
	return &GatewayHandler{svc: svc, cfg: cfg}
}

// ============================================================
// 技能默认工具映射 (method=execute 时调用技能的默认工具)
// ============================================================
var skillDefaultTools = map[string]string{
	"db-inspection":        "inspect_database",
	"log-desensitization":  "desensitize",
	"alert-convergence":    "analyze_alerts",
	"requirement-analysis": "analyze_requirement",
}

// seedSkillsDir 技能服务器目录 (可通过环境变量 SEED_SKILLS_DIR 覆盖)
func seedSkillsDir() string {
	if dir := os.Getenv("SEED_SKILLS_DIR"); dir != "" {
		return dir
	}
	return filepath.Join("..", "seed-skills")
}

// InvokeSkill POST /api/v1/gateway/invoke — 统一技能调用入口
// 真实链路: JWT 认证 → Token 权限校验 → 敏感输入检测 → MCP 协议转发 → 输出脱敏 → 审计
func (h *GatewayHandler) InvokeSkill(c *gin.Context) {
	traceID := c.GetString("trace_id")
	if traceID == "" {
		traceID = fmt.Sprintf("tr_%x%04x", time.Now().Unix(), rand.Intn(65536))
	}
	userID := c.GetString("user_id")
	sourceIP := c.ClientIP()

	var req struct {
		SkillKey string                 `json:"skill_key" binding:"required"`
		Method   string                 `json:"method" binding:"required"`
		Params   map[string]interface{} `json:"params"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	// ========== 安全层 1: 身份认证 (由 JWT 中间件保证) ==========

	// ========== 安全层 2: 技能 Token 权限校验 ==========
	// 携带 X-Skill-Token 时校验安装记录; 未携带则仅要求已登录 (用于市场在线试玩)
	if skillToken := c.GetHeader("X-Skill-Token"); skillToken != "" {
		if !h.svc.VerifySkillToken(req.SkillKey, userID, skillToken) {
			h.recordAudit(traceID, req.SkillKey, userID, "forbidden", 0, sourceIP, false)
			c.JSON(http.StatusForbidden, gin.H{"error": "技能 Token 无效或未安装该技能"})
			return
		}
	}

	// ========== 安全层 3: 限流 (简化: 演示环境不做) ==========

	// ========== 安全层 4: 敏感信息检测 (输入) ==========
	// 脱敏类技能(白名单)处理的就是敏感数据, 跳过输入拦截, 由技能完成脱敏
	if !isDesensitizeSkill(req.SkillKey) && hasSensitiveData(req.Params) {
		h.recordAudit(traceID, req.SkillKey, userID, "blocked", 0, sourceIP, true)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "检测到敏感信息, 请求已阻止。请对输入参数进行脱敏处理。",
		})
		return
	}

	// ========== 层 5: MCP 协议适配 + 真实转发到技能服务 ==========
	start := time.Now()
	result, err := h.realInvoke(c.Request.Context(), req.SkillKey, req.Method, req.Params)
	if err != nil {
		h.recordAudit(traceID, req.SkillKey, userID, "failed", int(time.Since(start).Milliseconds()), sourceIP, false)
		c.JSON(http.StatusBadGateway, gin.H{
			"trace_id":    traceID,
			"status":      "failed",
			"duration_ms": time.Since(start).Milliseconds(),
			"error":       err.Error(),
		})
		return
	}
	duration := time.Since(start)

	// ========== 层 6: 敏感信息检测 (输出) ==========
	result = maskSensitiveOutput(result)

	// ========== 层 7: 审计记录 ==========
	h.recordAudit(traceID, req.SkillKey, userID, "success", int(duration.Milliseconds()), sourceIP, false)
	_ = h.svc.BumpCallCount(req.SkillKey)

	c.JSON(http.StatusOK, gin.H{
		"trace_id":    traceID,
		"status":      "success",
		"duration_ms": duration.Milliseconds(),
		"data":        result,
	})
}

// realInvoke 真实调用技能服务: HTTP 转发到 SKILL RUNNER
// POST {SKILL_RUNNER_URL}/<skill_key>/mcp (JSON-RPC 2.0, MCP/2024-11-05)
func (h *GatewayHandler) realInvoke(ctx context.Context, skillKey, method string, params map[string]interface{}) (map[string]interface{}, error) {
	// 路径安全: 拒绝穿越
	if strings.ContainsAny(skillKey, "/\\..") {
		return nil, fmt.Errorf("非法技能标识")
	}

	// 解析工具名: method=execute 时使用技能默认工具
	toolName := method
	if toolName == "" || toolName == "execute" {
		toolName = skillDefaultTools[skillKey]
		if toolName == "" {
			return nil, fmt.Errorf("未知技能: %s", skillKey)
		}
	}

	// 技能服务地址 (docker-compose 中为 http://skill-runner:8081, 本地开发为 http://localhost:8081)
	baseURL := strings.TrimRight(os.Getenv("SKILL_RUNNER_URL"), "/")
	if baseURL == "" {
		baseURL = "http://localhost:8081"
	}
	skillURL := fmt.Sprintf("%s/%s/mcp", baseURL, skillKey)

	// 构造 MCP tools/call 请求
	reqBody, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      toolName,
			"arguments": params,
		},
	})

	// HTTP 转发调用技能服务
	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(cmdCtx, http.MethodPost, skillURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("技能服务不可达 (%s): %v", skillURL, err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取技能响应失败: %v", err)
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("技能服务返回 %d: %s", httpResp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	// 解析 MCP 响应
	var resp struct {
		Result *struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("技能返回解析失败: %v", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("技能错误: %s", resp.Error.Message)
	}
	if resp.Result == nil {
		return nil, fmt.Errorf("技能返回为空")
	}

	// 组装结果
	text := ""
	for _, content := range resp.Result.Content {
		text += content.Text
	}
	return map[string]interface{}{
		"skill_key": skillKey,
		"tool":      toolName,
		"result":    text,
		"content":   resp.Result.Content,
	}, nil
}

// findPython 查找可用的 Python 运行时 (已废弃: 改为 HTTP 转发 skill-runner)


func (h *GatewayHandler) recordAudit(traceID, skillKey, userID, status string, durationMs int, sourceIP string, pii bool) {
	_ = h.svc.RecordAudit(&model.AuditLog{
		TraceID:        traceID,
		SkillID:        skillKey,
		UserID:         userID,
		Method:         skillKey,
		ResponseStatus: status,
		DurationMs:     durationMs,
		SourceIP:       sourceIP,
		PIIDetected:    pii,
	})
}

// isDesensitizeSkill 脱敏类技能白名单 (输入天然包含敏感数据)
func isDesensitizeSkill(skillKey string) bool {
	switch skillKey {
	case "log-desensitization", "pii-detection", "data-masking":
		return true
	}
	return false
}

// ============================================================
// 敏感信息检测 (简化版, 实际应集成脱敏模型)
// ============================================================

var sensitivePatterns = []string{
	"id_card", "身份证", "phone", "手机", "password", "密码",
	"token", "secret", "api_key", "access_key",
}

func hasSensitiveData(params map[string]interface{}) bool {
	for k, v := range params {
		// 检查键名
		for _, pattern := range sensitivePatterns {
			if containsKeyword(k, pattern) {
				return true
			}
		}
		// 检查值
		if s, ok := v.(string); ok {
			for _, pattern := range sensitivePatterns {
				if len(s) > 0 && containsKeyword(s, pattern) {
					return true
				}
			}
		}
	}
	return false
}

func maskSensitiveOutput(data map[string]interface{}) map[string]interface{} {
	// 简化: 实际应对身份证/手机号/银行卡号等格式做正则脱敏
	return data
}

func containsKeyword(s, keyword string) bool {
	return len(s) >= len(keyword) && searchSubstring(s, keyword)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// GenerateToken 生成登录 Token (简化)
func GenerateToken(cfg *config.JWTConfig, userID, username, role, department string) (string, error) {
	claims := jwt.MapClaims{
		"user_id":    userID,
		"username":   username,
		"role":       role,
		"department": department,
		"exp":        time.Now().Add(time.Duration(cfg.ExpireHour) * time.Hour).Unix(),
		"iat":        time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(cfg.Secret))
}
