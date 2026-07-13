# AI-CONTEXT

Dense orientation for AI agents editing this repo. Read first; respect the invariants. Rationale/"why" lives in [ARCHITECTURE.md](ARCHITECTURE.md); API facts in [API.md](API.md) / [openapi.yaml](openapi.yaml).

## What
`fileGo` (Go module `fileserver`): self-hosted file server. Discord guild members get role/member-scoped, per-directory access. Generic OIDC (Keycloak/Google) is a secondary path.

Principles:
- Exactly ONE auth provider (`discord` or `oidc`), never both → `User.ID` == provider `subject` (no composite key).
- Discord first-class; OIDC secondary (no live membership re-check; roles only from ID Token).

## Stack
Go 1.26 · `modernc.org/sqlite` (pure-Go, no cgo) · `go-chi/chi/v5` · `golang.org/x/oauth2`+Discord REST / `coreos/go-oidc/v3` · `bwmarrin/discordgo` (gateway) · `log/slog` JSON. Distroless, non-root UID 65532, no shell. web assets + config template embedded via `go:embed` (`assets.go`). Base image pinned literally (Dependabot).

## Packages (all under `internal/`; deps flow main → handler/middleware → permission/authprovider/storage → config/models)
| pkg | role |
|---|---|
| config | config.yaml load, env overrides, grants eval; `bootstrap.go` fetches example if missing |
| authprovider | `Provider` iface + `discord.go`/`oidc.go`/`factory.go`; `discord_gateway.go` = realtime role sync |
| rolestore | persist OIDC roles to DB |
| permission | grants-based `Checker`; `ReadFilter` for SSE filtering |
| middleware | `AuthMiddleware` (session+membership), `AdminMiddleware`, Logger/RealIP/Recoverer |
| handler | auth/file/chunk/admin/sse + `helpers.go` |
| storage | `storage.go` (files) + `upload_manager.go` (chunks) |
| logging | slog level (`LOG_LEVEL`) + `ContextHandler` (adds `request_id`) |
| models | shared models + context keys; `SanitizeDirName` |
| database | SQLite init + schema |

## Auth
login → CSRF `state` cookie `oauth_state`; callback verifies state → exchange → FetchUserInfo → IsMember (Discord: guild; OIDC: email allowlist) → upsert `users`, create `sessions`, set `session_token` cookie (HttpOnly, 7d). Discord precreates user dir at login. `AuthMiddleware` re-checks membership per request. User in ctx via `models.UserContextKey`.

## Authz (`permission.Checker`)
grants = `directories[].grants`: {`role`|`user`|`"*"`} × {read|write|delete}.
- `admin_role_id` → all dirs, all ops.
- `user_private` dirs (e.g. `user`): owner + admin only; `/user/{name}`, name=username=dirname. write always allowed (creates on first upload); read/delete only if dir exists.
- Resilience: role-independent grants (`*`, user) evaluated first → role-fetch failure still allows public/personal.
- Methods: `StaticPermissions`/`RolePermissions`/`EffectivePermissions`/`Config.HasAdminRole`.

## Role/membership sourcing (details: ARCHITECTURE.md)
- **Tier1 gateway** (Server Members Intent on): `GetUserRoles`/`VerifyMembership` resolve from memory; role changes live-update and refresh the SSE permission snapshot.
- **Tier2 REST** (auto fallback on intent-missing / close 4014): per-user REST, 5-min cache + rate limiter + singleflight + TTL jitter + stale-while-error.
- `auth.provider.gateway_enabled` (default true). Async start; serves via REST until ready.

## Data model (`database.go`, `CREATE TABLE IF NOT EXISTS`; no migrations)
`users` (id=subject, UNIQUE(provider,subject)) · `sessions` (session_token PK, expires_at; expired rows purged at start + hourly) · `file_metadata` (uploader, sha256, UNIQUE(directory,filename)) · `oidc_user_roles` (PK(provider,subject), OIDC-only; persisted because OIDC roles come only from the ID Token). `access_logs` removed (`DROP` on start; access logging is stdout JSON).

## Invariants / pitfalls
- **Never put secret VALUES in env vars** (leak via `docker inspect`, process list, logs). Secrets come from `config.yaml`, or from a file whose *path* is given by `FILEGO_BOT_TOKEN_FILE` / `FILEGO_CLIENT_SECRET_FILE` (`*_FILE` convention, for Docker/K8s secrets). Do NOT add env vars that carry the secret value itself.
- **All app env vars are `FILEGO_`-prefixed** (unprefixed names collide in shared environments and are NOT read; a startup WARN flags leftovers). Env > config.yaml > defaults. Invalid env values are a startup **error**, never silently ignored.
- `config.yaml` is gitignored; never commit. Source of truth = `config.yaml.example`. Also gitignored: `.mcp.json`, `*.db`.
- Cookies: `session_token` (not `session_id`); CSRF `oauth_state`.
- Errors = plain text (`http.Error`); success = JSON via `handler/helpers.go` `writeJSON`.
- Username → directory must pass `models.SanitizeDirName`.
- SSE dir events are filtered by recipient read permission (`ReadFilter` snapshot); never broadcast dir events unfiltered.
- Upload counter (`UploadManager.userUploads`) is inaccurate across restart; `releaseUploadSlot` floors at 0.

## Build / test
`go build ./...` · `go vet ./...` · `gofmt -l .` (empty = ok) · `go test -race ./...`. Run the binary from repo root (`web/` must be visible; templates parsed once at startup). Lint: `.golangci.yml` (golangci-lint v2). Tests exist for security-critical pure fns + gateway store (models/permission/middleware/handler/authprovider); coverage is minimal.

## Endpoints
public: `GET /`, `/health`, `/auth/login|callback|logout`. auth (`AuthMiddleware`): `/api/user`, `/api/events` (SSE), `/files*`. admin (`AdminMiddleware`): `/admin`, `/api/admin/uploads`, `/api/admin/stats`. Full: [API.md](API.md), [openapi.yaml](openapi.yaml).

## Conventions
Japanese for code comments / logs / user-facing messages; godoc starts with the identifier name. Commits: Japanese Conventional Commits (`feat`/`fix`/`refactor`/`docs`/…; breaking = `!` + `BREAKING CHANGE:`). Comment roles: code = how / why-not, tests = what, commit = why; no thinking-process comments. `permission`/`middleware`/`authprovider` are sensitive — verify these invariants before changing.
