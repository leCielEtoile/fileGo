# 開発ガイド

fileGoのローカル開発環境のセットアップとCI/CD情報を説明します。

## 目次

- [ローカル開発環境](#ローカル開発環境)
- [プロジェクト構成](#プロジェクト構成)
- [テスト](#テスト)
- [CI/CD](#cicd)
- [リリース手順](#リリース手順)
- [貢献ガイドライン](#貢献ガイドライン)

## ローカル開発環境

### 前提条件

- Go 1.23以降
- Docker & Docker Compose
- Git
- golangci-lint（オプション）

### セットアップ

```bash
# リポジトリをクローン
git clone https://github.com/leCielEtoile/fileGo.git
cd fileGo

# 依存関係のインストール
go mod download

# 設定ファイルを作成
cp config.yaml.example config.yaml
vi config.yaml  # Discord情報を設定

# 必要なディレクトリを作成
mkdir -p config data/uploads logs
```

### ローカルで実行

#### 方法1: Goで直接実行

```bash
# ビルド
go build -o fileserver .

# 実行
./fileserver

# バックグラウンド実行
nohup ./fileserver > logs/app.log 2>&1 &
```

#### 方法2: Docker Composeで実行

```bash
# ビルド＆起動
docker compose up -d --build

# ログ確認
docker compose logs -f

# 停止
docker compose down
```

#### 方法3: ホットリロード（Air使用）

```bash
# Airをインストール
go install github.com/cosmtrek/air@latest

# .air.tomlを作成
cat > .air.toml <<EOF
root = "."
tmp_dir = "tmp"

[build]
  cmd = "go build -o ./tmp/fileserver ."
  bin = "tmp/fileserver"
  include_ext = ["go"]
  exclude_dir = ["tmp", "vendor"]
  delay = 1000
EOF

# 起動（ファイル変更時に自動再起動）
air
```

### 環境変数での開発

開発時は環境変数を使用すると便利です：

```bash
# 開発用の.envを作成
cat > .env.dev <<EOF
DISCORD_BOT_TOKEN=your_dev_bot_token
DISCORD_CLIENT_ID=your_dev_client_id
DISCORD_CLIENT_SECRET=your_dev_client_secret
DISCORD_GUILD_ID=your_dev_guild_id
DISCORD_REDIRECT_URL=http://localhost:8080/auth/callback
SERVER_PORT=8080
EOF

# 環境変数を読み込んで実行
export $(cat .env.dev | xargs) && go run .
```

## プロジェクト構成

### ディレクトリ構造

```
fileGo/
├── main.go                      # エントリーポイント
├── internal/                    # 内部パッケージ
│   ├── config/                  # 設定管理
│   │   └── config.go           # Config構造体、環境変数読み込み
│   ├── database/                # データベース
│   │   ├── database.go         # SQLite初期化
│   │   └── schema.go           # スキーマ定義
│   ├── discord/                 # Discord API
│   │   ├── client.go           # Discord APIクライアント
│   │   └── oauth.go            # OAuth2認証
│   ├── handler/                 # HTTPハンドラー
│   │   ├── auth.go             # 認証エンドポイント
│   │   ├── file.go             # ファイル操作エンドポイント
│   │   └── chunk.go            # チャンクアップロード
│   ├── middleware/              # ミドルウェア
│   │   ├── auth.go             # 認証ミドルウェア
│   │   └── logging.go          # ログミドルウェア
│   ├── models/                  # データモデル
│   │   ├── user.go             # ユーザーモデル
│   │   ├── session.go          # セッションモデル
│   │   └── upload.go           # アップロードモデル
│   ├── permission/              # 権限管理
│   │   └── checker.go          # ロールベース権限チェック
│   └── storage/                 # ストレージ管理
│       ├── storage.go          # ファイル操作
│       └── chunk.go            # チャンクアップロード処理
├── docs/                        # ドキュメント
│   ├── SETUP.md
│   ├── API.md
│   ├── DEPLOYMENT.md
│   └── DEVELOPMENT.md
├── .github/workflows/           # GitHub Actions
│   ├── ci.yml                  # CI（テスト、ビルド）
│   ├── docker.yml              # Dockerビルド＆Push
│   └── release.yml             # リリース自動化
├── config.yaml.example          # 設定ファイルサンプル
├── .env.example                 # 環境変数サンプル
├── Dockerfile                   # Dockerイメージ定義
├── docker-compose.yml           # 開発用
├── docker-compose.deploy.yml    # 本番用
├── entrypoint.sh                # コンテナ起動スクリプト
├── go.mod                       # Go依存関係
└── go.sum                       # Go依存関係チェックサム
```

### パッケージ設計

- **internal/config**: 設定ファイル・環境変数の読み込み
- **internal/database**: SQLiteデータベース初期化とスキーマ管理
- **internal/discord**: Discord OAuth2とBot API
- **internal/handler**: HTTPエンドポイントのハンドラー
- **internal/middleware**: 認証、ログなどのミドルウェア
- **internal/models**: データベースモデル
- **internal/permission**: ロールベース権限チェック
- **internal/storage**: ファイル操作とチャンクアップロード

## テスト

### ユニットテスト

```bash
# 全テスト実行
go test ./...

# 詳細出力
go test -v ./...

# カバレッジ
go test -cover ./...

# カバレッジレポート生成
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

### 特定パッケージのテスト

```bash
# config パッケージのテスト
go test -v ./internal/config

# storage パッケージのテスト
go test -v ./internal/storage
```

### ベンチマーク

```bash
# ベンチマーク実行
go test -bench=. ./...

# メモリ使用量も表示
go test -bench=. -benchmem ./...
```

### Lintチェック

```bash
# golangci-lintをインストール
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Lint実行
golangci-lint run

# 自動修正
golangci-lint run --fix
```

### テストカバレッジ目標

- **全体**: 70%以上
- **handler**: 80%以上
- **permission**: 90%以上

## CI/CD

fileGoはGitHub Actionsを使用した自動化されたCI/CDパイプラインを備えています。

### ワークフロー

#### 1. CI Workflow (`.github/workflows/ci.yml`)

すべてのプッシュとプルリクエストで実行：

```yaml
トリガー: push, pull_request
ジョブ:
  - Lint: golangci-lintによるコード品質チェック
  - Test: ユニットテスト実行とカバレッジレポート
  - Build: 複数プラットフォーム向けビルド
    - Linux (amd64, arm64)
    - macOS (amd64, arm64)
    - Windows (amd64)
  - Security: Trivy & Gosecによる脆弱性スキャン
```

#### 2. Docker Workflow (`.github/workflows/docker.yml`)

mainブランチへのプッシュとタグ作成時に実行：

```yaml
トリガー: push to main, tags (v*.*.*)
ジョブ:
  - Build: マルチプラットフォームDockerイメージビルド
  - Push: GitHub Container Registry (ghcr.io)へpush
  - Scan: Trivyによるイメージ脆弱性スキャン
  - SBOM: ソフトウェア部品表の生成
  - Test: Docker Composeでヘルスチェック
```

**イメージタグ:**
- `latest`: mainブランチの最新
- `v1.0.0`: バージョンタグ
- `main-abc1234`: コミットハッシュ

#### 3. Release Workflow (`.github/workflows/release.yml`)

バージョンタグ（`v*.*.*`）プッシュ時に実行：

```yaml
トリガー: tags (v*.*.*)
ジョブ:
  - Changelog: コミット履歴からリリースノート自動生成
  - Build: 全プラットフォーム向けバイナリビルド
  - Release: GitHub Releaseの作成
  - Docker: リリースタグ付きDockerイメージpush
```

### ローカルでCI環境を再現

```bash
# Lintチェック
golangci-lint run

# テスト実行
go test -cover ./...

# ビルドテスト（全プラットフォーム）
GOOS=linux GOARCH=amd64 go build -o build/fileserver-linux-amd64 .
GOOS=linux GOARCH=arm64 go build -o build/fileserver-linux-arm64 .
GOOS=darwin GOARCH=amd64 go build -o build/fileserver-darwin-amd64 .
GOOS=darwin GOARCH=arm64 go build -o build/fileserver-darwin-arm64 .
GOOS=windows GOARCH=amd64 go build -o build/fileserver-windows-amd64.exe .

# 脆弱性スキャン
go install github.com/securego/gosec/v2/cmd/gosec@latest
gosec ./...
```

## リリース手順

### バージョニング

セマンティックバージョニング（SemVer）を使用：

- **MAJOR**: 互換性のない変更
- **MINOR**: 後方互換性のある機能追加
- **PATCH**: 後方互換性のあるバグ修正

例: `v1.2.3`

### リリース手順

```bash
# 1. mainブランチを最新化
git checkout main
git pull origin main

# 2. バージョンタグを作成
git tag -a v1.0.0 -m "Release v1.0.0

新機能:
- Discord OAuth2認証
- チャンクアップロード対応
- ロールベース権限管理

バグ修正:
- ファイルダウンロード時のメモリリーク修正
"

# 3. タグをプッシュ
git push origin v1.0.0
```

これにより以下が自動実行されます：
1. 全プラットフォーム向けバイナリのビルド
2. GitHub Releaseの作成
3. Dockerイメージのビルドと公開（`ghcr.io/lecieletoile/filego:v1.0.0`）
4. リリースノートの生成

### プレリリース

```bash
# ベータ版のリリース
git tag -a v1.0.0-beta.1 -m "Beta release v1.0.0-beta.1"
git push origin v1.0.0-beta.1
```

### リリース後の確認

```bash
# GitHub Releaseページで確認
open https://github.com/leCielEtoile/fileGo/releases

# Dockerイメージの確認
docker pull ghcr.io/lecieletoile/filego:v1.0.0
docker run --rm ghcr.io/lecieletoile/filego:v1.0.0 --version
```

## 貢献ガイドライン

### ブランチ戦略

- **main**: 本番環境にデプロイ可能な安定版
- **feature/xxx**: 新機能開発
- **fix/xxx**: バグ修正
- **docs/xxx**: ドキュメント更新

### プルリクエスト手順

```bash
# 1. フォークしてクローン
git clone https://github.com/YOUR_USERNAME/fileGo.git
cd fileGo

# 2. ブランチを作成
git checkout -b feature/add-new-feature

# 3. 変更をコミット
git add .
git commit -m "feat: 新機能の追加"

# 4. プッシュ
git push origin feature/add-new-feature

# 5. GitHub上でプルリクエストを作成
```

### コミットメッセージ規約

Conventional Commitsに従う：

```
<type>: <subject>

<body>
```

**Type:**
- `feat`: 新機能
- `fix`: バグ修正
- `docs`: ドキュメント変更
- `style`: フォーマット変更（コード動作に影響なし）
- `refactor`: リファクタリング
- `test`: テスト追加・修正
- `chore`: ビルド・設定変更

**例:**
```
feat: チャンクアップロードのレジューム機能を追加

- アップロード状態の永続化を実装
- /files/chunk/status エンドポイントを追加
- クライアント側のレジューム処理を実装

Closes #123
```

### コードスタイル

```bash
# フォーマット
go fmt ./...

# インポート整理
goimports -w .

# Lintチェック
golangci-lint run
```

### プルリクエストチェックリスト

- [ ] テストが追加されているか
- [ ] Lintチェックが通っているか
- [ ] ドキュメントが更新されているか
- [ ] コミットメッセージが規約に従っているか
- [ ] 変更内容が説明されているか

### レビュー基準

- **機能性**: 期待通りに動作するか
- **テスト**: 十分なテストカバレッジがあるか
- **パフォーマンス**: パフォーマンス低下がないか
- **セキュリティ**: セキュリティリスクがないか
- **可読性**: コードが読みやすいか

## 開発ツール

### 推奨VSCode拡張機能

```json
{
  "recommendations": [
    "golang.go",
    "ms-azuretools.vscode-docker",
    "eamodio.gitlens",
    "editorconfig.editorconfig"
  ]
}
```

### デバッグ設定 (`.vscode/launch.json`)

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Launch fileserver",
      "type": "go",
      "request": "launch",
      "mode": "debug",
      "program": "${workspaceFolder}",
      "env": {
        "DISCORD_BOT_TOKEN": "your_dev_token",
        "DISCORD_CLIENT_ID": "your_dev_client_id"
      }
    }
  ]
}
```

## 次のステップ

- [API仕様](API.md) - エンドポイント一覧
- [デプロイガイド](DEPLOYMENT.md) - 本番環境での運用
- [セットアップガイド](SETUP.md) - 詳細なセットアップ手順
