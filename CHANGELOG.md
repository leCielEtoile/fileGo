# 変更履歴

本プロジェクトの重要な変更を記録します。
書式は [Keep a Changelog](https://keepachangelog.com/ja/1.1.0/) に、バージョニングは [Semantic Versioning](https://semver.org/lang/ja/) に準拠します。

## [Unreleased]

### Fixed（修正）
- **Discord APIを叩き続けてBotトークンが濫用検知される問題を修正**（重大）。次の4点が原因だった。
  - discordgo の既定（`ShouldReconnectOnError`）は**致命的closeコードでも無限に再接続**するため、無効トークン(4004)や未許可インテント(4014)で成功し得ないIDENTIFYを撃ち続けていた。自動再接続を無効化し、致命的コードでは即中止・それ以外は上限付き指数バックオフで再接続する自前の監視に置き換えた。
  - ゲートウェイ接続失敗時にセッションを閉じておらず、内部ゴルーチンが残り得た。必ず後始末する。
  - REST経路に**ネガティブキャッシュが無く**、失敗時はキャッシュを更新しないため、`AuthMiddleware` が全リクエストで呼ぶ `VerifyMembership` が**毎回Discordを叩いていた**（レートリミッターは毎秒25回を許可するだけで歯止めにならない）。失敗時に呼び出し自体を止めるサーキットブレーカーを追加（認証エラーは15分、429は `Retry-After` 尊重、その他は指数バックオフ）。
  - ひな型トークン（`YOUR_BOT_TOKEN`）のままでもDiscordへ接続を試みていた。公開イメージの初回起動で全ユーザーが無効な認証を撃つため、ひな型ではネットワークへ出さないようにした。

### Changed（変更）
- `docker-compose.yml` を**公開イメージ（GHCR）利用**に変更。clone / ダウンロード後そのまま `docker compose up -d` で起動でき、ソースからのビルドが不要になった。旧 `docker-compose.deploy.yml` の本番設定（healthcheck・`read_only`・`security_opt`・リソース制限・`env_file`）を統合。

### Added（追加）
- 開発用オーバーレイ `docker-compose.develop.yml` を新設。ローカルのソースからビルドして動かす場合に重ねて使う。
  ```
  docker compose -f docker-compose.yml -f docker-compose.develop.yml up -d --build
  ```

### Removed（削除）
- `docker-compose.deploy.yml`（内容を `docker-compose.yml` へ統合したため）。

## [0.1.1] - 2026-07-13

リバースプロキシ配下での SSE 切断とアップロード失敗を修正するホットフィックス。

### Fixed（修正）
- `http.Server` の `ReadTimeout` / `WriteTimeout`（各30秒）がボディ読み取りとレスポンス書き込みの全体に期限を課していたため、SSE（`/api/events`）が接続から30秒で強制切断されていた問題。応答ヘッダ送出後の打ち切りとなるためリバースプロキシが `RST_STREAM` を返し、ブラウザ側では `ERR_HTTP2_PROTOCOL_ERROR` として観測される。
- 同じタイムアウトにより、30秒以内に送り切れないアップロード（チャンク20MB／通常100MB）が中断されていた問題。
- HTTP/2 では送出が禁止されているホップバイホップヘッダ `Connection: keep-alive` を SSE ハンドラが付与していた問題。

### Changed（変更）
- スローロリス対策を、ヘッダ読み取りのみに期限を課す `ReadHeaderTimeout`（30秒）へ移行。

## [0.1.0] - 2026-07-12

最初のベータリリース。

### Added（追加）
- Discordゲートウェイによるロールの**リアルタイム同期**（Tier1）。起動時に全メンバーを一括ロードし、以後 `GuildMemberAdd/Update/Remove` でメモリを常時最新化。準備完了後はロール／在籍参照がREST APIを介さずメモリで解決される。
- ロール変更を即座にUIへ反映する SSE イベント `permissions_updated`。フロントは受信時にアクセス可能ディレクトリを再取得する。
- `/api/user` 応答に `is_admin` を追加し、管理者にのみヘッダーへ **管理ページ（`/admin`）へのリンク**を表示。
- 設定 `auth.provider.gateway_enabled`（未指定=有効）。特権インテント未許可の環境では起動時に自動検出してREST方式へフォールバック。
- 環境変数 `LOG_LEVEL`（debug/info/warn/error）と、全ログ行への `request_id` 付与。
- 実行ユーザーをホストユーザーへ合わせる `PUID` / `PGID`（bindマウントの所有者整合）。
- セキュリティ上重要な純粋関数とゲートウェイのストア論理に対する単体テスト。
- コミュニティ向けドキュメント: `SECURITY.md`・`CONTRIBUTING.md`・`CHANGELOG.md`・`docs/ARCHITECTURE.md`。

### Changed（変更）
- SSE配信を、接続ユーザーの**読み取り可能ディレクトリに絞り込む**方式へ変更（接続時に権限スナップショットを解決し、配信ホットパスからI/Oを排除）。
- Discordのロール取得（REST方式）にレートリミッター・singleflight・TTLジッターを追加し、多人数の一斉更新でもレート制限に当たらないよう平準化。
- ダウンロードの `Content-Disposition` を RFC 6266 準拠（ASCIIフォールバック＋`filename*`）に。
- リバースプロキシ配下の実クライアントIP判定を、X-Forwarded-For の右端から信頼済みプロキシを剥がす方式へ変更。
- 期限切れセッション行を定期的に物理削除。

### Fixed（修正）
- SSEが全ログインユーザーへ全ファイルイベントを配信し、閲覧権限のないプライベート領域のファイル名・操作が漏えいしていた問題。
- プロバイダー由来のユーザー名を無検証でディレクトリ名に使っていた問題（`SanitizeDirName` による多層防御）。
- 乱数生成失敗時に予測可能な値をセッショントークン/stateへ流用していた問題（フェイルクローズ化）。
- `upload_id` を未検証のまま内部のグロブ探索へ渡していた問題（UUID検証を追加）。
- SSE接続の書き込みエラー時にチャネル/ゴルーチンがリークしていた問題。

### Removed（削除）
- 未使用の `access_logs` テーブル（アクセスログは標準出力へ構造化ログとして出力する方式に統一）。
- 未参照の golangci-lint v1 設定（`.golangci-v1.yml`）。

### Security（セキュリティ）
- 認可・アップロード・パス処理・ヘッダ生成・IP判定にわたる脆弱性レビュー指摘の修正（詳細は Fixed / Changed を参照）。

---

[Unreleased]: https://github.com/leCielEtoile/fileGo/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/leCielEtoile/fileGo/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/leCielEtoile/fileGo/releases/tag/v0.1.0
