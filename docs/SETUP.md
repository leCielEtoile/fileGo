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

### ログインできる人を絞る（required_roles）

既定では、**Discordサーバーに在籍していれば誰でもログインでき**、個人ディレクトリが払い出されます。誰でも参加できる公開サーバーでは、これは望ましくない場合があります。

`auth.provider.required_roles` を設定すると、**在籍に加えて特定ロールの保有を要求**できます（いずれか1つ保有していればログイン可＝OR判定）。

```yaml
auth:
  provider:
    # ...
    required_roles:
      - "234567890123456789"   # approved ロール
      - "123456789012345678"   # 管理者ロール
```

- **未設定なら従来どおり**、在籍しているだけでログインできます。
- 条件を満たさないユーザーはログインできず、**個人ディレクトリも作成されません**。
- **ロールを剥奪すると、既存セッションも次のリクエストで失効します**（ログイン時だけでなく、リクエストごとの在籍確認でも判定するため）。

> ⚠️ **締め出しに注意**：管理者もこの条件の対象です。管理者ロールしか持たない人をログインさせる場合は、そのロールIDも `required_roles` に含めてください。

汎用OIDCでも同様に指定でき、`groups_claim` の値と照合されます。`allowed_email_domains` / `allowed_emails` と併用した場合、設定されている条件は**すべて満たす必要があります**（AND）。ただしOIDCには在籍の継続確認が無いため、**ロール剥奪が既存セッションへ即時反映されるのはDiscordのみ**です。

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
docker compose up -d

# ログ確認
docker compose logs -f
```

### 開発環境で起動（ソースからビルド）

既定の `docker-compose.yml` は公開イメージ（GHCR）を使います。ローカルの変更を動かして確かめたい場合は、開発用オーバーレイを重ねてビルドします。

```bash
# 設定ファイルを準備
mkdir -p config
cp config.yaml.example config/config.yaml
vi config/config.yaml

# ソースからビルドして起動
docker compose -f docker-compose.yml -f docker-compose.develop.yml up -d --build

# ログ確認
docker compose -f docker-compose.yml -f docker-compose.develop.yml logs -f
```

### 配布バイナリで起動（Dockerを使わない場合）

Dockerが使えない環境向けに、[リリースページ](https://github.com/leCielEtoile/fileGo/releases) で各OS向けのバイナリを配布しています。**Webアセットと設定ひな型はバイナリに埋め込まれている**ため、実行に追加ファイルは不要です（Go処理系も不要）。

| OS / アーキテクチャ | アセット |
|---|---|
| Linux (x86_64) | `fileserver-linux-amd64.tar.gz` |
| Linux (ARM64) | `fileserver-linux-arm64.tar.gz` |
| macOS (Intel) | `fileserver-darwin-amd64.tar.gz` |
| macOS (Apple Silicon) | `fileserver-darwin-arm64.tar.gz` |
| Windows (x86_64) | `fileserver-windows-amd64.zip` |

```bash
# 1. ダウンロードして展開（例: Linux x86_64。VERSION は v0.1.2 など）
curl -LO https://github.com/leCielEtoile/fileGo/releases/download/VERSION/fileserver-linux-amd64.tar.gz
tar xzf fileserver-linux-amd64.tar.gz
mv fileserver-linux-amd64 fileserver     # 扱いやすいようリネーム（任意）
chmod +x fileserver

# 2. 初回起動で config.yaml のひな型が実行ファイルと同じ場所に自動生成される
#    （認証情報が未設定のため、警告を出しつつ起動する）
./fileserver

# 3. 生成された config.yaml に認証情報を記入する
vi config.yaml
```

保存先の既定値（`./data/fileserver.db` / `./data/uploads`）はそのままで動作します。設定が必要なのは主に認証情報です。

```yaml
server:
  secure_cookie: false              # HTTPで動かす場合（HTTPS配信なら true のまま）

auth:
  provider:
    bot_token: "..."                # Discordの認証情報を設定
    client_id: "..."
    client_secret: "..."
    guild_id: "..."
    redirect_url: "http://localhost:8080/auth/callback"
```

```bash
# 4. 起動
./fileserver
```

> 相対パス（`./data/...`）は**カレントディレクトリ基準**で解決されます。常駐させる場合は下記 systemd の `WorkingDirectory` を設定するか、絶対パスを指定してください。

設定ファイルを編集せず、環境変数で上書きすることもできます。

```bash
DATABASE_PATH=./data/fileserver.db \
STORAGE_UPLOAD_PATH=./data/uploads \
SERVER_PORT=8080 \
  ./fileserver
```

別の場所の設定ファイルを使う場合は `CONFIG_PATH` を指定します（未指定時は実行ファイルと同じディレクトリの `config.yaml`）。

```bash
CONFIG_PATH=/etc/filego/config.yaml ./fileserver
```

#### systemd サービスとして常駐させる（Linux）

```ini
# /etc/systemd/system/filego.service
[Unit]
Description=fileGo file server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=filego
WorkingDirectory=/opt/filego
ExecStart=/opt/filego/fileserver
Environment=CONFIG_PATH=/opt/filego/config.yaml
Environment=LOG_LEVEL=info
Restart=on-failure
RestartSec=5s
# 書き込みは data ディレクトリのみに限定する
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/opt/filego/data /opt/filego/config.yaml

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now filego
sudo systemctl status filego
journalctl -u filego -f     # ログ（JSON構造化ログが出力される）
```

### ソースからビルドする場合

```bash
go mod download
go build -o fileserver .
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
docker compose logs fileserver

# config.yaml の認証設定を再確認（プレースホルダのままになっていないか）
docker compose exec fileserver cat /app/config/config.yaml
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
docker compose logs fileserver

# ユーザーのロール情報を確認（起動ログに表示）
docker compose logs | grep "roles"
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
docker compose ps

# エラーログ確認
docker compose logs

# 完全にクリーンアップして再起動
docker compose down -v
docker compose up -d

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
docker compose logs | grep "設定ファイル"

# ホスト側ディレクトリの所有者を実行ユーザーに合わせる
# 既定(65532)のまま使う場合:
sudo chown -R 65532:65532 ./config ./data
# ホストユーザーで動かす場合（.env に PUID/PGID を設定した場合）:
#   sudo chown -R "$(id -u):$(id -g)" ./config ./data

# 再起動
docker compose restart
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
