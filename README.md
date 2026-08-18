# 私域 Skill 市场及配套机制建设应用

> 中南财经政法大学 × 九江银行 金融科技专项赛  
> 选题: 私域 Skill 市场及配套机制建设应用  
> 对标: 腾讯 SkillHub  

---

## 快速开始

```bash
# 1. 启动基础设施
cd docker && docker compose up -d

# 2. 启动后端 API (需要 Go 1.21+)
cd backend && go run cmd/main.go

# 3. 启动前端 (需要 Node 18+)
cd frontend && npm install && npm start

# 4. 启动种子技能 (4 个终端)
cd seed-skills/db-inspection && python server.py
cd seed-skills/log-desensitization && python server.py
cd seed-skills/alert-convergence && python server.py
cd seed-skills/requirement-analysis && python server.py
```

打开 http://localhost:3000 即可访问。

## 项目结构

```
skill-market-platform/
├── README.md                   # 本文件
├── docker/                     # 基础设施
│   ├── docker-compose.yml      # PostgreSQL + Redis + ES + MinIO + RabbitMQ
│   └── .env.example
├── backend/                    # 后端 API (Go + Gin)
│   ├── cmd/main.go             # 入口
│   ├── internal/
│   │   ├── config/config.go    # 配置
│   │   ├── model/skill.go      # 数据模型
│   │   ├── handler/            # HTTP 处理器
│   │   │   ├── skill_handler.go
│   │   │   └── gateway_handler.go
│   │   ├── service/skill_service.go  # 业务逻辑
│   │   └── middleware/auth.go  # JWT 认证
│   └── migrations/001_init.sql  # 数据库 DDL
├── frontend/                   # 前端 (React + Ant Design)
│   ├── src/
│   │   ├── App.js              # 主应用 (路由 + 布局)
│   │   ├── pages/
│   │   │   ├── MarketPage.js       # 技能市场首页
│   │   │   ├── SkillDetailPage.js  # 技能详情页
│   │   │   ├── LoginPage.js
│   │   │   ├── DeveloperPage.js    # 开发者工作台
│   │   │   ├── AdminPage.js        # 管理后台
│   │   │   └── MyInstallationsPage.js
│   │   └── services/api.js     # API 封装
│   └── package.json
├── seed-skills/                # 4 个种子技能 (MCP Server)
│   ├── db-inspection/          # 数据库智能巡检
│   ├── log-desensitization/    # 日志敏感信息识别
│   ├── alert-convergence/      # 告警收敛分析
│   └── requirement-analysis/   # AI 需求分析助手
└── roadshow/                   # 路演材料
    ├── rehearsal-script.md     # 15 分钟逐页讲稿
    ├── demo-manual.md          # 5 分钟演示操作手册
    └── qa-handbook.md          # 40+ 评委预判问题应答
```

## 核心 API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/health` | 健康检查 |
| POST | `/api/v1/auth/login` | 登录 |
| GET | `/api/v1/skills` | 搜索技能 |
| GET | `/api/v1/skills/:id` | 技能详情 |
| POST | `/api/v1/skills` | 创建技能 (需认证) |
| POST | `/api/v1/skills/:id/install` | 安装技能 (需认证) |
| GET | `/api/v1/skills/my/installations` | 我的安装 (需认证) |
| POST | `/api/v1/skills/:id/rate` | 评价 (需认证) |
| POST | `/api/v1/gateway/invoke` | 调用技能 (需认证) |
| GET | `/api/v1/skills/audit-logs` | 审计日志 (需认证) |
| GET | `/api/v1/skills/stats/overview` | 运营统计 |

## 技术栈

| 层 | 技术 |
|---|------|
| 前端 | React 18 + Ant Design 5 + React Router 6 |
| 后端 API | Go 1.21 + Gin + JWT |
| 技能 Server | Python 3 + MCP Protocol |
| 数据库 | PostgreSQL 15 |
| 缓存 | Redis 7 |
| 搜索 | Elasticsearch 8 |
| 存储 | MinIO |
| 队列 | RabbitMQ |
| 密管 | Vault |

## 版本

v1.0.0 — 2026 年 8 月 — 比赛 MVP 版本
