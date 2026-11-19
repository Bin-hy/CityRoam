# TripPilot v1 MVP Tasks

> 状态：已批准（2026-11-19）

| 项目 | 内容 |
|---|---|
| 版本 | v0.1 |
| 状态 | 待审批 |
| 关联 | [spec.md](spec.md) · [plan.md](plan.md) |
| 说明 | 任务卡是可独立验证的切片；内部步骤按 2–5 分钟设计。完成前必须运行「验证」并保留输出，未通过不得进入下一任务 |

> 四份文档全部批准前不写实现代码。执行时按本文件的依赖顺序推进；每个任务完成后提交一次代码。

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 新建 | `go.mod`、`cmd/api/main.go` | Go module、API 入口 |
| 新建 | `.gitignore`、`.env.example`、`README.md` | 仓库基础 |
| 新建 | `internal/config/config.go` | 环境变量加载 |
| 新建 | `internal/logging/logging.go` | 结构化日志 |
| 新建 | `deploy/docker-compose.yml`、`deploy/Caddyfile` | 本地/单机部署 |
| 新建 | `.github/workflows/ci.yml`、`Makefile` | CI 与常用命令 |
| 新建 | `openapi.yaml` | API 契约 |
| 新建 | `internal/domain/constraint.go` | 约束类型、默认值、合并 |
| 新建 | `internal/domain/poi.go` | POI、营业时间 |
| 新建 | `internal/domain/itinerary.go` | DraftPlan、Itinerary |
| 新建 | `internal/domain/violation.go` | 违规类型 |
| 新建 | `internal/domain/scheduler/scheduler.go` | 时间、交通、费用 |
| 新建 | `internal/domain/validator/validator.go` | 校验入口 |
| 新建 | `internal/domain/validator/rules.go` | C1–C12 |
| 新建 | `internal/domain/diff/diff.go` | 版本差异 |
| 新建 | `internal/domain/budget/budget.go` | 预算汇总 |
| 新建 | `migrations/0001_init.sql` | 全部表 |
| 新建 | `migrations/0002_eval_cases.sql` | 评测用例种子 |
| 新建 | `sqlc.yaml`、`internal/repo/queries/*.sql` | SQL 查询 |
| 新建 | `internal/repo/sqlc/*.go` | sqlc 生成代码 |
| 新建 | `internal/repo/store.go` | Store 实现 |
| 新建 | `data/poi/beijing.json` | 北京 POI 快照 |
| 新建 | `cmd/seed/main.go` | 快照导入 |
| 新建 | `internal/tools/tool.go` | Tool 接口、Registry |
| 新建 | `internal/tools/snapshot.go` | 快照搜索 |
| 新建 | `internal/tools/amap.go` | 高德客户端 |
| 新建 | `internal/tools/route.go` | 路径与降级 |
| 新建 | `internal/tools/weather.go` | 天气 |
| 新建 | `internal/mcp/hub.go` | MCP 白名单、超时、熔断 |
| 新建 | `internal/mcp/adapter.go` | MCP Tool 适配 |
| 新建 | `internal/mcp/normalize.go` | Evidence 归一化 |
| 新建 | `internal/agent/llm.go` | LLMClient |
| 新建 | `internal/agent/prompts/*.tmpl` | Prompt 模板 |
| 新建 | `internal/agent/prompts/schemas.go` | JSON Schema |
| 新建 | `internal/agent/nodes.go` | 图节点 |
| 新建 | `internal/agent/repair.go` | DayPatch 合并、停止条件 |
| 新建 | `internal/agent/graph.go` | Eino 图 |
| 新建 | `internal/agent/state.go` | TripState 编解码 |
| 新建 | `internal/agent/runner.go` | Run 生命周期 |
| 新建 | `cmd/cli/main.go` | 本地跑一次规划 |
| 新建 | `internal/events/broker.go` | 进程内事件总线 |
| 新建 | `internal/events/sse.go` | SSE 编码 |
| 新建 | `internal/api/router.go` | chi 路由 |
| 新建 | `internal/api/middleware.go` | 中间件 |
| 新建 | `internal/api/errors.go` | 错误映射 |
| 新建 | `internal/api/trips.go` | 行程接口 |
| 新建 | `internal/api/runs.go` | 运行接口 |
| 新建 | `internal/api/sse.go` | SSE 接口 |
| 新建 | `internal/api/share.go` | 分享接口 |
| 新建 | `internal/api/eval.go` | 评测接口 |
| 新建 | `internal/api/meta.go` | 元信息、健康检查 |
| 新建 | `internal/eval/cases.go` | 用例加载 |
| 新建 | `internal/eval/runner.go` | 跑批 |
| 新建 | `internal/eval/metrics.go` | 指标 |
| 新建 | `cmd/eval/main.go` | 评测 CLI |
| 新建 | `eval/cases/*.json` | 20 条用例 |
| 新建 | `web/` | React + Vite 应用 |
| 新建 | `web/src/api/*` | API 客户端、类型 |
| 新建 | `web/src/features/*` | 创建、工作台、分享、评测 |
| 新建 | `web/src/hooks/useRunStream.ts` | SSE hook |
| 新建 | `web/src/lib/amap.ts` | 高德 JS 加载 |
| 新建 | `e2e/golden.spec.ts` | 端到端场景 |
| 新建 | `Dockerfile`、`.dockerignore` | API 镜像 |
| 新建 | `deploy/web/Dockerfile` | Web + Caddy 镜像 |
| 新建 | `deploy/backup.sh` | 数据库备份 |
| 修改 | `docs/` | 架构、计划、演示脚本 |

## P0 工程骨架

### P0-01 初始化 Go module 与目录

**文件：** `go.mod`、`cmd/api/main.go`、`.gitignore`、`.env.example`、`README.md`
**依赖：** 无

**步骤：**
1. 执行 `go mod init trippilot`。
2. 创建 `cmd/api/main.go`，打印版本并退出。
3. 写 `.gitignore`：`.env`、`bin/`、`web/node_modules/`、`web/dist/`、`*.log`。
4. 写 `.env.example`，列出 `DATABASE_URL`、`LLM_*`、`AMAP_*`、`ADMIN_TOKEN`。
5. README 写一句项目描述和快速开始占位。

**验证：** `go build ./...` 通过；`go run ./cmd/api` 输出版本号。

### P0-02 配置与日志

**文件：** `internal/config/config.go`、`internal/config/config_test.go`、`internal/logging/logging.go`
**依赖：** P0-01

**步骤：**
1. 定义 `Config`，从环境变量读取必填和可选字段。
2. 必填缺失时返回明确错误，不 panic。
3. 用 `log/slog` 初始化 JSON 日志。
4. 写表驱动测试，覆盖缺失、默认值、非法端口。

**验证：** `go test ./internal/config/...` 通过；缺 `DATABASE_URL` 时启动报明确错误。

### P0-03 Compose 与 Caddy 骨架

**文件：** `deploy/docker-compose.yml`、`deploy/Caddyfile`、`Dockerfile`、`deploy/web/Dockerfile`
**依赖：** P0-01

**步骤：**
1. Compose 定义 `postgres`、`api`、`caddy` 三个服务。
2. 配 `pgdata`、`caddy_data` 卷和 Postgres 健康检查。
3. Caddyfile 配 `/api/*` 反代和 `/` 静态占位。
4. Dockerfile 先写多阶段占位，保证 `docker build` 可执行。

**验证：** `docker compose -f deploy/docker-compose.yml config` 无错误。

### P0-04 CI 与 Makefile

**文件：** `.github/workflows/ci.yml`、`Makefile`
**依赖：** P0-01

**步骤：**
1. CI 分 backend（setup-go、vet、test）和 frontend（setup-node、npm ci、build）两个 job。
2. 增加 `docker compose config` 校验 job。
3. Makefile 提供 `test`、`lint`、`build`、`migrate`、`seed`、`eval`。
4. README 记录常用命令。

**验证：** `make test` 能执行；CI YAML 用 `yamllint` 或编辑器校验无语法错误。

### P0-05 OpenAPI 骨架

**文件：** `openapi.yaml`
**依赖：** P0-01

**步骤：**
1. 写 `info`、`servers`、`/api/trips`、`/api/runs/{id}/events`。
2. 定义 `Error`、`CreateTripRequest`、`RunSummary` 三个 schema。
3. 统一错误响应结构。
4. 标注 SSE 为 `text/event-stream`。

**验证：** `npx @redocly/cli lint openapi.yaml` 通过。

### P0-06 README 与文档链接

**文件：** `README.md`
**依赖：** P0-01、P0-05

**步骤：**
1. 写项目一句话、架构图链接、API 准备清单。
2. 写本地运行、测试、评测、部署四段。
3. 链接 `docs/mew-spec/v1-mvp/spec.md` 和 `plan.md`。
4. 列出已知限制和 P1/P2。

**验证：** README 中所有相对链接指向存在的文件。

## P1 领域核心

### P1-01 ConstraintSet 类型与默认值

**文件：** `internal/domain/constraint.go`、`internal/domain/constraint_test.go`
**依赖：** P0-01

**步骤：**
1. 定义 `ConstraintSet`、`BudgetConstraint`、`DailyConstraint`、`MealConstraint`、`WalkingConstraint`、`WeatherConstraint`、`Pace`。
2. 实现 `ApplyDefaults`：只补非关键默认值，不补 `City`、`Days`。
3. 默认节奏 `normal`，每日 09:00–21:00，最多 9 小时，午餐 11:30–13:30，晚餐 17:30–19:30。
4. 写测试确认 `City`、`Days` 缺失时仍为空。

**验证：** `go test ./internal/domain -run TestConstraintDefaults` 通过。

### P1-02 约束合并与缺失检测

**文件：** `internal/domain/constraint.go`、`internal/domain/constraint_test.go`
**依赖：** P1-01

**步骤：**
1. 实现 `Merge(base, patch ConstraintSet) ConstraintSet`。
2. 实现 `MissingCritical()`，返回缺失的 `city`、`days`。
3. 实现 `Validate()`：天数 1–4、预算非负、时间格式合法。
4. 测试硬/软约束切换和部分字段合并。

**验证：** `go test ./internal/domain -run TestConstraintMerge` 通过。

### P1-03 POI 与营业时间

**文件：** `internal/domain/poi.go`、`internal/domain/poi_test.go`
**依赖：** P0-01

**步骤：**
1. 定义 `POI`、`OpenRange`，坐标固定 GCJ-02。
2. 实现 `IsOpen(day int, at time.Time) bool`，支持跨午休的两段营业时间。
3. 实现 `OpenDays()`，供 C2 使用。
4. 营业时间未知时返回 `unknown` 标记，不直接判违规。

**验证：** `go test ./internal/domain -run TestPOIOpenHours` 通过。

### P1-04 行程与草案类型

**文件：** `internal/domain/itinerary.go`、`internal/domain/itinerary_test.go`
**依赖：** P1-01、P1-03

**步骤：**
1. 定义 `DraftPlan`、`DraftDay`、`DraftItem`、`Explanation`。
2. 定义 `Itinerary`、`DayPlan`、`Block`、`Transport`、`Totals`。
3. 实现 `POIIDs()`、`Clone()`、`DayByIndex()`。
4. 测试空行程、单天、多天。

**验证：** `go test ./internal/domain -run TestItinerary` 通过。

### P1-05 违规类型

**文件：** `internal/domain/violation.go`、`internal/domain/violation_test.go`
**依赖：** P1-04

**步骤：**
1. 定义 `Level`、`Violation`。
2. 实现 `HasBlocking`、`CountByLevel`、`Sort`。
3. 保证每条违规有 `code`、`level`、`message`。
4. 测试排序和计数。

**验证：** `go test ./internal/domain -run TestViolation` 通过。

### P1-06 调度器：时间解析与起步

**文件：** `internal/domain/scheduler/scheduler.go`、`scheduler_test.go`
**依赖：** P1-03、P1-04

**步骤：**
1. 实现 `HH:MM` 解析和分钟加减。
2. 每天从 `Daily.StartTime` 开始。
3. 第一个 POI 不计算市内出发交通。
4. 测试跨越中午、结束时间。

**验证：** `go test ./internal/domain/scheduler -run TestSchedulerStart` 通过。

### P1-07 调度器：交通、等待与缓冲

**文件：** `internal/domain/scheduler/scheduler.go`、`scheduler_test.go`
**依赖：** P1-06

**步骤：**
1. 遍历草案点位，插入 `transit` 块。
2. 到达时间 = 上一块结束 + 路程 + 缓冲。
3. 同区域缓冲 10 分钟，跨区域 20 分钟。
4. 早到写入等待，不改变 POI 建议时长。
5. 用 fake `RouteProvider` 测三种情况。

**验证：** `go test ./internal/domain/scheduler -run TestSchedulerTransit` 通过。

### P1-08 调度器：费用与总计

**文件：** `internal/domain/scheduler/scheduler.go`、`scheduler_test.go`
**依赖：** P1-07

**步骤：**
1. 汇总门票、餐饮、交通费用。
2. 交通费用未知时按 2 元/段估算并标 `estimated`。
3. 计算每天费用和总费用。
4. 计算 POI 数和步行公里数。

**验证：** `go test ./internal/domain/scheduler -run TestSchedulerCost` 通过。

### P1-09 校验入口与 C1/C2

**文件：** `internal/domain/validator/validator.go`、`rules.go`、`validator_test.go`
**依赖：** P1-03、P1-04、P1-05、P1-06

**步骤：**
1. 定义 `Validate(plan, constraints, pois, routes)`。
2. 实现 C1：到访时间在营业时间内；未知营业时间降为警告。
3. 实现 C2：星期几闭馆；从 `OpenRange.Days` 推导。
4. 测试营业中、闭馆、未知三种情况。

**验证：** `go test ./internal/domain/validator -run 'TestC1|TestC2'` 通过。

### P1-10 C3/C4/C10

**文件：** `internal/domain/validator/rules.go`、`validator_test.go`
**依赖：** P1-09

**步骤：**
1. C3：相邻块间隔 ≥ 路程 + 缓冲；降级估算时给警告。
2. C4：每日总时长 ≤ `MaxHours`。
3. C10：不早于开始时间、不晚于结束时间。
4. 每种规则至少一个通过和一个阻断用例。

**验证：** `go test ./internal/domain/validator -run 'TestC3|TestC4|TestC10'` 通过。

### P1-11 C5/C6

**文件：** `internal/domain/validator/rules.go`、`validator_test.go`
**依赖：** P1-09

**步骤：**
1. C5：门票 + 餐饮 + 交通 ≤ 预算；硬约束阻断，软约束警告。
2. C6：餐饮开启时每天至少一个餐饮块；缺失阻断。
3. C6：超出午餐/晚餐窗口 30 分钟给警告。
4. 测试预算边界和关闭餐饮。

**验证：** `go test ./internal/domain/validator -run 'TestC5|TestC6'` 通过。

### P1-12 C7/C8/C9/C12

**文件：** `internal/domain/validator/rules.go`、`validator_test.go`
**依赖：** P1-09

**步骤：**
1. C7：必去点全部出现。
2. C8：同一 POI 不跨天、不跨块重复。
3. C9：轻松 2–3、正常 3–4、紧凑 4–5 个 POI/天，给警告。
4. C12：停留时间不低于建议时长 60%，给警告。

**验证：** `go test ./internal/domain/validator -run 'TestC7|TestC8|TestC9|TestC12'` 通过。

### P1-13 C11 与全量校验

**文件：** `internal/domain/validator/rules.go`、`validator_test.go`
**依赖：** P1-09、P1-10、P1-11、P1-12

**步骤：**
1. C11：雨天户外点比例 >60% 给警告。
2. 写 C1–C12 表驱动测试，确保 code 唯一。
3. 测试多违规同时出现时的排序。
4. 确认校验器为纯函数，不读环境变量。

**验证：** `go test ./internal/domain/validator/...` 全部通过。

### P1-14 版本 diff 与预算汇总

**文件：** `internal/domain/diff/diff.go`、`internal/domain/budget/budget.go`、测试
**依赖：** P1-04、P1-05

**步骤：**
1. `diff` 输出新增/删除/修改的天和块，供前端展示。
2. `budget` 汇总门票、餐饮、交通，支持按天和总计。
3. 处理空版本和只有一天的情况。
4. 写表驱动测试。

**验证：** `go test ./internal/domain/diff/... ./internal/domain/budget/...` 通过。

## P2 数据与工具

### P2-01 初始化数据库迁移

**文件：** `migrations/0001_init.sql`
**依赖：** P0-03

**步骤：**
1. 按 plan 第 8 节创建 11 张表。
2. 加 `CHECK`、唯一约束和索引。
3. 开启 `pgcrypto`。
4. 写 `cmd/api` 启动时执行 goose 的入口。

**验证：** `goose -dir migrations postgres "$DATABASE_URL" up` 成功；`psql -c '\dt'` 看到全部表。

### P2-02 sqlc 与核心查询

**文件：** `sqlc.yaml`、`internal/repo/queries/trips.sql`、`runs.sql`、`versions.sql`、`events.sql`
**依赖：** P2-01

**步骤：**
1. 配置 sqlc 使用 `pgx/v5`。
2. 写 trips/runs/versions/events 的 CRUD 查询。
3. 写幂等查询：按 `request_id` 查找。
4. 生成代码。

**验证：** `sqlc generate` 成功；`go build ./...` 通过。

### P2-03 工具、证据、分享、评测查询

**文件：** `internal/repo/queries/tools.sql`、`evidence.sql`、`share.sql`、`eval.sql`
**依赖：** P2-02

**步骤：**
1. 写 tool_calls、evidence、poi_cache、plugin_cache 查询。
2. 写 share_links、eval_cases、eval_runs 查询。
3. 加按 run、trip、case 的索引查询。
4. 生成代码。

**验证：** `sqlc generate && go build ./...` 通过。

### P2-04 Store：行程、运行、版本、事件

**文件：** `internal/repo/store.go`、`store_test.go`
**依赖：** P2-02

**步骤：**
1. 实现 `Store` 接口中的 trip/run 创建和查询。
2. 实现 `SaveRunState`、`UpdateRunStatus`。
3. 实现 `SaveVersion`，事务内计算 `seq`。
4. 实现 `AppendEvent`、`ListEventsAfter`。
5. 用真实 Postgres 或 testcontainers 写集成测试。

**验证：** `go test ./internal/repo -run TestStoreCore` 通过。

### P2-05 Store：分享与评测

**文件：** `internal/repo/store.go`、`store_test.go`
**依赖：** P2-03

**步骤：**
1. 实现分享链接创建、查询、撤销。
2. 实现评测用例列表和评测结果写入。
3. 分享 slug 用加密随机数生成。
4. 测试重复 slug 冲突重试。

**验证：** `go test ./internal/repo -run TestStoreShareEval` 通过。

### P2-06 北京 POI 快照

**文件：** `data/poi/beijing.json`
**依赖：** P1-03

**步骤：**
1. 整理 40–60 个北京 POI，覆盖历史、博物馆、公园、街区、餐饮。
2. 每个 POI 填名称、类别、区域、地址、GCJ-02 坐标、营业时间、门票、建议时长、评分、户外标记、来源、核对日期。
3. 至少包含 5 个周一闭馆点、5 个餐饮点、5 个必去候选。
4. 检查 ID 唯一、坐标在北京范围内。

**验证：** `jq 'length' data/poi/beijing.json` 在 40–60；`jq` 检查 ID 唯一。

### P2-07 快照导入命令

**文件：** `cmd/seed/main.go`
**依赖：** P2-01、P2-06

**步骤：**
1. 读取并解析 `beijing.json`。
2. 校验必填字段和坐标范围。
3. 按主键 upsert 到 `poi_cache`。
4. 输出导入数量和跳过数量。

**验证：** 运行 `go run ./cmd/seed`；`SELECT count(*) FROM poi_cache` 与 JSON 数量一致。

### P2-08 快照搜索与工具注册

**文件：** `internal/tools/tool.go`、`snapshot.go`、测试
**依赖：** P1-03、P1-04、P2-07

**步骤：**
1. 实现 `Tool` 接口和 `Registry`。
2. `search_poi` 支持城市、类别、标签、必须包含的 POI。
3. 去重后限制 60 个；必去点优先保留。
4. 写测试：必去点命中、数量上限、空结果。

**验证：** `go test ./internal/tools -run TestSnapshotSearch` 通过。

### P2-09 高德客户端

**文件：** `internal/tools/amap.go`、`amap_test.go`
**依赖：** P0-02

**步骤：**
1. 封装高德 Web 服务 HTTP 客户端。
2. 实现 POI 搜索、地理编码、天气三个方法。
3. 统一超时、错误码、配额错误。
4. 用 `httptest` 模拟成功、超时、配额不足。

**验证：** `go test ./internal/tools -run TestAMap` 通过。

### P2-10 路径、直线距离与天气

**文件：** `internal/tools/route.go`、`weather.go`、测试
**依赖：** P2-09、P1-06

**步骤：**
1. 实现 `RouteProvider`：步行、公交、驾车。
2. 高德失败时用 haversine 估算：步行 4 km/h、公交 15 km/h，加 20% 缓冲。
3. 标记 `Estimated=true`。
4. 天气实现为软依赖，失败返回空结果。

**验证：** `go test ./internal/tools -run 'TestRoute|TestWeather'` 通过。

### P2-11 MCP Hub、适配器与 mock

**文件：** `internal/mcp/hub.go`、`adapter.go`、`normalize.go`、测试
**依赖：** P2-03、P2-09

**步骤：**
1. 实现白名单、超时 15 秒、重试 2 次、熔断。
2. 实现 MCP Tool 到 `Tool` 接口的适配。
3. 把结果归一化为 `Evidence`；无坐标不进入候选。
4. 写一个 mock MCP server 跑通链路。

**验证：** `go test ./internal/mcp/...` 通过；mock 调用被记录。

## P3 Agent

### P3-01 LLMClient

**文件：** `internal/agent/llm.go`、`llm_test.go`
**依赖：** P0-02

**步骤：**
1. 实现 OpenAI 兼容 Chat Completions 调用。
2. 支持 JSON 输出和 JSON Schema。
3. 解析失败重试 1 次，附带错误信息。
4. 记录 token、耗时、模型名。
5. 用 `httptest` 模拟合法、非法、超时。

**验证：** `go test ./internal/agent -run TestLLM` 通过。

### P3-02 Prompt 与 JSON Schema

**文件：** `internal/agent/prompts/*.tmpl`、`schemas.go`、测试
**依赖：** P3-01

**步骤：**
1. 写 parse、draft、repair、adjust、explain 五个模板。
2. 定义 `ConstraintSet`、`DraftPlan`、`DayPatch` 的 JSON Schema。
3. 用 `embed` 打包模板。
4. 测试模板渲染和 schema 编译。

**验证：** `go test ./internal/agent -run TestPromptSchemas` 通过。

### P3-03 Parse 节点

**文件：** `internal/agent/nodes.go`、测试
**依赖：** P1-02、P3-02

**步骤：**
1. 调用 LLM 解析输入。
2. 解码 `ConstraintSet`，合并默认值。
3. 缺 `city` 或 `days` 时返回 `Missing`。
4. 发布 `constraint.parsed` 或 `constraint.needs_input`。

**验证：** `go test ./internal/agent -run TestParseNode` 通过；缺城市时进入等待。

### P3-04 Retrieve 节点

**文件：** `internal/agent/nodes.go`、测试
**依赖：** P1-03、P2-08、P3-03

**步骤：**
1. 并行调用快照搜索和高德搜索。
2. 去重后保留 40–60 个候选。
3. 必去点缺失时扩大范围重试一次。
4. 发布 `candidate.retrieved`。

**验证：** `go test ./internal/agent -run TestRetrieveNode` 通过；必去点一定在集合。

### P3-05 Draft 节点

**文件：** `internal/agent/nodes.go`、测试
**依赖：** P1-04、P3-02、P3-04

**步骤：**
1. 把紧凑候选列表和约束送入 Prompt。
2. 解码 `DraftPlan`。
3. 校验所有 `poi_id` 在候选集中；非法立即重试一次，再失败则报错。
4. 餐饮开启时检查每天至少一个 meal。

**验证：** `go test ./internal/agent -run TestDraftNode` 通过；未知 ID 被拒绝。

### P3-06 Schedule 与 Validate 节点

**文件：** `internal/agent/nodes.go`、测试
**依赖：** P1-06、P1-13、P3-05

**步骤：**
1. 调用 `Scheduler.Build` 生成精确行程。
2. 调用 `Validator.Validate` 生成违规列表。
3. 发布 `plan.drafted` 和 `validation.result`。
4. 保存当前最优解。

**验证：** `go test ./internal/agent -run 'TestScheduleNode|TestValidateNode'` 通过。

### P3-07 Repair 节点与 DayPatch 合并

**文件：** `internal/agent/repair.go`、`nodes.go`、测试
**依赖：** P1-13、P3-06

**步骤：**
1. 从违规中确定受影响的天。
2. 检索同类别替代点，附带营业状态。
3. 调用 LLM 输出 `DayPatch`。
4. 只替换目标天，重新调度、全量校验。

**验证：** `go test ./internal/agent -run TestRepairNode` 通过；未受影响天不变。

### P3-08 停止条件与最优解

**文件：** `internal/agent/repair.go`、测试
**依赖：** P3-07

**步骤：**
1. 记录每轮阻断数、警告数、成本、步行距离。
2. 实现清零、3 轮、连续两轮无改善、用户取消四个停止条件。
3. 停止时选择最优解并保留剩余违规。
4. 生成放宽建议文本。

**验证：** `go test ./internal/agent -run TestStopConditions` 通过；不会无限循环。

### P3-09 Present 节点

**文件：** `internal/agent/nodes.go`、测试
**依赖：** P1-14、P3-08

**步骤：**
1. 生成版本快照：行程、约束、违规、原因。
2. 计算与上一版的 diff。
3. 合并解释文本；缺失时用模板兜底。
4. 发布 `plan.updated`，状态转 `awaiting_user`。

**验证：** `go test ./internal/agent -run TestPresentNode` 通过。

### P3-10 Eino 图与 TripState

**文件：** `internal/agent/graph.go`、`state.go`、测试
**依赖：** P3-03、P3-04、P3-05、P3-06、P3-07、P3-08、P3-09

**步骤：**
1. 用 Eino `compose` 建图，节点为 Lambda。
2. 加条件边：缺字段、阻断、停止、完成。
3. 定义 `TripState` 和 JSON 编解码。
4. 用 fake LLM 跑完整图。

**验证：** `go test ./internal/agent -run TestGraph` 通过；状态可 roundtrip。

### P3-11 Runner 生命周期

**文件：** `internal/agent/runner.go`、测试
**依赖：** P3-10、P2-04

**步骤：**
1. 实现 `Start`：异步执行，发布 `run.started`。
2. 实现 `Resume`：补充约束、接受、拒绝。
3. 实现 `Cancel`：context cancel，状态置 `cancelled`。
4. 实现 `Recover`：`queued` 重入队，`running` 标失败，`awaiting_user` 保留。
5. 每个阶段写 `runs.state`。

**验证：** `go test ./internal/agent -run TestRunner` 通过；重启后可恢复等待状态。

### P3-12 调整意图、并发与幂等

**文件：** `internal/agent/runner.go`、`nodes.go`、测试
**依赖：** P3-11

**步骤：**
1. 解析调整意图和影响范围。
2. 只对受影响天检索替代点和生成 `AdjustmentProposal`。
3. 信号量限制 3 个并发 run，超出保持 `queued`。
4. 用 `request_id` 实现幂等。

**验证：** `go test ./internal/agent -run 'TestAdjustment|TestConcurrency|TestIdempotency'` 通过。

### P3-13 CLI 跑通

**文件：** `cmd/cli/main.go`
**依赖：** P3-11

**步骤：**
1. 读取一句话输入和可选约束。
2. 调用 Runner 或直接调用图。
3. 打印最终行程 JSON 和违规列表。
4. 支持 `--fake-llm` 本地调试。

**验证：** `go run ./cmd/cli --fake-llm "北京两天..."` 输出行程 JSON。

## P4 事件与 API

### P4-01 Broker 与 SSE 编码

**文件：** `internal/events/broker.go`、`sse.go`、测试
**依赖：** P2-04

**步骤：**
1. 实现按 run 的订阅/取消订阅。
2. `Publish` 先写数据库，再广播。
3. SSE 编码 `id/event/data`，支持心跳。
4. 慢客户端丢弃，不阻塞发布。

**验证：** `go test ./internal/events/...` 通过。

### P4-02 路由、中间件与错误映射

**文件：** `internal/api/router.go`、`middleware.go`、`errors.go`、测试
**依赖：** P0-02、P4-01

**步骤：**
1. 用 chi 建路由，挂 healthz、meta、trips、runs、share、eval。
2. 实现 request ID、recover、body limit、IP 限流、admin token。
3. 统一错误响应 `{error:{code,message,request_id}}`。
4. 写中间件测试。

**验证：** `go test ./internal/api -run TestMiddleware` 通过。

### P4-03 创建与查询行程

**文件：** `internal/api/trips.go`、测试
**依赖：** P4-02、P3-11、P2-04

**步骤：**
1. `POST /api/trips` 校验输入、创建 trip+run、启动 Runner。
2. 返回 202 和 `events_url`。
3. `GET /api/trips/{id}` 返回摘要和最新版本。
4. `DELETE /api/trips/{id}` 软删除。
5. 幂等 key 重复时返回已有记录。

**验证：** `go test ./internal/api -run TestTrips` 通过。

### P4-04 版本、diff 与调整

**文件：** `internal/api/trips.go`、`runs.go`、测试
**依赖：** P4-03、P3-12

**步骤：**
1. 版本列表、版本详情。
2. 两版 diff 查询。
3. `POST /api/trips/{id}/adjustments` 创建调整 run。
4. 返回 202 和 `events_url`。

**验证：** `go test ./internal/api -run 'TestVersions|TestAdjustments'` 通过。

### P4-05 Resume 与 Cancel

**文件：** `internal/api/runs.go`、测试
**依赖：** P4-03、P3-11

**步骤：**
1. `POST /api/runs/{id}/resume` 支持 `provide_constraints`、`accept`、`reject`。
2. `POST /api/runs/{id}/cancel` 取消运行。
3. 校验 run 状态，非法动作返回 409。
4. 测试终态不可变。

**验证：** `go test ./internal/api -run 'TestResume|TestCancel'` 通过。

### P4-06 SSE 接口

**文件：** `internal/api/sse.go`、测试
**依赖：** P4-01、P4-02

**步骤：**
1. 设置 `text/event-stream`、禁止缓冲、keep-alive。
2. 支持 `Last-Event-ID` 和 `?after=` 回放。
3. 订阅 broker，推实时事件。
4. 每 2 秒心跳；客户端断开清理。

**验证：** `go test ./internal/api -run TestSSE` 通过；断线重连不丢事件。

### P4-07 分享、Meta 与健康检查

**文件：** `internal/api/share.go`、`meta.go`、测试
**依赖：** P4-02、P2-05

**步骤：**
1. 创建分享链接，只指向最终版本。
2. `GET /api/share/{slug}` 返回只读行程。
3. `GET /api/meta` 返回城市、默认值、版本。
4. `/healthz` 与 `/readyz`。

**验证：** `go test ./internal/api -run 'TestShare|TestMeta|TestHealth'` 通过。

### P4-08 评测接口、Admin 与限流

**文件：** `internal/api/eval.go`、`middleware.go`、测试
**依赖：** P4-02、P6-02

**步骤：**
1. Admin token 保护评测接口。
2. 用例列表、启动评测、查询结果。
3. 公开接口按 IP 10 次/小时限流。
4. 测试 429 和 401。

**验证：** `go test ./internal/api -run 'TestEval|TestRateLimit'` 通过。

### P4-09 API 集成与 OpenAPI 更新

**文件：** `internal/api/api_integration_test.go`、`openapi.yaml`
**依赖：** P4-03、P4-04、P4-05、P4-06、P4-07、P4-08

**步骤：**
1. 用 fake LLM 跑完整流程：创建 → SSE → 等待 → 接受 → 分享。
2. 覆盖调整接受/拒绝。
3. 更新 OpenAPI 到实际接口。
4. 跑 `redocly lint`。

**验证：** `go test ./internal/api/...` 通过；OpenAPI lint 通过。

## P5 Web

### P5-01 Vite + Tailwind + shadcn 骨架

**文件：** `web/package.json`、`vite.config.ts`、`src/main.tsx`、`src/index.css`、`tailwind.config.ts`、`components.json`
**依赖：** P0-01

**步骤：**
1. 创建 React + TypeScript + Vite 应用。
2. 配置 Tailwind 和 shadcn/ui。
3. 加基础布局、字体、颜色变量。
4. 写 `/` 占位页。

**验证：** `cd web && npm ci && npm run build` 通过。

### P5-02 API 类型与请求层

**文件：** `web/src/api/client.ts`、`types.ts`、`queryClient.ts`
**依赖：** P5-01、P0-05

**步骤：**
1. 用 `openapi-typescript` 生成类型。
2. 封装 fetch：错误码、request ID、JSON 解析。
3. 配 TanStack Query。
4. 写一个查询 hook 测试。

**验证：** `npm run build` 通过；类型与 OpenAPI 一致。

### P5-03 创建页与约束编辑

**文件：** `web/src/features/create/*`、`features/constraints/*`
**依赖：** P5-02

**步骤：**
1. 一句话输入、示例按钮、高级选项。
2. 提交时生成幂等 key。
3. 硬/软约束分开展示、可编辑、可切换。
4. 缺城市/天数时显示追问表单。

**验证：** Vitest 测试表单提交和约束切换。

### P5-04 SSE Hook 与运行轨迹

**文件：** `web/src/hooks/useRunStream.ts`、`features/trace/RunTrace.tsx`
**依赖：** P5-02、P4-06

**步骤：**
1. 用 `fetch-event-source` 订阅 SSE。
2. reducer 维护事件、阶段、轮数、违规。
3. 断线用 Last-Event-ID 重连，10 秒无心跳重试。
4. RunTrace 展示阶段、工具、校验、修复轮。

**验证：** 用 mock SSE 测 reducer；事件顺序正确。

### P5-05 工作台布局

**文件：** `web/src/features/workspace/WorkspacePage.tsx`
**依赖：** P5-03、P5-04

**步骤：**
1. 三栏：左轨迹、中地图+时间轴、右预算+版本。
2. 桌面三栏，移动端纵向堆叠。
3. 加载、空、失败三种状态。
4. 接入行程查询。

**验证：** `npm run build` 通过；路由可进入工作台。

### P5-06 时间轴

**文件：** `web/src/features/timeline/ItineraryTimeline.tsx`
**依赖：** P5-05

**步骤：**
1. 用 vis-timeline 按天分组。
2. 渲染 POI、餐饮、交通块。
3. 点击块抛出选中事件。
4. 支持只读和编辑两种模式。

**验证：** 组件测试渲染多天行程；点击返回 block ID。

### P5-07 高德地图

**文件：** `web/src/features/map/TripMap.tsx`、`web/src/lib/amap.ts`
**依赖：** P5-05

**步骤：**
1. 用 `@amap/amap-jsapi-loader` 加载。
2. 读取 `VITE_AMAP_JS_KEY` 和安全密钥。
3. 按天画不同颜色折线、标记。
4. 加载失败显示降级占位，不阻塞其他面板。

**验证：** 本地配置 Key 后地图显示；无 Key 时降级占位。

### P5-08 地图联动与预算

**文件：** `TripMap.tsx`、`ItineraryTimeline.tsx`、`features/budget/BudgetPanel.tsx`
**依赖：** P5-06、P5-07

**步骤：**
1. 选中时间块后地图居中并高亮。
2. 点击地图标记反向高亮时间块。
3. 预算面板显示门票、餐饮、交通、总计。
4. 估算费用加标识。

**验证：** 联动测试通过；预算数字与行程一致。

### P5-09 版本与 diff

**文件：** `web/src/features/versions/*`
**依赖：** P5-05、P4-04

**步骤：**
1. 版本下拉、切换、回退。
2. 用 jsondiffpatch 展示差异。
3. 差异按天和块分组。
4. 回退前二次确认。

**验证：** 组件测试切换版本和 diff 渲染。

### P5-10 分享页

**文件：** `web/src/features/share/SharePage.tsx`
**依赖：** P5-02、P4-07

**步骤：**
1. 通过 slug 拉取只读行程。
2. 展示时间轴、预算、AI 标识、来源。
3. 复制纯文本行程。
4. 未分享或撤销时显示 404。

**验证：** 组件测试复制文本和只读状态。

### P5-11 评测面板

**文件：** `web/src/features/eval/EvalDashboard.tsx`
**依赖：** P5-02、P4-08

**步骤：**
1. 列出 20 条用例和最近结果。
2. 一键启动评测。
3. 展示指标表：硬约束、轮数、延迟、POI 有效率、JSON 合法率。
4. 失败用例可展开 trace。

**验证：** 组件测试指标渲染；启动按钮调用正确接口。

### P5-12 移动端、错误态与 E2E

**文件：** `web/src/**/*.css`、`e2e/golden.spec.ts`、`playwright.config.ts`
**依赖：** P5-03、P5-04、P5-05、P5-06、P5-07、P5-08、P5-09、P5-10、P5-11

**步骤：**
1. 移动端三栏改纵向堆叠，地图和时间轴用 Tab。
2. 统一 loading、empty、error 组件。
3. 写 Playwright：创建 → SSE → 等待 → 接受 → 调整 → 分享。
4. 加地图失败场景。

**验证：** `npx playwright test` 通过；移动端视口无横向滚动。

## P6 评测、部署与文档

### P6-01 20 条评测用例

**文件：** `eval/cases/*.json`
**依赖：** P1-01

**步骤：**
1. 按 1 天 4 条、2 天 6 条、3 天 5 条、4 天 2 条、雨天/闭馆 2 条、必去/预算冲突 1 条。
2. 每条包含 `case_id`、`input`、`expected`。
3. `expected` 写天数、必须满足规则、必去、避开。
4. 检查 JSON 合法。

**验证：** `jq -s 'length' eval/cases/*.json` 为 20；每条有 `case_id`。

### P6-02 评测加载、跑批与指标

**文件：** `internal/eval/cases.go`、`runner.go`、`metrics.go`、`cmd/eval/main.go`、测试
**依赖：** P6-01、P3-11

**步骤：**
1. 从文件或数据库加载用例。
2. 逐条创建 eval run，记录耗时和违规。
3. 计算硬约束满足率、平均轮数、p95、POI 有效率、JSON 合法率。
4. 输出 Markdown 表。

**验证：** `go test ./internal/eval/...` 通过；`go run ./cmd/eval --dry-run` 输出表头。

### P6-03 真实评测跑批

**文件：** 无（执行任务）
**依赖：** P6-02、P3-13

**步骤：**
1. 配置真实 LLM 和高德 Key。
2. 跑 20 条用例。
3. 保存指标报告到 `docs/eval-report.md`。
4. 标记失败用例和原因。

**验证：** 硬约束满足率 ≥90%；JSON 合法率 ≥99%；3 天 p95 ≤60s；报告生成。

### P6-04 API Dockerfile

**文件：** `Dockerfile`、`.dockerignore`
**依赖：** P0-03

**步骤：**
1. 多阶段：`golang:1.24` 构建，`distroless/static` 运行。
2. 嵌入 migrations、prompts、POI 数据。
3. 暴露 8080，配置健康检查。
4. 排除 `.env`、`web/node_modules`。

**验证：** `docker build -t trippilot-api .` 通过；镜像可启动。

### P6-05 Caddy 与完整 Compose

**文件：** `deploy/web/Dockerfile`、`deploy/Caddyfile`、`deploy/docker-compose.yml`
**依赖：** P5-01、P6-04

**步骤：**
1. Web 多阶段构建，产物放进 Caddy 镜像。
2. Caddy `/` 静态、`/api/*` 反代，SSE 关闭缓冲。
3. Compose 串起 caddy、api、postgres。
4. 加环境变量和卷。

**验证：** `docker compose up -d` 后 `/healthz` 返回 200，首页可打开。

### P6-06 备份与运维

**文件：** `deploy/backup.sh`、`README.md`
**依赖：** P6-05

**步骤：**
1. `pg_dump` 到带日期的文件。
2. 保留 7 天，删除更早文件。
3. README 写 cron 示例和恢复命令。
4. 脚本失败时非零退出。

**验证：** 手动执行脚本生成 dump；`pg_restore --list` 可读。

### P6-07 README、文档与演示脚本

**文件：** `README.md`、`docs/demo-script.md`
**依赖：** P6-03、P6-05、P6-06

**步骤：**
1. README 写架构图、快速开始、API 准备、评测、部署、限制。
2. 文档补一次完整「生成 → 违规 → 修复 → 调整 → 分享」说明。
3. 写 3 分钟 demo 脚本：输入、预期、讲解点。
4. 检查所有链接。

**验证：** 按 README 从零启动一次；链接全部有效。

### P6-08 最终回归与彩排

**文件：** 无（执行任务）
**依赖：** 全部

**步骤：**
1. 跑 `go test -race ./...`、`npm test`、`npm run build`、`npx playwright test`。
2. 跑 20 条评测，确认指标。
3. 按 demo 脚本彩排一次，记录失败点。
4. 修复后重跑，准备 `checklist.md` 验收。

**验证：** 全部测试通过；评测达标；demo 无阻断问题。

## 执行顺序

```
P0-01 → P0-02 → P0-04 → P0-05 → P0-06
P0-01 → P0-03 → P2-01 → P2-02 → P2-03 → P2-04 → P2-05
P0-01 → P1-01 → P1-02 → P1-03 → P1-04 → P1-05
P1-05 → P1-06 → P1-07 → P1-08
P1-05 → P1-09 → P1-10 → P1-11 → P1-12 → P1-13
P1-04 → P1-14

P1-03 → P2-06 → P2-07 → P2-08
P2-09 → P2-10
P2-03 → P2-11

P1-02 → P3-01 → P3-02 → P3-03
P3-04 → P3-05 → P3-06 → P3-07 → P3-08 → P3-09
P3-03..P3-09 → P3-10 → P3-11 → P3-12 → P3-13

P2-04 → P4-01 → P4-02 → P4-03 → P4-04 → P4-05
P4-01 → P4-06
P4-02 → P4-07
P6-02 → P4-08
P4-03..P4-08 → P4-09

P0-01 → P5-01 → P5-02 → P5-03 → P5-04 → P5-05
P5-05 → P5-06
P5-05 → P5-07
P5-06/P5-07 → P5-08
P5-05 → P5-09
P5-02 → P5-10
P5-02 → P5-11
P5-03..P5-11 → P5-12

P1-01 → P6-01 → P6-02 → P6-03
P0-03 → P6-04 → P6-05 → P6-06
P6-03/P6-05/P6-06 → P6-07 → P6-08
```

**可并行**

- `P1-14` 与 `P1-09..P1-13` 可并行。
- `P2-06/P2-07` 可与 `P2-01..P2-05` 并行。
- `P5-01..P5-02` 可与 `P3/P4` 并行，但 `P5-03` 之后需要 API 可用。
- `P6-01` 可与 `P2/P3` 并行。

**阻塞规则**

- 每个任务的「验证」未通过，不得开始依赖它的任务。
- 如果接口或类型与 `plan.md` 不一致，先改文档再改代码。
- 如果任务超过 30 分钟仍未完成，拆成更小任务并补充到本文件。

## 审批

请确认本 task。确认后进入 `docs/mew-spec/v1-mvp/checklist.md`，把 spec 的 17 条验收标准转成可观测的行为检查；`checklist.md` 通过后才开始实现。