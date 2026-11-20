# TripPilot v1.7 技能渐进加载与治理 Tasks

状态：已批准（task）
日期：2026-11-20
依据：[spec.md](spec.md)（已批准）、[plan.md](plan.md)（已批准）

## 文件清单

| 操作 | 文件 | 职责 |
|------|------|------|
| 新建 | `internal/skills/catalog.go` | `CatalogLine`/`CatalogLines`/`CatalogText`/`ValidateCatalog`，8 KiB 预算 |
| 新建 | `internal/skills/lint.go` | `LintIssue`、`Lint`，正文行数与触发词检查 |
| 新建 | `internal/skills/service.go` | `Provider`、`Source`、`Service`、`Open`、L1 与 30s 指纹刷新 |
| 新建 | `internal/skills/cache.go` | `BodyCache`、`CacheStats`、L3 `RemoteCache` 预留 |
| 新建 | `internal/skills/catalog_test.go` | 渲染格式、预算、错误信息 |
| 新建 | `internal/skills/lint_test.go` | 行数、触发词、严格模式所需问题 |
| 新建 | `internal/skills/service_test.go` | TTL、指纹刷新、失败保留、无回退、版本索引 |
| 新建 | `internal/skills/cache_test.go` | 命中、singleflight、LRU、字节淘汰、错误不缓存 |
| 修改 | `internal/skills/skill.go` | 300 字符单行描述、`List`/`Resolve`、预算接入 |
| 修改 | `internal/skills/skill_test.go` | 描述与预算校验用例 |
| 修改 | `internal/agent/state.go` | `SkillPins` 字段 |
| 修改 | `internal/agent/nodes.go` | `skillProvider`、`WithSkillProvider`，默认 Provider |
| 修改 | `internal/agent/graph.go` | `admit` 固定目录；research 传 pin |
| 修改 | `internal/agent/skill.go` | `skillTool` 统一 legacy/pin 解析 |
| 修改 | `internal/agent/research.go` | 目录来源、`loadedSkill`、降级原因、事件载荷 |
| 修改 | `internal/agent/research_prompt.go` | `SkillLines` 字段 |
| 修改 | `internal/agent/prompts/research.tmpl` | 消费预渲染技能行 |
| 修改 | `internal/agent/skill_test.go` | pin 加载、缺失降级、旧正文兼容 |
| 修改 | `internal/agent/graph_test.go` / `runner_test.go` | 状态体积与恢复用例 |
| 修改 | `internal/api/router.go` | `SkillCatalog` 改为函数 |
| 修改 | `internal/api/meta_test.go` | 动态目录用例 |
| 修改 | `internal/api/share_test.go` | 分享不泄露技能内容 |
| 新建 | `internal/skillseval/runner.go` | 用例、运行、集合比对 |
| 新建 | `internal/skillseval/runner_test.go` | 用假模型验证通过/失败判定 |
| 新建 | `cmd/skilleval/main.go` | 环境变量、CLI、表格输出、退出码 |
| 新建 | `cmd/skills/main_test.go` | lint/strict 参数与退出行为 |
| 修改 | `cmd/skills/main.go` | `-lint` / `-strict` |
| 修改 | `cmd/api/main.go` | `skills.Open` 装配与 meta 目录回调 |
| 新建 | `eval/skills-fixtures/skills.lock.json` | 评测独立锁 |
| 新建 | `eval/skills-fixtures/museum-closure/1.0.0/SKILL.md` | 闭馆日技能 fixture |
| 新建 | `eval/skills-fixtures/rainy-day/1.0.0/SKILL.md` | 雨天替代技能 fixture |
| 新建 | `eval/skills-cases/cases.json` | 正例、区分例、负例 |
| 修改 | `Makefile` | `skills-lint` / `skills-lint-strict` / `skills-eval` |
| 修改 | `docs/skills.md` | pin-only、缓存、预算、lint/评测、保留规则 |
| 修改 | `README.md` | 技能说明与命令入口 |
| 新建 | `skills/beijing-research-playbook/1.0.1/SKILL.md` | 描述合规的新版本 |
| 修改 | `skills/skills.lock.json` | 固定 1.0.1 |
| 修改 | `web/src/features/conversation/deriveConversation.ts` | 技能加载对话项 |
| 修改 | `web/src/features/conversation/model.test.ts` | 对话项去重与元数据断言 |
| 修改 | `web/src/features/eval/trace/traceReducer.ts` | 评测轨迹技能事件 |
| 修改 | `web/src/features/eval/trace/traceReducer.test.ts` | 轨迹文案断言 |

## 阶段 A：skills 基础能力

### T1: 目录渲染与预算常量

**文件：** `internal/skills/catalog.go`、`internal/skills/catalog_test.go`
**依赖：** 无

**步骤：**
1. 定义 `MaxCatalogBytes = 8 << 10`。
2. 实现 `CatalogLine(entry)`，格式固定为 `- name@version：description`。
3. 实现 `CatalogLines(entries)`（保持传入顺序）和 `CatalogText(entries)`（行间 `\n`）。
4. 实现 `ValidateCatalog(entries)`：超过字节上限时返回错误，信息含总字节、上限、占用最大的前 3 个 `name@version`。
5. 编写测试：单行格式、UTF-8 字节计数、空目录、超限错误包含具体条目。

**验证：** `go test ./internal/skills -run 'TestCatalog'` 通过。

### T2: 描述规则与 Load/BuildLock 预算接入

**文件：** `internal/skills/skill.go`、`internal/skills/skill_test.go`
**依赖：** T1

**步骤：**
1. 把 `MaxDescriptionSize` 改为 300。
2. `validateMeta` 拒绝包含 `\n` 或 `\r` 的描述；错误包含技能路径。
3. `Load` 在全部技能加载后调用 `ValidateCatalog`，失败返回错误。
4. `BuildLock` 收集已加载技能，写锁前同样调用 `ValidateCatalog`。
5. 测试：301 字符、换行描述、目录超 8 KiB（可用 `ValidateCatalog` 直接构造 32 条长描述）、合法边界。

**验证：** `go test ./internal/skills -run 'TestLoad|TestCatalog'` 通过。

### T3: 编写规范 lint

**文件：** `internal/skills/lint.go`、`internal/skills/lint_test.go`
**依赖：** T2

**步骤：**
1. 定义 `LintIssue{Skill, Version, Rule, Message}`。
2. `Lint(dir)` 先调用 `Load`；硬错误直接返回。
3. 正文行数 > 500 产生 `body_too_long` warning。
4. 描述缺少触发词产生 `missing_trigger_cue` warning：中文子串“当/用于/需要/适合”，英文按词边界匹配 `when`/`use when`/`for`。
5. 结果按技能名、规则稳定排序；测试使用临时目录并写入锁，覆盖两类 warning 和合法描述。

**验证：** `go test ./internal/skills -run 'TestLint'` 通过。

### T4: UnavailableError 与 LocalSource

**文件：** `internal/skills/service.go`、`internal/skills/service_test.go`
**依赖：** T2

**步骤：**
1. 定义原因码常量：`not_found`、`version_mismatch`、`hash_mismatch`、`invalid_pin`、`source_error`。
2. 实现 `UnavailableError` 及 `Error`/`Unwrap`；错误不含正文、完整描述或绝对路径。
3. 实现 `LocalSource.Scan`：调用 `Load(dir)`，再生成 `Versions` 索引。
4. 实现 `LocalSource.Load`：校验 pin 的名称/版本/哈希格式，读取 `<dir>/<name>/<version>/SKILL.md`，复用 `loadSkillFile`，最后比对 pin 哈希。
5. 测试：精确版本解析、哈希不匹配、目录缺失、非法 pin、当前启用 v2 时仍能解析 v1。

**验证：** `go test ./internal/skills -run 'TestLocalSource|TestUnavailable'` 通过。

### T5: L2 BodyCache 与 singleflight

**文件：** `internal/skills/cache.go`、`internal/skills/cache_test.go`
**依赖：** T4

**步骤：**
1. 定义 `cacheKey{Name,Version,SHA256}`（哈希小写）、`CacheStats`、`BodyCache`。
2. 实现默认上限 64 条 / 4 MiB、LRU 淘汰、字节统计。
3. 实现 `GetOrLoad`：命中计数；同键 inflight 只执行一次加载；等待者拿到相同结果；错误不写缓存。
4. 实现 `warm`（不计 `SourceLoads`）和 `stats` 快照。
5. 定义 `RemoteCache` 接口并注明 L3 预留、v1.7 不实例化。
6. 测试：命中/未命中、并发 10 次只加载 1 次、错误不缓存、条目上限、字节上限、淘汰后可重载。

**验证：** `go test ./internal/skills -run 'TestBodyCache'` 通过。

### T6: Service 骨架、Open 与 L1 TTL

**文件：** `internal/skills/service.go`、`internal/skills/service_test.go`
**依赖：** T5

**步骤：**
1. 定义 `Provider`、`Manifest{Catalog,Versions}`、`ServiceOption`（`WithSource`/`WithClock`/`WithRefreshInterval`）。
2. `Open`：空目录返回空服务；首次 `Source.Scan` 失败即返回错误；成功后建立 L1。
3. `List`：30 秒内直接返回目录副本；并发用 `refreshMu` 串行。
4. 实现 `Catalog`、`Versions`、`Len`、`Stats`。
5. 测试：TTL 内不调用 `Scan`；初次失败返回错误；空目录可用。

**验证：** `go test ./internal/skills -run 'TestServiceList|TestOpen'` 通过。

### T7: 指纹刷新与失败语义

**文件：** `internal/skills/service.go`、`internal/skills/service_test.go`
**依赖：** T6

**步骤：**
1. 实现本地指纹：锁文件内容哈希 + 每个技能/版本目录 + `SKILL.md` 的 size/mtime。
2. `List` 超过 TTL 后：指纹未变只更新 `checkedAt`；变化才 `Source.Scan`。
3. 刷新成功替换 L1、更新指纹、清空 `refreshErr`。
4. 刷新失败保留旧 L1、记录 `refreshErr`、`List` 返回错误；30 秒内不重复扫描。
5. 测试：指纹未变不 Scan、锁变化触发 Scan、Scan 失败后新 List 报错且旧 `Catalog` 仍可读。

**验证：** `go test ./internal/skills -run 'TestServiceRefresh'` 通过。

### T8: Service.Resolve、预热与统计

**文件：** `internal/skills/service.go`、`internal/skills/service_test.go`
**依赖：** T7

**步骤：**
1. `Resolve` 只查 L2 和 `Source.Load`，不触发刷新、不查 L1 版本。
2. 刷新成功后把当前版本正文 `warm` 进 L2。
3. `Stats` 汇总 L2 计数与 `Refreshes`/`RefreshFailures`。
4. 测试：刷新失败时已缓存 pin 仍可 Resolve；未缓存 pin 按源读取；L2 命中不增加 `SourceLoads`。

**验证：** `go test ./internal/skills -run 'TestServiceResolve'` 通过。

### T9: Registry 实现 Provider

**文件：** `internal/skills/skill.go`、`internal/skills/skill_test.go`
**依赖：** T4

**步骤：**
1. 增加 `(*Registry).List(context.Context) ([]CatalogEntry, error)`，返回目录副本。
2. 增加 `(*Registry).Resolve(context.Context, CatalogEntry) (Skill, error)`：名称/版本/哈希必须精确匹配，否则返回 `UnavailableError`。
3. 保留 `Catalog`/`Lookup`/`Snapshot` 供现有调用方使用。
4. 测试：精确命中、版本不匹配、哈希不匹配、未知名称。

**验证：** `go test ./internal/skills -run 'TestRegistry'` 通过。

### T10: cmd/skills lint 模式

**文件：** `cmd/skills/main.go`、`cmd/skills/main_test.go`
**依赖：** T3

**步骤：**
1. 新增 `-lint`、`-strict` 标志；`-lint` 与 `-write` 互斥，`-strict` 仅用于 `-lint`。
2. `-lint` 调用 `skills.Lint`，逐条打印 `warning: name@version rule: message`。
3. `-strict` 且存在 warning 时返回错误；非严格模式退出 0。
4. 拆分 `run` 参数以便测试；覆盖默认、`-write`、`-lint`、`-strict` 四种路径。

**验证：** `go test ./cmd/skills` 通过；`go run ./cmd/skills -lint` 输出 warning 且退出 0。

### T11: Makefile lint 入口

**文件：** `Makefile`
**依赖：** T10

**步骤：**
1. 在 `.PHONY` 和 help 中加入 `skills-lint`、`skills-lint-strict`。
2. `skills-lint` 执行 `go run ./cmd/skills -lint`。
3. `skills-lint-strict` 执行 `go run ./cmd/skills -lint -strict`。

**验证：** `make skills-lint` 退出 0；`make skills-lint-strict` 在当前技能上通过。

### T12: 生产示例技能 1.0.1

**文件：** `skills/beijing-research-playbook/1.0.1/SKILL.md`、`skills/skills.lock.json`
**依赖：** T11

**步骤：**
1. 复制 1.0.0 正文，前置元数据版本改为 `1.0.1`。
2. 描述改为以“用于北京行程研究：”开头，保持单行、≤300 字符。
3. 保留 1.0.0 目录不动。
4. 运行 `go run ./cmd/skills -dir skills -write` 更新锁。
5. 确认锁只指向 1.0.1，1.0.0 目录仍存在。

**验证：** `go run ./cmd/skills` 显示 1.0.1；`make skills-lint-strict` 退出 0。

## 阶段 B：agent 集成

### T13: 运行状态与 Provider 装配

**文件：** `internal/agent/state.go`、`internal/agent/nodes.go`
**依赖：** T9

**步骤：**
1. `TripState` 增加 `SkillPins []skills.CatalogEntry`，保留 `Skills` 与 `SkillsPinned`。
2. `Nodes.skillRegistry` 改为 `skillProvider skills.Provider`。
3. `NewNodes` 默认 `skills.NewRegistry()`。
4. `WithSkillRegistry` 内部设置 provider；新增 `WithSkillProvider`。
5. 更新受影响的编译错误，不改变行为。

**验证：** `go build ./...` 通过。

### T14: admit 只固定目录

**文件：** `internal/agent/graph.go`、`internal/agent/skill_test.go`
**依赖：** T13

**步骤：**
1. `admit` 签名使用 `ctx`；未固定时调用 `provider.List(ctx)`。
2. 写入 `state.SkillPins`，不再写 `state.Skills`。
3. `List` 失败返回 `NodeError{Code:"skills_unavailable"}`。
4. 已固定运行直接返回，不重新读取目录。
5. 重写 `TestAdmitPinsSkillsForRun`：v1 固定目录、编码/解码、v2 恢复仍用 v1 pin、新运行用 v2 pin；断言没有正文。
6. 新增旧状态用例：`SkillsPinned=true` + `Skills` 正文保持原样。

**验证：** `go test ./internal/agent -run 'TestAdmit'` 通过。

### T15: ResearchInput、提示词与图传参

**文件：** `internal/agent/research.go`、`internal/agent/research_prompt.go`、`internal/agent/prompts/research.tmpl`、`internal/agent/graph.go`、`internal/agent/research_test.go`
**依赖：** T14

**步骤：**
1. `ResearchInput` 增加 `SkillPins []skills.CatalogEntry`。
2. `graph.research` 传入 `Skills` 和 `SkillPins`。
3. `ResearchPromptData.Skills` 改为 `SkillLines []string`，由 `skills.CatalogLines` 生成。
4. 模板改为直接输出预渲染行；旧正文优先取 catalog，其次 pin。
5. 更新 `RenderResearchPrompt` 测试，断言格式与 8 KiB 预算一致。

**验证：** `go test ./internal/agent -run 'TestRenderResearchPrompt|TestResearchLoadsPinnedSkill'` 通过（后者在 T16/T17 完成后完整）。

### T16: skillTool 统一 legacy/pin

**文件：** `internal/agent/skill.go`、`internal/agent/research.go`、`internal/agent/skill_test.go`
**依赖：** T15

**步骤：**
1. `skillTool` 持有 `pins`、`byName`、`legacy`、`provider`。
2. `newSkillTool(legacy, pins, provider)` 构建枚举；旧正文仅在无 pin 时优先。
3. `Call`：旧正文命中直接返回；否则 `provider.Resolve`。
4. 解析失败返回 `skill_unavailable`，载荷带 `skill.name/version`，不降级为 unknown。
5. Research 工具门控改为有工具且（旧正文或（pin 且 provider 非空））。
6. 测试：旧正文、pin 解析、缺失 pin 错误码、未知技能不触发降级。

**验证：** `go test ./internal/agent -run 'TestResearchLoadsPinnedSkill|TestResearchUnknownSkill'` 通过。

### T17: 加载记录、事件与降级

**文件：** `internal/agent/research.go`、`internal/agent/skill_test.go`
**依赖：** T16

**步骤：**
1. `loadedSkills` 改为 `[]loadedSkill{Name,Version,SHA256}`，按名称去重。
2. 成功加载发布 `research.skill_loaded`；`research.completed.skills_loaded` 输出对象数组。
3. `executeResearchTool` 识别 `skill_unavailable`，记录首个缺失 pin。
4. 最终状态条件加入 `skillUnavailable != nil`；原因格式 `skill_unavailable: name@version`。
5. 测试：模型先调缺失技能再输出 brief，断言 degraded、原因含版本、仍返回 brief；`research.completed` 载荷为对象。

**验证：** `go test ./internal/agent -run 'TestResearchSkill|TestResearchMissing'` 通过。

### T18: 图装配与状态体积/兼容测试

**文件：** `internal/agent/graph_test.go`、`internal/agent/runner_test.go`
**依赖：** T17

**步骤：**
1. 图级测试：新运行 `Encode` 后不含技能正文标记，只含 `skill_pins`。
2. 构造 10 个 10 KiB 正文的技能，断言状态大小远小于正文总量（如 < 64 KiB）。
3. 旧状态恢复测试：含 `skills` 正文，研究仍能加载，不需要迁移。
4. 调整现有测试中依赖 `state.Skills` 的断言。

**验证：** `go test ./internal/agent` 通过。

## 阶段 C：API 与 Web

### T19: API meta 动态目录

**文件：** `internal/api/router.go`、`internal/api/meta_test.go`、`cmd/api/main.go`
**依赖：** T8、T13

**步骤：**
1. `Deps.SkillCatalog` 改为 `func() []skills.CatalogEntry`。
2. meta 处理函数每次调用回调；为空时不输出 `skills`。
3. `cmd/api` 使用 `skills.Open`，传 `service.Catalog`；启动日志打印目录与缓存统计。
4. 更新 meta 测试为回调形式，增加“刷新后 meta 变化”的用例（直接替换回调返回值）。

**验证：** `go test ./internal/api -run 'TestMeta'` 通过。

### T20: 分享载荷不泄露技能内容

**文件：** `internal/api/share_test.go`
**依赖：** T19

**步骤：**
1. 构造含技能 pin 与旧正文的运行/版本数据。
2. 调用分享接口，断言响应不含技能正文、描述、`skill_pins` 或 `skills` 字段内容。
3. 若现有分享实现已天然隔离，仅补断言，不改生产代码。

**验证：** `go test ./internal/api -run 'TestShare'` 通过。

### T21: Web 对话技能加载项

**文件：** `web/src/features/conversation/deriveConversation.ts`、`web/src/features/conversation/model.test.ts`
**依赖：** T17

**步骤：**
1. 新增 `research.skill_loaded` 分支；slot key 为 `attempt + skill:name@version`。
2. 生成 `AgentMessage`：`stage:"research"`、`tone:"info"`、文本 `已加载技能 name@version`。
3. 重复事件合并，不新增条目；不读取 description/sha/body。
4. 测试：单次事件、重复事件、两个技能、事件乱序/重复输入稳定。

**验证：** `cd web && npm test -- model.test.ts` 通过。

### T22: Web 评测轨迹技能事件

**文件：** `web/src/features/eval/trace/traceReducer.ts`、`web/src/features/eval/trace/traceReducer.test.ts`
**依赖：** T17

**步骤：**
1. `describe` 增加 `research.skill_loaded`：标题 `加载技能 name`，详情 `name@version`，级别 `info`。
2. 状态机阶段保持 `research`，不新增阶段。
3. 测试：条目文案、阶段不变、载荷不含描述/正文。

**验证：** `cd web && npm test -- traceReducer.test.ts` 通过。

## 阶段 D：触发评测与文档

### T23: skillseval runner

**文件：** `internal/skillseval/runner.go`、`internal/skillseval/runner_test.go`
**依赖：** T17

**步骤：**
1. 定义 `Case`、`Config`、`Result`、`Report`、`LoadCases`、`Run`。
2. `Run` 每个用例使用 fixture 目录的 `skills.Open` 作为 provider，构造 `agent.Nodes`（tool LLM、noop `search_travel_notes`、限制 45s/4 次）。
3. 事件记录器收集 `research.skill_loaded` 名称，与 `expect_loaded` 精确比较。
4. 输出结果包含期望、实际、耗时、错误；任一失败 `Report.Failed > 0`。
5. 测试用假 `agent.ToolLLM` 模拟加载/不加载，验证通过、失败、错误不写入缓存等判定。

**验证：** `go test ./internal/skillseval` 通过。

### T24: cmd/skilleval 与 fixture

**文件：** `cmd/skilleval/main.go`、`eval/skills-fixtures/**`、`eval/skills-cases/cases.json`
**依赖：** T23

**步骤：**
1. 创建两个相似 fixture 技能：`museum-closure`（闭馆日）和 `rainy-day`（雨天替代），描述含明确触发场景。
2. 运行 `go run ./cmd/skills -dir eval/skills-fixtures -write` 生成独立锁。
3. 编写三个用例：应加载 museum-closure、应加载 rainy-day（区分）、都不应加载。
4. CLI 直接读取 `LLM_BASE_URL`/`LLM_API_KEY`/`LLM_MODEL`；缺失时列出变量并退出非零。
5. 构造 `agent.NewOpenAIClient`，调用 `skillseval.Run`，打印表格；失败非零退出。
6. 不调用 `config.Load`，不要求 `DATABASE_URL`。

**验证：** 未配置凭据时命令报错并列出变量；配置后 `go run ./cmd/skilleval` 三个用例均通过（人工执行）。

### T25: Makefile 评测入口与文档

**文件：** `Makefile`、`docs/skills.md`、`README.md`
**依赖：** T24

**步骤：**
1. Makefile 增加 `skills-eval: go run ./cmd/skilleval` 与 help。
2. `docs/skills.md` 更新：pin-only 状态、按需解析与缺失行为、L1/L2/L3、30s 指纹、预算、编写规范、lint/评测命令、版本保留规则。
3. README 更新技能说明、命令入口和“正文不进入状态”的描述。
4. 文档明确：技能正文不进入 `/api/meta`、公开分享或日志；评测需要模型凭据。

**验证：** `make help` 显示三个新目标；文档中的命令可执行。

### T26: 全量验证

**文件：** 无新增
**依赖：** 全部

**步骤：**
1. `go build ./...`。
2. `go test ./...`。
3. `go vet ./...`。
4. `make skills-verify`、`make skills-lint-strict`。
5. `cd web && npm test`。
6. 人工执行 `make skills-eval`（需模型凭据），记录三个用例结果。
7. 检查 `git status`，确认没有新增数据库迁移。

**验证：** 上述命令全部通过；把实际输出记录到验收报告。

## 执行顺序

```
T1 → T2 ─┬→ T3 → T10 → T11 → T12
         └→ T4 → T5 → T6 → T7 → T8 ─┐
T4 → T9 ─────────────────────────────┴→ T13 → T14 → T15 → T16 → T17
                                                          ├→ T18
                                                          ├→ T21
                                                          ├→ T22
                                                          └→ T23 → T24 → T25
T13 → T19 → T20
T12 ─────────────────────────────────────────────────────────────→ T26
T18/T20/T21/T22/T25 ────────────────────────────────────────────→ T26
```

可并行：T18/T20/T21/T22 在 T17 后互不依赖；T19/T20 可与 agent 阶段并行；T25 在 T24 后。
