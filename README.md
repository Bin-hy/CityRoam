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
       parse ─► retrieve ─► draft ─► schedule ─► validate
                                      ▲            │
                                      └── repair ◄─┘（≤3 轮）
                                            │
                                            ▼
                                    present / await user
                                            │
                              adjust ─► proposal ─► accept / reject
                                            │
                                            ▼
                                    finalize + share

工具：POI 快照（go:embed）、高德 Web 服务（POI/路径/天气）、候选查询
数据：PostgreSQL 16（trip、run、version、event、eval）
```

- **闭集选点**：`draft` 只能返回检索到的 POI ID，未知 ID 直接判失败。
- **确定性校验**：C1–C12 覆盖营业时间、闭馆日、路程、日时长、预算、用餐窗口、必去点、重复、节奏、起止时间、雨天与停留时长。
- **可回放**：每次运行的事件按 `seq` 追加，SSE 支持 `Last-Event-ID` 断线续传。
- **版本只追加**：调整生成新版本，接受后旧版本标记 superseded；回滚复制旧版本为新 `seq`。

设计细节见 [系统架构设计](docs/architecture/系统架构设计.md) 与 [Plan](docs/mew-spec/v1-mvp/plan.md)。

## 端到端流程

1. **生成**：用户输入「北京两天，历史与博物馆，预算 800 元」。`parse` 抽取城市、天数、预算、必去点和避开项；`retrieve` 返回候选 POI；`draft` 选点；`schedule` 计算时间、路程和费用；`validate` 产出 violations。
2. **违规与修复**：出现阻断项（如周一闭馆、必去点缺失、预算超限）时，`repair` 只重写受影响的那一天，最多 3 轮。警告项（节奏偏紧、雨天、时长偏短）不阻断，保留在方案上。
3. **确认**：方案进入 `awaiting_user`，前端展示时间轴、地图、预算和违规列表。用户可接受当前版本，也可直接提出调整。
4. **调整**：调整请求生成 `adjustment` run，先返回提案（影响范围、费用/步行变化、警告），用户接受后写入新版本。
5. **分享**：为已接受版本生成随机 slug，只读页面与纯文本导出不暴露内部 ID。
6. **评测**：固定 20 条用例覆盖 1～4 天、预算、节奏、必去点、闭馆日与雨天，输出硬约束满足率、修复轮数、p95 延迟、POI 与 JSON 有效率。

## 状态

v1 MVP 已完成：领域校验、Agent Graph、事件与 API、Web 工作台、评测、Docker Compose 部署。指标见 [评测报告](docs/eval-report.md)。

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go 1.24+、chi、Eino |
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
internal/
  api/               HTTP 路由、入参校验、SSE、限流
  agent/             Eino Graph、节点、状态机、修复
  domain/            C1–C12 校验、预算、调度、diff
  tools/             高德、POI 快照、直线距离、天气
  repo/              pgx + sqlc
  events/            SSE 事件与回放
  eval/              评测用例、指标、跑批
migrations/          goose 迁移
data/                内嵌 POI 快照
deploy/              Compose、Caddy、Web 镜像、备份脚本
web/                 React 前端与 Playwright E2E
docs/                规格、架构、评测与演示文档
```

## 环境要求

- Go 1.24+
- Node.js 20+
- Docker 与 Docker Compose
- PostgreSQL 16（本地或 Compose）

## API 准备

所有凭证放在 `.env`，不要提交到仓库：

| 变量 | 用途 |
|---|---|
| `DATABASE_URL` | PostgreSQL 连接串 |
| `LLM_BASE_URL` / `LLM_API_KEY` / `LLM_MODEL` | OpenAI 兼容 LLM |
| `AMAP_WEB_KEY` | 高德 Web 服务（POI、路径、地理编码、天气） |
| `VITE_AMAP_JS_KEY` / `VITE_AMAP_SECURITY_CODE` | 高德 JS API 与安全密钥 |
| `ADMIN_TOKEN` | 评测接口保护 |
| `RATE_LIMIT_PER_HOUR` | 匿名 IP 限流，默认 10 |

高德需分别申请「Web 服务」和「Web 端（JS API）」两类 Key，并配置域名白名单；前后端统一使用 GCJ-02 坐标。未配置 `AMAP_WEB_KEY` 时后端只使用内嵌 POI 快照和直线距离估算；未配置 `VITE_AMAP_JS_KEY` 时前端展示地图降级提示。

```sh
cp .env.example .env
# 编辑 .env 填入凭证
```

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
- [系统架构](docs/architecture/系统架构设计.md)：部署对象、前端选型与技术栈
- [后端架构选型](docs/architecture/后端架构选型.md)：为什么选择 Go + Eino
- [产品需求](docs/prd/项目立项.md)：PRD v1.1
- [评测报告](docs/eval-report.md)：最近一次跑批结果
- [演示脚本](docs/demo-script.md)：3 分钟演示流程
- [OpenAPI](openapi.yaml)：HTTP 契约

## 已知限制

- 只支持北京，1～4 天，不支持跨城交通。
- 不提供机票、酒店、门票预订与支付。
- 不做实时导航、实时路况和排队时间；路径异常时用直线距离估算并标注。
- 价格与营业时间为快照/估算值，出发前需以官方信息为准。
- 单人单实例部署，不做高可用、多地域、多租户。
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
- v1 不生成图片、音频或视频，不做未经授权的真人声音/形象克隆。
