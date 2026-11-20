# TripPilot v1.3 用户区分与 OIDC 登录 Plan

状态：待审批（plan）
日期：2026-11-20
输入：[spec.md](spec.md)

## 架构概览

```
浏览器 ── Cookie(会话) ──> Caddy/同源 ──> Go API
                                             ├─ internal/api      路由、CORS/CSRF、归属中间件
                                             ├─ internal/auth     OIDC、会话、Principal、RBAC
                                             ├─ internal/repo      users/sessions/auth_requests/trips.owner_id
                                             └─ Postgres
浏览器 ── 302 ──> yufu / 任意 OIDC IdP（认证）
```

- 新增 `internal/auth`：唯一的认证、会话和 RBAC 边界，不依赖 HTTP 业务代码。
- `internal/api` 只做路由、资源归属校验、CORS/CSRF 和错误映射，不放 OIDC 细节。
- 数据层沿用 pgx + sqlc + goose，不引入 Redis、JWT 会话或新服务；会话存 Postgres。
- 前端新增 `useAuth`、登录页、路由守卫、顶栏和「我的行程」；工作台与分享页尽量少改。

### 登录与会话链路

1. 浏览器访问 `GET /api/auth/login?return_to=/trips/xxx`。
2. Go 校验 `return_to` 只能是站内相对路径；生成 `state`、`nonce`、PKCE verifier，写入 `auth_requests`（一次性），并设置 10 分钟有效的 HttpOnly 绑定 Cookie（值为 `state`）。
3. 302 到 IdP 授权端点。
4. 回调 `GET /api/auth/callback?code&state`：要求绑定 Cookie 与 `state` 一致；原子删除并读取 `auth_requests`（防重放）；兑换 code；校验 ID Token（issuer、aud=client_id、nonce、exp）；配置 `AUDIENCE` 时再校验 JWT access token；读取 UserInfo（失败时回退 ID Token claims）。
5. 按 `issuer+subject` upsert 本地用户，按白名单同步角色；生成随机会话 token，数据库只存 SHA-256，浏览器拿原始值。
6. 302 回 `AUTH_WEB_BASE_URL + return_to`（默认同源）；失败跳登录页并带稳定错误码。
7. 退出：删除服务端会话、清 Cookie；配置了 IdP `end_session_endpoint` 时返回跳转地址。

### 请求鉴权链路

- 认证开启时，行程/运行路由先经过会话中间件：无有效会话返回 401 `unauthorized`。
- 行程路由再经过 `TripOwner`：加载行程，非所有者或不存在统一 404；admin 不绕过归属。
- 运行路由经过 `RunOwner`：加载 run → 所属 trip → 校验所有者，覆盖 resume/cancel/events。
- eval 路由使用 `AdminOrToken`：本地 `admin` 会话或 `ADMIN_TOKEN` 任一通过；普通用户返回 403。
- 公开路由：`/healthz`、`/readyz`、`/api/meta`、`/api/share/{slug}`、`/api/auth/*`。
- 认证关闭时会话/归属中间件透明放行，限流回退到现有 IP 口径。

### 开关行为

| 能力 | `AUTH_ENABLED=false` | `AUTH_ENABLED=true` |
|---|---|---|
| 创建/查看行程 | 无需登录，行为不变 | 必须登录且只能访问自己的 |
| 我的行程列表 | 前端不展示 | 首页展示 |
| eval | 只认 `ADMIN_TOKEN` | admin 会话或 `ADMIN_TOKEN` |
| 分享页 | 公开 | 公开 |
| API Key | 服务端全局 | 服务端全局 |

## 核心数据结构

### 数据库迁移 `migrations/0003_auth.sql`

```sql
-- +goose Up
CREATE TABLE users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  issuer text NOT NULL,
  subject text NOT NULL,
  email text,
  email_verified boolean NOT NULL DEFAULT false,
  name text,
  avatar_url text,
  role text NOT NULL DEFAULT 'user' CHECK (role IN ('user','admin')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  last_login_at timestamptz,
  UNIQUE (issuer, subject)
);

CREATE TABLE sessions (
  id text PRIMARY KEY,              -- hex(sha256(cookie token))
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  user_agent text,
  ip text
);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);
CREATE INDEX sessions_user_idx ON sessions (user_id);

CREATE TABLE auth_requests (
  state text PRIMARY KEY,
  nonce text NOT NULL,
  code_verifier text NOT NULL,
  return_to text NOT NULL,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX auth_requests_expires_idx ON auth_requests (expires_at);

ALTER TABLE trips ADD COLUMN owner_id uuid REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX trips_owner_created_idx
  ON trips (owner_id, created_at DESC) WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS trips_owner_created_idx;
ALTER TABLE trips DROP COLUMN IF EXISTS owner_id;
DROP TABLE IF EXISTS auth_requests;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
```

- `sqlc.yaml` 的 schema 列表追加 `migrations/0003_auth.sql`；`owner_id` 经现有 uuid 可空覆盖生成 `*string`。
- 不建邮箱索引：本期没有按邮箱查用户，邮箱只用于展示与白名单匹配。
- 现有 `trips` 行保持 `owner_id = NULL`，满足 F11。

### `internal/auth` 类型

```go
type Config struct {
    Enabled          bool
    CookieSecure     string // "auto" | "true" | "false"
    Issuer           string
    ClientID         string
    ClientSecret     string
    RedirectURI      string
    Scopes           []string
    Audience         string
    ProviderName     string
    PostLogoutRedirectURI string
    WebBaseURL       string
    SessionTTL       time.Duration
    AdminEmails      []string
    AdminSubs        []string
}

type Principal struct {
    UserID  string
    Issuer  string
    Subject string
    Role    string // "user" | "admin"
    Email   *string
    Name    *string
}
```

```go
type Store interface {
    UpsertUser(ctx context.Context, arg repo.UpsertUserParams) (repo.User, error)
    GetUserByIssuerSubject(ctx context.Context, issuer, subject string) (repo.User, error)
    CreateSession(ctx context.Context, arg repo.CreateSessionParams) error
    GetSessionUser(ctx context.Context, id string) (repo.Session, repo.User, error)
    DeleteSession(ctx context.Context, id string) error
    DeleteExpiredSessions(ctx context.Context) (int64, error)
    CreateAuthRequest(ctx context.Context, arg repo.CreateAuthRequestParams) error
    ConsumeAuthRequest(ctx context.Context, state string) (repo.AuthRequest, error)
    DeleteExpiredAuthRequests(ctx context.Context) (int64, error)
}
```

### `internal/repo` 类型变化

- `repo.User`：`ID, Issuer, Subject, Role string`，`Email *string`，`EmailVerified bool`，`Name, AvatarURL *string`，时间字段。
- `repo.Session`：`ID, UserID string`，`ExpiresAt time.Time`，`UserAgent, IP *string`。
- `repo.AuthRequest`：`State, Nonce, CodeVerifier, ReturnTo string`，`ExpiresAt time.Time`。
- `repo.Trip` 增加 `OwnerID *string`。
- `CreateTripParams` 增加 `OwnerID *string`。
- `handleCreateTrip` 将当前用户 ID 写入 `owner_id`；认证关闭时为 `nil`。

### sqlc 查询

```sql
-- users.sql
-- name: UpsertUser :one
INSERT INTO users (issuer, subject, email, email_verified, name, avatar_url, role)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (issuer, subject) DO UPDATE SET
  email = EXCLUDED.email,
  email_verified = EXCLUDED.email_verified,
  name = EXCLUDED.name,
  avatar_url = EXCLUDED.avatar_url,
  role = EXCLUDED.role,
  updated_at = now(),
  last_login_at = now()
RETURNING *;

-- name: GetUserByIssuerSubject :one
SELECT * FROM users WHERE issuer = $1 AND subject = $2;

-- sessions.sql
-- name: CreateSession :exec
INSERT INTO sessions (id, user_id, expires_at, user_agent, ip)
VALUES ($1, $2, $3, $4, $5);

-- name: GetSessionUser :one
SELECT s.id AS session_id, u.id AS user_id, s.expires_at,
       s.created_at AS session_created_at,
       u.issuer, u.subject, u.email, u.email_verified,
       u.name, u.avatar_url, u.role, u.created_at AS user_created_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.id = $1 AND s.expires_at > now();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = $1;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE expires_at <= now();

-- auth_requests.sql
-- name: CreateAuthRequest :exec
INSERT INTO auth_requests (state, nonce, code_verifier, return_to, expires_at)
VALUES ($1, $2, $3, $4, $5);

-- name: ConsumeAuthRequest :one
DELETE FROM auth_requests
WHERE state = $1 AND expires_at > now()
RETURNING *;

-- name: DeleteExpiredAuthRequests :execrows
DELETE FROM auth_requests WHERE expires_at <= now();
```

```sql
-- trips.sql 追加
-- name: ListTripsByOwner :many
SELECT t.*, r.id AS latest_run_id, r.status AS latest_run_status,
       r.stage AS latest_run_stage, v.seq AS latest_version_seq,
       v.status AS latest_version_status, v.created_at AS latest_version_created_at
FROM trips t
LEFT JOIN LATERAL (
  SELECT id, status, stage FROM runs
  WHERE trip_id = t.id ORDER BY created_at DESC LIMIT 1
) r ON true
LEFT JOIN LATERAL (
  SELECT seq, status, created_at FROM plan_versions
  WHERE trip_id = t.id ORDER BY seq DESC LIMIT 1
) v ON true
WHERE t.owner_id = $1 AND t.deleted_at IS NULL
ORDER BY t.created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListAnonymousTrips :many
-- 同上，条件改为 t.owner_id IS NULL
```

## 模块设计

### `internal/auth`

**职责：** OIDC 发现与登录回调、state/nonce/PKCE、会话签发与校验、角色解析、Principal 上下文、认证中间件、过期数据清理。

**对外接口：**

```go
func New(ctx context.Context, cfg Config, store Store, logger *slog.Logger) (*Service, error)
func (s *Service) Enabled() bool
func (s *Service) Middleware(next http.Handler) http.Handler
func (s *Service) AdminOrToken(token string) func(http.Handler) http.Handler
func (s *Service) Me(w http.ResponseWriter, r *http.Request)
func (s *Service) Login(w http.ResponseWriter, r *http.Request)
func (s *Service) Callback(w http.ResponseWriter, r *http.Request)
func (s *Service) Logout(w http.ResponseWriter, r *http.Request)
func (s *Service) Cleanup(ctx context.Context) error
func (s *Service) StartCleanup(ctx context.Context, interval time.Duration)
func FromContext(ctx context.Context) (Principal, bool)
```

**关键约束：**

- `New` 在启用时完成 discovery，10 秒超时；失败返回包含 issuer 的错误，由 `main` 直接退出。
- ID Token 使用 `coreos/go-oidc` 校验；`Audience` 非空时用 `oidc.NewRemoteKeySet` + `oidc.NewVerifier` 校验 JWT access token（要求 access token 为 JWT；yufu 满足）。
- 会话 Cookie 原始 token 为 32 字节随机值的 base64url；数据库存 SHA-256 十六进制；Cookie 名 `trippilot_session`，`Path=/`，`HttpOnly`，`SameSite=Lax`；`Secure` 按 `AUTH_COOKIE_SECURE` 或请求 TLS/`X-Forwarded-Proto` 判定。
- 绑定 Cookie 名 `trippilot_oidc`，值为 `state`，10 分钟有效；回调要求一致，成功后清除。
- 关闭时：`Enabled=false`，中间件放行，`Login/Callback` 返回 404 `auth_disabled`，`Me` 返回 `enabled=false`，`Logout` 仍清 Cookie。
- 角色同步：`ADMIN_EMAILS` 仅对 `email_verified=true` 生效，`ADMIN_SUBS` 精确匹配；每次登录覆盖角色并记录 `from/to`；会话查询每次 join users，角色变更立即生效。
- 清理：启动时执行一次，之后每 6 小时清理过期 sessions 和 auth_requests；错误只记日志。

### `internal/api`

**依赖：** `Deps` 增加 `Auth Authenticator`、`AllowedOrigins []string`。

```go
type Authenticator interface {
    Enabled() bool
    Middleware(next http.Handler) http.Handler
    AdminOrToken(token string) func(http.Handler) http.Handler
    Me(http.ResponseWriter, *http.Request)
    Login(http.ResponseWriter, *http.Request)
    Callback(http.ResponseWriter, *http.Request)
    Logout(http.ResponseWriter, *http.Request)
}
```

- `NewRouter` 在 `Deps.Auth == nil` 时注入内部 `disabledAuth`，保证现有测试与认证关闭行为不变。
- 路由分组：
  - 公开组：auth 四个接口、`/api/meta`、`/api/share/{slug}`，按 IP 限流。
  - 受保护组：`Auth.Middleware`；`POST/GET /api/trips`；`/trips/{tripID}/*` 加 `TripOwner`；`/runs/{runID}/*` 加 `RunOwner`；仅变更类接口加限流，登录后按用户 key。
  - eval 组：`AdminOrToken`。
- `TripOwner`：认证关闭时放行；开启时加载行程，不存在或 `owner_id` 不匹配一律 404。`RunOwner` 先取 run 再取 trip 做同样判断。
- `handleCreateTrip`：写入 `owner_id`；幂等命中且归属不同返回 409 `request_id_conflict`，不返回他人 trip ID。
- `handleListTrips`：`limit` 默认 20、最大 100，`offset` 非负；认证关闭时查询 `owner_id IS NULL`。
- 限流：新增 `RateLimitBy(limit, now, keyFn)`，key 为 `user:<id>` 或 `ip:<IP>`；原 `RateLimit` 保留并委托。
- CORS：`CORS(allowedOrigins)` 仅在配置非空时生效，精确回显来源，`Allow-Credentials: true`，处理 `OPTIONS`；拒绝 `*`。
- CSRF：`SameOrigin(allowedOrigins)` 在认证开启时对 POST/PUT/PATCH/DELETE 检查 Origin/Referer；无该头时放行。
- 错误映射：非所有者 404（`not_found`）；未登录 401（`unauthorized`）；非 admin 403（`forbidden`）；新增 `auth_disabled`、`invalid_return_to`、`login_state_invalid`、`oidc_error`、`request_id_conflict`。

### `internal/config`

新增环境变量：

| 变量 | 默认 | 说明 |
|---|---|---|
| `AUTH_ENABLED` | `false` | 认证总开关 |
| `OIDC_ISSUER` | 无 | IdP issuer |
| `OIDC_CLIENT_ID` / `OIDC_CLIENT_SECRET` | 无 | 注册的 Web 客户端 |
| `OIDC_REDIRECT_URI` | 无 | 必须与 IdP 注册值逐字符一致 |
| `OIDC_SCOPES` | `openid profile email` | 空格分隔 |
| `OIDC_AUDIENCE` | 空 | 非空时校验 JWT access token |
| `OIDC_PROVIDER_NAME` | `统一登录` | 登录按钮文案 |
| `OIDC_POST_LOGOUT_REDIRECT_URI` | 空 | IdP 注销后回跳 |
| `AUTH_WEB_BASE_URL` | 空 | 分离部署时前端基址；空则同源 |
| `AUTH_SESSION_TTL` | `168h` | 会话绝对有效期 |
| `AUTH_COOKIE_SECURE` | `auto` | `auto/true/false`；auto 依据 TLS 或 `X-Forwarded-Proto` |
| `ADMIN_EMAILS` | 空 | 逗号分隔，要求邮箱已验证 |
| `ADMIN_SUBS` | 空 | 逗号分隔，精确匹配 subject |
| `CORS_ALLOWED_ORIGINS` | 空 | 逗号分隔；非空才启用 CORS |

- `config.Load` 只做格式与启用必填校验；URL 解析、`AUTH_COOKIE_SECURE` 枚举、CORS 不允许 `*`。
- 网络型 discovery 放在 `auth.New`，避免配置层产生网络 I/O。

### `cmd/api`

- 构建 `auth.Config`，调用 `auth.New(startupCtx, ...)`；启用且失败时返回错误并退出。
- 启动清理协程，绑定到进程生命周期；退出时取消。
- 将 `Auth` 与 `AllowedOrigins` 注入 `api.Deps`。
- `*repo.Store` 已实现 `auth.Store`，必要时补包装方法。

### 前端模块

- `web/src/api/auth.ts`：`getMe()`、`logout()`、`loginURL(returnTo)`；登录用整页跳转，不走 fetch。
- `useAuth()`：查询 `/api/auth/me`，`staleTime` 60 秒；`AuthGate` 在 `enabled && !authenticated` 时跳 `/login` 并携带来源路径。
- `App.tsx`：增加 `AppLayout`（顶栏：品牌、用户名、退出）；`/` 与 `/trips/:tripId` 套 `AuthGate`；`/share`、`/login` 不守卫。
- `CreatePage`：认证开启且已登录时展示 `MyTripList`；查询 `GET /api/trips?limit=20`，包含加载、错误和空状态。
- `LoginPage`：按钮跳转 `API_BASE + /api/auth/login?return_to=...`；读取 `?error=` 显示稳定文案。
- `client.ts` 与 `useRunStream` 统一加 `credentials: "include"`；401 时受保护页面给出重新登录入口。
- `EvalDashboard`：`me.user.role === "admin"` 时跳过 token 输入；已登录非 admin 显示 403 文案；未登录沿用 token 输入框。
- 退出：调用 `POST /api/auth/logout`，清空 QueryClient 缓存，再跳首页或 IdP 注销地址。

## 模块交互

```
main → config.Load → auth.New(discovery) → api.NewRouter
auth.Middleware → repo.GetSessionUser → Principal
api.TripOwner → repo.GetTrip → owner 校验
api.RunOwner → repo.GetRun → repo.GetTrip → owner 校验
auth.Callback → oauth2 token → oidc verify → repo.UpsertUser → repo.CreateSession
web useAuth → /api/auth/me → repo.GetSessionUser
```

## 文件组织

```
go.mod                                  + coreos/go-oidc/v3
migrations/0003_auth.sql                新增 users/sessions/auth_requests/trips.owner_id
sqlc.yaml                               追加 0003 schema
internal/config/
  config.go, config_test.go             AUTH_*/OIDC_*/ADMIN_*/CORS_*
internal/auth/
  service.go                            Service、Config、New、清理
  oidc.go                               discovery、登录/回调、token 校验
  session.go                            Cookie、哈希、会话签发/查找
  middleware.go                         Middleware、AdminOrToken
  principal.go                          Principal 与 context
  roles.go                              白名单角色解析
  *_test.go                             单元与假 IdP 集成测试
internal/repo/queries/
  users.sql, sessions.sql, auth_requests.sql
  trips.sql                             改 CreateTrip、加列表查询
internal/api/
  router.go                             路由分组与中间件顺序
  auth.go                               me/login/callback/logout 转发
  authz.go                              TripOwner、RunOwner
  cors.go                               CORS、SameOrigin
  middleware.go                         新增 RateLimitBy
  trips.go                              归属写入、列表、幂等冲突
  *_test.go                             鉴权、归属、列表、CORS 测试
cmd/api/main.go                         构建 auth.Service、启动清理
openapi.yaml                            auth/trips list/schema 与安全方案
web/src/
  api/auth.ts, client.ts                me/logout/loginURL、credentials
  features/auth/{LoginPage,AuthGate}.tsx
  components/AppHeader.tsx
  features/trips/MyTripList.tsx
  App.tsx, features/create/CreatePage.tsx, features/eval/EvalDashboard.tsx
  hooks/useRunStream.ts                 SSE 带凭据
  api/schema.d.ts                       重新生成
deploy/docker-compose.yml               透传新环境变量
.env.example, docs/环境变量配置.md, README.md
```

## 技术决策

| 决策点 | 选择 | 理由 |
|---|---|---|
| OIDC 实现 | `coreos/go-oidc` + `x/oauth2` 通用实现 | 可接 yufu/Keycloak/Auth0；不绑定 yufu SDK；yufu 作为验收环境 |
| 会话形态 | 服务端会话 + HttpOnly Cookie | token 不进浏览器；可服务端注销；符合现有同源部署 |
| 会话存储 | Postgres `sessions`，存 token 哈希 | 重启不丢；单实例无需 Redis；索引查询成本低 |
| 登录防护 | PKCE S256 + state + nonce + 绑定 Cookie | 防拦截、重放和登录 CSRF |
| 授权模型 | 本地 `user/admin` + 环境变量白名单 | yufu 只做认证；授权留在项目内；无需管理后台 |
| 归属校验 | `trips.owner_id` + API 中间件 | 覆盖所有行程/运行入口；RLS 对本 demo 过重，应用层配合测试足够 |
| 非所有者响应 | 统一 404 | 不暴露资源是否存在 |
| 限流 | 关闭按 IP，开启按用户（回退 IP） | 避免反代后所有用户共享一个 IP 额度 |
| Access Token 校验 | 配置 `AUDIENCE` 时按 JWT 校验 | yufu 支持；不透明 token 的 IdP 不配该变量 |
| eval 管理 | admin 会话或 `ADMIN_TOKEN` | 保留脚本/应急路径；普通用户明确 403 |
| 共享与 Key | 分享公开；Key 全局 | 维持 demo 定位，不做用户维度配置 |
| 分离部署 | CORS 白名单 + Cookie，不引入 token 模式 | 安全面更小；跨站场景优先反代统一源 |

## 验证策略

- **单元测试**：配置解析；角色匹配（未验证邮箱不匹配 admin 邮箱）；`return_to` 校验；会话哈希与 Cookie 标志；关闭/开启中间件；CORS 与 Origin 校验；限流 key 选择。
- **假 IdP 集成测试**：httptest 提供 discovery/JWKS/token/UserInfo，覆盖成功回调、state 重放、nonce 不匹配、错误 audience、过期 auth_request。
- **API 测试**：用假 `Authenticator` 注入 Principal；覆盖 A 创建/B 访问 404、列表隔离、幂等冲突、eval 的 admin/user/token 三种路径、认证关闭回归。
- **前端测试**：登录页错误文案、AuthGate 回跳、我的行程空/有数据、顶栏退出、eval 管理员免 token 与普通用户 403、请求带 `credentials`。
- **手工验收**：本地部署 yufu，注册 Web 应用，完成一次真实登录/退出；检查 Cookie 为 HttpOnly、浏览器存储无 IdP token。
- **回归**：认证关闭时跑现有 Go/前端测试、构建、Playwright 主流程与 v1/插件评测；迁移执行 up/down。

## 上线与回滚

1. 默认 `AUTH_ENABLED=false`，迁移只加表/列，现有行为不变。
2. 启用前注册 OIDC Web 客户端，`OIDC_REDIRECT_URI` 指向 `https://<host>/api/auth/callback`。
3. 配置 `ADMIN_EMAILS` 或 `ADMIN_SUBS`，否则没有登录态 admin（仍可用 `ADMIN_TOKEN`）。
4. 同源部署无需 CORS；分离部署配置 `CORS_ALLOWED_ORIGINS` 与 `AUTH_WEB_BASE_URL`，HTTPS 下 `AUTH_COOKIE_SECURE=true`。
5. 回滚只需关闭开关；用户/会话表可保留，不删除行程数据。