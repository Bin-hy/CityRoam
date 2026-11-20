# TripPilot

多约束旅行规划 Agent。首版聚焦北京 1～4 天行程：自然语言输入、确定性校验、自动修复、版本调整和分享。

核心思路是把「选点与叙事」交给 LLM，把「营业时间、路程、预算、时长」交给确定性代码；LLM 只能从工具返回的候选 POI 中选择，校验不通过则进入最多 3 轮的修复循环。

## 架构

```
浏览器
  │  React 19 + assistant-ui + 高德 JS + vis-timeline
  ▼
Caddy ──静态资源 / SPA──► web/dist
  │  /api/*、SSE
  ▼
Go API (chi)
  ├─ Trip / Run / Version / Share / Eval HTTP 接口
  ├─ SSE Broker（事件回放 + 实时推送）
  └─ Agent Runner
       │
       ▼
     Eino Graph
       parse ─► retrieve ─► research ─► draft ─► schedule ─► validate
                              ▲                        ▲            │
                              │                        └── repair ◄─┘（≤3 轮）
                              │                               │
                              │                               ▼
                              │                       present / await user
                              │                               │
                              │                 adjust ─► proposal ─► accept / reject
                              │                               │
                              └── 可选工具：search_hotels /  ──┘
                                  search_travel_notes

工具：POI 快照（go:embed）、高德 Web 服务（POI/路径/天气）、
      可选插件（FlyAI 酒店 sidecar、DeepSeek WebSearch 攻略摘要）
数据：PostgreSQL 16（trip、run、version、event、eval、plugin_cache）
```

- **闭集选点**：`draft` 只能返回检索到的 POI ID，未知 ID 直接判失败。
- **确定性校验**：C1–C12 覆盖营业时间、闭馆日、路程、日时长、预算、用餐窗口、必去点、重复、节奏、起止时间、雨天与停留时长。
- **可回放**：每次运行的事件按 `seq` 追加，SSE 支持 `Last-Event-ID` 断线续传。
- **版本只追加**：调整生成新版本，接受后旧版本标记 superseded；回滚复制旧版本为新 `seq`。
- **插件化研究**：酒店与攻略工具默认关闭；启用后由 LLM 自主调用，工具结果写入 `research` 版本快照，不影响关闭路径。
- **版本化技能**：`skills/<name>/<version>/SKILL.md` 由 `skills.lock.json` 固定版本与内容哈希；research 提示只带技能目录，正文由 `load_skill` 按需加载；run 开始时只固定版本 pins，升级或回滚不会改变进行中的 run。

设计细节见 [系统架构设计](docs/architecture/系统架构设计.md) 与 [Plan](docs/mew-spec/v1-mvp/plan.md)。

## 端到端流程

1. **生成**：用户输入「北京两天，历史与博物馆，预算 800 元」。`parse` 抽取城市、天数、预算、必去点和避开项；`retrieve` 返回候选 POI；启用插件时 `research` 查询酒店和公开攻略摘要；`draft` 选点；`schedule` 计算时间、路程和费用；`validate` 产出 violations。
2. **违规与修复**：出现阻断项（如周一闭馆、必去点缺失、预算超限）时，`repair` 只重写受影响的那一天，最多 3 轮。警告项（节奏偏紧、雨天、时长偏短）不阻断，保留在方案上。
3. **确认**：方案进入 `awaiting_user`，前端展示时间轴、地图、预算和违规列表。用户可接受当前版本，也可直接提出调整。
4. **调整**：调整请求生成 `adjustment` run，先返回提案（影响范围、费用/步行变化、警告），用户接受后写入新版本。
5. **分享**：为已接受版本生成随机 slug，只读页面与纯文本导出不暴露内部 ID。
6. **评测**：固定 20 条用例覆盖 1～4 天、预算、节奏、必去点、闭馆日与雨天，输出硬约束满足率、修复轮数、p95 延迟、POI 与 JSON 有效率。

## 状态

v1 MVP 已完成：领域校验、Agent Graph、事件与 API、Web 工作台、评测、Docker Compose 部署。v1.1 增加可选酒店/攻略研究插件，关闭时保持原主链与 20 条评测不变。v1.3 增加可选 OIDC 登录、服务端会话、用户归属隔离与管理员角色（仅 eval），`AUTH_ENABLED=false` 时保持原匿名流程。指标见 [v1 评测报告](docs/eval-report.md) 与 [插件评测报告](docs/eval-report-plugins.md)。

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go 1.25+、chi、Eino |
| 数据 | PostgreSQL 16、pgx/v5、sqlc、goose |
| 前端 | React 19、Vite、TypeScript、Tailwind、Radix、assistant-ui |
| 地图与时间轴 | 高德 JS API（GCJ-02）、vis-timeline |
| 流式与观测 | SSE、log/slog |
| 部署 | Docker Compose、Caddy、distroless |
| CI | GitHub Actions |

## 目录结构

```
cmd/api/             API 入口
cmd/eval/            评测 CLI
cmd/healthcheck/     容器健康检查
cmd/skills/          技能校验与锁文件生成 CLI
cmd/skilleval/       真实模型技能触发评测 CLI
internal/
  api/               HTTP 路由、入参校验、SSE、限流
  agent/             Eino Graph、节点、状态机、修复
  domain/            C1–C12 校验、预算、调度、diff
  skills/            SKILL.md 加载、版本与哈希校验、L1/L2 缓存
  skillseval/        技能触发评测实现
  tools/             高德、POI 快照、直线距离、天气
  repo/              pgx + sqlc
  events/            SSE 事件与回放
  eval/              评测用例、指标、跑批
skills/              版本化 Agent Skills 与 skills.lock.json
eval/skills-fixtures 技能触发评测的独立 fixture 与锁
eval/skills-cases/   技能触发评测用例
migrations/          goose 迁移
data/                内嵌 POI 快照
deploy/              Compose、Caddy、Web 镜像、备份脚本
web/                 React 前端与 Playwright E2E
docs/                规格、架构、评测与演示文档
```

## 环境要求

- Go 1.25+
- Node.js 20+
- Docker 与 Docker Compose
- PostgreSQL 16（本地或 Compose）

## API 准备

所有凭证放在本地 `.env.local`（或 Compose 的 `deploy/.env`），不要提交到仓库。完整中文注释见 [`.env.example`](.env.example) 与 [环境变量配置](docs/环境变量配置.md)。

| 变量 | 用途 |
|---|---|
| `DATABASE_URL` | PostgreSQL 连接串 |
| `LLM_BASE_URL` / `LLM_API_KEY` / `LLM_MODEL` | OpenAI 兼容 LLM |
| `AMAP_WEB_KEY` | 高德 Web 服务（POI、路径、地理编码、天气） |
| `VITE_AMAP_JS_KEY` / `VITE_AMAP_SECURITY_CODE` | 高德 JS API 与安全密钥 |
| `ADMIN_TOKEN` | 评测接口保护 |
| `RATE_LIMIT_PER_HOUR` | 写接口限流，默认 10；登录启用后按用户计数 |
| `AUTH_ENABLED` / `OIDC_*` | 可选 OIDC 登录，默认关闭 |
| `ADMIN_EMAILS` / `ADMIN_SUBS` | 管理员白名单（仅 eval 权限） |
| `CORS_ALLOWED_ORIGINS` | 前后端分离部署时允许的 Web 源 |
| `WEATHER_CACHE_TTL` / `WEATHER_TOOL_TIMEOUT` / `WEATHER_MAX_CALLS_PER_RUN` | 天气工具缓存、超时与调用上限；`AMAP_WEB_KEY` 非空即启用 |
| `SKILLS_DIR` | 技能目录，默认 `skills`；目录不存在时按无技能启动 |

高德需分别申请「Web 服务」和「Web 端（JS API）」两类 Key，并配置域名白名单；前后端统一使用 GCJ-02 坐标。未配置 `AMAP_WEB_KEY` 时后端只使用内嵌 POI 快照和直线距离估算，天气工具同时被禁用；未配置 `VITE_AMAP_JS_KEY` 时前端展示地图降级提示。

```sh
cp .env.example .env.local
# 编辑 .env.local 填入凭证
set -a; . ./.env.local; set +a
go run ./cmd/api
```

## 可选插件：酒店与攻略

两个研究插件默认关闭，只有显式开启且依赖可用时才会把工具注入 Agent 上下文。关闭时不会探测外部服务，主链、API 响应和 v1 评测结果不受影响。

| 变量 | 用途 |
|---|---|
| `HOTEL_PLUGIN_ENABLED` | 酒店查询开关，默认 `false` |
| `HOTEL_TOOL_URL` / `HOTEL_TOOL_TOKEN` | 酒店 sidecar 地址与 Bearer token（可留空） |
| `HOTEL_SEARCH_TIMEOUT` / `HOTEL_CACHE_TTL` | 单次搜索超时与缓存时间 |
| `GUIDE_PLUGIN_ENABLED` | 攻略检索开关，默认 `false` |
| `GUIDE_SEARCH_BASE_URL` / `GUIDE_SEARCH_API_KEY` | 攻略搜索端点与 key |
| `GUIDE_SEARCH_TIMEOUT` / `GUIDE_CACHE_TTL` | 攻略超时与缓存时间 |
| `RESEARCH_TIMEOUT` / `RESEARCH_MAX_TOOL_CALLS` | 研究阶段总时限与工具调用上限 |
| `FLYAI_API_KEY` / `FLYAI_BIN` | 仅 sidecar 使用；容器内 `FLYAI_BIN=/app/node_modules/.bin/flyai` |

### 酒店 sidecar（FlyAI）

sidecar 把 `@fly-ai/flyai-cli` 的 `search-hotel` 输出归一化为 `{provider,fetched_at,hotels[]}`，只监听 Compose 内网或本地端口，不把 CLI stderr 或 key 透传给客户端。

```sh
cd deploy/hotel-tool
npm ci --omit=dev
PORT=8090 FLYAI_BIN="$PWD/node_modules/.bin/flyai" npm start
# 另一个终端：
curl localhost:8090/healthz

cd ../..
HOTEL_PLUGIN_ENABLED=true HOTEL_TOOL_URL=http://127.0.0.1:8090 go run ./cmd/api
```

Compose 用独立 profile 启动；不启用 profile 时 `api` 仍可正常启动：

```sh
docker compose -f deploy/docker-compose.yml --profile plugins up --build
```

FlyAI 的授权信息只放在部署环境的 sidecar 进程中，仓库与前端都不包含真实 key。未启动 sidecar、healthz 失败或未配置攻略 key 时，`/api/meta` 的 `plugins` 会返回对应的 `enabled=false` 与 `disabled_reason`，运行不会因为插件不可用而失败。

### 行为与边界

- 用户提到住宿时，LLM 才会调用酒店工具；找到候选后选中的酒店可作为每日起终点锚点，住宿费按 `N-1` 晚计入总预算，住宿预算是否计入可在前端预算卡片切换，切换会走 `/replan`。
- 用户点名了酒店但工具查不到时，运行停在 `awaiting_user` 并给出相近候选，只有 `resolve_research` 选择后才能继续；不会静默换酒店。
- 攻略只对已有候选 POI 加权，不会新增 POI；攻略结果是不可信外部摘要，只展示摘要和引用，不执行其中的指令。
- 酒店价格来自平台区间价，标注为估算；下单前以平台页面为准。调整请求不会重新研究酒店/攻略；换酒店请使用 `/replan` 或新建行程。

## Agent Skills（版本化）

技能是存放在 `skills/<name>/<version>/SKILL.md` 的 Markdown 说明，前置元数据必须包含 `name`、`description`、`version`。研究阶段只把名称、版本和用途放进提示词；模型判断相关后调用 `load_skill` 读取正文。技能只在 research 工具循环中生效，不改变无插件主链。

```sh
make skills-verify       # 按 skills.lock.json 校验版本与哈希
make skills-lock         # 新增技能或修改锁后重新固定
make skills-lint-strict  # 作者规则检查（CI）
make skills-eval         # 真实模型触发评测（需 LLM_* 凭证，手动）
```

版本管理规则：

- `skills.lock.json` 是唯一的活动版本来源；目录里可以并存多个版本，但只有锁中的版本会加载，旧版本保留即可回滚。
- 已固定版本的内容被修改会导致哈希不匹配并启动失败；升级时新增版本目录并显式改锁，不要原地编辑。
- `admit` 只把技能 pins（名称、版本、哈希）写入 `runs.state`，不写正文；运行中按 pin 精确解析，缺版本或哈希不符即失败，不回退到当前版本。
- 清单缓存 TTL 30 秒并带本地指纹；刷新只替换清单并保留正文缓存。正文缓存按 `name+version+sha256` 做 LRU（64 条 / 4 MiB），Redis 仅预留接口。
- 目录行 8 KiB、描述 300 字符、正文 32 KiB、技能 32 个、正文合计 256 KiB；超出即启动失败。
- `/api/meta` 的 `skills[]` 暴露当前固定的 `name`、`version`、`sha256`；正文和 description 不进入 meta、分享页或日志。

细节见 [Agent Skills](docs/skills.md)。

## 用户与登录

默认 `AUTH_ENABLED=false`，保持匿名 demo：任何人可创建行程，行程列表展示旧的无主数据，分享页公开。开启后行为变为：

- **强制登录**：浏览器通过通用 OIDC（Authorization Code + PKCE）登录，服务端换取用户信息后只下发不透明会话 Cookie；IdP token 不进入浏览器，也不写日志。
- **归属隔离**：行程与运行按用户隔离，非本人资源统一 404；首页展示「我的行程」。旧的无主行程在登录开启后隐藏，不做迁移或删除。
- **角色**：yufu 等 IdP 只负责认证，TripPilot 自己授权。`user` 可正常使用；`admin` 由 `ADMIN_EMAILS`（要求已验证邮箱）或 `ADMIN_SUBS` 白名单授予，仅额外开放 `/api/eval/*`。管理员不能查看其他用户的行程。
- **公开页面**：`/share/:slug` 始终无需登录；登录页 `/login` 独立于业务布局。
- **API Key 仍为全局**：LLM、高德与插件密钥是服务端共享配置，不按用户区分。

最小配置：

```ini
AUTH_ENABLED=true
OIDC_ISSUER=http://127.0.0.1:9410
OIDC_CLIENT_ID=trippilot
OIDC_CLIENT_SECRET=<server-side secret>
OIDC_REDIRECT_URI=http://localhost:8080/api/auth/callback
ADMIN_EMAILS=admin@example.com
```

前后端分离部署再加 `AUTH_WEB_BASE_URL=<web origin>` 与 `CORS_ALLOWED_ORIGINS=<web origin>`；前端请求已带 `credentials: "include"`。完整变量与排查见 [环境变量配置](docs/环境变量配置.md)。

## 快速开始

### 本地开发

```sh
# 1. 启动 PostgreSQL
docker run -d --name trippilot-pg \
  -e POSTGRES_USER=trippilot -e POSTGRES_PASSWORD=trippilot -e POSTGRES_DB=trippilot \
  -p 5432:5432 postgres:16

# 2. 启动 API（自动执行 goose 迁移）
set -a; . ./.env.local; set +a
go run ./cmd/api

# 3. 启动前端（Vite 代理 /api 到 8080）
cd web
npm ci
npm run dev
```

打开 `http://localhost:5173`，输入需求后进入 `/trips/:id` 工作台。

### Docker Compose

Compose 从 `deploy/` 目录读取变量，把环境文件放在那里：

```sh
cp .env.example deploy/.env
# 编辑 deploy/.env
make dev                      # 等价于 docker compose -f deploy/docker-compose.yml up --build
```

访问 `http://localhost`。Caddy 提供 SPA 与 `/api/*` 反向代理，并对 SSE 关闭响应缓冲。只暴露 80 端口，PostgreSQL 不对外。

## 开发命令

```sh
make test            # go test -race ./...
make lint            # go vet ./...
make build           # go build ./...
make fmt             # gofmt -w .
make compose-config  # 校验 Compose 文件
make migrate         # goose 迁移（数据库就绪后）
make eval            # go run ./cmd/eval
```

前端：

```sh
cd web
npm test             # Vitest（单元与组件）
npm run build        # tsc -b && vite build
npm run test:e2e     # Playwright
```

OpenAPI 契约校验：

```sh
npx @redocly/cli lint openapi.yaml
```

## 评测

评测 CLI 直接调用 Agent 全流程（含 LLM 与可选高德），每个用例创建临时 trip/run，自动接受方案后计算指标：

```sh
set -a; . ./.env.local; set +a
go run ./cmd/eval --cases eval/cases --output docs/eval-report.md
```

- 用例：20 条，覆盖 1～4 天、预算、节奏、必去点、闭馆日、雨天与冲突场景。
- 指标：硬约束满足率、平均修复轮数、p95 延迟、POI 有效率、JSON 合法率、人工评分占位。
- 目标：硬约束满足率 ≥90%，JSON 合法率 ≥99%，3 天 p95 ≤60s。
- 只检查用例与表格、不调用 LLM：`go run ./cmd/eval --dry-run`。

插件评测使用独立用例目录和报告，不混入 v1 的 20 条结果：

```sh
HOTEL_PLUGIN_ENABLED=true HOTEL_TOOL_URL=http://127.0.0.1:8090 \
GUIDE_PLUGIN_ENABLED=true GUIDE_SEARCH_API_KEY=... \
go run ./cmd/eval -plugins -cases eval/cases-plugins -output docs/eval-report-plugins.md
```

报告包含 `research_status`、研究耗时、工具调用数、住宿金额和攻略匹配数；插件不可用时命令会直接报错，不会伪造通过。最近一次结果见 [插件评测报告](docs/eval-report-plugins.md)。

管理员接口（需 `ADMIN_TOKEN`，未配置时返回 503）：

```sh
curl -H "X-Admin-Token: $ADMIN_TOKEN" http://localhost:8080/api/eval/cases
curl -H "X-Admin-Token: $ADMIN_TOKEN" -X POST http://localhost:8080/api/eval/runs
```

## 部署

- 单机 2C4G，Docker Compose 运行 `caddy`、`api`、`postgres`。
- API 镜像为 distroless 静态二进制，内嵌 POI 快照与评测用例；非 root 运行，带健康检查。
- 迁移在 API 启动时自动执行；升级前先备份。
- 只暴露 80；如需 HTTPS，把 Caddyfile 的站点地址改为域名由 Caddy 自动签发，或在外层负载均衡终止 TLS。
- 密钥通过环境变量注入，不打进镜像；前端只使用受限的 JS Key。

备份：

```sh
DATABASE_URL='postgres://...' ./deploy/backup.sh
# 输出 trippilot-<时间>.dump；保留 7 天（RETENTION_DAYS 可调）
```

## 文档

- [Spec](docs/mew-spec/v1-mvp/spec.md)：功能、非功能需求与验收标准
- [Plan](docs/mew-spec/v1-mvp/plan.md)：架构、数据结构、接口与模块设计
- [Tasks](docs/mew-spec/v1-mvp/task.md)：任务拆解、依赖与验证方式
- [Checklist](docs/mew-spec/v1-mvp/checklist.md)：行为检查与验收清单
- [v1.1 Spec](docs/mew-spec/v1.1-plugins/spec.md)、[Plan](docs/mew-spec/v1.1-plugins/plan.md)、[Checklist](docs/mew-spec/v1.1-plugins/checklist.md)：酒店与攻略插件
- [v1.3 Spec](docs/mew-spec/v1.3-auth/spec.md)、[Plan](docs/mew-spec/v1.3-auth/plan.md)、[Checklist](docs/mew-spec/v1.3-auth/checklist.md)：用户区分与 OIDC 登录
- [系统架构](docs/architecture/系统架构设计.md)：部署对象、前端选型与技术栈
- [后端架构选型](docs/architecture/后端架构选型.md)：为什么选择 Go + Eino
- [产品需求](docs/prd/项目立项.md)：PRD v1.1
- [评测报告](docs/eval-report.md)：最近一次跑批结果
- [演示脚本](docs/demo-script.md)：3 分钟演示流程
- [OpenAPI](openapi.yaml)：HTTP 契约

## 已知限制

- 只支持北京，1～4 天，不支持跨城交通。
- 不提供机票、门票或酒店预订与支付；酒店链接仅跳转平台，库存和最终价格以平台为准。
- 不做实时导航、实时路况和排队时间；路径异常时用直线距离估算并标注。
- 价格与营业时间为快照/估算值，出发前需以官方信息为准。
- 单人单实例部署，不做高可用、多地域；用户隔离是 demo 级归属控制，不是多租户 SaaS（无按用户的密钥、配额或计费）。
- 评测依赖真实 LLM 与高德配额，未提供离线回放。

## P1 / P2 规划

| 优先级 | 内容 |
|---|---|
| P1 | 偏好记忆、历史行程、分享页地图、PDF/日历导出、偏好设置 |
| P2 | 多城市扩展、出发前天气与营业状态提醒、替代方案建议 |

## 安全与合规

- API Key 仅保存在服务端，前端只使用受限的 JS Key。
- 匿名使用按 IP 限流（默认 10 次/小时）；输入有长度限制。
- 分享链接使用随机 slug；不采集身份信息；行程删除后保留 7 天，工具调用保留 30 天。
- 地图与 POI 数据遵守来源方条款；价格标注为估算，页面明确标注 AI 生成。
- 外部攻略只读取公开搜索摘要并按不可信内容处理；不抓取未授权页面，不登录或绕过平台反爬。
- 酒店与攻略 key 只注入服务端；sidecar 不映射宿主端口，`.env.local` / `deploy/.env` 不提交。
- v1 不生成图片、音频或视频，不做未经授权的真人声音/形象克隆。
