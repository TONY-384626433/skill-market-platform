package main

import (
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"

	"github.com/jjbank/skill-market/internal/config"
	"github.com/jjbank/skill-market/internal/handler"
	"github.com/jjbank/skill-market/internal/middleware"
	"github.com/jjbank/skill-market/internal/service"
	"golang.org/x/crypto/bcrypt"
)

// ============================================================
// 验证码存储 (演示用, 内存实现; 生产环境请接入短信/邮件服务)
// ============================================================
var codeStore = struct {
	sync.RWMutex
	m map[string]string // key: phone/email -> code
}{m: make(map[string]string)}

func generateCode() string {
	return fmt.Sprintf("%06d", rand.Intn(1000000))
}

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

	// 兼容旧库: 补充 users.phone 列 (幂等)
	if _, err := db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS phone VARCHAR(32)`); err != nil {
		log.Printf("⚠ 补充 users.phone 列失败(可忽略): %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash TEXT`); err != nil {
		log.Printf("⚠ 补充 users.password_hash 列失败: %v", err)
	}
	demoHash, _ := bcrypt.GenerateFromPassword([]byte("demo"), bcrypt.DefaultCost)
	if _, err := db.Exec(`UPDATE users SET password_hash=$1 WHERE username IN ('admin','zhangsan','lisi','wangwu','zhaoliu') AND COALESCE(password_hash,'')=''`, string(demoHash)); err != nil {
		log.Printf("⚠ 初始化演示账号密码失败: %v", err)
	}

	// 初始化服务
	skillSvc := service.NewSkillService(db, cfg)
	skillHandler := handler.NewSkillHandler(skillSvc)
	gatewayHandler := handler.NewGatewayHandler(skillSvc, cfg)
	githubService := service.NewGitHubService(cfg.GitHub)
	githubHandler := handler.NewGitHubHandler(githubService)

	// 设置 Gin
	r := gin.Default()
	if err := r.SetTrustedProxies([]string{"127.0.0.1", "::1"}); err != nil {
		log.Fatalf("配置可信代理失败: %v", err)
	}
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

	// 健康检查别名 (前端 checkHealth 使用 /api/v1 前缀)
	r.GET("/api/v1/health", func(c *gin.Context) {
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
		var userID, role, dept, passwordHash string
		_ = db.QueryRow(`SELECT id, role, COALESCE(department,''), COALESCE(password_hash,'') FROM users WHERE username=$1`,
			req.Username).Scan(&userID, &role, &dept, &passwordHash)

		if userID == "" || passwordHash == "" || bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)) != nil {
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

	// ============================================================
	// 认证体系: 注册 / 验证码 / 第三方 / 快捷登录 (演示实现)
	// ============================================================

	// 发送验证码 (演示: 验证码直接返回, 便于演示环境使用)
	r.POST("/api/v1/auth/send-code", func(c *gin.Context) {
		var req struct {
			Channel string `json:"channel" binding:"required"` // phone / email
			Target  string `json:"target" binding:"required"`  // 手机号或邮箱
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
			return
		}
		code := generateCode()
		codeStore.Lock()
		codeStore.m[req.Target] = code
		codeStore.Unlock()

		c.JSON(http.StatusOK, gin.H{
			"message":    "验证码已发送",
			"dev_code":   code, // 演示环境直接返回, 生产环境请接入短信/邮件服务
			"expires_in": 300,
		})
	})

	// 注册 (手机号/邮箱 + 验证码)
	r.POST("/api/v1/auth/register", func(c *gin.Context) {
		var req struct {
			Channel     string `json:"channel" binding:"required"` // phone / email
			Target      string `json:"target" binding:"required"`  // 手机号或邮箱
			Code        string `json:"code" binding:"required"`
			Username    string `json:"username" binding:"required,min=3,max=32"`
			Password    string `json:"password" binding:"required,min=6"`
			DisplayName string `json:"display_name"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
			return
		}

		// 校验验证码
		codeStore.RLock()
		stored, ok := codeStore.m[req.Target]
		codeStore.RUnlock()
		if !ok || stored != req.Code {
			c.JSON(http.StatusBadRequest, gin.H{"error": "验证码错误或已过期"})
			return
		}

		// 检查用户名唯一
		var exists string
		_ = db.QueryRow(`SELECT id FROM users WHERE username=$1`, req.Username).Scan(&exists)
		if exists != "" {
			c.JSON(http.StatusConflict, gin.H{"error": "用户名已被占用"})
			return
		}

		// 检查手机号/邮箱是否已注册
		var dup string
		if req.Channel == "phone" {
			_ = db.QueryRow(`SELECT id FROM users WHERE phone=$1`, req.Target).Scan(&dup)
		} else {
			_ = db.QueryRow(`SELECT id FROM users WHERE email=$1`, req.Target).Scan(&dup)
		}
		if dup != "" {
			c.JSON(http.StatusConflict, gin.H{"error": "该手机号/邮箱已注册, 请直接登录"})
			return
		}

		displayName := req.DisplayName
		if displayName == "" {
			displayName = req.Username
		}

		passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "注册失败"})
			return
		}
		var userID string
		if req.Channel == "phone" {
			err := db.QueryRow(`INSERT INTO users (username, display_name, phone, password_hash, department, role) VALUES ($1,$2,$3,$4,$5,'user') RETURNING id`,
				req.Username, displayName, req.Target, string(passwordHash), "").Scan(&userID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "注册失败: " + err.Error()})
				return
			}
		} else {
			err := db.QueryRow(`INSERT INTO users (username, display_name, email, password_hash, department, role) VALUES ($1,$2,$3,$4,$5,'user') RETURNING id`,
				req.Username, displayName, req.Target, string(passwordHash), "").Scan(&userID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "注册失败: " + err.Error()})
				return
			}
		}

		// 清除验证码
		codeStore.Lock()
		delete(codeStore.m, req.Target)
		codeStore.Unlock()

		token, _ := handler.GenerateToken(&cfg.JWT, userID, req.Username, "user", "")
		c.JSON(http.StatusOK, gin.H{
			"token":      token,
			"token_type": "Bearer",
			"expires_in": cfg.JWT.ExpireHour * 3600,
			"user": gin.H{
				"id":       userID,
				"username": req.Username,
				"role":     "user",
			},
		})
	})

	// 手机验证码登录 (无账号自动创建)
	r.POST("/api/v1/auth/phone-login", func(c *gin.Context) {
		var req struct {
			Phone string `json:"phone" binding:"required"`
			Code  string `json:"code" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
			return
		}

		codeStore.RLock()
		stored, ok := codeStore.m[req.Phone]
		codeStore.RUnlock()
		if !ok || stored != req.Code {
			c.JSON(http.StatusBadRequest, gin.H{"error": "验证码错误或已过期"})
			return
		}

		// 查手机号用户, 不存在则自动创建
		var userID, username, role string
		err := db.QueryRow(`SELECT id, username, role FROM users WHERE phone=$1`, req.Phone).Scan(&userID, &username, &role)
		if err == sql.ErrNoRows {
			username = fmt.Sprintf("user_%s", req.Phone[len(req.Phone)-4:])
			err = db.QueryRow(`INSERT INTO users (username, display_name, phone, department, role) VALUES ($1,$2,$3,$4,'user') RETURNING id`,
				username, "手机用户"+req.Phone[len(req.Phone)-4:], req.Phone, "").Scan(&userID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "登录失败: " + err.Error()})
				return
			}
			role = "user"
		}

		codeStore.Lock()
		delete(codeStore.m, req.Phone)
		codeStore.Unlock()

		token, _ := handler.GenerateToken(&cfg.JWT, userID, username, role, "")
		c.JSON(http.StatusOK, gin.H{
			"token":      token,
			"token_type": "Bearer",
			"expires_in": cfg.JWT.ExpireHour * 3600,
			"user": gin.H{
				"id":       userID,
				"username": username,
				"role":     role,
			},
		})
	})

	// 第三方快捷登录 (微信/QQ, 演示: 模拟 OAuth 回调, 自动建号)
	r.POST("/api/v1/auth/oauth", func(c *gin.Context) {
		var req struct {
			Provider string `json:"provider" binding:"required"` // wechat / qq
			OpenID   string `json:"open_id"`
			Nickname string `json:"nickname"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
			return
		}

		openID := req.OpenID
		if openID == "" {
			openID = fmt.Sprintf("%08x", rand.Int63())
		}
		username := fmt.Sprintf("%s_%s", req.Provider, openID)

		var userID, role string
		err := db.QueryRow(`SELECT id, role FROM users WHERE username=$1`, username).Scan(&userID, &role)
		if err == sql.ErrNoRows {
			nickname := req.Nickname
			if nickname == "" {
				nickname = map[string]string{"wechat": "微信用户", "qq": "QQ用户"}[req.Provider]
			}
			err = db.QueryRow(`INSERT INTO users (username, display_name, department, role) VALUES ($1,$2,$3,'user') RETURNING id`,
				username, nickname, "").Scan(&userID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "登录失败: " + err.Error()})
				return
			}
			role = "user"
		}

		token, _ := handler.GenerateToken(&cfg.JWT, userID, username, role, "")
		c.JSON(http.StatusOK, gin.H{
			"token":      token,
			"token_type": "Bearer",
			"expires_in": cfg.JWT.ExpireHour * 3600,
			"user": gin.H{
				"id":       userID,
				"username": username,
				"role":     role,
			},
		})
	})

	// 快捷登录 (演示: 扫码模拟 / 游客体验)
	r.POST("/api/v1/auth/quick", func(c *gin.Context) {
		var req struct {
			Mode string `json:"mode"` // scan / guest
		}
		if req.Mode == "" {
			req.Mode = "guest"
		}

		username := fmt.Sprintf("guest_%d", time.Now().UnixNano()%1000000)
		var userID string
		err := db.QueryRow(`INSERT INTO users (username, display_name, department, role) VALUES ($1,$2,$3,'user') RETURNING id`,
			username, "访客用户", "").Scan(&userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "登录失败: " + err.Error()})
			return
		}

		token, _ := handler.GenerateToken(&cfg.JWT, userID, username, "user", "")
		c.JSON(http.StatusOK, gin.H{
			"token":      token,
			"token_type": "Bearer",
			"expires_in": cfg.JWT.ExpireHour * 3600,
			"user": gin.H{
				"id":       userID,
				"username": username,
				"role":     "user",
			},
		})
	})

	// 公开: 浏览技能市场
	r.GET("/api/v1/skills", skillHandler.SearchSkills)
	r.GET("/api/v1/skills/categories", skillHandler.GetCategories)
	r.GET("/api/v1/skills/:id", skillHandler.GetSkill)
	r.GET("/api/v1/skills/:id/ratings", skillHandler.GetSkillRatings)
	r.GET("/api/v1/skills/stats/overview", skillHandler.GetStats)
	r.GET("/api/v1/github/status", githubHandler.Status)
	r.GET("/api/v1/github/skills/search", githubHandler.SearchSkills)
	r.GET("/api/v1/github/skills/preview", githubHandler.PreviewSkill)
	r.GET("/api/v1/github/skills/download", githubHandler.DownloadSkill)

	// ============================================================
	// 需认证路由
	// ============================================================
	auth := r.Group("/api/v1")
	auth.Use(middleware.JWTAuth(cfg.JWT.Secret))
	{
		// 安装管理
		auth.POST("/skills/:id/install", skillHandler.InstallSkill)
		auth.GET("/skills/my/installations", skillHandler.GetMyInstallations)
		auth.DELETE("/skills/installations/:instId", skillHandler.RevokeInstallation)

		// 评价
		auth.POST("/skills/:id/rate", skillHandler.RateSkill)

		// 技能调用网关
		auth.POST("/gateway/invoke", gatewayHandler.InvokeSkill)

		developer := auth.Group("")
		developer.Use(middleware.RequireRole("developer", "admin"))
		{
			developer.POST("/skills", skillHandler.CreateSkill)
			developer.GET("/skills/my/submissions", skillHandler.GetMySubmissions)
		}

		admin := auth.Group("/admin")
		admin.Use(middleware.RequireRole("admin"))
		{
			admin.GET("/review-queue", skillHandler.GetReviewQueue)
			admin.POST("/skills/:id/review", skillHandler.ReviewSkill)
			admin.GET("/audit-logs", skillHandler.GetAuditLogs)
		}
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
