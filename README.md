# Discord File Server

[![CI](https://github.com/leCielEtoile/fileGo/actions/workflows/ci.yml/badge.svg)](https://github.com/leCielEtoile/fileGo/actions/workflows/ci.yml)
[![Docker](https://github.com/leCielEtoile/fileGo/actions/workflows/docker.yml/badge.svg)](https://github.com/leCielEtoile/fileGo/actions/workflows/docker.yml)
[![Release](https://github.com/leCielEtoile/fileGo/actions/workflows/release.yml/badge.svg)](https://github.com/leCielEtoile/fileGo/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/leCielEtoile/fileGo)](https://goreportcard.com/report/github.com/leCielEtoile/fileGo)
[![License](https://img.shields.io/badge/License-BSD_3--Clause-blue.svg)](https://opensource.org/licenses/BSD-3-Clause)

Discord OAuth2認証とロールベース権限管理を備えたセキュアなファイル共有サーバー

## 特徴

- 🔐 **Discord OAuth2認証** - Discordアカウントでログイン
- 👥 **ロールベース権限管理** - Discordのロールに基づいたアクセス制御
- 📤 **通常アップロード** - 最大100MBのファイル
- 📦 **チャンクアップロード** - 最大500GBの大容量ファイル（レジューム対応）
- 🎬 **Range Request対応** - 動画ストリーミング可能
- 🔒 **セキュアな設計** - パストラバーサル対策、セッション管理、アクセスログ

## クイックスタート

### 前提条件

- Docker & Docker Compose
- Discord アカウント
- Discord サーバーの管理者権限

### Discord Application設定

1. [Discord Developer Portal](https://discord.com/developers/applications) でアプリケーション作成
2. **OAuth2**: Client ID, Client Secret, Redirect URL (`https://yourdomain.com/auth/callback`) を設定
3. **Bot**: Bot Tokenを取得し、`SERVER MEMBERS INTENT`を有効化
4. **Bot招待**: 生成されたURLでBotをサーバーに招待
5. **Role ID取得**: サーバー設定からロールIDをコピー

詳細は [Discord設定ガイド](docs/SETUP.md#discord-application作成) を参照

### デプロイ

```bash
# 1. 必要なファイルをダウンロード
mkdir fileGo && cd fileGo
curl -O https://raw.githubusercontent.com/leCielEtoile/fileGo/main/docker-compose.deploy.yml
curl -O https://raw.githubusercontent.com/leCielEtoile/fileGo/main/.env.example

# 2. 環境変数を設定
cp .env.example .env
vi .env  # Discord設定を編集（必須）

# 3. 起動（config.yaml・ディレクトリは自動生成）
docker compose -f docker-compose.deploy.yml up -d

# 4. ログ確認
docker compose -f docker-compose.deploy.yml logs -f
```

**最低限必要な環境変数 (.env):**
```bash
DISCORD_BOT_TOKEN=your_bot_token_here
DISCORD_CLIENT_ID=your_client_id_here
DISCORD_CLIENT_SECRET=your_client_secret_here
DISCORD_GUILD_ID=your_guild_id_here
DISCORD_REDIRECT_URL=https://yourdomain.com/auth/callback
```

サーバーが起動したら `http://localhost:8080/health` でヘルスチェック

> 💡 **開発者向け**: ソースコードから開発する場合は [開発ガイド](docs/DEVELOPMENT.md) を参照

## 技術スタック

- **言語**: Go 1.23
- **データベース**: SQLite (modernc.org/sqlite)
- **認証**: Discord OAuth2
- **ルーター**: chi/v5
- **コンテナ**: Docker / Docker Compose

## ドキュメント

- 📖 [セットアップガイド](docs/SETUP.md) - 詳細なインストール手順
- 🚀 [デプロイ・運用ガイド](docs/DEPLOYMENT.md) - 本番環境での運用方法
- 📡 [API仕様](docs/API.md) - エンドポイント一覧と使用方法
- 💻 [開発ガイド](docs/DEVELOPMENT.md) - ローカル開発・CI/CD情報

## アーキテクチャ

```
┌─────────────────────────────────────────────┐
│ Discord OAuth2 認証                         │
├─────────────────────────────────────────────┤
│ ロールベース権限チェック                     │
│ (Discord Bot API - 5分キャッシュ)           │
├─────────────────────────────────────────────┤
│ ファイル操作                                 │
│ - 通常アップロード (100MB)                   │
│ - チャンクアップロード (500GB, レジューム)   │
│ - Range Request ダウンロード                 │
├─────────────────────────────────────────────┤
│ SQLite データベース                          │
│ (users, sessions, access_logs)              │
└─────────────────────────────────────────────┘
```

## ディレクトリ構造

```
fileGo/
├── main.go                    # エントリーポイント
├── internal/
│   ├── config/               # 設定管理（環境変数サポート）
│   ├── database/             # データベース初期化
│   ├── discord/              # Discord API クライアント
│   ├── handler/              # HTTPハンドラー
│   ├── middleware/           # ミドルウェア
│   ├── models/               # データモデル
│   ├── permission/           # 権限チェッカー
│   └── storage/              # ストレージ管理
├── docs/                     # ドキュメント
├── config.yaml.example       # 設定サンプル
├── .env.example              # 環境変数サンプル
├── Dockerfile
├── docker-compose.yml        # 開発用
├── docker-compose.deploy.yml # 本番用
└── entrypoint.sh             # 起動スクリプト
```

## ライセンス

BSD 3-Clause License - 詳細は [LICENSE](LICENSE) を参照

## 貢献

Issue・Pull Requestを歓迎します。

## 作者

leCielEtoile ([@leCielEtoile](https://github.com/leCielEtoile))
