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
docker compose -f docker-compose.deploy.yml pull
docker compose -f docker-compose.deploy.yml up -d

# 4. 起動確認
docker compose -f docker-compose.deploy.yml ps
docker compose -f docker-compose.deploy.yml logs -f
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

### デプロイ後の確認

```bash
# ヘルスチェック
curl http://localhost:8080/health

# 起動ログの確認
docker compose -f docker-compose.deploy.yml logs fileserver | head -50

# 環境変数の確認
docker compose -f docker-compose.deploy.yml logs fileserver | grep "Environment variables"
```

## 環境変数設定

### 必須環境変数

本番環境では必ず以下の環境変数を設定してください：

```bash
# Discord設定（必須）
DISCORD_BOT_TOKEN=your_actual_bot_token
DISCORD_CLIENT_ID=your_client_id
DISCORD_CLIENT_SECRET=your_client_secret
DISCORD_GUILD_ID=your_guild_id
DISCORD_REDIRECT_URL=https://yourdomain.com/auth/callback
```

### 推奨環境変数

セキュリティとパフォーマンスのため、以下も設定することを推奨：

```bash
# サーバー設定
SERVER_PORT=8080
SERVER_BEHIND_PROXY=true
SERVER_TRUSTED_PROXIES=172.16.0.0/12,10.0.0.0/8

# ストレージ設定
STORAGE_MAX_FILE_SIZE=104857600  # 100MB
STORAGE_MAX_CHUNK_FILE_SIZE=536870912000  # 500GB

# データベース設定
DATABASE_MAX_CONNECTIONS=10
```

### 環境別の設定例

#### 開発環境
```bash
IMAGE_TAG=latest
HOST_PORT=8080
DISCORD_REDIRECT_URL=http://localhost:8080/auth/callback
STORAGE_MAX_FILE_SIZE=52428800  # 50MB（開発用に小さく）
```

#### ステージング環境
```bash
IMAGE_TAG=main-latest
HOST_PORT=8080
DISCORD_REDIRECT_URL=https://staging.yourdomain.com/auth/callback
SERVER_BEHIND_PROXY=true
```

#### 本番環境
```bash
IMAGE_TAG=v1.0.0
HOST_PORT=8080
DISCORD_REDIRECT_URL=https://files.yourdomain.com/auth/callback
SERVER_BEHIND_PROXY=true
SERVER_TRUSTED_PROXIES=172.16.0.0/12
STORAGE_MAX_FILE_SIZE=104857600
DATABASE_MAX_CONNECTIONS=20
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

### docker-compose.deploy.ymlでNginxを追加

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
docker compose -f docker-compose.deploy.yml logs -f fileserver

# 最新100行
docker compose -f docker-compose.deploy.yml logs --tail=100 fileserver

# 特定の時間範囲
docker compose -f docker-compose.deploy.yml logs --since 2024-01-01T00:00:00 fileserver

# エラーログのみ
docker compose -f docker-compose.deploy.yml logs fileserver | grep ERROR
```

### アプリケーションログ

コンテナ内の `/root/logs/` にログが保存されます：

```bash
# アクセスログ確認
docker compose -f docker-compose.deploy.yml exec fileserver cat /root/logs/access.log

# ログをホストにコピー
docker cp fileserver:/root/logs/access.log ./logs/
```

### 再起動

```bash
# 通常の再起動
docker compose -f docker-compose.deploy.yml restart

# 完全な再起動（イメージ再取得）
docker compose -f docker-compose.deploy.yml down
docker compose -f docker-compose.deploy.yml pull
docker compose -f docker-compose.deploy.yml up -d

# ゼロダウンタイム再起動
docker compose -f docker-compose.deploy.yml up -d --no-deps --build fileserver
```

### 設定変更

環境変数を変更した場合：

```bash
# .envを編集
vi .env

# コンテナを再作成
docker compose -f docker-compose.deploy.yml up -d --force-recreate
```

### アップデート

新しいバージョンをデプロイ：

```bash
# .envでバージョンを変更
vi .env  # IMAGE_TAG=v1.1.0

# 新しいイメージをpull
docker compose -f docker-compose.deploy.yml pull

# ローリングアップデート
docker compose -f docker-compose.deploy.yml up -d
```

## バックアップ

### データベースバックアップ

SQLiteデータベースをバックアップ：

```bash
# データベースファイルをコピー
docker compose -f docker-compose.deploy.yml exec fileserver \
  cp /root/config/fileserver.db /root/config/fileserver.db.backup

# ホストにコピー
docker cp fileserver:/root/config/fileserver.db ./backup/fileserver_$(date +%Y%m%d).db

# 定期バックアップスクリプト（cron）
0 2 * * * cd /path/to/fileGo && docker cp fileserver:/root/config/fileserver.db ./backup/fileserver_$(date +\%Y\%m\%d).db
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
docker cp fileserver:/root/config/fileserver.db "$BACKUP_DIR/fileserver.db"

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
docker cp ./backup/fileserver_20240101.db fileserver:/root/config/fileserver.db
docker compose -f docker-compose.deploy.yml restart

# アップロードファイルリストア
rm -rf data/uploads/*
tar -xzf ./backup/uploads_20240101.tar.gz -C .
docker compose -f docker-compose.deploy.yml restart
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
docker compose -f docker-compose.deploy.yml exec fileserver \
  ls -lh /root/config/fileserver.db
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
    docker compose -f docker-compose.deploy.yml restart
fi

# ディスク使用量チェック（80%以上で警告）
DISK_USAGE=$(df -h /path/to/fileGo/data | awk 'NR==2 {print $5}' | sed 's/%//')
if [ "$DISK_USAGE" -gt 80 ]; then
    echo "Disk usage is ${DISK_USAGE}%" | mail -s "Alert: High disk usage" "$ALERT_EMAIL"
fi
```

### Prometheus + Grafana統合（オプション）

```yaml
# docker-compose.deploy.ymlに追加
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

```bash
# ログを確認
docker compose -f docker-compose.deploy.yml logs

# コンテナの状態確認
docker compose -f docker-compose.deploy.yml ps -a

# 強制再作成
docker compose -f docker-compose.deploy.yml down -v
docker compose -f docker-compose.deploy.yml up -d
```

### データベースロックエラー

```bash
# データベース接続を確認
docker compose -f docker-compose.deploy.yml exec fileserver \
  sqlite3 /root/config/fileserver.db "PRAGMA integrity_check;"

# コンテナを再起動
docker compose -f docker-compose.deploy.yml restart
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
# リソース制限を緩和（docker-compose.deploy.yml）
deploy:
  resources:
    limits:
      cpus: '4.0'
      memory: 4G

# データベース接続数を増やす（.env）
DATABASE_MAX_CONNECTIONS=20

# 再起動
docker compose -f docker-compose.deploy.yml up -d
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

Discord Developer Portalで定期的にTokenをリセットし、環境変数を更新：

```bash
# .envを更新
vi .env  # DISCORD_BOT_TOKENを新しい値に変更

# 再起動
docker compose -f docker-compose.deploy.yml up -d --force-recreate
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
