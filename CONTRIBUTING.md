# コントリビューションガイド

Issue・Pull Request を歓迎します。このドキュメントは貢献をスムーズに進めるための指針です。

## はじめに

- **バグ報告・機能提案**: まず [Issue](https://github.com/leCielEtoile/fileGo/issues) を作成してください。再現手順・期待する挙動・環境（OS / デプロイ方法）を添えていただけると助かります。
- **セキュリティ上の問題**: Issue ではなく [SECURITY.md](SECURITY.md) の非公開経路で報告してください。
- **大きな変更**: 実装前に Issue で方針を相談すると、手戻りを防げます。

## 開発環境

セットアップ・テスト・CI の詳細は [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) を参照してください。プロジェクト全体像と設計判断（不変条件）は [docs/AI-CONTEXT.md](docs/AI-CONTEXT.md) と [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) にまとまっています。

## Pull Request の流れ

1. リポジトリを fork し、`main` からブランチを作成する（例: `feat/xxx`、`fix/xxx`、`docs/xxx`）。
2. 変更を実装する。
3. **提出前に以下がすべて通ることを確認する。**
   ```bash
   gofmt -l .        # 出力が空であること（フォーマット逸脱なし）
   go vet ./...
   go build ./...
   go test -race ./...
   golangci-lint run # 導入している場合
   ```
4. Pull Request を作成し、変更の目的（なぜ）と概要を記載する。UI 変更にはスクリーンショットがあると助かります。
5. CI（lint / test / build / 脆弱性スキャン）がグリーンであることを確認する。

## コーディング規約

- **言語**: コード内コメント・ログ・ユーザー向けメッセージは**日本語**。
- **コメントの役割**: コード=how／why-not、テスト=what、コミットログ=why。思考過程のコメントは残さない。godoc は識別子名で始める。
- **フォーマット**: `gofmt` 準拠。周辺コードの命名・スタイルに合わせる。
- **テスト**: セキュリティ・認可・パス処理に関わる変更にはテストを添える。テストコードは「何を保証するか（what）」を表す。

## コミットメッセージ

**日本語の [Conventional Commits](https://www.conventionalcommits.org/ja/)** を用います。

```
<type>(<scope>): <要約>

<本文: なぜこの変更が必要かを説明>
```

- `type` の例: `feat` / `fix` / `refactor` / `docs` / `test` / `chore` / `perf`
- 破壊的変更は `type!` とし、本文に `BREAKING CHANGE:` を記載する。
- 例: `fix(permission): user_private の read 判定を修正`

## 認可・セキュリティに関わる変更の注意

`permission` / `middleware` / `authprovider` は機微です。変更時は [docs/AI-CONTEXT.md](docs/AI-CONTEXT.md) の不変条件と [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) の設計意図に反しないか確認してください。特に次を守ってください。

- **認証情報は `config.yaml` のみ。** 環境変数で `bot_token` / `client_secret` を扱う機能を復活させない。
- **`config.yaml` はコミットしない**（秘密情報を含む。正は `config.yaml.example`）。
- エラーレスポンスはプレーンテキスト、成功系は JSON（`handler/helpers.go` の `writeJSON`）。

## ライセンス

コントリビューションは本プロジェクトと同じ **BSD 3-Clause License** の下で提供されることに同意したものとみなされます。
