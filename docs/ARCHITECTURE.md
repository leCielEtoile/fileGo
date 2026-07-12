# アーキテクチャと設計思想

このドキュメントは fileGo が **なぜこの形になっているか**（背景・トレードオフ・設計判断）を説明します。手順は [SETUP.md](SETUP.md) / [DEPLOYMENT.md](DEPLOYMENT.md)、APIの事実は [API.md](API.md) / [openapi.yaml](openapi.yaml)、変更時に守るべき不変条件は [AI-CONTEXT.md](AI-CONTEXT.md) を参照してください。

## 設計思想

fileGo は「**Discordの特定サーバー（ギルド）のメンバーだけが使える、ロール／メンバー単位のアクセス制御を持つファイルサーバー**」です。ここから2つの原則が導かれます。

1. **認証プロバイダーは1つに限定する。** 複数プロバイダーを同時に有効化すると、同一人物が別ユーザーとして重複生成される複雑さが生じます。これを避けるため、`config.yaml` で `discord` か `oidc` を1つだけ選びます。結果として `User.ID` はプロバイダー内の `subject` そのもので済み、合成キーが不要になります。
2. **Discordが第一級、OIDCは補助。** Discordは在籍の継続確認やロールの随時取得ができますが、汎用OIDCにはその手段がありません。迷ったらDiscordの体験を優先します。

## リクエストパイプライン

ミドルウェアは次の順で適用されます。順序には意味があります。

```
RequestID → RealIP → Logger → Recoverer → (AuthMiddleware) → (AdminMiddleware) → handler
```

- **RequestID を最初に**置くことで、以降のすべてのログ行とレスポンスヘッダ `X-Request-Id` に相関IDが載ります。
- **RealIP を Logger の前に**置き、ログや後続処理が実クライアントIPを見られるようにします。
- **Recoverer は最内**で、ハンドラの panic を捕捉してスタックトレース付きで記録します。
- 認証が必要なルートだけ `AuthMiddleware` グループに入れ、管理者ルートはさらに `AdminMiddleware` で包みます。

## 認証

ログインはブラウザ経由の OAuth2 / OIDC フローです。

1. `GET /auth/login` で CSRF 用の `state` を Cookie に保存し、認可URLへリダイレクト。
2. `GET /auth/callback` で `state` を照合し、トークン交換 → ユーザー情報取得 → 参加可否判定（Discordはギルド在籍、OIDCはメールallowlist）。許可されれば `users` にupsert、`sessions` を作成し、`session_token` Cookie（`HttpOnly`、7日）を発行。
3. 以降のリクエストで `AuthMiddleware` が Cookie を検証し、**在籍を継続確認**します（退出者はここで弾かれ、Cookieが失効します）。

CSRF は `state` Cookie 照合と `SameSite=Lax` で守ります。`Strict` にしない理由は、外部IdPからのリダイレクト（トップレベルナビゲーション）で Cookie が送出されるようにするためです。

## 認可（grants モデル）

権限の源泉は `config.yaml` の `storage.directories[].grants` です。1つの grant は「誰に（`role` / `user` / `*`）」×「何を（`read` / `write` / `delete`）」を表します。

判定（`permission.Checker`）の要点と、その背景:

- **管理者ロール（`admin_role_id`）は全ディレクトリ・全操作を許可。** 運用者のエスケープハッチです。
- **`user_private` ディレクトリ（例 `user`）は本人と管理者のみ。** `/user/{name}` の `{name}` はユーザー名＝ディレクトリ名です。プロバイダーが細工した名前で親ディレクトリへ抜け出せないよう、`SanitizeDirName` で単一のパス構成要素へ正規化します（多層防御）。
- **ロール取得失敗に強い（resilience）。** `CheckPermission` はロール非依存の付与（`*`・ユーザー個人指定）を先に評価します。Discord/IdP側の一時障害でロールが取れなくても、公開ディレクトリや個人指定は許可され続けます。

## ロールの取得：2段構え（Tier1 / Tier2）

ロールと在籍の取得には**即時性**と**レート制限回避**という2つの要求があり、環境（特権インテントの可否）によって最適解が変わります。そこで2段構えにし、起動時に自動で切り替えます。

### 制約：即時のロール変更通知には特権インテントが必要

Discordで「外部で起きたロール変更を即座に知る」には、ゲートウェイの `GUILD_MEMBER_UPDATE` 等のイベントが必要で、これは **Server Members Intent（特権インテント）** を要します。一括取得（List Guild Members）も同じインテントを要します。したがって **インテントが無い環境では、即時にロール変更を知る手段は存在しません**（本人が次にアクセスした時の再取得＝プルが限界）。

### Tier1：ゲートウェイ・リアルタイム同期（インテント有効時）

- discordgo でゲートウェイに常時接続し、起動時に**全メンバーを一括ロード**（`RequestGuildMembers` → `GuildMembersChunk`）。
- 以後 `GuildMemberAdd/Update/Remove` でメモリ上のロール表を最新化。
- `GetUserRoles` / `VerifyMembership` は**メモリ参照で即時解決**（REST呼び出しゼロ＝レート制限と無縁）。
- ロール変更時は `onChange` で SSE の該当ユーザーの権限スナップショットを即再解決し、`permissions_updated` を送って UI も更新させます（**リアルタイム認可**）。

### Tier2：REST方式（フォールバック）

インテント未許可（ゲートウェイが `Ready` に到達しない＝close code 4014 等）の場合、起動時に検出して自動的にこちらへ切り替わります。

- Botトークンで **1人ずつ** REST 取得し、5分TTLのメモリキャッシュに載せる。
- 多人数の**一斉更新の殺到（thundering herd）**でレート制限に当たらないよう、次を組み合わせます。
  - **レートリミッター**（毎秒上限）でグローバル制限を常に下回らせる。
  - **singleflight** で同一ユーザーへの同時ミスを1回のAPI呼び出しに集約。
  - **TTLジッター** で有効期限を時間方向に散らし、同時刻ログイン勢の期限が揃うのを防ぐ。
  - **stale-while-error**：ライブ取得に失敗しても、期限切れキャッシュを返して全面停止を避ける。
- 反映はキャッシュTTLぶん遅れます（即時性はインテント無しの物理的限界）。

起動は**非同期**で、ゲートウェイ準備を待つ間もサーバーはREST方式で応答します。準備完了時点でメモリ解決へ透過的に切り替わります。

## SSE配信と権限フィルタ

`/api/events` はファイル操作をリアルタイム通知しますが、**イベントは受信者の権限で絞り込みます**。素朴に全員へ配信すると、閲覧権限のないプライベート領域のファイル名・操作が漏れてしまうためです。

- 接続時に「そのユーザーが読めるディレクトリ集合」を一度だけ解決し、`permission.ReadFilter` として保持（`atomic.Pointer` でロックなし共有）。
- 配信のたびにロールを問い合わせず、**メモリ上の集合判定だけ**で可視性を決めます（ホットパスからI/Oを排除）。
- スナップショットは定期更新し、Tier1では `permissions_updated` 契機で即時に取り直します。

## データモデルの判断

スキーマは `internal/database/database.go` の `CREATE TABLE IF NOT EXISTS` が唯一の定義です（マイグレーション機構は持たず、開発段階ではスキーマ変更時にDB削除を許容）。

- **`oidc_user_roles` を永続化する理由**：OIDCのロールはログイン時のID Tokenからしか得られず、サーバー側で再取得できません。再起動でメモリキャッシュが消えても復元できるよう保存します。Discordのロールはいつでも取得できるため永続化しません。
- **`access_logs` を廃止した理由**：未使用だったため。アクセスログは標準出力への構造化ログ（JSON）へ統一しました。

## ロギング

`log/slog` による JSON 構造化ログを標準出力へ出します。

- レベルは `LOG_LEVEL` で切替。`ContextHandler` が `request_id` を全行へ自動付与します。
- アクセスログはステータスでレベルを出し分け（5xx=Error / 4xx=Warn / それ以外=Info）、`/health` や `/static/*` は平常時ノイズになるため Debug に落とします。
- 監査用途の永続テーブルは持たず、ログ収集基盤側に集約する前提です（コンテナは json-file ドライバでローテーション）。
