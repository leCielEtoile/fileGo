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
   - `guilds.members.read`

### Discord Bot作成

1. 同じアプリケーションで **Bot** タブに移動
2. **Add Bot** をクリック
3. **Bot Token** をコピー（後で使用）
   - ⚠️ トークンは一度しか表示されません。紛失した場合は"Reset Token"で再生成
4. **Privileged Gateway Intents** で以下を有効化:
   - ✅ **SERVER MEMBERS INTENT**
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

### 方法1: 環境変数で設定（推奨）

セキュアな情報は環境変数で管理することを推奨します。

```bash
# .envファイルを作成
cp .env.example .env

# .envファイルを編集
vi .env
```

**必須の環境変数:**

```bash
# Discord設定
DISCORD_BOT_TOKEN=YOUR_BOT_TOKEN_HERE
DISCORD_CLIENT_ID=YOUR_CLIENT_ID_HERE
DISCORD_CLIENT_SECRET=YOUR_CLIENT_SECRET_HERE
DISCORD_GUILD_ID=YOUR_GUILD_ID_HERE
DISCORD_REDIRECT_URL=https://yourdomain.com/auth/callback
```

**オプションの環境変数:**

```bash
# サーバー設定
SERVER_PORT=8080
SERVER_BEHIND_PROXY=true
SERVER_TRUSTED_PROXIES=172.16.0.0/12,10.0.0.0/8,192.168.0.0/16

# データベース設定
DATABASE_PATH=/root/config/fileserver.db
DATABASE_MAX_CONNECTIONS=10

# ストレージ設定
STORAGE_UPLOAD_PATH=/root/data/uploads
STORAGE_MAX_FILE_SIZE=104857600  # 100MB
STORAGE_CHUNK_SIZE=20971520  # 20MB
STORAGE_MAX_CHUNK_FILE_SIZE=536870912000  # 500GB
STORAGE_MAX_CONCURRENT_UPLOADS=3
STORAGE_CHUNK_UPLOAD_ENABLED=true
STORAGE_UPLOAD_SESSION_TTL=48h
STORAGE_CLEANUP_INTERVAL=1h
```

### 方法2: config.yamlで設定

環境変数を使用しない場合、config.yamlで設定できます。

```bash
# 設定ファイルを作成
mkdir -p config
cp config.yaml.example config/config.yaml

# 設定ファイルを編集
vi config/config.yaml
```

**config.yamlの例:**

```yaml
server:
  port: "8080"
  behind_proxy: true
  trusted_proxies:
    - "172.16.0.0/12"
    - "10.0.0.0/8"
    - "192.168.0.0/16"

discord:
  bot_token: "YOUR_BOT_TOKEN_HERE"
  client_id: "YOUR_CLIENT_ID_HERE"
  client_secret: "YOUR_CLIENT_SECRET_HERE"
  guild_id: "YOUR_GUILD_ID_HERE"
  redirect_url: "https://yourdomain.com/auth/callback"

database:
  path: "/root/config/fileserver.db"
  max_connections: 10

storage:
  upload_path: "/root/data/uploads"
  max_file_size: 104857600
  chunk_upload_enabled: true
  chunk_size: 20971520
  max_chunk_file_size: 536870912000
  max_concurrent_uploads: 3
  upload_session_ttl: 48h
  cleanup_interval: 1h

  # ディレクトリごとの権限設定
  directories:
    - path: "admin"
      required_roles: ["1111111111111111111"]  # Admin Role ID
      permissions: ["read", "write", "delete"]

    - path: "staff"
      required_roles: ["2222222222222222222"]  # Staff Role ID
      permissions: ["read", "write"]

    - path: "members"
      required_roles: ["3333333333333333333"]  # Member Role ID
      permissions: ["read", "write"]

    - path: "public"
      required_roles: []  # 全員アクセス可能
      permissions: ["read"]
```

**設定の優先順位:**

```
環境変数 > config.yaml > デフォルト値
```

### ディレクトリ権限の設定

`storage.directories` で各ディレクトリの権限を設定します。

**required_roles:**
- 空配列 `[]`: 全員がアクセス可能
- ロールID配列: 指定されたロールのいずれかを持つユーザーのみアクセス可能

**permissions:**
- `read`: ファイル一覧表示・ダウンロード
- `write`: ファイルアップロード
- `delete`: ファイル削除

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

# 期待されるレスポンス
{"status":"ok"}
```

## トラブルシューティング

### 認証エラー

**症状**: ログインできない、OAuth2エラーが表示される

**確認項目:**
1. ✅ Discord Developer PortalのRedirect URLが正しく設定されているか
2. ✅ `DISCORD_CLIENT_ID`と`DISCORD_CLIENT_SECRET`が正しいか
3. ✅ BotがDiscordサーバーに招待されているか
4. ✅ BotのSERVER MEMBERS INTENTが有効になっているか

**解決方法:**
```bash
# ログを確認
docker compose -f docker-compose.deploy.yml logs fileserver

# 環境変数を再確認
docker compose -f docker-compose.deploy.yml exec fileserver env | grep DISCORD
```

### 権限エラー

**症状**: ファイルにアクセスできない、403 Forbiddenエラー

**確認項目:**
1. ✅ ユーザーが適切なロールを持っているか（Discordサーバーで確認）
2. ✅ `config.yaml`の`required_roles`に正しいRole IDが設定されているか
3. ✅ ディレクトリの`permissions`配列に必要な権限が含まれているか

**デバッグ方法:**
```bash
# アクセスログを確認
docker compose -f docker-compose.deploy.yml exec fileserver cat /root/logs/access.log

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

**確認項目:**
1. ✅ `entrypoint.sh`が正しく実行されているか
2. ✅ ボリュームマウントが正しいか
3. ✅ ファイルシステムの書き込み権限があるか

**解決方法:**
```bash
# entrypoint.shのログを確認
docker compose -f docker-compose.deploy.yml logs | grep "config.yaml"

# 手動で作成
mkdir -p config
docker compose -f docker-compose.deploy.yml exec fileserver cat /root/config/config.yaml > config/config.yaml

# 再起動
docker compose -f docker-compose.deploy.yml restart
```

## 次のステップ

- [デプロイ・運用ガイド](DEPLOYMENT.md) - 本番環境での運用方法
- [API仕様](API.md) - エンドポイント一覧
- [開発ガイド](DEVELOPMENT.md) - ローカル開発環境のセットアップ
