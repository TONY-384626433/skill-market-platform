# 九江银行 SkillHub

九江银行内部 AI 能力中心，用于检索、安装、调用、发布和治理组织内的 MCP 服务、工作流与 API。

当前版本内置 4 个可真实调用的 MCP 技能，所有市场数量与运营指标均来自 PostgreSQL 实时数据，不使用前端硬编码演示值。

## 产品能力

- 技能市场：关键词、分类、接入形态和排序筛选，展示版本、稳定性、维护团队、安装量与评分。
- GitHub 开源索引：搜索公开 `SKILL.md`，解析元数据与类别，按本机操作系统和运行时标注安装兼容性；下载前可在只读抽屉中预览渲染文档与原始源码。
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

## GitHub 开源技能索引

SkillHub 支持两个边界清晰的数据源：

- 企业审核库：来自 PostgreSQL，技能经过平台审核，可安装并通过网关调用。
- GitHub 开源源：来自 GitHub 公开代码索引，可搜索和下载，但不代表已经通过企业安全审核。

一键启动脚本会在本机已执行 `gh auth login` 时，将 GitHub Token 仅传给后端子进程，不会打印或写入仓库。手动启动可设置：

```powershell
$env:GITHUB_TOKEN = gh auth token
go run ./cmd
```

没有 Token 时会降级为公开仓库搜索。GitHub Code Search 对单个查询最多开放前 1000 条结果，可通过关键词继续缩小范围。兼容性探针会检测 Windows/Linux/macOS、CPU 架构及 Node.js、Python、Docker、Go、Rust、Bash、PowerShell 等运行时，并区分“本机可安装”“需要配置”和“当前不兼容”。点击技能卡会先以只读方式加载 `SKILL.md`，可切换渲染预览与原始源码；预览成功后才开放下载。下载接口只打包对应 Skill 目录；目录超过 160 个文件或 20 MB 时自动降级为 `SKILL.md` 与安全说明，不执行任何第三方代码。

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
| 对本人技能运行可用性自检 | 否 | 是 | 是 |
| 运行全量可用性审核、发布门禁、全量审计 | 否 | 否 | 是 |

服务端通过 JWT 中间件和 `RequireRole` 同时校验，前端菜单隐藏不作为权限边界。

## 核心 API

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| `GET` | `/api/health` | 公开 | 健康检查 |
| `POST` | `/api/v1/auth/login` | 公开 | 账号密码登录 |
| `GET` | `/api/v1/skills` | 公开 | 搜索公开技能 |
| `GET` | `/api/v1/skills/categories` | 公开 | 实时分类统计 |
| `GET` | `/api/v1/skills/stats/overview` | 公开 | 实时运营指标 |
| `GET` | `/api/v1/github/status` | 公开 | GitHub 连接与本机运行时状态 |
| `GET` | `/api/v1/github/skills/search` | 公开 | 搜索 GitHub 公开 Skill |
| `GET` | `/api/v1/github/skills/preview` | 公开 | 只读预览单个公开 `SKILL.md` |
| `GET` | `/api/v1/github/skills/download` | 公开 | 下载单个公开 Skill ZIP |
| `POST` | `/api/v1/skills/:id/install` | 登录 | 安装并签发令牌 |
| `GET` | `/api/v1/skills/my/installations` | 登录 | 我的安装 |
| `POST` | `/api/v1/gateway/invoke` | 登录 | 调用真实技能 |
| `POST` | `/api/v1/skills` | 开发者/管理员 | 提交技能审核 |
| `GET` | `/api/v1/skills/my/submissions` | 开发者/管理员 | 我的发布 |
| `GET` | `/api/v1/admin/review-queue` | 管理员 | 待人工复核队列 |
| `POST` | `/api/v1/admin/skills/:id/review` | 管理员 | 通过或驳回（通过前必须已过可用性审核） |
| `GET` | `/api/v1/admin/audit-logs` | 管理员 | 调用审计 |
| `GET` | `/api/v1/admin/audit-overview` | 管理员 | 可用性审核概览 |
| `GET` | `/api/v1/admin/audit-queue` | 管理员 | 审核队列（含最近结论与得分） |
| `POST` | `/api/v1/admin/skills/:id/audit` | 管理员 | 运行可用性自动检测 |
| `GET` | `/api/v1/admin/skills/:id/audits` | 管理员 | 技能审核历史 |
| `GET` | `/api/v1/admin/audits/recent` | 管理员 | 最近检测流水 |
| `GET` | `/api/v1/admin/audits/:auditId` | 管理员 | 单次审核报告 |
| `POST` | `/api/v1/skills/:id/self-check` | 开发者/管理员 | 开发者自检（仅本人技能） |
| `GET` | `/api/v1/skills/:id/audit-badge` | 公开 | 可用性徽章 |
| `GET` | `/api/v1/agent/status` | 登录 | 智能体运行模式与可调度能力数 |
| `GET` | `/api/v1/agent/tools` | 登录 | 智能体工具清单（来自真实 `tools/list`） |
| `POST` | `/api/v1/agent/chat` | 登录 | 自然语言对话并自动编排技能 |

## AI 智能体

智能体把“自然语言”翻译成“技能调用”：

```
用户提问 → 工具发现（仅已过审技能）→ 编排决策 → 真实 tools/call → 汇总回答
                                             ↑
                          大模型 function calling　或　本地意图引擎（降级）
```

- **工具发现**：智能体可调度的工具不是写死的，而是从「已发布 + 已通过可用性审核」的技能上真实拓 `tools/list` 得到（带 60s 缓存）。未过审的技不可能被智能体调到。
- **双模式**：
  - `llm` — 配置了 `LLM_API_KEY` 时，用 OpenAI 兼容的 function calling 自主编排，最多 4 轮工具调用；
  - `local-intent` — 未配置 Key（或大模型不可用）时自动降级为关键词意图引擎，演示环境永远可用。
- **安全一致**：智能体侧同样执行敏感输入拦截（脱敏类技能白名单放行），并将每次调用写入 `skill_audit_logs`（`source_ip=agent`，带 trace_id）并计入技能调用量。

启用大模型编排（OpenAI 兼容接口，任选一家）：

```powershell
$env:LLM_API_KEY  = "sk-xxxxxxxx"
$env:LLM_API_BASE = "https://api.deepseek.com/v1"   # 或 https://api.openai.com/v1 等
$env:LLM_MODEL    = "deepseek-chat"
# 重启后端后 GET /api/v1/agent/status 会显示 mode=llm
```

验收：`python e2e_agent_test.py`（15 项断言：工具发现 / 四类意图真实调用 / 拦截 / 白名单 / 审计）。

## 技能可用性审核

审核要回答的问题不是"元数据填得全不全"，而是**"这个技能到底能不能正常用"**。引擎对每个技能跑 6 项检查并加权评分（总分 100）：

| 检查项 | 类别 | 级别 | 检测内容 |
|---|---|---|---|
| `AVAIL-01` 元数据完整性 | 静态 | 致命 | `skill_key` 命名、名称/简介/分类、语义化版本、标签、责任人 |
| `AVAIL-02` 接口定义可解析 | 静态 | 致命 | 接入形态白名单、`endpoint_url` 协议、`manifest` 结构与输入输出契约 |
| `AVAIL-03` 服务可达与协议握手 | 运行 | 致命 | 真实发送 `initialize` + `tools/list`，校验协议版本与工具契约 |
| `AVAIL-04` 核心功能可调用 | 运行 | 致命 | 按 `inputSchema` 自动生成探针入参并真实 `tools/call`，确认返回有效结果 |
| `AVAIL-05` 响应性能基线 | 运行 | 重要 | 连续 3 次调用统计成功率、平均与峰值耗时 |
| `AVAIL-06` 安全合规基线 | 安全 | 重要 | 权限声明、输出是否含明文 PII、网关敏感输入拦截是否一致 |

判定规则：总分低于 70 或存在"致命"项失败 → **不合格**；90/80/70 分对应 A/B/C 等级。

门禁：`POST /api/v1/admin/skills/:id/review` 通过发布前会校验审核结论，未通过返回 `409`；`POST /api/v1/skills/:id/install` 对未通过审核的技能直接拒绝安装。检测结果同时回写 `skills.audit_status / audit_score / last_audit_at`，并写入 `skill_audits` 与 `skill_reviews` 流水。

```bash
# 验收脚本（27 项断言，含故意构造的不可用技能必须被判不合格）
python e2e_audit_test.py
```

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

## 技能安全治理 (供应链安全 · 可落地银行内网)

面向「技能能不能进银行」的四道闸门：**静态查毒、动态沙箱、防盗用、先审后下**。

### 1. 静态安全扫描 (查毒 / 危险行为 / 提示注入)

- 规则库外置为数据文件 `backend/rules/security-rules.json`（44 条规则 / 12 个分类，含 `DYN-01~08` 动态行为规则），**不编入二进制**：
  可独立升级、回滚、审计，同时避免病毒特征串被编入可执行文件导致引擎自身被杀软误报隔离（实测会触发）。
- 未装载规则库时后端**拒绝启动**（fail-closed），不会出现「无规则放行」。
- 分类覆盖：`malware`(病毒/挖矿/勒索/EICAR)、`execution`(命令执行/eval)、`obfuscation`(Base64 载荷)、
  `network`(外联回传/环境变量外发)、`credential`(凭据窃取/硬编码密钥)、`persistence`、`destructive`、
  `injection`(提示注入/零宽字符/注释夹带)、`supply_chain`(未固定依赖/URL 安装/容器逃逸)、
  `compliance`(声明与实现不一致 —— 防幻觉/隐瞒能力)、`integrity`(路径穿越/签名缺失/篡改)。
- 二进制载荷（含 NUL 的 exe/dll 等）仍做病毒特征匹配，查毒不因文件类型或编码绕过。
- 结论三级：`safe` / `suspicious`(≥45 分或 ≥2 项高危) / `malicious`(任一严重项 → 阻断并隔离)。

### 2. 防盗用 / 溯源

- **指纹查重**：内容规范化 + SHA-256 + SimHash，改名重传同样命中（阈值 75%），命中即转人工复核。
- **水印**：技能级 `WM-xxxx` + 单次交付 `DL-xxxx`（绑定下载人与时间），泄漏可反查到人。
- **包签名**：交付包自动注入 `MANIFEST.skillhub.json`（逐文件 SHA-256 + HMAC-SHA256 签名）与
  `.skillhub-provenance.json`，安装前可验签；内容被改动即判定 `tampered`。
- **下载审计**：每次下载留痕（用户/IP/水印/通道），`/admin/download-audits` 可查。

### 3. GitHub 导入门禁（先审核，后下载）

- `GET /github/skills/download` **必须携带已通过审查的 `request_id`**，否则 403 `security_review_required`。
- 流程：提交审查 → 抓包静态查毒 + 指纹查重 → `safe` 自动放行（可用 `IMPORT_AUTO_APPROVE=0` 改为全人工）/
  `suspicious` 转人工复核 / `malicious` 直接阻断（**不允许人工放行**）→ 通过后下载才注入签名与水印。

### 核心 API

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/v1/security/rules` | 公开 | 规则库 + 引擎元信息 + 安全策略（可审计） |
| GET | `/api/v1/skills/:id/security-badge` | 公开 | 技能安全徽章（结论/风险分/水印） |
| POST | `/api/v1/github/import-requests` | 登录 | 提交外部技能安全审查（先审后下入口） |
| GET | `/api/v1/github/import-requests/:reqId` | 登录 | 查询审查单 |
| GET | `/api/v1/admin/security-overview` | 管理员 | 安全治理概览 |
| GET | `/api/v1/admin/security-queue` | 管理员 | 技能安全队列 |
| POST | `/api/v1/admin/skills/:id/security-scan` | 管理员 | 运行静态安全扫描 |
| GET | `/api/v1/admin/security-scans[/:scanId]` | 管理员 | 扫描记录 / 报告详情 |
| GET/POST | `/api/v1/admin/skills/:id/provenance[/verify]` | 管理员 | 溯源档案 / 完整性校验 |
| GET | `/api/v1/admin/download-audits` | 管理员 | 下载审计（水印可反查） |
| GET | `/api/v1/admin/import-requests` | 管理员 | 导入审查队列 |
| POST | `/api/v1/admin/import-requests/:reqId/scan` | 管理员 | 重新审查 |
| POST | `/api/v1/admin/import-requests/:reqId/decision` | 管理员 | 通过 / 驳回（阻断项不可放行） |
| GET | `/api/v1/admin/security-rules/export` | 管理员 | 导出规则库（审计/备份） |

### 验收脚本

```bash
# 引擎单测 (30 项: 安全技能/恶意样本/路径穿越/幻觉一致性/零宽注入/指纹查重/签名篡改/水印/规则库/fail-closed/沙箱规则/沙箱客户端)
go test ./internal/security/ ./internal/service/

# 端到端 (35 项: 规则库→内部扫描→徽章→溯源→篡改检测→门禁 403→审查放行→签名复核→审计留痕→高危阻断→上传预检)
python backend/e2e_security_test.py

# 动态沙箱端到端 (22 项: 隔离自检 → DYN 规则 → 内部技能真跑不误伤 → 正常包放行 → 行为型样本命中 DYN-01/02/04/06/08 → 阻断 → 报告可回读取证)
python backend/e2e_sandbox_test.py
```

### 环境变量

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `SKILLHUB_SECURITY_RULES` | `rules/security-rules.json` | 规则库文件路径 |
| `SKILLHUB_SIGNING_KEY` | 内置演示密钥 | 包签名/水印密钥（生产必须由密钥管理注入） |
| `IMPORT_AUTO_APPROVE` | `1` | 审查结论为 `safe` 时是否自动放行（`0`=全部人工审批） |
| `SEED_SKILLS_DIR` | `../seed-skills` | 本地技能包目录（扫描对象） |
| `SKILLHUB_DYNAMIC_SANDBOX` | `1` | 是否启用动态沙箱验证（`0`=仅静态扫描） |
| `SKILLHUB_SANDBOX_URL` | `http://localhost:8090` | 沙箱服务地址 |
| `SKILLHUB_SANDBOX_TIMEOUT` | `60s` | 单次动态验证超时 |

### 4. 上传包安全预检（入库前第一道闸门）

```bash
curl -F "file=@skill.zip" -H "Authorization: Bearer <admin-token>" \
  http://localhost:8080/api/v1/admin/security/scan-package
```

- 支持 `.zip` 或单文件（上限 24MB / 200 个文件），上传即做：查毒 + 危险行为 + 提示注入 + 查重；
- 命中任何严重项 → `blocked=true`（禁止入库）；结论写入扫描记录（`subject_type=package`，`trigger_type=upload`）；
- 包内容只在内存与隔离临时目录中处理，**不执行任何技能代码**。

### 5. 外部查毒引擎（ClamAV / YARA）适配

- 自动探测 `clamscan` / `clamdscan` / `yara`；**有则调用并合并结论**（`MAL-06 ClamAV 命中` / `MAL-07 YARA 命中`，均为阻断级），
- **无则降级**为内置特征库，并在 `/security/rules` 的 `engine.av_engines` 与扫描记录中标注 `available=false` + 原因，绝不静默假装扫过；
- 可指定：`SKILLHUB_CLAMAV_BIN`、`SKILLHUB_YARA_BIN`、`SKILLHUB_YARA_RULES`、`SKILLHUB_AV_TIMEOUT`（默认 90s，超时记为高优先级待复核项）。

### 6. 签名密钥管理（KMS 对接）

- 解析顺序：`SKILLHUB_SIGNING_KEY_FILE`（推荐，对接 KMS/密钥管理挂载）> `SKILLHUB_SIGNING_KEY` > 内置演示密钥；
- `SKILLHUB_ENV=production`（或 `prod`/`bank`）时若仍使用内置演示密钥 → **后端拒绝启动**；
- 对外只公开密钥指纹 `key_id`（如 `kid_4f3339f8`），写入签名清单并参与签名载荷，可审计「签发所用密钥版本」，换密钥后旧签名自动失效。

### 7. 动态沙箱行为验证（静态查毒之外的第二道防线）

静态扫描能看出「代码里想干什么」，动态沙箱则验证「跑起来究竟干了什么」——可执行技能会在隔离容器里被**真跑一次**。

```bash
# 构建并启动沙箱（首次或代码变更后）
docker compose -f docker/docker-compose.yml build sandbox sandbox-gw
docker compose -f docker/docker-compose.yml up -d --no-build sandbox sandbox-gw
curl http://localhost:8090/health   # 含隔离自检结果
```

- **隔离手段**：独立内网（`internal: true`，无外网可达）+ 只读根文件系统 + 非 root（uid 65534）+ `cap_drop: ALL` + `no-new-privileges` + 内存 512MB / 进程 128 / CPU 1.0 限额；
- **行为观测**（不依赖 strace/LD_PRELOAD，离线可用）：Python 用 `sys.addaudithook`（`PYTHONPATH` 注入 `sitecustomize.py`），Node 用 `--require` 预加载钩子；
- **规则**（`DYN-01 ~ DYN-08`）：外联尝试 / 系统命令执行 / 越权写文件 / 读取敏感凭据 / 监听端口 / 写持久化启动项 / 运行稳定性 / 删除文件；命中 `critical` 直接并入安全结论并标记**阻断级**（与静态规则同一套打分与门禁）；
- **网络拓扑**：`skillhub-sandbox` 只连离线内网，`skillhub-sandbox-gw` 仅运行本项目的 TCP 转发代码，把 `host:8090` 桥接到沙箱（Docker Desktop 下 `internal` 网络不发布端口，故采用双容器拓扑）；
- **报告**：隔离自检、行为事件、检查项、文件系统前后比对、运行输出尾部，随扫描记录**持久化**（`skill_security_scans.sandbox`），前端报告抽屉「动态沙箱行为验证」区块可回读取证；
- **不误伤**：技能写自己的目录/临时文件、正常退出、非 MCP 脚本均不算风险；沙箱不可达时明确标注 `unreachable` 而非静默跳过。

### 一键演示（银行汇报用）

```bash
python backend/demo_security.py
```

输出：引擎/策略/密钥指纹 → 内部 4 技能查毒 → 上传正常包（放行）/恶意包（阻断，列出 12 条命中规则）/路径穿越包（拦截）→ GitHub 未审核下载被 403 拒绝。
恶意样本在 `backend/tools/security_samples.py` 中**运行时片段拼接 + XOR 编码**生成，不落盘明文，避免污染本机与误报。
