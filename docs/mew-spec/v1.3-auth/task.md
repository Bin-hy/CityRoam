# TripPilot v1.3 用户区分与 OIDC 登录 Tasks

状态：待审批（task）
日期：2026-11-20
输入：[spec.md](spec.md) · [plan.md](plan.md)

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 新建 | `migrations/0003_auth.sql` | users、sessions、auth_requests、trips.owner_id |
| 修改 | `sqlc.yaml` | 纳入 0003 schema |
| 新建 | `internal/repo/queries/users.sql` | 用户 upsert/查询 |
| 新建 | `internal/repo/queries/sessions.sql` | 会话增删查、过期清理 |
| 新建 | `internal/repo/queries/auth_requests.sql` | 登录 state 一次性存储 |
| 修改 | `internal/repo/queries/trips.sql` | 写入 owner_id、按 owner 列表 |
| 新建 | `internal/repo/auth.go` | Store 上的用户/会话/登录请求包装 |
| 修改 | `internal/repo/models.go` 等生成文件 | sqlc 重新生成 |
| 修改 | `internal/config/config.go` | AUTH/OIDC/ADMIN/CORS 配置 |
| 修改 | `internal/config/config_test.go` | 配置解析与必填校验 |
| 新建 | `internal/auth/principal.go` | Principal 与 context |
| 新建 | `internal/auth/roles.go` | 白名单角色解析 |
| 新建 | `internal/auth/session.go` | Cookie、token 哈希、会话读写 |
| 新建 | `internal/auth/service.go` | Service、Config、New、清理 |
| 新建 | `internal/auth/oidc.go` | discovery、登录、回调、token 校验 |
| 新建 | `internal/auth/middleware.go` | Middleware、AdminOrToken |
| 新建 | `internal/auth/*_test.go` | 单元与假 IdP 集成测试 |
| 新建 | `internal/api/auth.go` | me/login/callback/logout 转发 |
| 新建 | `internal/api/authz.go` | TripOwner、RunOwner |
| 新建 | `internal/api/cors.go` | CORS、SameOrigin |
| 修改 | `internal/api/router.go` | 公开/受保护/eval 分组 |
| 修改 | `internal/api/middleware.go` | RateLimitBy |
| 修改 | `internal/api/trips.go` | owner 写入、列表、幂等冲突 |
| 修改 | `internal/api/errors.go` | 新错误码 |
| 新建/修改 | `internal/api/*_test.go` | 鉴权、归属、列表、CORS、限流测试 |
| 修改 | `cmd/api/main.go` | 构建 auth.Service、启动清理 |
| 修改 | `openapi.yaml` | auth、行程列表、401/403、Cookie 安全方案 |
| 修改 | `web/src/api/schema.d.ts` | 由 OpenAPI 重新生成 |
| 新建 | `web/src/api/auth.ts` | getMe/logout/loginURL |
| 修改 | `web/src/api/client.ts` | credentials: include |
| 新建 | `web/src/features/auth/useAuth.ts` | 认证状态查询 |
| 新建 | `web/src/features/auth/AuthGate.tsx` | 路由守卫 |
| 新建 | `web/src/features/auth/LoginPage.tsx` | 登录页 |
| 新建 | `web/src/components/AppHeader.tsx` | 用户名与退出 |
| 新建 | `web/src/features/trips/MyTripList.tsx` | 我的行程 |
| 修改 | `web/src/App.tsx` | 布局与路由守卫 |
| 修改 | `web/src/features/create/CreatePage.tsx` | 接入行程列表 |
| 修改 | `web/src/features/workspace/WorkspacePage.tsx` | 401/404 处理 |
| 修改 | `web/src/hooks/useRunStream.ts` | SSE 带凭据 |
| 修改 | `web/src/features/eval/EvalDashboard.tsx` | admin 免 token |
| 修改 | `deploy/docker-compose.yml` | 透传新环境变量 |
| 修改 | `.env.example`、`docs/环境变量配置.md`、`README.md` | 配置与使用文档 |
| 修改 | `.gitignore` | 忽略 `.yufu-onboarding/` |

## T1: 新增认证数据库迁移

**文件：** `migrations/0003_auth.sql`、`sqlc.yaml`
**依赖：** 无
**步骤：**
1. 按 plan 的 Up 段创建 `users`、`sessions`、`auth_requests`，给 `trips` 增加可空 `owner_id` 与外键。
2. 创建 `sessions_expires_idx`、`sessions_user_idx`、`auth_requests_expires_idx`、`trips_owner_created_idx`。
3. Down 段按依赖逆序删除索引、列、表。
4. 在 `sqlc.yaml` 的 `schema` 列表追加 `migrations/0003_auth.sql`。

**验证：** 启动本地 Postgres 后执行
`go run github.com/pressly/goose/v3/cmd/goose@v3.24.3 -dir migrations postgres "$DATABASE_URL" up`，
再 `down` 一次、`up` 一次；`psql "$DATABASE_URL" -c '\d users'` 能看到唯一约束和角色检查。

## T2: 新增用户/会话/登录请求查询

**文件：** `internal/repo/queries/users.sql`、`sessions.sql`、`auth_requests.sql`
**依赖：** T1
**步骤：**
1. `UpsertUser`、`GetUserByIssuerSubject` 按 plan 实现；upsert 更新资料、角色与 `last_login_at`。
2. `CreateSession`、`GetSessionUser`、`DeleteSession`、`DeleteExpiredSessions` 按 plan 实现；联表查询使用 `session_id`、`user_id` 等不重复别名。
3. `CreateAuthRequest`、`ConsumeAuthRequest`（`DELETE ... RETURNING`）、`DeleteExpiredAuthRequests` 实现。

**验证：** 安装或复用 sqlc 后运行 `sqlc generate`，退出码 0；`internal/repo` 下出现 `users.sql.go`、`sessions.sql.go`、`auth_requests.sql.go`。

## T3: 生成 repo 代码并补 Store 包装

**文件：** `internal/repo/*.sql.go`、`internal/repo/models.go`、`internal/repo/auth.go`
**依赖：** T2
**步骤：**
1. 若本机无 sqlc，安装固定版本：`go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0`，确认 `sqlc version`。
2. 运行 `sqlc generate` 生成用户、会话、登录请求代码。
3. 在 `internal/repo/auth.go` 为 `*Store` 添加 `UpsertUser`、`GetUserByIssuerSubject`、`CreateSession`、`GetSessionUser`、`DeleteSession`、`DeleteExpiredSessions`、`CreateAuthRequest`、`ConsumeAuthRequest`、`DeleteExpiredAuthRequests` 包装方法。
4. 检查生成的 `User`、`Session`、`AuthRequest` 字段与 plan 一致。

**验证：** `go build ./internal/repo/...` 通过；`grep -n "type User struct" internal/repo/models.go` 可见角色与资料字段。

## T4: 行程归属与列表查询

**文件：** `internal/repo/queries/trips.sql`、生成文件
**依赖：** T1、T2
**步骤：**
1. `CreateTrip` 的 INSERT 增加 `owner_id` 列与参数。
2. 按 plan 添加 `ListTripsByOwner`、`ListAnonymousTrips`，使用 LATERAL 取最近 run 与最新 version。
3. 重新运行 `sqlc generate`。
4. 修正 `repo.Trip`、`CreateTripParams` 使用处，确保未设置 owner 时为零值。

**验证：** `go build ./...` 通过；`grep -n "OwnerID" internal/repo/trips.sql.go` 可见新参数与字段。

## T5: 扩展配置加载

**文件：** `internal/config/config.go`
**依赖：** 无
**步骤：**
1. 增加 `AuthEnabled`、OIDC 字段、`AuthWebBaseURL`、`AuthSessionTTL`、`AuthCookieSecure`、`AdminEmails/Subs`、`CORSAllowedOrigins`。
2. 实现逗号分隔列表解析、`AUTH_SESSION_TTL` 时长解析、`AUTH_COOKIE_SECURE` 仅接受 `auto/true/false`。
3. 认证启用时校验 `OIDC_ISSUER/CLIENT_ID/CLIENT_SECRET/REDIRECT_URI` 非空；解析 `AUTH_WEB_BASE_URL`、`OIDC_REDIRECT_URI` 为合法 URL。
4. `CORS_ALLOWED_ORIGINS` 中出现 `*` 时返回错误。

**验证：** `go test ./internal/config/...` 通过。

## T6: 配置测试

**文件：** `internal/config/config_test.go`
**依赖：** T5
**步骤：**
1. 覆盖认证关闭时缺失全部 OIDC 配置仍可加载。
2. 覆盖启用时逐个缺失必填变量返回包含变量名的错误。
3. 覆盖 scopes 默认值、csv 列表去空格、cookie secure 非法值、CORS 通配符拒绝。

**验证：** `go test ./internal/config/... -run Auth -v` 全部通过。

## T7: Principal 与角色解析

**文件：** `internal/auth/principal.go`、`internal/auth/roles.go`、`internal/auth/roles_test.go`
**依赖：** 无
**步骤：**
1. 定义 `Principal`、私有 context key、`WithPrincipal`、`FromContext`。
2. 实现 `resolveRole(email *string, emailVerified bool, sub string, admins []string)`。
3. 邮箱白名单大小写不敏感且要求已验证；sub 白名单精确匹配；无匹配返回 `user`。
4. 测试未验证邮箱、邮箱大小写、sub 命中、全部未命中。

**验证：** `go test ./internal/auth/... -run Role` 通过。

## T8: 会话 Cookie 与哈希

**文件：** `internal/auth/session.go`、`internal/auth/session_test.go`
**依赖：** T7
**步骤：**
1. 实现 32 字节随机 token、base64url 编码、SHA-256 十六进制哈希。
2. 实现会话 Cookie 构造：名 `trippilot_session`、Path `/`、HttpOnly、SameSite=Lax、MaxAge 取 TTL。
3. 实现 `secure` 判定：配置为 `true/false` 时优先，`auto` 时看 `r.TLS` 或 `X-Forwarded-Proto=https`。
4. 实现绑定 Cookie 构造与清除。
5. 测试 token 不重复、哈希稳定、Cookie 标志正确、auto 在代理头下为 Secure。

**验证：** `go test ./internal/auth/... -run Session` 通过。

## T9: auth.Service 与 OIDC 发现

**文件：** `internal/auth/service.go`、`internal/auth/oidc.go`
**依赖：** T7、T8
**步骤：**
1. 添加依赖：`go get github.com/coreos/go-oidc/v3/oidc`，确认 `golang.org/x/oauth2` 变为直接依赖。
2. 定义 `Service`、`Config`、`Store`；`New` 在关闭时返回可用实例且不触网。
3. 启用时用 10 秒上下文执行 `oidc.NewProvider`，构造 `oauth2.Config` 与 ID Token verifier。
4. `Audience` 非空时用 `oidc.NewRemoteKeySet` + `oidc.NewVerifier` 构造 access token verifier。
5. 从 discovery 解析 `end_session_endpoint`；失败返回带 issuer 的错误。

**验证：** `go build ./internal/auth/...` 通过；用 httptest discovery 的最小测试确认启用时能构造 Service。

## T10: 登录入口

**文件：** `internal/auth/oidc.go`
**依赖：** T9
**步骤：**
1. 实现 `return_to` 校验：必须以 `/` 开头且不以 `//` 开头，拒绝绝对 URL、反斜杠、控制字符。
2. 生成 `state`、`nonce`、PKCE verifier；计算 S256 challenge。
3. 写入 `auth_requests`（10 分钟过期），设置绑定 Cookie。
4. 302 到 IdP 授权 URL；关闭时返回 404 `auth_disabled`；写库失败返回 500。
5. 测试合法/非法 return_to、state 长度、auth_request 参数。

**验证：** `go test ./internal/auth/... -run Login` 通过。

## T11: 回调与用户建立

**文件：** `internal/auth/oidc.go`
**依赖：** T10
**步骤：**
1. 校验绑定 Cookie 与查询 `state` 一致；不一致返回 `login_state_invalid`。
2. 原子消费 `auth_requests`；重复/过期回调返回明确错误。
3. 兑换 code，校验 ID Token（issuer、client_id、nonce、exp）；配置 audience 时校验 access token。
4. 读取 UserInfo，失败时回退 ID Token claims；要求 subject 非空。
5. 查询旧用户角色，按白名单解析新角色，upsert 用户。
6. 创建会话、设置 Cookie、清绑定 Cookie；302 到 `WebBaseURL + return_to`。
7. 失败时清绑定 Cookie，跳登录页带错误码；日志记录细节但不含 token。

**验证：** 假 IdP 测试覆盖成功、state 重放、nonce 不匹配、错误 audience，全部通过。

## T12: 当前用户与退出

**文件：** `internal/auth/service.go`、`internal/auth/oidc.go`
**依赖：** T11
**步骤：**
1. `Me` 读取会话；返回 `enabled/authenticated/provider_name/user`，设置 `Cache-Control: no-store`；无会话返回 200 和 `authenticated=false`。
2. `Logout` 删除服务端会话、清 Cookie；配置了 `end_session_endpoint` 时返回 200 `{"redirect_url": ...}`，否则 204。
3. 关闭时 `Me` 仍返回 `enabled=false`；`Logout` 仍清 Cookie。
4. 测试 HttpOnly Cookie、退出后旧 Cookie 失效、关闭态响应。

**验证：** `go test ./internal/auth/... -run "Me|Logout"` 通过。

## T13: 认证中间件与管理员校验

**文件：** `internal/auth/middleware.go`
**依赖：** T9、T12
**步骤：**
1. `Middleware`：关闭时直接放行；开启时读 Cookie、查会话用户，失败返回 401 `unauthorized`；成功写入 Principal。
2. `AdminOrToken`：先做常量时间 token 比较；再查会话；无凭据 401，普通用户 403，admin 放行。
3. 会话查询失败只记录内部错误，不向响应泄露细节。
4. 测试关闭放行、无 Cookie 401、伪造 Cookie 401、user 403、admin 200、token 200。

**验证：** `go test ./internal/auth/... -run Middleware` 通过。

## T14: 过期数据清理

**文件：** `internal/auth/service.go`、`internal/auth/cleanup.go`
**依赖：** T9
**步骤：**
1. `Cleanup` 调用删除过期 sessions 与 auth_requests，记录删除数量。
2. `StartCleanup` 启动 ticker，默认 6 小时，响应 context 取消。
3. 测试使用短 ticker 或直接调用 `Cleanup`，确认过期行被删除、未过期行保留。

**验证：** `go test ./internal/auth/... -run Cleanup` 通过。

## T15: 假 IdP 集成测试

**文件：** `internal/auth/oidc_test.go`
**依赖：** T10–T12
**步骤：**
1. 用 httptest 提供 discovery、JWKS、token、UserInfo；生成带 nonce 的签名 ID Token。
2. 覆盖完整登录：Login 产出 state → Callback 建立会话 → Me 返回用户。
3. 覆盖失败路径：错误 state、Cookie 不匹配、过期 auth_request、nonce 不匹配、错误 audience。
4. 确认响应与日志中不出现 client secret、access token、ID Token 原文。

**验证：** `go test ./internal/auth/... -v` 全部通过。

## T16: API 认证窄接口与路由分组

**文件：** `internal/api/router.go`、`internal/api/auth.go`
**依赖：** T15
**步骤：**
1. 在 `api` 定义 `Authenticator` 接口，字段与 plan 一致；`*auth.Service` 实现。
2. `NewRouter` 在 `Deps.Auth == nil` 时注入内部 `disabledAuth`，保持现有测试兼容。
3. 路由拆为公开组、受保护组、eval 组；公开组挂 auth 四接口、meta、share。
4. `/trips/{tripID}/*` 加占位 `TripOwner`，`/runs/{runID}/*` 加占位 `RunOwner`（T18 实现）。
5. 保留 `/healthz`、`/readyz`。

**验证：** `go build ./internal/api/...` 通过；现有 `go test ./internal/api/...` 不出现路由 404。

## T17: API 认证处理器

**文件：** `internal/api/auth.go`
**依赖：** T16
**步骤：**
1. 四个 handler 直接委托 `Deps.Auth`；不重复实现 OIDC。
2. `Deps.Auth == nil` 时由 `disabledAuth` 返回关闭态行为。
3. 确保 `/api/auth/me` 不参与认证中间件。

**验证：** `go test ./internal/api/... -run Auth` 通过（新增测试在 T22）。

## T18: 行程/运行归属中间件

**文件：** `internal/api/authz.go`
**依赖：** T16、T4
**步骤：**
1. `TripOwner`：读取 `{tripID}`，认证关闭时放行；开启时 `GetTrip`，`ErrNoRows` 或 owner 不匹配返回 404。
2. `RunOwner`：读取 `{runID}`，`GetRun` 后 `GetTrip`，同样判断；不存在统一 404。
3. 不把内部错误或 owner 信息写入响应。
4. 测试：无会话 401、他人行程 404、本人 200、删除/分享/事件同样受限。

**验证：** `go test ./internal/api/... -run Owner` 通过。

## T19: 创建归属、幂等冲突与行程列表

**文件：** `internal/api/trips.go`
**依赖：** T18
**步骤：**
1. 创建时从 Principal 取用户 ID 写入 `CreateTripParams.OwnerID`；关闭时 `nil`。
2. 幂等命中时比较 `trip.OwnerID` 与当前用户；不一致返回 409 `request_id_conflict`，不返回 trip ID。
3. 新增 `GET /api/trips`：解析 `limit/offset`，认证开启调用 `ListTripsByOwner`，关闭调用 `ListAnonymousTrips`。
4. 响应只包含摘要字段，不暴露 owner 或内部请求 ID。
5. 测试创建归属、跨用户幂等冲突、列表分页与空列表。

**验证：** `go test ./internal/api/... -run "Trips|List"` 通过。

## T20: 按用户限流

**文件：** `internal/api/middleware.go`
**依赖：** T16
**步骤：**
1. 新增 `RateLimitBy(limit, now, keyFn)`，复用现有窗口与清理逻辑；`RateLimit` 委托为 IP key。
2. keyFn 优先取 `auth.FromContext` 的用户 ID，回退 `clientIP`。
3. 路由中仅变更类接口挂限流，保持现有覆盖范围。
4. 测试两个 Principal 各自独立计数、无 Principal 时共享 IP 计数。

**验证：** `go test ./internal/api/... -run Rate` 通过。

## T21: CORS 与同源校验

**文件：** `internal/api/cors.go`
**依赖：** T16
**步骤：**
1. 实现 `CORS(allowedOrigins)`：来源精确匹配才回显，设置 `Allow-Credentials`、`Vary: Origin`；`OPTIONS` 返回 204。
2. 实现 `SameOrigin(allowedOrigins)`：认证开启时对 POST/PUT/PATCH/DELETE 检查 Origin/Referer；允许同 host 或白名单；无头放行。
3. 白名单为空时 CORS 不生效；不允许 `*`。
4. 测试允许来源、拒绝来源、预检、无 Origin 的脚本请求。

**验证：** `go test ./internal/api/... -run "CORS|Origin"` 通过。

## T22: API 测试补齐

**文件：** `internal/api/auth_test.go`、`authz_test.go`、`trips_test.go`、`eval_test.go`
**依赖：** T17–T21
**步骤：**
1. 假 `Authenticator` 可在测试中注入 Principal 或返回 401。
2. 覆盖 A 创建、B 查询 404、B 创建分享 404、B 访问 run events 404。
3. 覆盖列表只返回本人、分页、关闭态匿名列表。
4. 覆盖 eval：admin 通过、普通用户 403、`ADMIN_TOKEN` 通过、无凭据 401。
5. 覆盖认证关闭时现有行程流程回归。

**验证：** `go test ./internal/api/...` 全部通过。

## T23: 主程序接线

**文件：** `cmd/api/main.go`
**依赖：** T15、T22
**步骤：**
1. 由 `config.Config` 构造 `auth.Config`，调用 `auth.New(startupCtx, ...)`。
2. 启用且 discovery/配置失败时返回错误，进程退出。
3. 以进程 context 启动 `StartCleanup`；退出时取消。
4. 向 `api.Deps` 注入 `Auth` 与 `CORSAllowedOrigins`。
5. 确认认证关闭时启动不触网、旧行为不变。

**验证：** `AUTH_ENABLED=false go run ./cmd/api` 能启动并通过 `/healthz`；启用但缺 issuer 时启动报错。

## T24: Go 质量门

**文件：** 无
**依赖：** T23
**步骤：**
1. 运行 `gofmt -w` 涉及文件。
2. 运行构建、vet、全量测试。

**验证：** `go build ./...`、`go vet ./...`、`go test -race ./...` 全部通过。

## T25: 更新 OpenAPI

**文件：** `openapi.yaml`、`web/src/api/schema.d.ts`
**依赖：** T16、T19
**步骤：**
1. 增加 `/api/auth/me`、`/api/auth/login`、`/api/auth/callback`、`/api/auth/logout`、`GET /api/trips`。
2. 增加 `AuthMe`、`AuthUser`、`TripSummary`、`TripList` schema。
3. 为受保护接口标注 Cookie 安全方案与 401/403 响应。
4. 在 `web` 目录运行 `npm run types` 重新生成 `schema.d.ts`。

**验证：** `npm --prefix web run types` 通过；生成文件包含 `AuthMe`、`TripList` 类型。

## T26: 前端 API 客户端

**文件：** `web/src/api/auth.ts`、`web/src/api/client.ts`
**依赖：** T25
**步骤：**
1. `apiRequest` 的 fetch 增加 `credentials: "include"`。
2. 新增 `getMe()`、`logout()`（POST，无 body）、`loginURL(returnTo)`。
3. `loginURL` 使用 `API_BASE` 与 `encodeURIComponent`。

**验证：** `npm --prefix web run build` 通过（类型阶段）。

## T27: 认证状态、守卫与登录页

**文件：** `web/src/features/auth/useAuth.ts`、`AuthGate.tsx`、`LoginPage.tsx`
**依赖：** T26
**步骤：**
1. `useAuth` 查询 `/api/auth/me`，`staleTime` 60 秒，失败不重试。
2. `AuthGate`：加载中显示占位；`enabled && !authenticated` 时跳 `/login` 并带上当前路径。
3. `LoginPage`：展示 provider 名称、登录按钮、错误码文案；已登录时直接回跳。
4. 错误码映射：`auth_failed`、`login_state_invalid`、`oidc_error` 等。

**验证：** `npm --prefix web test -- --run -- src/features/auth` 通过（测试在 T32 补齐）。

## T28: 布局、顶栏与路由

**文件：** `web/src/App.tsx`、`web/src/components/AppHeader.tsx`
**依赖：** T27
**步骤：**
1. 新增 `AppLayout`，认证开启且已登录时展示用户名与退出按钮；退出调用 `logout()` 并清空 QueryClient。
2. `/` 与 `/trips/:tripId` 包 `AuthGate`；`/login`、`/share/:slug`、`/eval` 保持独立。
3. 认证关闭时不渲染登录相关元素。
4. 保留现有 lazy loading。

**验证：** `npm --prefix web run build` 通过。

## T29: 我的行程与首页接入

**文件：** `web/src/features/trips/MyTripList.tsx`、`web/src/features/create/CreatePage.tsx`
**依赖：** T27
**步骤：**
1. 新增行程列表查询 `GET /api/trips?limit=20`，只在 `enabled && authenticated` 时启用。
2. 展示城市、天数、状态、创建时间与最新版本；点击跳工作台。
3. 提供加载、错误重试和空状态。
4. 接入 CreatePage 表单下方；认证关闭时不渲染。

**验证：** `npm --prefix web test -- --run -- src/features/trips` 通过（测试在 T32 补齐）。

## T30: 工作台与 SSE 凭据

**文件：** `web/src/features/workspace/WorkspacePage.tsx`、`web/src/hooks/useRunStream.ts`
**依赖：** T26
**步骤：**
1. `fetchEventSource` 增加 `credentials: "include"`。
2. 工作台请求返回 401 时展示“会话已过期”并提供登录链接；404 保持“行程不存在”。
3. 不影响认证关闭时的现有错误展示。

**验证：** `npm --prefix web run build` 通过；现有 Playwright 主流程在认证关闭时通过。

## T31: eval 看板管理员接入

**文件：** `web/src/features/eval/EvalDashboard.tsx`
**依赖：** T27
**步骤：**
1. 读取 `useAuth`；admin 登录时跳过 token 输入，直接请求 eval。
2. 已登录非 admin 显示“需要管理员角色”，不发起请求。
3. 未登录或认证关闭时保留现有 token 输入流程。
4. 保持 401 时清 token 的现有行为。

**验证：** `npm --prefix web test -- --run -- src/features/eval` 通过。

## T32: 前端测试补齐

**文件：** `web/src/features/auth/*.test.tsx`、`web/src/features/trips/MyTripList.test.tsx`、`web/src/components/AppHeader.test.tsx`、`web/src/features/eval/EvalDashboard.test.tsx`、`web/src/api/client.test.ts`
**依赖：** T27–T31
**步骤：**
1. 覆盖 AuthGate 跳转与回跳、登录页错误文案、登录按钮 URL。
2. 覆盖行程列表加载/空/错误、点击跳转。
3. 覆盖顶栏用户名与退出调用。
4. 覆盖 eval 的 admin 免 token、普通用户 403、未登录 token 输入。
5. 断言 API 请求带 `credentials: include`。

**验证：** `npm --prefix web test -- --run` 全部通过。

## T33: 前端质量门

**文件：** 无
**依赖：** T32
**步骤：**
1. 运行类型生成与构建。
2. 运行全部前端单元测试。

**验证：** `npm --prefix web run types`、`npm --prefix web run build`、`npm --prefix web test -- --run` 全部通过。

## T34: 部署与环境变量

**文件：** `deploy/docker-compose.yml`、`.env.example`
**依赖：** T5
**步骤：**
1. compose 的 `api.environment` 增加 `AUTH_ENABLED`、`OIDC_*`、`AUTH_WEB_BASE_URL`、`AUTH_SESSION_TTL`、`AUTH_COOKIE_SECURE`、`ADMIN_EMAILS`、`ADMIN_SUBS`、`CORS_ALLOWED_ORIGINS`。
2. `.env.example` 增加同组变量与注释，secret 留空。
3. `.gitignore` 增加 `.yufu-onboarding/`。

**验证：** `docker compose -f deploy/docker-compose.yml config` 通过。

## T35: 文档更新

**文件：** `docs/环境变量配置.md`、`README.md`
**依赖：** T34
**步骤：**
1. 环境变量文档增加认证分组、启用步骤、yufu 注册参数、CORS/分离部署说明、失败表现。
2. README 增加“用户与登录”章节：开关、登录/退出、我的行程、admin 角色、分享仍公开。
3. 明确 API Key 仍为服务端全局，不做用户维度。

**验证：** 文档中的变量名与 `config.go` 完全一致（用 `grep` 对照）。

## T36: 手工验收与回归

**文件：** 无
**依赖：** T24、T33、T34
**步骤：**
1. 认证关闭：跑现有 Go/前端测试、Playwright 主流程，确认行为不变。
2. 启动本地 yufu，注册 Web 应用，配置回调为 `/api/auth/callback`。
3. 开启认证，完成真实登录、退出、再次登录；检查 Cookie HttpOnly、浏览器存储无 IdP token。
4. 用两个 yufu 用户验证行程隔离与列表；用白名单用户验证 eval 管理员。
5. 记录结果，异常回到对应任务修复。

**验证：** 手工步骤全部成功；`npm --prefix web run test:e2e` 在认证关闭时通过。

## T37: 最终质量门

**文件：** 无
**依赖：** T36
**步骤：**
1. 后端：`go build ./...`、`go vet ./...`、`go test -race ./...`。
2. 前端：`npm --prefix web run types`、`npm --prefix web run build`、`npm --prefix web test -- --run`。
3. 部署：`docker compose -f deploy/docker-compose.yml config`。
4. 迁移：本地 Postgres 执行 up/down/up。

**验证：** 以上命令全部成功，输出作为验收证据。

## 执行顺序

```
T1 → T2 → T3 → T4
T5 → T6
T7 → T8 → T9 → T10 → T11 → T12 → T13
                         ↘ T14
T10..T14 → T15 → T16 → T17 → T18 → T19 → T20 → T21 → T22 → T23 → T24
T16, T19 → T25 → T26 → T27 → T28 → T29 → T30 → T31 → T32 → T33
T5 → T34 → T35
T24, T33, T34 → T36 → T37
```

- T1–T4 是数据层，必须先完成。
- T5–T15 是认证内核，可与 T16 的接口骨架并行设计，但合并前需完成。
- T25–T33 依赖 OpenAPI 类型，T34–T35 可与前端并行。