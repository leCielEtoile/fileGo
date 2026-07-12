# 変更履歴

本プロジェクトの重要な変更を記録します。
書式は [Keep a Changelog](https://keepachangelog.com/ja/1.1.0/) に、バージョニングは [Semantic Versioning](https://semver.org/lang/ja/) に準拠します。

## [Unreleased]

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

[Unreleased]: https://github.com/leCielEtoile/fileGo/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/leCielEtoile/fileGo/releases/tag/v0.1.0
