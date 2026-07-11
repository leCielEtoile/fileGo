# AI-CONTEXT

このドキュメントは、**事前のコンテキストを持たないAIエージェント**がこのリポジトリを理解し、安全に変更を加えられるようにするための起点です。作業を始める前にこのファイルを最初に読んでください。ここに書かれた不変条件（invariants）と設計判断を尊重してください。

---

## 1. このプロジェクトは何か

`fileGo`（モジュール名 `fileserver`）は Go 製のセルフホスト型ファイル共有サーバーです。

**設計思想（最重要）:** Discordの特定サーバー（ギルド）への参加を前提に、**ロールやメンバー単位でディレクトリごとのアクセス権限を管理する**ファイルサーバー。Discordでの利用を主眼に設計され、汎用OIDC（Keycloak / Google等）は「補助的に使える」粒度で対応する。

この思想から導かれる2つの原則:
1. **認証プロバイダーは1つに限定する。** 複数プロバイダーを同時に有効化しない（別々のユーザーが生成される複雑さを避けるため）。
2. **Discordが第一級、OIDCは二級。** OIDCはDiscordのAPIを持たないため、在籍の継続確認ができない等の機能差がある。迷ったらDiscordの体験を優先する。

---

## 2. 技術スタックと動作環境

- **Go 1.26**（`go.mod` 参照。モジュール名は `fileserver`）
- **SQLite**: `modernc.org/sqlite`（純Go実装、cgo不要）
- **ルーター**: `github.com/go-chi/chi/v5`
- **認証**: `golang.org/x/oauth2` + Discord REST API / `github.com/coreos/go-oidc/v3`
- **ロギング**: 標準 `log/slog`（JSON出力）
- 配布は Docker（`Dockerfile` / `docker-compose*.yml`）。マルチステージ＋非root(UID 65532)＋シェル無し最小ランタイム（distroless、ダイジェスト固定）
- ベースイメージはDependabotが解決できるようliteralに記述する（ARG経由の`FROM`は更新対象外になる）。差し替えは`--build-context`で行う
- web資産と設定ひな型はバイナリへ埋め込み（`assets.go` の go:embed）。ランタイムイメージに外部ファイル・シェルを持たない

---

## 3. パッケージ構成と責務

すべて `internal/` 配下。依存の向きは概ね `main → handler/middleware → permission/authprovider/storage → config/models`。

| パッケージ | 責務 |
|-----------|------|
| `config` | `config.yaml` の読み込み・環境変数上書き・grants評価メソッド。`bootstrap.go` は設定ファイル未存在時に `config.yaml.example` をローカル/GitHubから取得 |
| `authprovider` | 認証プロバイダーの抽象化。`Provider` インターフェースと `discord.go` / `oidc.go` 実装、`factory.go`（`New()` で単一プロバイダーを構築） |
| `rolestore` | OIDCロールをDBに永続化する `Store`（`authprovider.RoleStore` を満たす） |
| `permission` | grantsベースの認可判定（`Checker`）。ディレクトリ×操作の許可を算出 |
| `middleware` | セッション検証＋在籍の継続確認（`AuthMiddleware`）、管理者判定（`AdminMiddleware`）、ロギング・RealIP・Recoverer |
| `handler` | HTTPハンドラー。`auth`（ログイン/コールバック/ログアウト）、`file`、`chunk`、`admin`、`sse`、共通処理の `helpers.go` |
| `storage` | ファイル保存・一覧・ダウンロード（`storage.go`）とチャンクアップロード管理（`upload_manager.go`） |
| `models` | 共有ドメインモデル（`User` / `Session` / `FileInfo` / `UploadSession` 等）とコンテキストキー |
| `database` | SQLite初期化とスキーマ（`CREATE TABLE IF NOT EXISTS`） |

---

## 4. 認証フロー

1. `GET /auth/login`：CSRF用の `state` をクッキー `oauth_state` に保存し、プロバイダーの認可URLへリダイレクト。
2. `GET /auth/callback`：`state` を照合 → `Exchange` でトークン取得 → `FetchUserInfo` → `IsMember`（Discordはギルド在籍、OIDCはallowlist）。許可されれば `users` にupsert、`sessions` にセッション作成、クッキー `session_token`（有効期限7日）を発行。Discordはこの時点でユーザーディレクトリを事前作成する。
3. 以降のリクエスト：`AuthMiddleware` が `session_token` を検証し、`VerifyMembership` で在籍を継続確認（Discordは5分キャッシュ。非メンバーはクッキー失効＋403）。ユーザーは `models.UserContextKey` でコンテキストに載る。

**重要な識別子:** 認証プロバイダーは1つに限定されるため、`User.ID` はプロバイダー内の `subject`（DiscordならユーザーID）そのもの。`provider:subject` のような合成はしていない。

---

## 5. 認可（grants）モデル

`config.yaml` の `storage.directories[].grants` が権限の源泉。1つの `grant` は次のいずれかを持つ:

- `role: "<ロールID>"` … そのロール保有者に付与（`role: "*"` は全メンバー）
- `user: "<ユーザーID>"` … 特定メンバー個人に付与
- `permissions: [read|write|delete]` … 許可する操作

判定の要点（`permission.Checker`）:
- `admin_role_id` を持つユーザーは**全ディレクトリで全操作**が許可される。
- `type: user_private` のディレクトリ（例 `user`）は特別扱い：本人と管理者のみ。`/user/{name}` の `{name}` はユーザー名（＝ディレクトリ名）。書き込みは常に許可（初回アップロードで作成）、読み取り/削除はディレクトリが実在する場合のみ。
- **resilience:** `CheckPermission` はロール非依存の付与（`*`・ユーザー個人指定）を先に評価し、ロール取得（`GetUserRoles`）が失敗しても公開ディレクトリ等は許可される。`GetAccessibleDirectories` も同様に、ロール取得失敗時はロール非依存の付与のみで一覧を返す。

`config.DirectoryConfig` の権限算出メソッド: `StaticPermissions`(ロール非依存) / `RolePermissions`(ロール依存) / `EffectivePermissions`(両者マージ) / `Config.HasAdminRole`。

---

## 6. データモデル（SQLite）

`internal/database/database.go` の `CREATE TABLE IF NOT EXISTS` が唯一のスキーマ定義（マイグレーション機構は無い。開発段階のためスキーマ変更時は既存DB削除を許容）。

| テーブル | 役割 |
|---------|------|
| `users` | 認証済みユーザー（`id` = subject、`UNIQUE(provider, subject)`） |
| `sessions` | セッション（`session_token` PK、`expires_at`） |
| `access_logs` | 監査ログ |
| `file_metadata` | アップロードファイルのメタ（アップロード者・SHA256ハッシュ、`UNIQUE(directory, filename)`） |
| `oidc_user_roles` | OIDCロールの永続化（`PRIMARY KEY(provider, subject)`）。**OIDC専用** |

**なぜ `oidc_user_roles` があるか:** OIDCのロールはログイン時のID Tokenからしか得られず、サーバー側で再取得できない。再起動でメモリキャッシュが消えても復元できるよう永続化する。Discordのロールはいつでもボットトークンで再取得できるため永続化しない。

---

## 7. 押さえるべき不変条件・落とし穴

- **認証情報は `config.yaml` のみ。** `bot_token` / `client_secret` 等を環境変数で上書きする機能は**意図的に廃止済み**。`overrideFromEnv` が扱うのは `server` / `database` / `storage` の一部だけ。ドキュメントやサンプルに `DISCORD_*` 環境変数を復活させないこと。
- **`config.yaml` は秘密情報を含むため `.gitignore` 済み。** 絶対にコミットしない。スキーマの正は `config.yaml.example`。
- **クッキー名は `session_token`**（`session_id` ではない）。CSRF用は `oauth_state`。
- **エラーレスポンスはプレーンテキスト**（`http.Error`）。成功系JSONは `handler/helpers.go` の `writeJSON` を使う。「エラーもJSON」ではない。
- **Discordのキャッシュ:** ロール/在籍は5分TTLのメモリキャッシュ。ライブ取得失敗時は期限切れキャッシュを返す（stale-while-error）。
- **同時アップロード数カウンタ**（`UploadManager.userUploads`）は再起動をまたぐと正確でない。減算は `releaseUploadSlot` で下限0保護済み。
- `authprovider/discord.go` の `ClearCache` / `SetCacheTTL` はテスト用フックだが**現在テストは無い**（デッドコード気味。削除する場合は将来のテスト方針を確認）。

---

## 8. ビルド・実行・検証

```bash
go build ./...        # ビルド
go vet ./...          # 静的解析
gofmt -l .            # フォーマット逸脱の検出（出力が空なら正常）
go build -o fileserver . && ./fileserver   # 実行（web/ を含むリポジトリルートで）
```

- テストは現状存在しない（方針として未整備）。
- lint 設定は `.golangci.yml`。
- テンプレート（`web/templates/*.html`）は**起動時に一度だけパース**される（`main.loadTemplate`）。実行は `web/` が見えるカレントディレクトリで行う必要がある。

---

## 9. 主要HTTPエンドポイント

認証不要: `GET /`（Web UI、UA判定でPC/モバイル切替）、`GET /health`、`GET /auth/login|callback|logout`。
認証必須（`AuthMiddleware`）: `GET /api/user`、`GET /api/events`(SSE)、`/files*`（一覧・アップロード・ダウンロード・削除・チャンク）。
管理者のみ（`AdminMiddleware`）: `GET /admin`、`GET /api/admin/uploads`、`GET /api/admin/stats`。

完全な仕様は [API.md](API.md) と [openapi.yaml](openapi.yaml) を参照。

---

## 10. コーディング規約

- コメント・ログ・ユーザー向けメッセージは**日本語**。godoc は識別子名で始める。
- コミットは**日本語のConventional Commits**（例: `feat(auth): ...` / `fix(storage): ...` / `refactor(handler): ...` / `docs: ...`）。破壊的変更は `!` と本文の `BREAKING CHANGE:`。
- コミット末尾に `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>` を付与。
- 認可（`permission` / `middleware`）は機微。変更時は本ドキュメントの不変条件に反しないか確認する。
