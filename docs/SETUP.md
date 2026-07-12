# セットアップガイド

このドキュメントでは、fileGoの詳細なセットアップ手順を説明します。

## 目次

- [前提条件](#前提条件)
- [Discord Application作成](#discord-application作成)
- [設定方法](#設定方法)
- [起動方法](#起動方法)
- [トラブルシューティング](#トラブルシューティング)

## 前提条件

### 必須要件

- Docker 20.10以降
- Docker Compose v2.0以降
- Discordアカウント
- Discordサーバーの管理者権限

### 推奨環境

- Linux (Ubuntu 20.04+, Debian 11+) または macOS
- メモリ: 512MB以上
- ストレージ: アップロードファイル用に十分な容量

## Discord Application作成

### OAuth2 Application

1. [Discord Developer Portal](https://discord.com/developers/applications) にアクセス
2. **New Application** をクリック
3. アプリケーション名を入力して作成
4. **OAuth2** > **General** で以下を取得:
   - **Client ID** （後で使用）
   - **Client Secret** （"Reset Secret"をクリックして取得）
5. **OAuth2** > **Redirects** で以下を追加:
   ```
   https://yourdomain.com/auth/callback
   ```
   開発環境の場合:
   ```
   http://localhost:8080/auth/callback
   ```
6. **OAuth2** > **URL Generator** で以下のスコープを選択:
   - `identify`
   - `email`
   - `guilds.members.read`

### Discord Bot作成

1. 同じアプリケーションで **Bot** タブに移動
2. **Add Bot** をクリック
3. **Bot Token** をコピー（後で使用）
   - ⚠️ トークンは一度しか表示されません。紛失した場合は"Reset Token"で再生成
4. **Privileged Gateway Intents** で以下を有効化:
   - ✅ **SERVER MEMBERS INTENT**
   - これによりロールのリアルタイム同期（ゲートウェイ常時接続）が使えます。ロール変更が即座に認可へ反映され、ロール参照でDiscord REST APIを叩かなくなります。未有効の場合は起動時に自動検出してREST方式へフォールバックします（`config.yaml` の `auth.provider.gateway_enabled` で無効化も可能）。詳細は [ARCHITECTURE.md](ARCHITECTURE.md) を参照。
5. **OAuth2** > **URL Generator** で以下を選択:
   - **Scopes**: `bot`
   - **Bot Permissions**: `Read Messages/View Channels`
6. 生成されたURLをコピーしてブラウザで開き、Botをサーバーに招待

### Guild ID (Server ID) 取得

1. Discordで**開発者モードを有効化**:
   - ユーザー設定 > 詳細設定 > 開発者モード（オン）
2. サーバーアイコンを右クリック > **IDをコピー**

### Role ID取得

1. サーバー設定 > ロール
2. 対象のロールを右クリック > **IDをコピー**
3. 複数のロールIDをメモ（admin, staff, members など）

**取得した情報をメモ:**
```
Bot Token: YOUR_BOT_TOKEN_HERE
Client ID: YOUR_CLIENT_ID_HERE
Client Secret: YOUR_CLIENT_SECRET_HERE
Guild ID: YOUR_GUILD_ID_HERE
Admin Role ID: YOUR_ADMIN_ROLE_ID_HERE
Staff Role ID: YOUR_STAFF_ROLE_ID_HERE
Member Role ID: YOUR_MEMBER_ROLE_ID_HERE
```

## 設定方法

認証情報（`bot_token` / `client_secret` 等）は **`config.yaml` の `auth.provider` にのみ**記述します。環境変数では設定できません（環境変数で上書きできるのは `server` / `database` / `storage` の一部のみ）。

```bash
# 設定ファイルを作成
mkdir -p config
cp config.yaml.example config/config.yaml

# 設定ファイルを編集
vi config/config.yaml
```

> `config.yaml` は秘密情報を含むため `.gitignore` 済みです。コミットしないでください。スキーマの正は `config.yaml.example` です。

**config.yamlの例（Discord）:**

```yaml
server:
  port: "8080"
  behind_proxy: true
  secure_cookie: true
  trusted_proxies:
    - "172.16.0.0/12"
    - "10.0.0.0/8"
    - "192.168.0.0/16"

# 認証プロバイダーは1つに限定（type: discord または oidc）
auth:
  provider:
    name: discord
    type: discord
    bot_token: "YOUR_BOT_TOKEN_HERE"
    client_id: "YOUR_CLIENT_ID_HERE"
    client_secret: "YOUR_CLIENT_SECRET_HERE"
    guild_id: "YOUR_GUILD_ID_HERE"
    redirect_url: "https://yourdomain.com/auth/callback"

database:
  path: "/app/config/fileserver.db"
  max_connections: 10

storage:
  upload_path: "/app/data/uploads"
  max_file_size: 104857600
  chunk_upload_enabled: true
  chunk_size: 20971520
  max_chunk_file_size: 536870912000
  max_concurrent_uploads: 3
  upload_session_ttl: 48h
  cleanup_interval: 1h
  admin_role_id: "1111111111111111111"  # 全ディレクトリ全操作を許可するロール

  # ディレクトリごとの権限設定（grants）
  directories:
    # 各ユーザーの個人ディレクトリ（本人と管理者のみ）
    - path: "user"
      type: user_private

    # 管理者専用
    - path: "admin"
      grants:
        - role: "1111111111111111111"
          permissions: ["read", "write", "delete"]

    # スタッフ用（editorは編集可、viewerは閲覧のみ）
    - path: "staff"
      grants:
        - role: "2222222222222222222"   # editorロール
          permissions: ["read", "write"]
        - role: "3333333333333333333"   # viewerロール
          permissions: ["read"]

    # 公開ディレクトリ（全メンバーが閲覧可）
    - path: "public"
      grants:
        - role: "*"
          permissions: ["read"]
```

汎用OIDC（Keycloak等）を使う場合は `type: oidc` とし、`issuer` / `scopes` / `groups_claim` / `allowed_email_domains` 等を指定します。詳細は `config.yaml.example` のコメントを参照してください。

**環境変数による上書き（任意）:**

`server` / `database` / `storage` の一部は環境変数で上書きできます（優先順位: 環境変数 > config.yaml > デフォルト値）。`SERVER_PORT` / `SERVER_BEHIND_PROXY` / `DATABASE_PATH` / `STORAGE_MAX_FILE_SIZE` / `STORAGE_ADMIN_ROLE_ID` など。**認証系（`auth.provider`）は対象外**です。

### ディレクトリ権限の設定（grants）

`storage.directories[].grants` で各ディレクトリの権限を設定します。1つの `grant` は次を持ちます。

- `role`: DiscordのロールID。`"*"` は全メンバーを表す
- `user`: 特定メンバーのユーザーID（個人への付与）
- `permissions`: 許可する操作の配列

**permissions:**
- `read`: ファイル一覧表示・ダウンロード
- `write`: ファイルアップロード
- `delete`: ファイル削除

`admin_role_id` を持つユーザーは全ディレクトリで全操作が許可されます。`type: user_private` のディレクトリ（例 `user`）は本人と管理者のみアクセスできます。

## 起動方法

### Docker Composeでデプロイ（推奨）

```bash
# リポジトリをクローン
git clone https://github.com/leCielEtoile/fileGo.git
cd fileGo

# 環境変数を設定
cp .env.example .env
vi .env

# 起動（ディレクトリとconfig.yamlは自動生成）
docker compose -f docker-compose.deploy.yml up -d

# ログ確認
docker compose -f docker-compose.deploy.yml logs -f
```

### 開発環境で起動

```bash
# 設定ファイルを準備
cp config.yaml.example config.yaml
vi config.yaml

# 開発用docker-composeで起動
docker compose up -d

# ログ確認
docker compose logs -f
```

### ローカルで直接ビルド・起動

```bash
# 依存関係のインストール
go mod download

# ビルド
go build -o fileserver .

# 実行
./fileserver
```

### 起動確認

```bash
# ヘルスチェック
curl http://localhost:8080/health

# 期待されるレスポンス（プレーンテキスト）
OK
```

## トラブルシューティング

### 認証エラー

**症状**: ログインできない、OAuth2エラーが表示される

**確認項目:**
1. ✅ Discord Developer PortalのRedirect URLが `config.yaml` の `redirect_url` と一致しているか
2. ✅ `config.yaml` の `auth.provider` の `client_id` / `client_secret` が正しいか
3. ✅ BotがDiscordサーバーに招待されているか
4. ✅ BotのSERVER MEMBERS INTENTが有効になっているか

**解決方法:**
```bash
# ログを確認
docker compose -f docker-compose.deploy.yml logs fileserver

# config.yaml の認証設定を再確認（プレースホルダのままになっていないか）
docker compose -f docker-compose.deploy.yml exec fileserver cat /app/config/config.yaml
```

### 権限エラー

**症状**: ファイルにアクセスできない、403 Forbiddenエラー

**確認項目:**
1. ✅ ユーザーが適切なロールを持っているか（Discordサーバーで確認）
2. ✅ `config.yaml` の該当ディレクトリの `grants` に正しい `role` / `user` が設定されているか
3. ✅ その `grant` の `permissions` 配列に必要な操作が含まれているか

**デバッグ方法:**
```bash
# アクセスログを確認（標準出力へJSON出力される）
docker compose -f docker-compose.deploy.yml logs fileserver

# ユーザーのロール情報を確認（起動ログに表示）
docker compose -f docker-compose.deploy.yml logs | grep "roles"
```

### アップロードエラー

**症状**: ファイルアップロードが失敗する

**確認項目:**
1. ✅ ファイルサイズが制限内か（通常: 100MB、チャンク: 500GB）
2. ✅ `data/uploads`ディレクトリに書き込み権限があるか
3. ✅ ディスク容量が十分にあるか

**解決方法:**
```bash
# ディスク容量確認
df -h

# ディレクトリ権限確認
ls -la data/uploads

# ファイルサイズ制限を変更（.env）
STORAGE_MAX_FILE_SIZE=209715200  # 200MBに変更
```

### Dockerコンテナが起動しない

**症状**: コンテナがすぐに停止する、起動ログにエラー

**確認項目:**
1. ✅ 環境変数が正しく設定されているか
2. ✅ `config.yaml`が存在するか（自動生成されるはず）
3. ✅ ポート8080が既に使用されていないか

**解決方法:**
```bash
# コンテナの状態確認
docker compose -f docker-compose.deploy.yml ps

# エラーログ確認
docker compose -f docker-compose.deploy.yml logs

# 完全にクリーンアップして再起動
docker compose -f docker-compose.deploy.yml down -v
docker compose -f docker-compose.deploy.yml up -d

# ポート使用状況確認
lsof -i :8080
```

### config.yamlが自動生成されない

**症状**: `config.yaml not found`エラーが出る

設定ひな型はバイナリに埋め込まれており、`/app/config/config.yaml` が無い場合に自動生成されます。

**確認項目:**
1. ✅ ボリュームマウント（`./config:/app/config`）が正しいか
2. ✅ ホスト側の `config` ディレクトリが実行UID/GID（既定は65532、`.env` の `PUID`/`PGID` で変更可）から書き込めるか

**解決方法:**
```bash
# 生成ログを確認
docker compose -f docker-compose.deploy.yml logs | grep "設定ファイル"

# ホスト側ディレクトリの所有者を実行ユーザーに合わせる
# 既定(65532)のまま使う場合:
sudo chown -R 65532:65532 ./config ./data
# ホストユーザーで動かす場合（.env に PUID/PGID を設定した場合）:
#   sudo chown -R "$(id -u):$(id -g)" ./config ./data

# 再起動
docker compose -f docker-compose.deploy.yml restart
```

> **補足**: `.env` に `PUID`/`PGID` を設定すると、コンテナはそのUID/GIDで動作し、
> `./config` `./data` に生成されるファイルの所有者もそれに一致します。
> ホストユーザーの `id -u` / `id -g` に合わせておくと、生成ファイルを
> root権限なしで管理・削除できます。ホスト側ディレクトリの所有者も
> 同じUID/GIDに揃えてから起動してください。

## 次のステップ

- [デプロイ・運用ガイド](DEPLOYMENT.md) - 本番環境での運用方法
- [API仕様](API.md) - エンドポイント一覧
- [開発ガイド](DEVELOPMENT.md) - ローカル開発環境のセットアップ
