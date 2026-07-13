# セットアップガイド

このドキュメントでは、fileGoの詳細なセットアップ手順を説明します。

## 目次

- [前提条件](#前提条件)
- [Discord Application作成](#discord-application作成)
- [設定](#設定)
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

## 設定

認証情報は `config.yaml` の `auth.provider` に記述します。初回起動時にひな型が自動生成されるので、それを編集してください。

```bash
# Docker: 初回起動でひな型が生成される
docker compose up -d
vi config/config.yaml

# 配布バイナリ: 実行ファイルと同じ場所に生成される
./fileserver
vi config.yaml
```

**起動に最低限必要なのは、認証情報とディレクトリ定義だけです**（他はすべて既定値）。

```yaml
auth:
  provider:
    type: discord
    bot_token: "..."          # Discord Developer Portal で取得した値
    client_id: "..."
    client_secret: "..."
    guild_id: "..."
    redirect_url: "https://yourdomain.com/auth/callback"

storage:
  directories:
    - path: "public"
      grants:
        - role: "*"           # "*" は全メンバー
          permissions: ["read"]
```

> HTTPで動かす場合は `server.secure_cookie: false` にしてください（`true` のままだとログインできません）。

**設定項目の全リファレンス**（既定値・環境変数・権限モデル・秘密情報の扱い）は **[設定リファレンス](CONFIGURATION.md)** を参照してください。よく使うのは次の2つです。

- [ログインできる人をロールで絞る（`required_roles`）](CONFIGURATION.md#ログインできる人を絞るrequired_roles) — 公開サーバーで「承認した人だけ」に限定する
- [ディレクトリ権限（`grants`）](CONFIGURATION.md#storagedirectories権限モデル) — ロール／個人ごとに read / write / delete を付与する

> `config.yaml` は秘密情報を含むため `.gitignore` 済みです。コミットしないでください。

## 起動方法

### Docker Composeでデプロイ（推奨）

```bash
# リポジトリをクローン
git clone https://github.com/leCielEtoile/fileGo.git
cd fileGo

# データ用ディレクトリを作り、実行UID/GIDを記録する（編集不要）
mkdir -p config data
printf 'PUID=%s\nPGID=%s\n' "$(id -u)" "$(id -g)" > .env

# 起動（config.yaml は自動生成される）
docker compose up -d

# ログ確認
docker compose logs -f
```

> ⚠️ **`mkdir -p config data` を省略しないでください。** ディレクトリが存在しないと **Docker が root 所有で作成**します。コンテナは非rootで動くため書き込めず、`permission denied` で起動に失敗します。
>
> `PUID` / `PGID` は**コンテナの実行UID/GID**です。ホストの自分自身に合わせることで、生成されるファイル（アップロードされたファイル・DB）の所有者があなたになり、`sudo` なしで管理・削除できます。未設定の場合はイメージ既定の `65532` で動作しますが、その場合はホスト側ディレクトリも `65532` 所有にしておく必要があります。
>
> なお Docker Compose は `id -u` を自動実行できないため（`.env` はコマンド置換に非対応）、この値は上記のように**生成しておく必要があります**。

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
