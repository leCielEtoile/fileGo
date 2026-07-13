# デプロイ・運用ガイド

本番環境でのfileGoのデプロイと運用方法を説明します。

## 目次

- [本番環境デプロイ](#本番環境デプロイ)
- [環境変数設定](#環境変数設定)
- [リバースプロキシ設定](#リバースプロキシ設定)
- [運用タスク](#運用タスク)
- [バックアップ](#バックアップ)
- [監視](#監視)
- [トラブルシューティング](#トラブルシューティング)

## 本番環境デプロイ

### GitHub Container Registryからデプロイ

GitHub ActionsでビルドされたDockerイメージを使用します。

```bash
# 1. リポジトリをクローン
git clone https://github.com/leCielEtoile/fileGo.git
cd fileGo

# 2. 環境変数を設定
cp .env.example .env
vi .env

# 3. イメージをpullして起動
docker compose pull
docker compose up -d

# 4. 起動確認
docker compose ps
docker compose logs -f
```

### 特定バージョンの指定

`.env`ファイルでイメージタグを指定できます：

```bash
# 最新版（mainブランチ）
IMAGE_TAG=latest

# 特定バージョン
IMAGE_TAG=v1.0.0

# 特定コミット
IMAGE_TAG=main-abc1234
```

### イメージのビルド

ランタイムはマルチステージビルドで、非root(UID 65532)・シェル無しの最小イメージ（distroless）として構築されます。

```bash
docker build -t filego:local .
```

ベースイメージは `Dockerfile` に直接記述しています。Dependabot の docker エコシステムは ARG 経由の `FROM` を解決できないため、build-arg で切り替え可能にすると**ベースイメージが自動更新の対象から外れてしまう**ためです。ランタイムの `:nonroot` はバージョンを持たないローリングタグなので、ダイジェストで固定して Dependabot に digest 更新を追従させています。

別のベースイメージでビルドしたい場合は、Dockerfile を変更せず `--build-context` で上書きできます。名前は `FROM` の記述と完全一致させる必要があります。

```bash
# 例: Docker Hardened Images (DHI) でビルドする
docker login dhi.io   # DHIは無償だがpullに Docker ID が必要
docker buildx build -t filego:dhi \
  --build-context golang:1.26-alpine=docker-image://dhi.io/golang:1.26-dev \
  --build-context "$(grep -m1 '^FROM gcr.io/distroless' Dockerfile | awk '{print $2}')=docker-image://dhi.io/static:latest" \
  .
```

### 既存デプロイからの移行（/root → /app）

旧来のイメージはコンテナ内 `/root` 配下・root実行でしたが、本バージョンから非root(UID 65532)・`/app` 配下に変更されました。既存デプロイからの移行手順：

```bash
# 1. 停止
docker compose down

# 2. ホスト側データの所有者を実行ユーザー(65532)へ変更
#    （.env に PUID/PGID を設定した場合は、そのUID/GIDへ合わせる）
sudo chown -R 65532:65532 ./config ./data

# 3. config.yaml 内の絶対パスを更新（/root → /app）
sed -i 's#/root/#/app/#g' ./config/config.yaml

# 4. 起動
docker compose up -d
```

> ログはファイル(`/root/logs`)ではなく標準出力へ出力される方式に統一されたため、`logs` ボリュームは不要になりました。

### 実行ユーザーをホストユーザーに合わせる（PUID/PGID）

bindマウント（`./config` `./data`）を使う構成では、コンテナが書き込むファイルの
所有者はコンテナの実行UID/GIDになります。既定では非rootユーザー(65532)のため、
ホスト側ではそのファイルを削除・編集するのに `sudo` が必要になります。

`.env` の `PUID`/`PGID` をホストユーザーに合わせると、生成ファイルの所有者が
ホストユーザーになり、root権限なしで管理できます。

```bash
# 1. .env に実行UID/GIDを設定（自分のIDに合わせる）
echo "PUID=$(id -u)" >> .env
echo "PGID=$(id -g)" >> .env

# 2. ホスト側ディレクトリの所有者も同じUID/GIDに揃える
mkdir -p config data
sudo chown -R "$(id -u):$(id -g)" config data

# 3. 起動（compose の user: "${PUID:-65532}:${PGID:-65532}" が適用される）
docker compose up -d
```

`PUID`/`PGID` を未設定のままにすると、従来どおりイメージ既定の65532で動作します。

### デプロイ後の確認

```bash
# ヘルスチェック
curl http://localhost:8080/health

# 起動ログの確認
docker compose logs fileserver | head -50

# 環境変数の確認
docker compose logs fileserver | grep "Environment variables"
```

## 認証設定（config.yaml）

**認証情報は環境変数では設定できません。** Discord/OIDCの認証設定は `config.yaml` の `auth.provider` に記述します（`redirect_url` もここで設定）。初回起動時にひな型が `/app/config/config.yaml` に自動生成されるので、編集して再起動してください。

```yaml
auth:
  provider:
    name: discord
    type: discord
    bot_token: "your_actual_bot_token"
    client_id: "your_client_id"
    client_secret: "your_client_secret"
    guild_id: "your_guild_id"
    redirect_url: "https://yourdomain.com/auth/callback"
```

環境別に `redirect_url` を変えます（開発: `http://localhost:8080/auth/callback`、本番: `https://files.yourdomain.com/auth/callback`）。

## 環境変数設定

環境変数で上書きできるのは `server` / `database` / `storage` の一部のみです（認証系は対象外）。**環境変数は `config.yaml` より優先されます。**

### イメージが固定しているパス

設定ひな型の既定値は「配布バイナリをそのまま実行できる」相対パス（`./data/...`）です。Dockerイメージはコンテナのボリューム構成に合わせ、次の環境変数で絶対パスを与えています。

| 環境変数 | イメージでの既定値 | ホスト側（compose のマウント） |
|---|---|---|
| `CONFIG_PATH` | `/app/config/config.yaml` | `./config/config.yaml` |
| `DATABASE_PATH` | `/app/config/fileserver.db` | `./config/fileserver.db` |
| `STORAGE_UPLOAD_PATH` | `/app/data/uploads` | `./data/uploads` |

そのため、コンテナでは `config.yaml` の `database.path` / `storage.upload_path` を編集しても**反映されません**（環境変数が優先されるため）。保存先を変えたい場合は compose の `environment` でこれらの環境変数を上書きしてください。

### 推奨環境変数

```bash
# Docker/サーバー設定
IMAGE_TAG=v1.0.0
HOST_PORT=8080
SERVER_PORT=8080
SERVER_BEHIND_PROXY=true
SERVER_TRUSTED_PROXIES=172.16.0.0/12,10.0.0.0/8
LOG_LEVEL=info                         # debug / info / warn / error（既定 info）
TZ=Asia/Tokyo                          # ログのタイムゾーン

# ストレージ設定
STORAGE_MAX_FILE_SIZE=104857600        # 100MB
STORAGE_MAX_CHUNK_FILE_SIZE=536870912000  # 500GB
STORAGE_ADMIN_ROLE_ID=123456789012345678

# データベース設定
DATABASE_MAX_CONNECTIONS=10
```

## リバースプロキシ設定

本番環境ではNginx、Caddy、Traefikなどのリバースプロキシの使用を推奨します。

### Nginx設定例

```nginx
upstream fileserver {
    server 127.0.0.1:8080;
}

server {
    listen 443 ssl http2;
    server_name files.yourdomain.com;

    ssl_certificate /etc/nginx/ssl/cert.pem;
    ssl_certificate_key /etc/nginx/ssl/key.pem;

    # クライアントボディサイズ制限（アップロードサイズ）
    client_max_body_size 500G;

    # タイムアウト設定（大容量ファイル対応）
    proxy_connect_timeout 600s;
    proxy_send_timeout 600s;
    proxy_read_timeout 600s;
    send_timeout 600s;

    location / {
        proxy_pass http://fileserver;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocketサポート（将来の拡張用）
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }

    # アクセスログ
    access_log /var/log/nginx/fileserver.access.log;
    error_log /var/log/nginx/fileserver.error.log;
}
```

### Caddy設定例

```caddyfile
files.yourdomain.com {
    reverse_proxy localhost:8080 {
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-For {remote_host}
    }

    request_body {
        max_size 500GB
    }

    log {
        output file /var/log/caddy/fileserver.log
    }
}
```

### docker-compose.ymlでNginxを追加

```yaml
services:
  nginx:
    image: nginx:alpine
    container_name: fileserver-nginx
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx/nginx.conf:/etc/nginx/nginx.conf:ro
      - ./nginx/ssl:/etc/nginx/ssl:ro
    depends_on:
      - fileserver
    networks:
      - fileserver_network
    restart: unless-stopped
```

## 運用タスク

### ログ確認

```bash
# リアルタイムログ
docker compose logs -f fileserver

# 最新100行
docker compose logs --tail=100 fileserver

# 特定の時間範囲
docker compose logs --since 2024-01-01T00:00:00 fileserver

# エラーログのみ
docker compose logs fileserver | grep ERROR
```

### アプリケーションログ

ログはファイルではなく標準出力へJSON形式で出力され、Dockerのログドライバが収集します。

```bash
# アクセスログを含む全ログを確認（リアルタイム）
docker compose logs -f fileserver

# ログをファイルへ書き出す
docker compose logs --no-color fileserver > ./fileserver.log
```

### 再起動

```bash
# 通常の再起動
docker compose restart

# 完全な再起動（イメージ再取得）
docker compose down
docker compose pull
docker compose up -d

# 新しいイメージを取得してコンテナだけ差し替える
docker compose pull fileserver
docker compose up -d --no-deps fileserver
```

### 設定変更

環境変数を変更した場合：

```bash
# .envを編集
vi .env

# コンテナを再作成
docker compose up -d --force-recreate
```

### アップデート

新しいバージョンをデプロイ：

```bash
# .envでバージョンを変更
vi .env  # IMAGE_TAG=v1.1.0

# 新しいイメージをpull
docker compose pull

# ローリングアップデート
docker compose up -d
```

## バックアップ

### データベースバックアップ

SQLiteデータベースをバックアップ：

```bash
# データベースファイルをコピー
docker compose exec fileserver \
  cp /app/config/fileserver.db /app/config/fileserver.db.backup

# ホストにコピー
docker cp fileserver:/app/config/fileserver.db ./backup/fileserver_$(date +%Y%m%d).db

# 定期バックアップスクリプト（cron）
0 2 * * * cd /path/to/fileGo && docker cp fileserver:/app/config/fileserver.db ./backup/fileserver_$(date +\%Y\%m\%d).db
```

### アップロードファイルバックアップ

```bash
# アップロードディレクトリをtar.gz化
tar -czf uploads_backup_$(date +%Y%m%d).tar.gz data/uploads/

# rsyncで別サーバーにバックアップ
rsync -avz --delete data/uploads/ backup-server:/backup/fileserver/uploads/

# 定期バックアップスクリプト
0 3 * * * cd /path/to/fileGo && tar -czf ./backup/uploads_$(date +\%Y\%m\%d).tar.gz data/uploads/
```

### 完全バックアップスクリプト

```bash
#!/bin/bash
BACKUP_DIR="/backup/fileserver/$(date +%Y%m%d)"
mkdir -p "$BACKUP_DIR"

# データベースバックアップ
docker cp fileserver:/app/config/fileserver.db "$BACKUP_DIR/fileserver.db"

# アップロードファイルバックアップ
tar -czf "$BACKUP_DIR/uploads.tar.gz" data/uploads/

# 設定ファイルバックアップ
cp .env "$BACKUP_DIR/.env"
cp config/config.yaml "$BACKUP_DIR/config.yaml" 2>/dev/null || true

# 古いバックアップを削除（30日以前）
find /backup/fileserver/ -type d -mtime +30 -exec rm -rf {} +

echo "Backup completed: $BACKUP_DIR"
```

### リストア

```bash
# データベースリストア
docker cp ./backup/fileserver_20240101.db fileserver:/app/config/fileserver.db
docker compose restart

# アップロードファイルリストア
rm -rf data/uploads/*
tar -xzf ./backup/uploads_20240101.tar.gz -C .
docker compose restart
```

## 監視

### ヘルスチェック

```bash
# HTTPヘルスチェック
curl -f http://localhost:8080/health || echo "Service is down"

# Dockerヘルスチェック
docker inspect --format='{{.State.Health.Status}}' fileserver
```

### リソース監視

```bash
# コンテナのリソース使用状況
docker stats fileserver

# ディスク使用量
df -h
du -sh data/uploads/

# データベースサイズ
docker compose exec fileserver \
  ls -lh /app/config/fileserver.db
```

### 監視スクリプト例

```bash
#!/bin/bash
# healthcheck.sh - cronで定期実行

HEALTH_URL="http://localhost:8080/health"
ALERT_EMAIL="admin@yourdomain.com"

if ! curl -sf "$HEALTH_URL" > /dev/null; then
    echo "fileserver is down!" | mail -s "Alert: fileserver DOWN" "$ALERT_EMAIL"
    # 自動再起動
    cd /path/to/fileGo
    docker compose restart
fi

# ディスク使用量チェック（80%以上で警告）
DISK_USAGE=$(df -h /path/to/fileGo/data | awk 'NR==2 {print $5}' | sed 's/%//')
if [ "$DISK_USAGE" -gt 80 ]; then
    echo "Disk usage is ${DISK_USAGE}%" | mail -s "Alert: High disk usage" "$ALERT_EMAIL"
fi
```

### Prometheus + Grafana統合（オプション）

```yaml
# docker-compose.ymlに追加
services:
  prometheus:
    image: prom/prometheus
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
      - prometheus_data:/prometheus
    networks:
      - fileserver_network

  grafana:
    image: grafana/grafana
    ports:
      - "3000:3000"
    volumes:
      - grafana_data:/var/lib/grafana
    networks:
      - fileserver_network

volumes:
  prometheus_data:
  grafana_data:
```

## トラブルシューティング

### コンテナが起動しない

起動時の一般的な原因と対処は [セットアップガイドのトラブルシューティング](SETUP.md#dockerコンテナが起動しない) を参照してください。非rootユーザー(UID 65532)への移行後は、ホスト側 `config` / `data` ディレクトリの書き込み権限不足が主な原因になります（`sudo chown -R 65532:65532 ./config ./data`）。

### データベースロックエラー

```bash
# データベース接続を確認
docker compose exec fileserver \
  sqlite3 /app/config/fileserver.db "PRAGMA integrity_check;"

# コンテナを再起動
docker compose restart
```

### ディスク容量不足

```bash
# 不要なDockerリソースを削除
docker system prune -a --volumes

# 古いログを削除
find logs/ -name "*.log" -mtime +30 -delete

# アップロードファイルを確認
du -sh data/uploads/*
```

### パフォーマンス問題

```bash
# リソース制限を緩和（docker-compose.yml）
deploy:
  resources:
    limits:
      cpus: '4.0'
      memory: 4G

# データベース接続数を増やす（.env）
DATABASE_MAX_CONNECTIONS=20

# 再起動
docker compose up -d
```

## セキュリティベストプラクティス

### 環境変数の保護

```bash
# .envファイルのパーミッション設定
chmod 600 .env

# Gitにコミットしない
echo ".env" >> .gitignore
```

### Discord Tokenの定期ローテーション

Discord Developer Portalで定期的にTokenをリセットし、config.yamlを更新：

```bash
# config.yaml の auth.provider.bot_token を新しい値に変更
vi config/config.yaml

# 再起動
docker compose up -d --force-recreate
```

### SSL/TLS証明書

Let's Encryptを使用した自動更新：

```bash
# Certbot導入
docker run -it --rm \
  -v /etc/letsencrypt:/etc/letsencrypt \
  certbot/certbot certonly --standalone \
  -d files.yourdomain.com
```

## 次のステップ

- [API仕様](API.md) - エンドポイント一覧
- [セットアップガイド](SETUP.md) - 詳細なセットアップ手順
- [開発ガイド](DEVELOPMENT.md) - ローカル開発環境
