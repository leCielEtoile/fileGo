# 設定リファレンス

fileGo の設定項目をすべて記載します。手順は [セットアップガイド](SETUP.md)、運用は [デプロイ・運用ガイド](DEPLOYMENT.md) を参照してください。

## 目次

- [設定の優先順位](#設定の優先順位)
- [config.yaml の場所](#configyaml-の場所)
- [最小構成](#最小構成)
- [設定項目](#設定項目)
  - [log_level](#log_level)
  - [server](#server)
  - [auth.provider](#authprovider)
  - [database](#database)
  - [storage](#storage)
  - [storage.directories（権限モデル）](#storagedirectories権限モデル)
- [ログインできる人を絞る（required_roles）](#ログインできる人を絞るrequired_roles)
- [環境変数](#環境変数)
- [秘密情報の扱い](#秘密情報の扱い)

## 設定の優先順位

```
環境変数  >  config.yaml  >  既定値
```

**省略した項目には既定値が使われます。** `config.yaml` は「既定から変えたい項目だけ」書けば動作します。必須なのは `auth.provider`（認証情報）と `storage.directories` だけです。

設定に不備がある場合（必須項目の欠落、環境変数の値が不正など）は、**何が悪いかを示して起動時にエラー**になります。黙って既定値で動き続けることはありません。

## config.yaml の場所

| 起動方法 | 既定の場所 |
|---|---|
| 配布バイナリ / ソースビルド | 実行ファイルと同じディレクトリの `config.yaml` |
| Docker | `/app/config/config.yaml`（ホストの `./config/config.yaml`） |

環境変数 `FILEGO_CONFIG_PATH` で明示できます。**ファイルが存在しない場合はひな型が自動生成される**ので、初回起動後に編集してください。

`config.yaml` は秘密情報を含むため `.gitignore` 済みです。**コミットしないでください。** スキーマの正は [config.yaml.example](../config.yaml.example) です。

## 最小構成

これだけで起動します（他はすべて既定値）。

```yaml
auth:
  provider:
    type: discord
    bot_token: "..."
    client_id: "..."
    client_secret: "..."
    guild_id: "..."
    redirect_url: "https://yourdomain.com/auth/callback"

storage:
  directories:
    - path: "public"
      grants:
        - role: "*"
          permissions: ["read"]
```

## 設定項目

### log_level

| キー | 型 | 既定値 | 説明 |
|---|---|---|---|
| `log_level` | enum | `info` | `debug` / `info` / `warn` / `error`。`debug` にすると `/health`・`/static` のアクセスログや認証の詳細も出る |

### server

| キー | 型 | 既定値 | 説明 |
|---|---|---|---|
| `server.port` | string | `8080` | 待ち受けポート |
| `server.service_name` | string | `Discord File Server` | 画面に表示する名称 |
| `server.behind_proxy` | bool | `false` | リバースプロキシ配下で動かすか。`true` のとき `X-Forwarded-For` を参照する |
| `server.secure_cookie` | bool | `false` | Cookieの `Secure` 属性。**HTTPS配信なら `true`**。HTTPで動かす場合は `false`（`true` のままだとログインできない） |
| `server.trusted_proxies` | []string | — | 信頼するプロキシのCIDR。`behind_proxy: true` のときのみ使用 |

`trusted_proxies` に含まれないIPからのリクエストでは `X-Forwarded-For` を信用しません（クライアントによるIP詐称を防ぐため）。

### auth.provider

**認証プロバイダーは1つだけ**設定できます（`type` に `discord` または `oidc`）。複数プロバイダーが別々のユーザーを作る複雑さを避けるためです。

**共通**

| キー | 型 | 必須 | 説明 |
|---|---|---|---|
| `type` | enum | ✅ | `discord` または `oidc` |
| `name` | string | | 表示名 |
| `client_id` | string | ✅ | OAuth2 Client ID |
| `client_secret` | string | ✅ | OAuth2 Client Secret（**秘密情報**） |
| `redirect_url` | string | ✅ | `https://yourdomain.com/auth/callback` |
| `required_roles` | []string | | ログインに必要なロール（[後述](#ログインできる人を絞るrequired_roles)） |

**Discord専用**

| キー | 型 | 必須 | 既定値 | 説明 |
|---|---|---|---|---|
| `bot_token` | string | ✅ | | Botトークン（**秘密情報**）。ロール/在籍確認に使う |
| `guild_id` | string | ✅ | | 対象サーバー（ギルド）のID |
| `gateway_enabled` | bool | | `true` | ロールのリアルタイム同期。要 Server Members Intent。未対応環境では起動時に自動検出してREST方式へフォールバックする（[ARCHITECTURE.md](ARCHITECTURE.md) 参照） |

**OIDC専用**

| キー | 型 | 必須 | 既定値 | 説明 |
|---|---|---|---|---|
| `issuer` | string | ✅ | | OIDCのissuer URL |
| `scopes` | []string | | | 例: `["openid","profile","email","groups"]` |
| `groups_claim` | string | | `groups` | ロール一覧を含むクレーム名。`grants` の `role` にはこの値を指定する |
| `allowed_email_domains` | []string | | | 許可するメールドメイン |
| `allowed_emails` | []string | | | 許可するメールアドレス |

> ⚠️ **Google は ID Token に `groups` を含めない**ため、Googleでロール制御はできません。ログイン用途に留め、`allowed_email_domains` で絞ってください。
>
> OIDCには在籍の継続確認がありません。ログイン後のアクセス制限は `allowed_*` / `required_roles`（ログイン時のみ判定）と `grants` で行います。

### database

| キー | 型 | 既定値 | 説明 |
|---|---|---|---|
| `database.path` | path | `./data/fileserver.db` | SQLiteファイル。親ディレクトリは自動作成される |
| `database.max_connections` | int | `10` | 最大接続数 |

> ⚠️ **Dockerでは `database.path` は無効です。** イメージが `FILEGO_DATABASE_PATH=/app/config/fileserver.db` を設定しており、環境変数が優先されます（[後述](#dockerイメージが固定しているパス)）。

### storage

| キー | 型 | 既定値 | 説明 |
|---|---|---|---|
| `storage.upload_path` | path | `./data/uploads` | アップロード先。**Dockerでは無効**（環境変数が優先） |
| `storage.max_file_size` | int64 | `104857600`(100MB) | 通常アップロードの上限 |
| `storage.chunk_upload_enabled` | bool | `true` | チャンクアップロードの有効化 |
| `storage.chunk_size` | int64 | `20971520`(20MB) | 1チャンクのサイズ |
| `storage.max_chunk_file_size` | int64 | `536870912000`(500GB) | チャンクアップロードの上限 |
| `storage.max_concurrent_uploads` | int | `3` | 1ユーザーの同時アップロード数 |
| `storage.upload_session_ttl` | duration | `48h` | 未完了アップロードの保持期間 |
| `storage.cleanup_interval` | duration | `1h` | 期限切れセッションの掃除間隔 |
| `storage.admin_role_id` | string | — | **全ディレクトリ・全操作**を許可するロールID |
| `storage.directories` | []dir | ✅必須 | 下記参照 |

### storage.directories（権限モデル）

ディレクトリごとに `grants` でアクセス権を与えます。1つの `grant` は「誰に」×「何を」の組です。

| キー | 説明 |
|---|---|
| `path` | ディレクトリ名（必須） |
| `type` | `user_private` を指定すると**ユーザー個人用**になる（本人と管理者のみ） |
| `grants[].role` | ロールID。`"*"` は**全メンバー**を表す |
| `grants[].user` | ユーザーID（特定個人への付与） |
| `grants[].permissions` | `read`（一覧・DL） / `write`（アップロード） / `delete`（削除） |

`role` と `user` は**どちらか一方**を指定します。同じディレクトリに複数の grant を並べ、役割ごとに異なる権限を与えられます。

```yaml
storage:
  admin_role_id: "1111111111111111111"   # 全ディレクトリ全操作

  directories:
    # 各ユーザーの個人ディレクトリ（本人と管理者のみ。初回アップロードで作成）
    - path: "user"
      type: user_private

    # 管理者専用
    - path: "admin"
      grants:
        - role: "1111111111111111111"
          permissions: ["read", "write", "delete"]

    # 役割ごとに権限を変える
    - path: "staff"
      grants:
        - role: "2222222222222222222"    # editor: 編集可
          permissions: ["read", "write"]
        - role: "3333333333333333333"    # viewer: 閲覧のみ
          permissions: ["read"]
        - user: "444444444444444444"     # 特定個人にも付与できる
          permissions: ["read", "write"]

    # 全メンバーが閲覧可
    - path: "public"
      grants:
        - role: "*"
          permissions: ["read"]
```

- `admin_role_id` を持つユーザーは**全ディレクトリで全操作**が許可されます。
- `type: user_private` は本人と管理者のみ。ディレクトリは**初回アップロード時に作成**されます。

## ログインできる人を絞る（required_roles）

既定では、**Discordサーバーに在籍していれば誰でもログインでき**、個人ディレクトリが払い出されます。誰でも参加できる公開サーバーでは望ましくない場合があります。

`required_roles` を設定すると、**在籍に加えて特定ロールの保有を要求**できます（いずれか1つ保有していればログイン可＝**OR判定**）。

```yaml
auth:
  provider:
    required_roles:
      - "234567890123456789"   # approved ロール
      - "123456789012345678"   # 管理者ロール
```

- **未設定なら従来どおり**、在籍しているだけでログインできます。
- 条件を満たさないユーザーはログインできず、**個人ディレクトリも作成されません**。
- **ロールを剥奪すると、既存セッションも次のリクエストで失効します**（ログイン時だけでなく、リクエストごとの在籍確認でも判定するため）。

> ⚠️ **締め出しに注意**：管理者もこの条件の対象です。管理者ロールしか持たない人をログインさせる場合は、そのロールIDも `required_roles` に含めてください。

OIDCでも指定でき、`groups_claim` の値と照合されます。`allowed_email_*` と併用した場合、設定されている条件は**すべて満たす必要があります**（AND）。ただしOIDCには在籍の継続確認が無いため、**ロール剥奪が既存セッションへ即時反映されるのはDiscordのみ**です。

## 環境変数

アプリの環境変数はすべて **`FILEGO_` 接頭辞**が付きます。`SERVER_PORT` や `DATABASE_PATH` のような一般的な名前は、Kubernetes・PaaS・CI などの共有環境で他コンポーネントと衝突するためです。

**接頭辞なしの旧名は読み込まれません**（残っていると起動時に警告します）。値が不正な場合（例: `FILEGO_DATABASE_MAX_CONNECTIONS=abc`）は**起動時にエラー**になります。

| 環境変数 | 型 | 対応する config.yaml |
|---|---|---|
| `FILEGO_CONFIG_PATH` | path | （設定ファイルの場所そのもの） |
| `FILEGO_LOG_LEVEL` | enum | `log_level` |
| `FILEGO_SERVER_PORT` | string | `server.port` |
| `FILEGO_SERVER_SERVICE_NAME` | string | `server.service_name` |
| `FILEGO_SERVER_BEHIND_PROXY` | bool | `server.behind_proxy` |
| `FILEGO_SERVER_SECURE_COOKIE` | bool | `server.secure_cookie` |
| `FILEGO_SERVER_TRUSTED_PROXIES` | csv | `server.trusted_proxies` |
| `FILEGO_DATABASE_PATH` | path | `database.path` |
| `FILEGO_DATABASE_MAX_CONNECTIONS` | int | `database.max_connections` |
| `FILEGO_STORAGE_UPLOAD_PATH` | path | `storage.upload_path` |
| `FILEGO_STORAGE_MAX_FILE_SIZE` | int64 | `storage.max_file_size` |
| `FILEGO_STORAGE_CHUNK_UPLOAD_ENABLED` | bool | `storage.chunk_upload_enabled` |
| `FILEGO_STORAGE_CHUNK_SIZE` | int64 | `storage.chunk_size` |
| `FILEGO_STORAGE_MAX_CHUNK_FILE_SIZE` | int64 | `storage.max_chunk_file_size` |
| `FILEGO_STORAGE_MAX_CONCURRENT_UPLOADS` | int | `storage.max_concurrent_uploads` |
| `FILEGO_STORAGE_UPLOAD_SESSION_TTL` | duration | `storage.upload_session_ttl` |
| `FILEGO_STORAGE_CLEANUP_INTERVAL` | duration | `storage.cleanup_interval` |
| `FILEGO_STORAGE_ADMIN_ROLE_ID` | string | `storage.admin_role_id` |
| `FILEGO_BOT_TOKEN_FILE` | path | `auth.provider.bot_token`（[ファイル経由](#秘密情報の扱い)） |
| `FILEGO_CLIENT_SECRET_FILE` | path | `auth.provider.client_secret`（[ファイル経由](#秘密情報の扱い)） |
| `TZ` | string | — | タイムゾーン（Goランタイムが解釈する標準変数のため接頭辞なし） |

bool は `true` / `false` に加え `1` / `0` / `TRUE` なども受け付けます。duration は `48h` / `1h30m` 形式です。

### Dockerイメージが固定しているパス

設定ひな型の既定値は「配布バイナリをそのまま実行できる」相対パス（`./data/...`）です。Dockerイメージはコンテナのボリューム構成に合わせ、次の環境変数で絶対パスを与えています。

| 環境変数 | イメージでの既定値 | ホスト側 |
|---|---|---|
| `FILEGO_CONFIG_PATH` | `/app/config/config.yaml` | `./config/config.yaml` |
| `FILEGO_DATABASE_PATH` | `/app/config/fileserver.db` | `./config/fileserver.db` |
| `FILEGO_STORAGE_UPLOAD_PATH` | `/app/data/uploads` | `./data/uploads` |

**そのためコンテナでは `config.yaml` の `database.path` / `storage.upload_path` を編集しても反映されません。** 変更したい場合は compose の `environment` でこれらの環境変数を上書きしてください。

## 秘密情報の扱い

**秘密情報（`bot_token` / `client_secret`）の「値」を環境変数に入れてはいけません。** `docker inspect`・プロセス一覧・ログ経由で漏れます。本アプリは秘密情報の値を環境変数から読みません。

設定方法は2通りです。

**1. `config.yaml` に記述する（既定）**

```yaml
auth:
  provider:
    bot_token: "..."
    client_secret: "..."
```

**2. ファイルとして渡し、`FILEGO_*_FILE` にそのパスを指定する**

Docker secrets / Kubernetes の Secret ボリューム向けです。環境変数に入るのは**パスだけ**で、秘密そのものは入りません。

```yaml
# docker-compose.yml
services:
  fileserver:
    environment:
      - FILEGO_BOT_TOKEN_FILE=/run/secrets/bot_token
      - FILEGO_CLIENT_SECRET_FILE=/run/secrets/client_secret
    secrets:
      - bot_token
      - client_secret

secrets:
  bot_token:
    file: ./secrets/bot_token
  client_secret:
    file: ./secrets/client_secret
```

指定したファイルが存在しない・空の場合は、黙って既定値で動かず**起動時にエラー**になります。
