package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"

	"github.com/jjbank/skill-market/internal/config"
	"github.com/jjbank/skill-market/internal/handler"
	"github.com/jjbank/skill-market/internal/middleware"
	"github.com/jjbank/skill-market/internal/service"
)

func main() {
	// 加载配置
	cfg := config.Load()

	// 连接数据库
	db, err := sql.Open("postgres", cfg.DB.DSN())
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("数据库 Ping 失败: %v", err)
	}
	log.Println("✅ 数据库连接成功")

	// 初始化服务
	skillSvc := service.NewSkillService(db, cfg)
	skillHandler := handler.NewSkillHandler(skillSvc)
	gatewayHandler := handler.NewGatewayHandler(skillSvc, cfg)

	// 设置 Gin
	r := gin.Default()
	r.Use(middleware.CORSMiddleware())

	// ============================================================
	// 公开路由
	// ============================================================
	r.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "Skill Market API",
			"version": "1.0.0",
		})
	})

	// 登录 (简化, 开发用)
	r.POST("/api/v1/auth/login", func(c *gin.Context) {
		var req struct {
			Username string `json:"username" binding:"required"`
			Password string `json:"password" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
			return
		}

		// TODO: 对接 LDAP 验证 — 简化实现
		var userID, role, dept string
		_ = db.QueryRow(`SELECT id, role, COALESCE(department,'') FROM users WHERE username=$1`,
			req.Username).Scan(&userID, &role, &dept)

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
			return
		}

		token, _ := handler.GenerateToken(&cfg.JWT, userID, req.Username, role, dept)

		c.JSON(http.StatusOK, gin.H{
			"token":      token,
			"token_type": "Bearer",
			"expires_in": cfg.JWT.ExpireHour * 3600,
			"user": gin.H{
				"id":       userID,
				"username": req.Username,
				"role":     role,
			},
		})
	})

	// 公开: 浏览技能市场
	r.GET("/api/v1/skills", skillHandler.SearchSkills)
	r.GET("/api/v1/skills/categories", skillHandler.GetCategories)
	r.GET("/api/v1/skills/:id", skillHandler.GetSkill)
	r.GET("/api/v1/skills/:id/ratings", skillHandler.GetSkillRatings)
	r.GET("/api/v1/skills/stats/overview", skillHandler.GetStats)

	// ============================================================
	// 需认证路由
	// ============================================================
	auth := r.Group("/api/v1")
	auth.Use(middleware.JWTAuth(cfg.JWT.Secret))
	{
		// 技能管理
		auth.POST("/skills", skillHandler.CreateSkill)

		// 安装管理
		auth.POST("/skills/:id/install", skillHandler.InstallSkill)
		auth.GET("/skills/my/installations", skillHandler.GetMyInstallations)
		auth.DELETE("/skills/installations/:instId", skillHandler.RevokeInstallation)

		// 评价
		auth.POST("/skills/:id/rate", skillHandler.RateSkill)

		// 审计日志
		auth.GET("/skills/audit-logs", skillHandler.GetAuditLogs)

		// 技能调用网关
		auth.POST("/gateway/invoke", gatewayHandler.InvokeSkill)
	}

	// 启动服务
	addr := fmt.Sprintf(":%s", cfg.ServerPort)
	log.Printf("🚀 Skill 市场 API 启动: http://localhost%s", addr)
	log.Printf("   📋 API 文档:  http://localhost%s/api/v1/skills", addr)
	log.Printf("   💚 健康检查: http://localhost%s/api/health", addr)

	if err := r.Run(addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
