// Package models はアプリケーション全体で共有するドメインモデルを定義します。
package models

import (
	"path/filepath"
	"strings"
	"time"
)

// SanitizeDirName はユーザー名を "user/" 直下に収まる安全な単一ディレクトリ名へ
// 正規化します。パス区切り・".."・先頭末尾のドット・NUL を無害化し、
// 万一プロバイダーが細工した username を返しても親ディレクトリへ抜け出せないようにします。
// 正常なユーザー名（区切り文字を含まない単一要素）は変化しません。
func SanitizeDirName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.NewReplacer("/", "_", "\\", "_", "..", "_", "\x00", "").Replace(name)
	name = strings.Trim(name, ".")
	if name == "" {
		return "_"
	}
	return name
}

// User は認証済みユーザーを表します。
// 認証プロバイダーを1つに限定しているため、ID にはプロバイダー内のsubjectをそのまま用います。
type User struct {
	CreatedAt time.Time `json:"created_at"`
	LastLogin time.Time `json:"last_login"`
	ID        string    `json:"id"`
	Provider  string    `json:"provider"`
	Subject   string    `json:"subject"`
	Username  string    `json:"username"`
	Avatar    string    `json:"avatar"`
	Email     string    `json:"email"`
}

// GetDirectoryName はユーザーのディレクトリ名を返します。
// ファイルシステム上のパス構成要素として使うため、常にサニタイズします。
func (u *User) GetDirectoryName() string {
	return SanitizeDirName(u.Username)
}

// Session はログイン中のユーザーのセッションを表します。
type Session struct {
	ExpiresAt    time.Time `json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
	SessionToken string    `json:"session_token"`
	UserID       string    `json:"user_id"`
	Provider     string    `json:"provider"`
}

// AccessLog はファイル操作の監査ログ1件を表します。
type AccessLog struct {
	Filesize   *int64    `json:"filesize"`
	Timestamp  time.Time `json:"timestamp"`
	UserID     *string   `json:"user_id"`
	Filename   *string   `json:"filename"`
	Filepath   *string   `json:"filepath"`
	IPAddress  string    `json:"ip_address"`
	UserAgent  string    `json:"user_agent"`
	Action     string    `json:"action"`
	ID         int64     `json:"id"`
	StatusCode int       `json:"status_code"`
}

// FileInfo は一覧表示に用いるファイルまたはディレクトリの情報を表します。
type FileInfo struct {
	ModifiedAt   time.Time `json:"modified_at"`
	Filename     string    `json:"filename"`
	OriginalName string    `json:"original_name"`
	Uploader     string    `json:"uploader"`
	Hash         string    `json:"hash"`
	Path         string    `json:"path"` // ファイル/ディレクトリの相対パス
	Size         int64     `json:"size"`
	IsDirectory  bool      `json:"is_directory"`
}

// UploadSession は進行中のチャンク分割アップロードの状態を表します。
type UploadSession struct {
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	UploadID       string    `json:"upload_id"`
	UserID         string    `json:"user_id"`
	Filename       string    `json:"filename"`
	Directory      string    `json:"directory"`
	UploadedChunks []int     `json:"uploaded_chunks"`
	TotalSize      int64     `json:"total_size"`
	ChunkSize      int64     `json:"chunk_size"`
	UploadedSize   int64     `json:"uploaded_size"`
	TotalChunks    int       `json:"total_chunks"`
}
