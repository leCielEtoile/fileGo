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

- Go 1.26以降
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

開発時は認証情報を含む `config.yaml` を用意します（認証設定は環境変数では上書きできません）。開発用の `redirect_url` は `http://localhost:8080/auth/callback` を使い、Discord Developer Portal の Redirects にも同じURLを登録します。HTTP開発では `secure_cookie: false` にしてください。サーバー系の一部（`SERVER_PORT` など）は環境変数で上書きできます。

```bash
cp config.yaml.example config.yaml
vi config.yaml   # auth.provider と secure_cookie: false を設定
```

テンプレートと静的ファイルは埋め込み済みのため、リポジトリルートから実行します。

```bash
# そのまま実行
go run .

# もしくはビルドして実行
go build -o fileserver .
./fileserver
```

ファイル変更時に自動で再ビルド・再起動したい場合は Air を使います。

```bash
go install github.com/air-verse/air@latest

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

air
```

Docker Compose での起動手順は [セットアップガイドの起動方法](SETUP.md#起動方法) を参照してください。

## プロジェクト構成

情報の重複による陳腐化を避けるため、構成の一次情報は他ドキュメントに集約しています。

- ディレクトリ構造（ファイル配置）: [README.md](../README.md#ディレクトリ構造)
- 各パッケージの責務と設計意図: [AI-CONTEXT.md](AI-CONTEXT.md#3-パッケージ構成と責務)

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

すべてのプッシュとプルリクエストで実行（Dockerのビルドもこのワークフローに含まれる）：

```yaml
トリガー: push (main, develop, v*.*.*), pull_request
ジョブ:
  - lint: golangci-lintによるコード品質チェック
  - test: go test -race（全パッケージのコンパイル検証を兼ねる）
  - build: 複数プラットフォーム向けバイナリビルド（非PR時のみ）
    - Linux (amd64, arm64) / macOS (amd64, arm64) / Windows (amd64)
  - security-scan: GosecのSARIFをGitHub Securityへ送信
  - docker: イメージビルドとghcr.ioへのpush、Trivyスキャン
    - PR時はAMD64のみビルドし、pushせずtarballをスキャン
    - CRITICALかつ修正済みの脆弱性が残る場合はジョブを失敗させる
  - docker-manifest: マルチプラットフォームのマニフェスト作成とSBOM生成
  - docker-compose-test: composeで起動しヘルスチェックを検証
```

Actionはすべて正確なバージョンへ固定し、更新はDependabot（`.github/dependabot.yml`）で追従する。Goのバージョンは各ワークフローで重複定義せず `go-version-file: go.mod` により `go.mod` を単一の情報源とする。

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
        "SERVER_PORT": "8080"
      }
    }
  ]
}
```

> 認証情報は環境変数では設定できません。`config.yaml` の `auth.provider` に記述してください（`config.yaml` はワークスペースルートに配置）。

## 次のステップ

- [API仕様](API.md) - エンドポイント一覧
- [デプロイガイド](DEPLOYMENT.md) - 本番環境での運用
- [セットアップガイド](SETUP.md) - 詳細なセットアップ手順
