# 九江银行 SkillHub

九江银行内部 AI 能力中心，用于检索、安装、调用、发布和治理组织内的 MCP 服务、工作流与 API。

当前版本内置 4 个可真实调用的 MCP 技能，所有市场数量与运营指标均来自 PostgreSQL 实时数据，不使用前端硬编码演示值。

## 产品能力

- 技能市场：关键词、分类、接入形态和排序筛选，展示版本、稳定性、维护团队、安装量与评分。
- 技能详情：能力说明、结构化接口、权限声明、调用示例、在线试玩和用户评价。
- 安装管理：生成独立访问令牌、查看令牌前缀、撤销授权和安全重新安装。
- 开发者工作台：分步提交 Manifest、接口定义、依赖和权限，查看审核状态与整改意见。
- 平台治理：实时运营指标、人工审核队列、通过/驳回和全链路调用审计。
- 身份与安全：bcrypt 密码校验、JWT、角色权限、加密随机令牌、敏感输入拦截和审计留痕。

## 快速开始

Windows 演示环境可直接双击：

```text
start-demo.bat
```

脚本会依次启动 Docker 基础服务、技能运行容器、Go API、前端预览和公网隧道。停止本地服务可运行 `stop-all.bat`。

手动启动：

```bash
# 1. PostgreSQL、Redis、MinIO 与 skill-runner
cd docker
docker compose up -d --build

# 2. Go API
cd ../backend
go run ./cmd

# 3. React 开发服务器
cd ../frontend
npm install
npm start
```

本地开发地址：[http://localhost:3000](http://localhost:3000)

## 演示账号

| 角色 | 用户名 | 密码 | 可用工作区 |
|---|---|---|---|
| 平台管理员 | `admin` | `demo` | 市场、我的技能、开发者、平台治理 |
| 开发者 | `zhangsan` | `demo` | 市场、我的技能、开发者 |
| 普通用户 | `zhaoliu` | `demo` | 市场、我的技能 |

演示账号仅用于本地环境。新注册账号密码会使用 bcrypt 哈希存储。

## 真实技能

| 技能标识 | 名称 | 主要能力 |
|---|---|---|
| `db-inspection` | 数据库智能巡检助手 | 数据库健康检查与巡检报告 |
| `log-desensitization` | 日志敏感信息识别 | PII 识别与日志脱敏 |
| `alert-convergence` | 告警收敛分析 | 告警聚合、根因识别与报告 |
| `requirement-analysis` | AI 需求分析助手 | 需求细化与规范化文档输出 |

后端通过 JSON-RPC over MCP 将在线试玩请求转发到 `skill-runner:8081`，不是前端模拟结果。

## 角色权限

| 操作 | 普通用户 | 开发者 | 管理员 |
|---|---:|---:|---:|
| 浏览、安装、评价、试玩 | 是 | 是 | 是 |
| 提交技能、查看我的发布 | 否 | 是 | 是 |
| 审核技能、查看全量审计 | 否 | 否 | 是 |

服务端通过 JWT 中间件和 `RequireRole` 同时校验，前端菜单隐藏不作为权限边界。

## 核心 API

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| `GET` | `/api/health` | 公开 | 健康检查 |
| `POST` | `/api/v1/auth/login` | 公开 | 账号密码登录 |
| `GET` | `/api/v1/skills` | 公开 | 搜索公开技能 |
| `GET` | `/api/v1/skills/categories` | 公开 | 实时分类统计 |
| `GET` | `/api/v1/skills/stats/overview` | 公开 | 实时运营指标 |
| `POST` | `/api/v1/skills/:id/install` | 登录 | 安装并签发令牌 |
| `GET` | `/api/v1/skills/my/installations` | 登录 | 我的安装 |
| `POST` | `/api/v1/gateway/invoke` | 登录 | 调用真实技能 |
| `POST` | `/api/v1/skills` | 开发者/管理员 | 提交技能审核 |
| `GET` | `/api/v1/skills/my/submissions` | 开发者/管理员 | 我的发布 |
| `GET` | `/api/v1/admin/review-queue` | 管理员 | 审核队列 |
| `POST` | `/api/v1/admin/skills/:id/review` | 管理员 | 通过或驳回 |
| `GET` | `/api/v1/admin/audit-logs` | 管理员 | 调用审计 |

## 项目结构

```text
skill-market-platform/
├── backend/                 Go + Gin API、权限、审核与网关
├── docker/                  PostgreSQL、Redis、MinIO、skill-runner
├── frontend/                React 18 + Ant Design 企业工作台
├── seed-skills/             4 个 MCP 技能实现
├── roadshow/                演示与答辩材料
├── start-demo.bat           Windows 一键启动
└── stop-all.bat             停止本地服务
```

## 构建验证

```bash
cd backend && go build ./...
cd frontend && npm run build
```

建议发布前同时用桌面 `1440×1000` 和手机 `390×844` 视口检查市场、详情、登录、开发者与治理页面。
