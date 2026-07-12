# セキュリティポリシー

fileGo は認証・認可を伴うファイル共有サーバーであり、セキュリティを重視しています。脆弱性の報告に感謝します。

## サポート対象バージョン

原則として **`main` ブランチの最新版**および直近のリリースを対象に修正を行います。旧バージョンへのバックポートは保証しません。

## 脆弱性の報告

**公開Issueで脆弱性を報告しないでください。** 詳細が公開されると悪用される恐れがあります。

以下のいずれかの非公開経路でご連絡ください。

1. **GitHub の Private Vulnerability Reporting（推奨）**
   リポジトリの **Security → Report a vulnerability** から報告できます。
   （<https://github.com/leCielEtoile/fileGo/security/advisories/new>）
2. 上記が使えない場合は、リポジトリオーナー（[@leCielEtoile](https://github.com/leCielEtoile)）へ非公開で連絡してください。

報告には可能な範囲で以下を含めてください。
- 影響を受けるエンドポイント／コンポーネント（例: 認可、チャンクアップロード、パス処理）
- 再現手順、または PoC
- 想定される影響（情報漏えい・権限昇格・DoS など）
- 該当バージョン／コミット

## 対応の目安

- **受領確認**: 数日以内を目標
- **初期評価と重大度判定**: 受領後できるだけ速やかに
- **修正と公開**: 重大度に応じて調整。修正リリース後に Security Advisory を公開します

責任ある開示に協力いただいた報告者は、希望があれば Advisory 内で謝辞を記載します。

## このプロジェクトのセキュリティ設計（参考）

報告の切り分けに役立つよう、主な防御の所在を示します。詳細は [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) を参照してください。

- **認証**: OAuth2/OIDC。セッションは `HttpOnly` Cookie（`session_token`、7日）。CSRF は `state` Cookie 照合＋`SameSite=Lax`。
- **認可**: grants ベース（ロール／ユーザー／`*` × read/write/delete）。`/admin` はサーバー側 `AdminMiddleware` で保護（UIの出し分けは導線に過ぎない）。
- **パス処理**: アップロード名のサニタイズ、`..` 排除、ユーザー名のディレクトリ正規化（`SanitizeDirName`）による多層防御。
- **アップロード**: `MaxBytesReader` による上限、チャンクの所有者・範囲・サイズ検証、`upload_id` のUUID検証。
- **秘密情報**: 認証情報は `config.yaml` のみ（環境変数上書き不可）。`config.yaml`・`.mcp.json`・`*.db` は Git 管理外。
- **配信の情報遮断**: SSE は接続ユーザーの読み取り可能ディレクトリのみへイベント配信（他者のプライベート領域を漏らさない）。

## 対象外（Out of Scope）の例

- テスト専用の Mock OIDC スタック（`docker-compose.oidc.yml`）に固有の挙動
- 運用者の設定ミス（例: リバースプロキシ不備、`config.yaml` の権限過剰付与）
- 有効な管理者ロール保有者による正当な操作
