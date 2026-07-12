# API仕様

fileGoのHTTP APIエンドポイント一覧と使用方法を説明します。

## 目次

- [認証](#認証)
- [認証エンドポイント](#認証エンドポイント)
- [ファイル操作エンドポイント](#ファイル操作エンドポイント)
- [チャンクアップロードエンドポイント](#チャンクアップロードエンドポイント)
- [エラーレスポンス](#エラーレスポンス)

## 認証

fileGoはOAuth2/OIDCとセッションCookieを使用した認証を行います。認証プロバイダーは1つに限定され、`config.yaml` の `auth.provider.type`（`discord` または `oidc`）で決まります。

### 認証フロー

1. `/auth/login` にアクセス
2. プロバイダー（Discord/OIDC）の認可ページにリダイレクト
3. ユーザーが承認
4. `/auth/callback` にリダイレクト（セッションCookie発行）
5. 以降のAPIリクエストでセッションCookieを使用

Discordの場合はギルドメンバーシップを確認し、以降のリクエストでも在籍を継続確認します（5分キャッシュ）。OIDCの場合は `allowed_email_domains` / `allowed_emails` のallowlistで制限します。

### セッション

- **有効期限**: 7日間
- **Cookie名**: `session_token`（CSRF用に `oauth_state` も一時発行）
- **保存方法**: HttpOnly, Secure (HTTPS時), SameSite=Lax

## 認証エンドポイント

### GET /auth/login

プロバイダーの認可ページにリダイレクトします。CSRF用の `oauth_state` クッキーを設定します。

**リクエスト:**
```http
GET /auth/login HTTP/1.1
Host: yourdomain.com
```

**レスポンス:**
```http
HTTP/1.1 307 Temporary Redirect
Location: https://discord.com/api/oauth2/authorize?client_id=...
Set-Cookie: oauth_state=...; Path=/; Max-Age=300; HttpOnly
```

---

### GET /auth/callback

OAuth2/OIDCコールバック（自動処理）。ユーザーが直接アクセスする必要はありません。

**パラメータ:**
- `code` (query): 認可コード
- `state` (query): CSRFトークン（`oauth_state` クッキーと一致する必要がある）

**レスポンス:**
```http
HTTP/1.1 303 See Other
Location: /
Set-Cookie: session_token=...; HttpOnly; Secure; SameSite=Lax
```

**エラー:**
- `400 Bad Request`: stateまたはcodeが不正
- `403 Forbidden`: アクセスが許可されていない（非メンバー / allowlist外）

---

### GET /auth/logout

ログアウトしてセッションを破棄します。

**リクエスト:**
```http
GET /auth/logout HTTP/1.1
Host: yourdomain.com
Cookie: session_token=...
```

**レスポンス:**
```http
HTTP/1.1 303 See Other
Location: /
Set-Cookie: session_token=; Max-Age=0
```

---

### GET /api/user

現在のユーザー情報を取得します。

**リクエスト:**
```http
GET /api/user HTTP/1.1
Host: yourdomain.com
Cookie: session_token=...
```

**レスポンス:**
```json
{
  "id": "123456789012345678",
  "provider": "discord",
  "subject": "123456789012345678",
  "username": "username",
  "avatar": "a_1234567890abcdef1234567890abcdef",
  "email": "user@example.com",
  "created_at": "2024-01-01T00:00:00Z",
  "last_login": "2024-01-02T00:00:00Z",
  "is_admin": false
}
```

`id` はプロバイダー内の `subject`（DiscordならユーザーID）です。`is_admin` は `admin_role_id` を保有するかの判定結果で、フロントが管理ページへの導線を出し分けるために使います（`/admin` 自体は `AdminMiddleware` でサーバー側保護されます）。

**エラー:**
- `401 Unauthorized`: セッションが無効または期限切れ

---

## ファイル操作エンドポイント

すべてのエンドポイントは認証が必要です（セッションCookie）。

### GET /files/directories

ユーザーがアクセス可能なディレクトリ一覧を取得します。

**リクエスト:**
```http
GET /files/directories HTTP/1.1
Host: yourdomain.com
Cookie: session_token=...
```

**レスポンス:**
```json
{
  "success": true,
  "directories": [
    {
      "path": "admin",
      "permissions": ["read", "write", "delete"]
    },
    {
      "path": "public",
      "permissions": ["read"]
    }
  ]
}
```

---

### POST /files/upload

通常ファイルアップロード（最大100MB）。

**リクエスト:**
```http
POST /files/upload HTTP/1.1
Host: yourdomain.com
Cookie: session_token=...
Content-Type: multipart/form-data; boundary=----WebKitFormBoundary

------WebKitFormBoundary
Content-Disposition: form-data; name="directory"

admin
------WebKitFormBoundary
Content-Disposition: form-data; name="file"; filename="example.txt"
Content-Type: text/plain

[ファイル内容]
------WebKitFormBoundary--
```

**パラメータ:**
- `directory` (form): アップロード先ディレクトリ名
- `file` (file): アップロードファイル

**レスポンス:**
```json
{
  "success": true,
  "filename": "uuid_example.txt",
  "size": 12345,
  "path": "admin/uuid_example.txt"
}
```

**エラー:**
- `400 Bad Request`: ファイルが指定されていない、ディレクトリ名が無効
- `403 Forbidden`: 書き込み権限がない
- `400 Bad Request`: ファイルサイズが制限を超えている

---

### GET /files

ディレクトリ内のファイル一覧を取得します。

**リクエスト:**
```http
GET /files?directory=admin HTTP/1.1
Host: yourdomain.com
Cookie: session_token=...
```

**パラメータ:**
- `directory` (query): ディレクトリ名

**レスポンス:**
```json
{
  "success": true,
  "directory": "admin",
  "files": [
    {
      "filename": "uuid_file1.txt",
      "original_name": "file1.txt",
      "size": 12345,
      "modified_at": "2024-01-01T00:00:00Z"
    },
    {
      "filename": "uuid_file2.zip",
      "original_name": "file2.zip",
      "size": 987654,
      "modified_at": "2024-01-02T00:00:00Z"
    }
  ]
}
```

**エラー:**
- `400 Bad Request`: ディレクトリ名が指定されていない
- `403 Forbidden`: 読み取り権限がない

---

### GET /files/download/{directory}/{filename}

ファイルをダウンロードします。Range Request対応。

**リクエスト:**
```http
GET /files/download/admin/uuid_example.txt HTTP/1.1
Host: yourdomain.com
Cookie: session_token=...
```

**Range Request（部分ダウンロード）:**
```http
GET /files/download/admin/uuid_video.mp4 HTTP/1.1
Host: yourdomain.com
Cookie: session_token=...
Range: bytes=0-1023
```

**レスポンス（通常）:**
```http
HTTP/1.1 200 OK
Content-Type: application/octet-stream
Content-Disposition: attachment; filename="example.txt"
Content-Length: 12345

[ファイル内容]
```

**レスポンス（Range Request）:**
```http
HTTP/1.1 206 Partial Content
Content-Type: video/mp4
Content-Range: bytes 0-1023/1048576
Content-Length: 1024

[部分的なファイル内容]
```

**エラー:**
- `403 Forbidden`: 読み取り権限がない
- `404 Not Found`: ファイルが存在しない
- `416 Range Not Satisfiable`: Range指定が無効

---

### DELETE /files/{directory}/{filename}

ファイルを削除します。

**リクエスト:**
```http
DELETE /files/admin/uuid_example.txt HTTP/1.1
Host: yourdomain.com
Cookie: session_token=...
```

**レスポンス:**
```json
{
  "success": true,
  "message": "ファイルを削除しました"
}
```

**エラー:**
- `403 Forbidden`: 削除権限がない
- `404 Not Found`: ファイルが存在しない

---

## チャンクアップロードエンドポイント

大容量ファイル（最大500GB）をレジューム可能な形式でアップロードします。

### POST /files/chunk/init

チャンクアップロードを初期化します。

**リクエスト:**
```http
POST /files/chunk/init HTTP/1.1
Host: yourdomain.com
Cookie: session_token=...
Content-Type: application/json

{
  "filename": "large_file.zip",
  "directory": "admin",
  "file_size": 1073741824,
  "chunk_size": 20971520
}
```

**パラメータ:**
- `filename` (string): ファイル名
- `directory` (string): アップロード先ディレクトリ
- `file_size` (int): ファイル全体のサイズ（バイト）
- `chunk_size` (int): チャンクサイズ（バイト、推奨: 20MB）

**レスポンス:**
```json
{
  "success": true,
  "upload_id": "550e8400-e29b-41d4-a716-446655440000",
  "total_chunks": 51,
  "chunk_size": 20971520
}
```

**エラー:**
- `400 Bad Request`: パラメータが無効
- `403 Forbidden`: 書き込み権限がない
- `400 Bad Request`: ファイルサイズが制限を超えている

---

### POST /files/chunk/upload/{upload_id}

チャンクをアップロードします。

**リクエスト:**
```http
POST /files/chunk/upload/550e8400-e29b-41d4-a716-446655440000?chunk_index=0 HTTP/1.1
Host: yourdomain.com
Cookie: session_token=...
Content-Type: application/octet-stream
Content-Length: 20971520

[チャンクデータ]
```

**パラメータ:**
- `upload_id` (path): アップロードID
- `chunk_index` (query): チャンクインデックス（0から開始）

**レスポンス:**
```json
{
  "success": true,
  "chunk_index": 0
}
```

**エラー:**
- `400 Bad Request`: chunk_indexが無効
- `404 Not Found`: upload_idが存在しない
- `500 Internal Server Error`: チャンク保存に失敗した

---

### GET /files/chunk/status/{upload_id}

アップロード状態を取得します（レジューム時に使用）。

**リクエスト:**
```http
GET /files/chunk/status/550e8400-e29b-41d4-a716-446655440000 HTTP/1.1
Host: yourdomain.com
Cookie: session_token=...
```

**レスポンス:**
```json
{
  "success": true,
  "upload_id": "550e8400-e29b-41d4-a716-446655440000",
  "filename": "large_file.zip",
  "directory": "admin",
  "total_chunks": 51,
  "uploaded_chunks": [0, 1, 2, 3, 4],
  "file_size": 1073741824,
  "uploaded_size": 104857600,
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-01T00:05:00Z"
}
```

**エラー:**
- `404 Not Found`: upload_idが存在しない

---

### POST /files/chunk/complete/{upload_id}

アップロードを完了し、チャンクを結合します。

**リクエスト:**
```http
POST /files/chunk/complete/550e8400-e29b-41d4-a716-446655440000 HTTP/1.1
Host: yourdomain.com
Cookie: session_token=...
```

**レスポンス:**
```json
{
  "success": true,
  "message": "アップロードが完了しました",
  "path": "admin/uuid_large_file.zip",
  "filename": "uuid_large_file.zip",
  "size": 1073741824
}
```

**エラー:**
- `400 Bad Request`: すべてのチャンクがアップロードされていない
- `404 Not Found`: upload_idが存在しない

---

### DELETE /files/chunk/cancel/{upload_id}

アップロードをキャンセルし、一時ファイルを削除します。

**リクエスト:**
```http
DELETE /files/chunk/cancel/550e8400-e29b-41d4-a716-446655440000 HTTP/1.1
Host: yourdomain.com
Cookie: session_token=...
```

**レスポンス:**
```json
{
  "success": true,
  "message": "アップロードをキャンセルしました"
}
```

**エラー:**
- `404 Not Found`: upload_idが存在しない

---

## システム・管理者エンドポイント

### GET /health

ヘルスチェック。認証不要。プレーンテキスト `OK` を返します。

### GET /api/events

Server-Sent Events。`text/event-stream` で配信します（認証必須）。イベント種別:

| event | 説明 |
|-------|------|
| `file_upload` / `file_download` / `file_delete` | ファイル操作。**そのディレクトリへの読み取り権限を持つ接続にのみ**配信される（接続時に解決した権限スナップショットで絞り込み） |
| `user_login` | ログイン通知（全接続へ配信） |
| `permissions_updated` | ロールのリアルタイム変更で当該ユーザーの権限が変化したことを通知（該当ユーザーの接続のみ）。フロントはこれを受けてディレクトリ一覧を再取得する |

### GET /admin

管理者ページ（HTML）。`admin_role_id` ロールを持つユーザーのみ（`AdminMiddleware`）。

### GET /api/admin/uploads

進行中のチャンクアップロードセッション一覧（JSON配列）。管理者のみ。

### GET /api/admin/stats

アップロード統計（総セッション数・総サイズ・ユーザー別件数など）。管理者のみ。

---

## エラーレスポンス

**エラーレスポンスのボディはプレーンテキスト**（`text/plain`）です。JSONではありません。
例: `403 Forbidden` の場合、ボディは `書き込み権限がありません` のような文字列になります。
JSONを返すのは成功レスポンスのみです。

### HTTPステータスコード

- `200 OK`: 成功
- `206 Partial Content`: Range Request成功
- `303 See Other` / `307 Temporary Redirect`: リダイレクト
- `400 Bad Request`: リクエストパラメータが無効
- `401 Unauthorized`: 認証が必要
- `403 Forbidden`: 権限がない / 在籍が確認できない
- `404 Not Found`: リソースが存在しない
- `416 Range Not Satisfiable`: Range指定が無効
- `500 Internal Server Error`: サーバーエラー（権限確認失敗・在籍確認失敗を含む）
- `500 Internal Server Error`: サーバーエラー

## 使用例

### curlでファイルアップロード

```bash
# ログイン（ブラウザで実施）
open http://localhost:8080/auth/login

# ファイルアップロード（セッションCookieを保存）
curl -X POST http://localhost:8080/files/upload \
  -b cookies.txt -c cookies.txt \
  -F "directory=admin" \
  -F "file=@/path/to/file.txt"

# ファイル一覧取得
curl http://localhost:8080/files?directory=admin \
  -b cookies.txt

# ファイルダウンロード
curl http://localhost:8080/files/download/admin/uuid_file.txt \
  -b cookies.txt \
  -o downloaded_file.txt
```

### JavaScriptでチャンクアップロード

```javascript
async function uploadLargeFile(file, directory) {
  const chunkSize = 20 * 1024 * 1024; // 20MB

  // 初期化
  const initResponse = await fetch('/files/chunk/init', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      filename: file.name,
      directory: directory,
      file_size: file.size,
      chunk_size: chunkSize
    })
  });

  const { upload_id, total_chunks } = await initResponse.json();

  // チャンクアップロード
  for (let i = 0; i < total_chunks; i++) {
    const start = i * chunkSize;
    const end = Math.min(start + chunkSize, file.size);
    const chunk = file.slice(start, end);

    await fetch(`/files/chunk/upload/${upload_id}?chunk_index=${i}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/octet-stream' },
      body: chunk
    });

    console.log(`Uploaded chunk ${i + 1}/${total_chunks}`);
  }

  // 完了
  const completeResponse = await fetch(`/files/chunk/complete/${upload_id}`, {
    method: 'POST'
  });

  return await completeResponse.json();
}
```

## 次のステップ

- [デプロイ・運用ガイド](DEPLOYMENT.md) - 本番環境での運用方法
- [セットアップガイド](SETUP.md) - 詳細なセットアップ手順
