package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jjbank/skill-market/internal/config"
	"github.com/jjbank/skill-market/internal/model"
	"github.com/jjbank/skill-market/internal/service"
	"time"
)

// GatewayHandler 技能调用网关
type GatewayHandler struct {
	svc *service.SkillService
	cfg *config.Config
}

func NewGatewayHandler(svc *service.SkillService, cfg *config.Config) *GatewayHandler {
	return &GatewayHandler{svc: svc, cfg: cfg}
}

// InvokeSkill POST /api/v1/gateway/invoke — 统一技能调用入口
func (h *GatewayHandler) InvokeSkill(c *gin.Context) {
	traceID := c.GetString("trace_id")
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

	// ========== 安全层 2: 权限校验 ==========
	// 检查用户是否安装了该技能且 Token 有效
	// (简化实现: 实际应从 X-Skill-Token header 校验安装记录)

	// ========== 安全层 3: 限流 ==========
	// (简化实现: 实际应集成 Redis 令牌桶)

	// ========== 安全层 4: 敏感信息检测 (输入) ==========
	if hasSensitiveData(req.Params) {
		// 记录审计日志
		h.recordAudit(traceID, req.SkillKey, userID, "blocked", 0, sourceIP, true)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "检测到敏感信息, 请求已阻止。请对输入参数进行脱敏处理。",
		})
		return
	}

	// ========== 层 5: 协议适配 + 转发调用 ==========
	// (简化: 返回模拟结果; 实际应代理转发到技能 Endpoint)
	start := time.Now()
	result := h.mockInvoke(req.SkillKey, req.Method, req.Params)
	duration := time.Since(start)

	// ========== 层 6: 敏感信息检测 (输出) ==========
	result = maskSensitiveOutput(result)

	// ========== 层 7: 审计记录 ==========
	h.recordAudit(traceID, req.SkillKey, userID, "success", int(duration.Milliseconds()), sourceIP, false)

	c.JSON(http.StatusOK, gin.H{
		"trace_id":    traceID,
		"status":      "success",
		"duration_ms": duration.Milliseconds(),
		"data":        result,
	})
}

// mockInvoke 模拟技能调用 (开发阶段, 实际应代理转发)
func (h *GatewayHandler) mockInvoke(skillKey, method string, params map[string]interface{}) map[string]interface{} {
	// 根据技能类型返回模拟数据
	switch skillKey {
	case "db-inspection":
		return map[string]interface{}{
			"report": "## 数据库巡检报告\n- 健康评分: 92/100\n- 慢查询: 3 条\n- 连接数: 正常",
			"score":  92,
			"issues": []string{"慢查询 > 5s: 3条", "磁盘使用率: 72%"},
		}
	case "log-desensitization":
		return map[string]interface{}{
			"masked_content": "用户 *** 于 *** 登录系统, 身份证号 **********",
			"entities_found": []map[string]interface{}{
				{"type": "person_name", "count": 1},
				{"type": "id_card", "count": 1},
			},
		}
	default:
		return map[string]interface{}{
			"result":  "success",
			"message": "技能 " + skillKey + "." + method + " 执行完成",
		}
	}
}

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

// ============================================================
// 敏感信息检测 (简化版, 实际应集成脱敏模型)
// ============================================================

var sensitivePatterns = []string{
	"id_card", "身份证", "phone", "手机", "password", "密码",
	"token", "secret", "api_key", "access_key",
}

func hasSensitiveData(params map[string]interface{}) bool {
	for _, v := range params {
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
