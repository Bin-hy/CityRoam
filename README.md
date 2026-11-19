# TripPilot

多约束旅行规划 Agent。首版聚焦北京 1～4 天行程：自然语言输入、确定性校验、自动修复、版本调整和分享。

核心思路是把「选点与叙事」交给 LLM，把「营业时间、路程、预算、时长」交给确定性代码；LLM 只能从工具返回的候选 POI 中选择，校验不通过则进入最多 3 轮的修复循环。架构图与部署拓扑见 [系统架构设计](docs/architecture/系统架构设计.md)。

## 状态

v1 MVP 开发中。四份规格文档已批准，按 `task.md` 逐步实现。当前已完成工程骨架、配置加载、Compose/Caddy 部署骨架、CI 和 OpenAPI 契约。

## 文档

- [Spec](docs/mew-spec/v1-mvp/spec.md)：功能、非功能需求与验收标准
- [Plan](docs/mew-spec/v1-mvp/plan.md)：架构、数据结构、接口与模块设计
- [Tasks](docs/mew-spec/v1-mvp/task.md)：任务拆解、依赖与验证方式
- [Checklist](docs/mew-spec/v1-mvp/checklist.md)：行为检查与验收清单
- [系统架构](docs/architecture/系统架构设计.md)：部署对象、前端选型与技术栈
- [后端架构选型](docs/architecture/后端架构选型.md)：为什么选择 Go + Eino
- [产品需求](docs/prd/项目立项.md)：PRD v1.1
- [OpenAPI](openapi.yaml)：HTTP 契约骨架

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go 1.24+、chi、Eino |
| 数据 | PostgreSQL 16、pgx/v5、sqlc、goose |
| 前端 | React 19、Vite、TypeScript、Tailwind、shadcn/ui、assistant-ui |
| 地图与时间轴 | 高德 JS API（GCJ-02）、vis-timeline |
| 流式与观测 | SSE、log/slog、OpenTelemetry |
| 部署 | Docker Compose、Caddy |
| CI | GitHub Actions |

## 目录结构

```
cmd/api/             API 入口
cmd/cli/             CLI（后续）
internal/
  api/               HTTP 路由、入参校验、SSE
  agent/             Eino Graph、节点、状态机
  domain/            C1–C12 校验、预算、调度、diff
  mcp/               McpHub（可选扩展）
  tools/             高德、POI 快照、直线距离、天气
  repo/              pgx + sqlc
  events/            SSE 事件与回放
  eval/              评测用例与指标
migrations/          goose 迁移
data/                POI 快照
deploy/              Compose、Caddy、Web 镜像
web/                 前端（后续）
```

## 环境要求

- Go 1.24+
- Node.js 20+（前端阶段使用）
- Docker 与 Docker Compose
- PostgreSQL 16，可直接用 Compose 中的 `postgres` 服务

## API 准备

本地开发需要以下凭证，全部放在 `.env`，不要提交到仓库：

| 变量 | 用途 |
|---|---|
| `DATABASE_URL` | PostgreSQL 连接串 |
| `LLM_BASE_URL` / `LLM_API_KEY` / `LLM_MODEL` | OpenAI 兼容 LLM |
| `AMAP_WEB_KEY` | 高德 Web 服务（POI、路径、地理编码、天气） |
| `VITE_AMAP_JS_KEY` / `VITE_AMAP_SECURITY_CODE` | 高德 JS API 与安全密钥 |
| `ADMIN_TOKEN` | 评测接口保护 |
| `RATE_LIMIT_PER_HOUR` | 匿名 IP 限流，默认 10 |

高德需分别申请「Web 服务」和「Web 端（JS API）」两类 Key，并配置域名白名单；前端与后端统一使用 GCJ-02 坐标。

```sh
cp .env.example .env
# 编辑 .env 填入凭证
```

## 快速开始

```sh
# 1. 校验 Compose 配置
make compose-config

# 2. 启动 PostgreSQL 与 API
docker compose -f deploy/docker-compose.yml up --build

# 3. 或仅本地运行 API（需要已运行的 PostgreSQL）
DATABASE_URL='postgres://trippilot:trippilot@localhost:5432/trippilot?sslmode=disable' \
  go run ./cmd/api
```

未配置 `DATABASE_URL` 时 API 会立即退出并提示配置错误。启动成功后输出 JSON 结构化日志。

## 开发命令

```sh
make test            # go test -race ./...
make lint            # go vet ./...
make build           # go build ./...
make fmt             # gofmt -w .
make compose-config  # 校验 Compose 文件
make migrate         # goose 迁移（数据库就绪后）
make seed            # 导入北京 POI 快照（后续）
make eval            # 运行评测（后续）
```

OpenAPI 契约校验：

```sh
npx @redocly/cli lint openapi.yaml
```

## 测试与评测

- 单元测试：`make test`，覆盖配置加载等领域逻辑。
- 契约测试：OpenAPI lint 通过后，P4 阶段补 HTTP handler 测试。
- 评测：20 条固定用例，覆盖 1～4 天、不同预算/节奏、必去点、闭馆日与雨天；指标包括硬约束满足率、修复轮数、端到端延迟和 POI 有效率。评测入口在 P6 阶段落地。

## 部署

- 单机 2C4G，Docker Compose 运行 `caddy`、`api`、`postgres`。
- Caddy 负责 TLS、静态资源和 `/api/*` 反向代理，并对 SSE 关闭响应缓冲。
- 只暴露 80/443；PostgreSQL 与 MCP 不对外。
- 每日 `pg_dump`，保留 7 天；迁移在发布前执行 `goose up`。
- 密钥通过环境变量或服务器 `.env` 注入，不打进镜像。

## 已知限制

- 首版只支持北京，1～4 天，不支持跨城交通。
- 不提供机票、酒店、门票预订与支付。
- 不做实时导航、实时路况和排队时间；路径异常时用直线距离估算并标注。
- 单人单实例部署，不做高可用、多地域、多租户。
- 评测目前依赖真实 LLM 与高德配额，尚未提供离线回放。
- 前端、数据库迁移、Agent 图和评测入口仍在按任务清单实现中。

## P1 / P2 规划

| 优先级 | 内容 |
|---|---|
| P1 | 偏好记忆、历史行程、分享页地图、PDF/日历导出、偏好设置 |
| P2 | 多城市扩展、出发前天气与营业状态提醒、替代方案建议 |

## 安全与合规

- API Key 仅保存在服务端，前端只使用受限的 JS Key。
- 匿名使用按 IP 限流；分享链接使用随机 slug。
- 不采集身份信息；行程删除后保留 7 天，工具调用保留 30 天。
- 地图与 POI 数据遵守来源方条款；价格标注为估算，页面明确标注 AI 生成。
- v1 不生成图片、音频或视频，不做未经授权的真人声音/形象克隆。
