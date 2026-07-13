# 変更履歴

本プロジェクトの重要な変更を記録します。
書式は [Keep a Changelog](https://keepachangelog.com/ja/1.1.0/) に、バージョニングは [Semantic Versioning](https://semver.org/lang/ja/) に準拠します。

## [Unreleased]

## [0.2.0] - 2026-07-13

設定まわりの大幅な見直しと、公開サーバー向けのログイン制限を追加しました。**環境変数の命名が変わる破壊的変更**を含みます。配布バイナリは設定を編集せずそのまま起動できるようになりました。Dockerでのファイル配置は従来と同一のため、**データ移行は不要**です。

### ⚠️ 破壊的変更

- **アプリの環境変数はすべて `FILEGO_` 接頭辞が必要になりました**（`SERVER_PORT` → `FILEGO_SERVER_PORT`、`LOG_LEVEL` → `FILEGO_LOG_LEVEL`、`CONFIG_PATH` → `FILEGO_CONFIG_PATH` 等）。`SERVER_PORT` や `DATABASE_PATH` のような一般的な名前は、Kubernetes・PaaS・CI などの共有環境で他コンポーネントと衝突するためです。接頭辞なしの旧名は読み込まれませんが、**残っていると起動時に警告**するため黙って無視されることはありません。`TZ` は Go ランタイムが解釈する標準変数のため接頭辞なしのままです。
- **Dockerでは `config.yaml` の `database.path` / `storage.upload_path` が反映されなくなりました**。イメージが環境変数で絶対パスを固定しており、環境変数が優先されるためです。保存先を変えたい場合は compose の `environment` で `FILEGO_DATABASE_PATH` / `FILEGO_STORAGE_UPLOAD_PATH` を上書きしてください（既定のまま使う場合は影響ありません）。

### Added（追加）

- **ログインに特定ロールを要求できる `auth.provider.required_roles`**（任意）。従来はDiscordサーバーに在籍していれば誰でもログインでき個人ディレクトリが払い出されたため、誰でも参加できる公開サーバーでは使いづらかった。在籍に加えてロール保有を要求できる（いずれか1つ保有でよい＝OR）。
  - 未設定なら従来動作（在籍のみでログイン可）。
  - **ロールを剥奪すると既存セッションも次のリクエストで失効する**（リクエストごとの在籍確認でもロールを判定するため）。
  - Discordはログイン時の在籍確認レスポンスにロールが含まれるため、**追加のAPI呼び出しは発生しない**。
  - OIDCでも `groups_claim` の値と照合して動作する（在籍の継続確認が無いため、剥奪の即時反映はDiscordのみ）。`allowed_email*` と併用時は全条件を満たす必要がある（AND）。
- **設定リファレンス [docs/CONFIGURATION.md](docs/CONFIGURATION.md) を新設**。設定項目・既定値・環境変数・権限モデル・秘密情報の扱いを集約した単一の情報源。
- Dockerを使わない環境向けに、**配布バイナリからの起動手順**を [SETUP.md](docs/SETUP.md) へ追加（対応OS/アーキ一覧・環境変数での上書き・systemdユニット例）。

### Security（セキュリティ）

- **秘密情報をファイルから読めるようにしました**（`*_FILE` 規約）。`FILEGO_BOT_TOKEN_FILE` / `FILEGO_CLIENT_SECRET_FILE` に**ファイルのパス**を指定すると、その中身を秘密情報として読み込みます。Docker secrets や Kubernetes の Secret ボリューム（`/run/secrets/...`）を素直に使えます。環境変数に入るのは「パス」だけで、秘密の値そのものは入りません（値を環境変数に置くと `docker inspect`・プロセス一覧・ログ経由で漏れるため）。

### Changed（変更）

- **設定ひな型の既定パスを相対パスに変更**（`database.path: ./data/fileserver.db` / `storage.upload_path: ./data/uploads`）。従来はDocker用の絶対パス（`/app/...`）が既定だったため、配布バイナリを実行すると `unable to open database file (14)` で起動できなかった。Dockerイメージは環境変数で絶対パスを与えるため、**コンテナ内のファイル配置は従来と同一**（DBは `./config/`、アップロードは `./data/`）。
- **`config.yaml` は「変えたい項目だけ」書けばよくなった**（省略時は既定値）。必須なのは `auth.provider` と `storage.directories` だけ。
- **設定の説明を CONFIGURATION.md に集約**。従来は SETUP.md（設定方法・grants・required_roles）と DEPLOYMENT.md（環境変数・認証設定）に二重に分散しており、どちらを見ればよいか分かりにくかった。SETUP は「起動するまでの手順」、DEPLOYMENT は「運用」に役割を分離した。
- `log_level` を `config.yaml` でも設定できるようにした（従来は環境変数のみ）。

### Fixed（修正）

- **手順どおりに実行するとDockerで起動できなかった問題を修正**。`config` / `data` を事前に作らずに `docker compose up` すると、**Dockerがそれらを root 所有で作成**するため、非rootで動くコンテナが書き込めず `permission denied` でクラッシュループしていた。手順に `mkdir -p config data` を追加し、`.env` も編集不要で生成する形に変更した（Docker Compose は `id -u` を自動実行できないため値の生成が必要）。
- **設定項目を省略すると壊れる問題を修正**（既定値が適用されていなかった）。従来は省略した項目がゼロ値になり、次の不具合が起きていた。
  - `storage.cleanup_interval` 省略 → `time.NewTicker(0)` が**パニックしプロセスが落ちる**（ゴルーチン内のため回復不能）
  - `storage.max_concurrent_uploads` 省略 → 判定が常に真になり**チャンクアップロードが常に拒否される**
  - `storage.max_file_size` 省略 → **0バイト超の全ファイルが拒否される**
  - `storage.chunk_upload_enabled` 省略 → **チャンクアップロードが黙って無効化される**
  - `server.port` 省略 → ランダムポートで待ち受け
- **起動時の設定検証を追加**。必須項目が欠けている場合、**何が足りないかを示して即座に起動失敗**する（実行時の不可解なエラーを防ぐ）。
- **不正な環境変数を黙って無視していた問題を修正**。`FILEGO_DATABASE_MAX_CONNECTIONS=abc` のように解釈できない値を指定すると、従来は**その項目を無視して起動**していたため、運用者は「設定したつもり」で気付けなかった。現在は**どの変数が不正かを示して起動時にエラー**にする。
- **真偽値の解釈が壊れていた問題を修正**。従来は `== "true"` の単純比較だったため、`FILEGO_SERVER_BEHIND_PROXY=1` や `TRUE` が**黙って false** になっていた（`CHUNK_UPLOAD_ENABLED=1` ならチャンクアップロードが無効化された）。`1` / `0` / `TRUE` / `True` なども正しく解釈する。
- **`secure_cookie` に環境変数が無かった**問題を修正（`FILEGO_SERVER_SECURE_COOKIE`）。他のサーバー設定にはあるのにこれだけ欠けていた。
- データベースの親ディレクトリが存在しない場合に `unable to open database file (14)` で起動できなかった問題を修正（SQLiteは親ディレクトリを作らないため、起動時に作成する）。

## [0.1.2] - 2026-07-13

Discord Botトークンの濫用検知（強制リセット）を招く重大な不具合を修正。**Discord連携を使う場合は必ず更新してください。**

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

[Unreleased]: https://github.com/leCielEtoile/fileGo/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/leCielEtoile/fileGo/compare/v0.1.2...v0.2.0
[0.1.2]: https://github.com/leCielEtoile/fileGo/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/leCielEtoile/fileGo/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/leCielEtoile/fileGo/releases/tag/v0.1.0
