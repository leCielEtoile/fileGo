#!/bin/sh
set -e

echo "=========================================="
echo "fileGo - Discord File Server"
echo "=========================================="

# 必要なディレクトリを作成
echo "📁 Creating required directories..."
mkdir -p /app/config
mkdir -p /app/data
mkdir -p /app/logs

# config.yamlが存在しない場合、デフォルト設定を生成
if [ ! -f /app/config/config.yaml ]; then
    echo "⚙️  config.yaml not found. Generating default configuration..."

    cat > /app/config/config.yaml <<'EOF'
server:
  port: "8080"
  service_name: "Discord File Server"
  behind_proxy: true
  trusted_proxies:
    - "172.16.0.0/12"
    - "10.0.0.0/8"
    - "192.168.0.0/16"

discord:
  bot_token: "YOUR_BOT_TOKEN"
  client_id: "YOUR_CLIENT_ID"
  client_secret: "YOUR_CLIENT_SECRET"
  guild_id: "YOUR_GUILD_ID"
  redirect_url: "https://yourdomain.com/auth/callback"

database:
  path: "/app/config/fileserver.db"
  max_connections: 10

storage:
  upload_path: "/app/data"
  max_file_size: 104857600
  chunk_upload_enabled: true
  chunk_size: 20971520
  max_chunk_file_size: 536870912000
  max_concurrent_uploads: 3
  upload_session_ttl: 48h
  cleanup_interval: 1h
  admin_role_id: "123456789012345678"
  directories:
    - path: "user"
      required_roles: []
      permissions: ["read", "write", "delete"]
      type: "user_private"
    - path: "public"
      required_roles: []
      permissions: ["read", "write"]
    - path: "admin"
      required_roles: ["123456789012345678"]
      permissions: ["read", "write", "delete"]
EOF

    echo "✅ Default config.yaml created at /app/config/config.yaml"
    echo ""
    echo "⚠️  IMPORTANT: Please configure the following settings:"
    echo "   - Set environment variables for Discord (DISCORD_BOT_TOKEN, etc.)"
    echo "   - Or edit /app/config/config.yaml manually"
    echo ""
else
    echo "✅ config.yaml already exists"
fi

# 設定確認
echo ""
echo "📋 Configuration:"
echo "   Config file: /app/config/config.yaml"
echo "   Database: ${DATABASE_PATH:-/app/config/fileserver.db}"
echo "   Data path: ${STORAGE_UPLOAD_PATH:-/app/data}"
echo "   Server port: ${SERVER_PORT:-8080}"
echo "   Service name: ${SERVER_SERVICE_NAME:-Discord File Server}"

# 環境変数での設定を確認
echo ""
echo "🔐 Environment variables:"
if [ -n "$DISCORD_BOT_TOKEN" ] && [ "$DISCORD_BOT_TOKEN" != "YOUR_BOT_TOKEN" ]; then
    echo "   ✅ DISCORD_BOT_TOKEN: Set"
else
    echo "   ⚠️  DISCORD_BOT_TOKEN: Not set or using placeholder"
fi

if [ -n "$DISCORD_CLIENT_ID" ] && [ "$DISCORD_CLIENT_ID" != "YOUR_CLIENT_ID" ]; then
    echo "   ✅ DISCORD_CLIENT_ID: Set"
else
    echo "   ⚠️  DISCORD_CLIENT_ID: Not set or using placeholder"
fi

if [ -n "$DISCORD_CLIENT_SECRET" ] && [ "$DISCORD_CLIENT_SECRET" != "YOUR_CLIENT_SECRET" ]; then
    echo "   ✅ DISCORD_CLIENT_SECRET: Set"
else
    echo "   ⚠️  DISCORD_CLIENT_SECRET: Not set or using placeholder"
fi

if [ -n "$DISCORD_GUILD_ID" ] && [ "$DISCORD_GUILD_ID" != "YOUR_GUILD_ID" ]; then
    echo "   ✅ DISCORD_GUILD_ID: Set"
else
    echo "   ⚠️  DISCORD_GUILD_ID: Not set or using placeholder"
fi

echo ""
echo "🚀 Starting fileserver..."
echo "=========================================="
echo ""

# アプリケーションを実行
exec "$@"
