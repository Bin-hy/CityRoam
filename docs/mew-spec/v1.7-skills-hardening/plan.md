# TripPilot v1.7 技能渐进加载与治理 Plan

状态：已批准（plan）
日期：2026-11-20
依据：[spec.md](spec.md)（已批准）

## 架构概览

v1.7 在现有 `internal/skills` 与 `internal/agent` 之间增加一层进程内技能服务，并把运行状态从“全量正文快照”改为“目录 pin”。

- **技能服务 `skills.Service`**：进程内单例，负责 L1 清单缓存、L2 正文缓存、30 秒指纹校验和按 pin 解析。它实现 `skills.Provider`，是 `admit` 和 `load_skill` 的唯一数据入口。本地文件通过 `skills.Source` 抽象读取，为未来数据库源留出替换点；Redis 作为 L3 只保留接口标记，不接入。
- **清单与锁 `skills.Registry` / `skills.Load`**：保留现有不可变注册表和启动校验，作为 `Service` 的回源实现和测试替身。新增 `List`/`Resolve` 方法使其满足 `Provider`，现有 `Catalog`/`Lookup` 继续供 API 和测试使用。
- **图状态 `TripState.SkillPins`**：`admit` 只写入 `name/version/description/sha256`，不写正文。旧状态里的 `Skills []Skill` 字段保留，仅用于兼容已存在的运行。
- **研究节点**：`ResearchInput` 同时接收旧正文和 pin；`load_skill` 优先使用旧正文，否则通过 `Provider.Resolve` 按 pin 取正文。固定版本缺失时返回稳定错误码并降级，不回退到当前版本。
- **触发评测 `internal/skillseval` + `cmd/skilleval`**：真实模型 + 独立 fixture 技能 + 无副作用 research 工具，校验“该加载/不该加载”，不进入默认测试和 CI。
- **Web**：只消费 SSE 元数据。对话流为每次 `research.skill_loaded` 增加一个非阻断加载项；评测轨迹增加同名事件条目；不展示描述或正文。

数据流总览：

```
启动：cmd/api -> skills.Open(dir)
                -> Source.Scan -> Load(锁/版本/哈希/预算校验)
                -> L1 清单 + 版本索引，L2 预热当前版本正文
                -> agent.WithSkillProvider(service)

新运行：admit -> Provider.List(ctx)
               -> L1 命中（≤30s）直接返回目录
               -> 超时后比对本地指纹，变化才回源 Scan 并替换 L1
               -> state.SkillPins = 目录；SkillsPinned = true（无正文）

研究：load_skill(name) -> 旧正文（如有）
                      -> Provider.Resolve(pin)
                      -> L2 key=name+version+sha256
                      -> 未命中：singleflight -> Source.Load(pin)
                      -> 校验名称/版本/哈希 -> 回填 L2 -> 返回正文
                      -> 失败：工具错误 skill_unavailable，研究 degraded，继续出方案
```

## 核心数据结构

### skills.Provider

调用方唯一依赖的技能接口。放在 `skills` 包，避免 `agent` 反向依赖。

```go
type Provider interface {
    List(ctx context.Context) ([]CatalogEntry, error)
    Resolve(ctx context.Context, pin CatalogEntry) (Skill, error)
}
```

- `List` 返回当前启用目录；缓存有效时不读取源。
- `Resolve` 只按传入 pin 解析，不允许替换为同名其他版本或当前启用版本。
- `*Registry` 和 `*Service` 都实现该接口；前者用于测试、离线评测和兼容旧装配，后者用于生产。

### skills.Service

```go
type Service struct {
    dir          string
    source       Source
    bodies       *BodyCache
    refresh      time.Duration
    now          func() time.Time
    remote       RemoteCache // L3 预留，v1.7 恒为 nil，不读取任何 Redis 配置

    mu           sync.RWMutex
    manifest     Manifest
    checkedAt    time.Time
    fingerprint  string
    refreshErr   error

    refreshMu    sync.Mutex
    stats        serviceStats
}

type Manifest struct {
    Catalog  []CatalogEntry
    Versions map[string][]string
}
```

构造函数与读取接口：

```go
func Open(dir string, opts ...ServiceOption) (*Service, error)
func (s *Service) List(ctx context.Context) ([]CatalogEntry, error)
func (s *Service) Resolve(ctx context.Context, pin CatalogEntry) (Skill, error)
func (s *Service) Catalog() []CatalogEntry                 // 最近一次有效清单，供 /api/meta
func (s *Service) Versions() map[string][]string           // 诊断/测试
func (s *Service) Stats() CacheStats
func (s *Service) Len() int
```

`Open` 完成首次 `Scan`；目录不存在或为空字符串时返回空服务；首次校验失败直接返回错误，由进程启动失败（fail-fast）。

### skills.Source

```go
type ScanResult struct {
    Skills   []Skill
    Versions map[string][]string
}

type Source interface {
    Scan(ctx context.Context, dir string) (ScanResult, error)
    Load(ctx context.Context, dir string, pin CatalogEntry) (Skill, error)
}

type LocalSource struct{}
```

- `LocalSource.Scan` 调用现有 `Load(dir)` 做锁/版本/哈希/预算校验，再用 `skillDirs`/`versionDirs` 生成版本索引。
- `LocalSource.Load` 校验 pin 的名称、版本、哈希格式后，读取 `<dir>/<name>/<version>/SKILL.md`，复用 `loadSkillFile` 校验内容，最后与 pin 的 SHA-256 精确比对。
- 未来数据库源实现同一接口；调用方不变。

### skills.UnavailableError

```go
type UnavailableError struct {
    Code    string // not_found | version_mismatch | hash_mismatch | invalid_pin | source_error
    Name    string
    Version string
    Err     error
}
```

错误信息只包含技能名、版本、原因码和底层错误，不包含正文、完整描述或本地绝对路径。`agent` 用它生成 `skill_unavailable` 工具错误。

### skills.BodyCache（L2）

```go
type CacheStats struct {
    Hits, Misses, SourceLoads, Evictions uint64
    Refreshes, RefreshFailures           uint64
    Entries                              int
    Bytes                                int64
}

type cacheKey struct {
    Name, Version, SHA256 string // SHA256 统一小写
}

type BodyCache struct {
    mu       sync.Mutex
    lru      *list.List
    entries  map[cacheKey]*list.Element
    inflight map[cacheKey]*bodyFlight
    bytes    int64
    maxItems int
    maxBytes int64
    stats    cacheStats
}
```

- 默认上限：64 条、4 MiB；常量 `DefaultBodyCacheEntries` / `DefaultBodyCacheBytes`。
- `GetOrLoad(ctx, pin, load)`：命中移到队首；未命中先登记 `inflight`，由首个调用者回源，其余等待同一个结果；失败不写缓存；回源成功后按条目数和字节数淘汰队尾。
- `warm(pin, skill)` 在清单刷新成功后写入当前版本正文，不增加 `SourceLoads`。
- 字节数按 `len(Body)` 计；条目开销忽略，上限本身已有余量。
- `RemoteCache` 接口只定义不调用：
  ```go
  // RemoteCache 是 L3 预留位；v1.7 不创建实例，不新增 Redis 依赖或配置。
  type RemoteCache interface {
      Get(ctx context.Context, key string) (Skill, bool, error)
      Put(ctx context.Context, key string, skill Skill, ttl time.Duration) error
  }
  ```

### agent.TripState

```go
Skills      []skills.Skill         `json:"skills,omitempty"`      // 仅旧状态兼容
SkillPins   []skills.CatalogEntry  `json:"skill_pins,omitempty"`  // 新运行：目录 pin
SkillsPinned bool                  `json:"skills_pinned,omitempty"`
```

- 新运行只写 `SkillPins`。
- 旧状态（`skills_pinned=true` 且有 `skills` 正文）继续按旧字段工作，不迁移、不删除。
- `SkillPins` 为空且 `SkillsPinned=true` 表示“该运行固定时没有技能”，不重新读取目录。

### agent.ResearchInput 与会话

```go
type ResearchInput struct {
    // 现有字段不变
    Skills    []skills.Skill        // 旧状态正文
    SkillPins []skills.CatalogEntry // 新状态目录
}

type loadedSkill struct {
    Name    string `json:"name"`
    Version string `json:"version"`
    SHA256  string `json:"sha256"`
}

type researchSession struct {
    // 现有字段不变
    loadedSkills     []loadedSkill
    skillUnavailable *loadedSkill // 首个不可用的固定版本
}
```

### agent.skillTool

```go
type skillTool struct {
    pins     []skills.CatalogEntry
    byName   map[string]skills.CatalogEntry
    legacy   map[string]skills.Skill
    provider skills.Provider
}
```

- 名称枚举来自 `pins`（新运行）或旧正文的 `CatalogEntry()`。
- `Call` 先查 `legacy`（仅旧状态），否则 `provider.Resolve`。
- 解析失败返回：
  ```json
  {"error":{"code":"skill_unavailable","message":"固定技能 <name>@<version> 不可用","skill":{"name":"...","version":"..."}}}
  ```
- 成功返回现有 `skillPayload`，正文由 `renderSkillContent` 包装为 `<skill>`，不经过 `<external_data>`。

## 模块设计

### internal/skills

**职责：** 清单/正文缓存、指纹刷新、按 pin 解析、目录预算、lint。

**对外接口：**

```go
func Open(dir string, opts ...ServiceOption) (*Service, error)
func (s *Service) List(ctx context.Context) ([]CatalogEntry, error)
func (s *Service) Resolve(ctx context.Context, pin CatalogEntry) (Skill, error)

func (r *Registry) List(context.Context) ([]CatalogEntry, error)
func (r *Registry) Resolve(context.Context, CatalogEntry) (Skill, error)

func CatalogLine(entry CatalogEntry) string
func CatalogLines(entries []CatalogEntry) []string
func CatalogText(entries []CatalogEntry) string
func ValidateCatalog(entries []CatalogEntry) error

func Lint(dir string) ([]LintIssue, error)
```

**刷新规则：**

1. `List` 先用读锁检查 `now - checkedAt < refresh`（默认/上限 30s），命中直接返回目录副本，不读锁文件、不读技能文件。
2. 超过间隔后，串行计算本地指纹：
   - `skills.lock.json` 内容 SHA-256（文件不存在时记 `absent`）；
   - 每个技能目录名、版本目录名、对应 `SKILL.md` 的 `size + mtimeUnixNano`。
3. 指纹与上次一致：只更新 `checkedAt`，不 `Scan`。
4. 指纹变化：调用 `Source.Scan`。成功则替换 `Manifest`、更新指纹、清空 `refreshErr`、预热 L2 当前版本正文；失败则保留上一次有效 `Manifest`，记录 `refreshErr`，`List` 返回包装后的错误。
5. `Resolve` **不触发刷新**，只查 L2 和 `Source.Load`，因此清单重载失败不影响已固定运行。
6. 并发 `List` 用 `refreshMu` 合并刷新；等待者拿到同一次结果。

**预算与 lint：**

- `MaxDescriptionSize` 改为 300 字符；`validateMeta` 拒绝 `\n`/`\r`，错误包含技能路径。
- 新增 `MaxCatalogBytes = 8 << 10`；`Load` 和 `BuildLock` 在加载全部技能后调用 `ValidateCatalog`。错误报告总字节、上限，以及占用最大的前 3 个目录行。
- `CatalogLine` 是唯一渲染格式：`- name@version：description`。研究提示词模板直接消费 `CatalogLines`，预算校验和实际提示词不会漂移。
- `Lint(dir)` 先 `Load`（硬错误即失败），再检查：
  - 正文行数 > 500 → `body_too_long` warning；
  - 描述小写后不含中文提示词（当/用于/需要/适合）或英文词（`when`/`use when`/`for`，按词边界）→ `missing_trigger_cue` warning。
- `Lint` 只返回问题，是否失败由 CLI 的 `-strict` 决定。

**依赖：** 标准库、`gopkg.in/yaml.v3`；不依赖 `agent`、数据库或 Redis。

### internal/agent

**职责：** 运行级 pin、按需解析、降级与事件。

**改动点：**

- `Nodes.skillRegistry *skills.Registry` → `skillProvider skills.Provider`；默认 `skills.NewRegistry()`。
- `WithSkillRegistry` 保留，内部设置 provider；新增 `WithSkillProvider` 供生产装配和测试注入。
- `admit`：未固定时调用 `provider.List(ctx)`，写入 `SkillPins`；失败返回 `NodeError{Code:"skills_unavailable"}`。
- `graph.research`：同时传 `Skills: state.Skills`、`SkillPins: state.SkillPins`。
- `Research`：工具门控改为“有研究工具且（有旧正文或（有 pin 且有 provider））”；目录取自旧正文或 pin。
- `executeResearchTool`：
  - 识别 `skill_unavailable`，记录首个缺失 pin，继续循环；
  - 成功加载时记录 `loadedSkill{Name,Version,SHA256}` 并发布 `research.skill_loaded`；
  - 最终状态在 `timedOut/llmFailed/toolErrors/limitHit/!parsed` 之外，增加 `skillUnavailable != nil` → degraded。
- 降级原因：`skill_unavailable: <name>@<version>`（稳定前缀 + 具体版本）。
- `research.completed.skills_loaded` 从字符串数组改为 `loadedSkill` 对象数组；`research.skill_loaded` 载荷不变。
- 未知技能、加载次数上限仍只返回工具错误，不降级（保持现有语义）。

### internal/skillseval 与 cmd/skilleval

**职责：** 真实模型触发评测，离线运行。

```go
type Case struct {
    ID           string        `json:"id"`
    Input        string        `json:"input"`
    City         string        `json:"city"`
    Days         int           `json:"days"`
    Candidates   []domain.POI  `json:"candidates"`
    ExpectLoaded []string      `json:"expect_loaded"` // 精确集合
}

type Config struct {
    SkillsDir string
    Cases     []Case
    LLM       agent.ToolLLM
    Timeout   time.Duration
}

type Result struct {
    Case     Case
    Loaded   []string
    Duration time.Duration
    Err      error
}

func LoadCases(path string) ([]Case, error)
func Run(ctx context.Context, cfg Config) (Report, error)
```

- 每个用例：`skills.Open(fixtureDir)` 得到 provider，构造 `Nodes`（`WithToolLLM`、`WithSkillProvider`、`WithResearchTools(noopGuideTool)`、`WithResearchLimits(timeout, 4)`），用事件记录器捕获 `research.skill_loaded`。
- 无副作用工具固定命名为 `search_travel_notes`，返回 `{"results":[]}`，不产生酒店/天气调用。
- 实际加载集合与 `expect_loaded` 精确比较；空数组表示负例。
- `cmd/skilleval` 直接读取 `LLM_BASE_URL`、`LLM_API_KEY`、`LLM_MODEL`，不调用 `config.Load`，因此不要求 `DATABASE_URL`。缺凭据时报错并打印所需变量。
- 输出逐用例表格（期望/实际/耗时/结果）；任一失败或运行错误以非零退出。
- fixture 位于 `eval/skills-fixtures/`，不放进 `skills/`；包含两个相似技能（如博物馆闭馆、雨天替代）和独立 `skills.lock.json`。用例位于 `eval/skills-cases/cases.json`，至少一个正例、一个区分例、一个负例。

### cmd/skills

- 默认模式：`Load` 校验并打印 pin（现有行为）。
- `-write`：`BuildLock` + `WriteLock`（现有行为，纳入目录预算校验）。
- `-lint`：`Lint(dir)` 打印 warning；与 `-write` 互斥。
- `-strict`：仅与 `-lint` 联用，存在 warning 时退出 1。
- 所有模式使用同一套 `Load`/预算校验。

### internal/api

- `Deps.SkillCatalog []skills.CatalogEntry` → `SkillCatalog func() []skills.CatalogEntry`。
- `/api/meta` 每次请求读取最近一次有效清单，刷新成功后自动反映新版本；刷新失败时仍返回上次有效清单，不暴露错误细节。
- 分享版本不包含 run state，本次不新增技能字段；补一条测试断言分享载荷不含技能正文/描述。

### Web

- `web/src/features/conversation/deriveConversation.ts`：新增 `research.skill_loaded` 分支，按 `attempt + name@version` 使用 `agentSlot` 生成/更新一条 `AgentMessage`：
  - `stage: "research"`、`tone: "info"`、文本 `已加载技能 name@version`；
  - 不展示 SHA-256、描述或正文；重复事件合并。
- `web/src/features/eval/trace/traceReducer.ts`：`describe` 新增 `research.skill_loaded`，标题 `加载技能 name`，详情 `name@version`，级别 `info`；状态机的 `research` 阶段保持不变。
- 单测覆盖对话去重、轨迹文案和“不含正文/描述”。

### 文档与 Makefile

- `docs/skills.md`：pin-only 状态、按需解析与缺失行为、L1/L2/L3、指纹刷新、预算、编写规范、lint/评测、版本保留规则。
- `README.md`：更新“版本化技能”说明和命令入口。
- `Makefile`：
  - `skills-lint`：`go run ./cmd/skills -lint`（只警告）；
  - `skills-lint-strict`：`go run ./cmd/skills -lint -strict`（CI 用）；
  - `skills-eval`：`go run ./cmd/skilleval`。
- 生产示例技能新增 `1.0.1`：描述改为以“用于”开头以通过严格 lint，正文不变；保留 `1.0.0` 目录并用 `make skills-lock` 更新锁，作为版本保留规则的实例。

## 模块交互

### 启动

```
cmd/api
  └─ skills.Open(cfg.SkillsDir)
       ├─ LocalSource.Scan -> skills.Load（锁、版本、哈希、描述、目录预算）
       ├─ 构建 Manifest{Catalog, Versions}
       ├─ BodyCache.warm(当前版本)
       └─ 失败 -> 返回错误，进程退出
  ├─ agent.WithSkillProvider(service)
  └─ api.Deps.SkillCatalog = service.Catalog
```

### admit（新运行）

```
Graph.admit
  └─ Service.List(ctx)
       ├─ now-checkedAt < 30s -> 返回 L1 目录（零源读取）
       ├─ 指纹未变 -> 更新 checkedAt，返回 L1
       ├─ 指纹变化 -> Source.Scan
       │     ├─ 成功 -> 替换 L1 + 预热 L2
       │     └─ 失败 -> 保留上次清单，记录 refreshErr，返回错误
       └─ 错误 -> NodeError skills_unavailable
  └─ state.SkillPins = 目录；SkillsPinned = true
```

### 恢复运行

```
DecodeTripState
  ├─ skills_pinned=true + skills 非空 -> 旧正文路径
  ├─ skills_pinned=true + skill_pins 非空 -> 新 pin 路径（不再 List）
  └─ 两者都空 -> 固定时无技能，不加载
```

### research 与 load_skill

```
Research
  ├─ 旧正文存在 -> 目录来自旧正文，skillTool.legacy 命中
  └─ pin 存在 -> 目录来自 pin，skillTool 调 Provider.Resolve
       └─ Service.Resolve
            ├─ BodyCache.GetOrLoad(key=name+version+sha256)
            │    ├─ hit -> 返回
            │    ├─ inflight -> 等待同一结果
            │    └─ miss -> LocalSource.Load -> 校验 -> 回填
            └─ 失败 -> UnavailableError -> skill_unavailable 工具错误
  └─ 最终：degraded（skill_unavailable: name@version），继续 draft/plan
```

### 评测

```
cmd/skilleval
  ├─ 读取 LLM_* 环境变量（无 DATABASE_URL 要求）
  ├─ skills.Open(eval/skills-fixtures)
  ├─ 逐用例运行 Research（noop 工具）
  ├─ 收集 research.skill_loaded 名称集合
  └─ 对比 expect_loaded -> 表格 -> 失败非零退出
```

## 文件组织

```
internal/skills/
├── skill.go              — 现有 Load/BuildLock/Registry；新增 List/Resolve、预算常量
├── service.go            — Provider、Source、Service、Open、L1 与指纹刷新
├── cache.go              — BodyCache、CacheStats、RemoteCache（L3 预留）
├── catalog.go            — CatalogLine/CatalogLines/CatalogText/ValidateCatalog
├── lint.go               — LintIssue、Lint
├── service_test.go
├── cache_test.go
├── catalog_test.go
└── lint_test.go

internal/agent/
├── state.go              — SkillPins 字段
├── nodes.go              — skillProvider、WithSkillProvider
├── graph.go              — admit 固定目录；research 传 pin
├── skill.go              — skillTool 统一 legacy/pin 解析
├── research.go           — 目录来源、loadedSkill、降级原因
├── research_prompt.go    — SkillLines
├── prompts/research.tmpl — 消费 SkillLines
└── *_test.go             — pin/兼容/降级/状态体积测试

internal/api/
├── router.go             — SkillCatalog 改为函数
├── meta_test.go
└── share_test.go         — 断言不泄露技能内容

internal/skillseval/
├── runner.go
├── runner_test.go
eval/
├── skills-fixtures/
│   ├── skills.lock.json
│   ├── museum-closure/1.0.0/SKILL.md
│   └── rainy-day/1.0.0/SKILL.md
└── skills-cases/cases.json

cmd/skilleval/main.go
cmd/skills/main.go        — -lint/-strict
cmd/api/main.go           — Service 装配
Makefile                  — skills-lint / skills-lint-strict / skills-eval
docs/skills.md
README.md
skills/beijing-research-playbook/1.0.1/SKILL.md
skills/skills.lock.json

web/src/features/conversation/deriveConversation.ts
web/src/features/conversation/model.test.ts
web/src/features/eval/trace/traceReducer.ts
web/src/features/eval/trace/traceReducer.test.ts
```

## 技术决策

| 决策点 | 选择 | 理由 |
|--------|------|------|
| Provider 归属 | 放在 `internal/skills` | `agent` 已依赖 `skills`，避免反向依赖；Registry/Service 同时实现，测试装配成本低 |
| 生产装配 | `skills.Open` 创建 Service；`cmd/eval` 继续用 Registry | API 需要L1/L2 与刷新；离线评测只需只读目录 |
| L1 刷新 | 30 秒上限 + 本地指纹，指纹变化才 `Scan` | 满足 F17；正常请求零文件读取，避免每请求扫目录 |
| 刷新失败 | 保留上次清单、记录错误；`List` 报错，`Resolve` 不受影响 | 新运行明确失败，已固定运行继续；不使用部分结果 |
| L2 刷新语义 | 不整体清空哈希键缓存，只替换 L1 并预热新当前版本 | 已固定旧版本仍需正文；键含哈希保证正确性，LRU 限制内存。这是对 F17“相关条目失效”的收窄解释：失效的是当前清单，不是所有历史 pin |
| 版本回退 | 任何 `Resolve` 只接受 pin 精确版本+哈希；不查当前清单 | F3/N9；避免升级/刷新改变进行中运行 |
| 缓存键 | `name + version + sha256`（哈希小写） | 同名不同版本、同版本不同内容互不覆盖 |
| 并发回源 | 进程内 singleflight，首个调用者加载，其余等待 | F18/N10；本地文件读取也避免突发重复 I/O |
| 缓存上限 | 64 条 / 4 MiB | 目录最多 32 个技能，64 条覆盖当前+若干旧 pin；4 MiB 远大于单次研究实际用量 |
| L3 | 只定义接口和字段，恒为 nil，不加配置 | F16/N11；避免“配置了不生效” |
| 状态兼容 | 保留 `Skills` 字段，新增 `SkillPins` | F5/N3；无迁移，旧运行可直接恢复 |
| 目录渲染 | `CatalogLine` 单一实现，模板消费预渲染行 | F6 预算与实际提示词一致，避免模板/校验漂移 |
| 预算校验位置 | `Load` 与 `BuildLock` 共用 `ValidateCatalog` | 启动、verify、lock、lint 同一套规则 |
| 降级范围 | 只有 `skill_unavailable` 触发研究 degraded | 未知技能/次数上限是模型可恢复错误，不应影响运行状态 |
| 评测隔离 | `eval/` + 独立命令 + 独立锁，不进 `go test`/CI | F9/F10；真实模型不确定性和成本不应污染默认测试 |
| 示例技能升级 | 新增 1.0.1 描述合规，保留 1.0.0 | 严格 lint 对生产技能也成立；同时演示版本保留规则 |

## Spec 覆盖对照

| 需求 | 落点 |
|------|------|
| F1 | `TripState.SkillPins` + `admit` 只写目录 |
| F2 | `Service.Resolve` + `skillTool`，单次运行 3 个上限不变 |
| F3 | `LocalSource.Load` 精确比对 pin；无回退分支 |
| F4 | `UnavailableError` → `skill_unavailable` → degraded 原因含 name@version |
| F5 | `TripState.Skills` 保留，research 旧正文优先 |
| F6 | 300 字符单行、8 KiB 目录、32 个；`Load`/`BuildLock`/lint 共用 |
| F7 | `-lint`/`-strict`，行数与触发词检查 |
| F8 | docs 版本保留规则；保留 1.0.0 目录 |
| F9/F10 | `internal/skillseval` + `cmd/skilleval` + `eval/` fixture |
| F11 | 事件载荷 name/version/sha；completed 为对象数组；降级原因含版本 |
| F12 | Web 对话项 + 轨迹条目，仅元数据 |
| F13 | docs/README/Makefile |
| F14 | L1 `Manifest.Catalog` + `Versions` + 指纹 TTL |
| F15 | `BodyCache`，键含哈希，条目/字节上限 |
| F16 | `RemoteCache` 接口标记，无实例/配置 |
| F17 | 30 秒上限、指纹校验、失败保留清单、`List` 报错 |
| F18 | `inflight` 合并、`CacheStats` 计数 |
| F19 | `Source` 接口；`Resolve` 只依赖“按 pin 取正文” |
| N1–N11 | 状态仅目录；无新增模型调用；无迁移；fail-fast/降级；评测可选；路径校验；单文件读取；缓存上限；键含哈希；singleflight；无 Redis |

## 实施前置

- 无新增密钥或部署组件；不引入 Redis。
- 触发评测需要：`LLM_BASE_URL`、`LLM_API_KEY`、`LLM_MODEL`（复用现有模型）。缺少时命令报错并列出变量。
- 完整 Web 研究端到端仍需要现有 `AMAP_*`、酒店/攻略插件配置；单元测试和触发评测的 noop 工具不需要这些。
- 生产示例技能从 1.0.0 升到 1.0.1 后需要重新部署；旧 1.0.0 目录保留。

## 风险与处理

- **真实模型评测不稳定**：用例使用语义明确的 fixture 描述和输入，命令输出实际加载集合；失败即非零退出，便于人工判断。评测不进入默认 CI。
- **30 秒刷新与请求并发**：`refreshMu` 串行刷新，等待者读同一结果；刷新失败有 30 秒退避，避免故障期间反复扫目录。
- **预算收紧影响存量技能**：当前生产技能描述很短；新增 1.0.1 仅改描述。旧 pin 正文仍可解析。
- **L2 保留旧版本**：内存上限固定，旧 pin 只在被访问时回填；不会因版本数量增长而无限占用。
- **模板与预算漂移**：统一使用 `CatalogLine`，模板不再自行拼接格式。
