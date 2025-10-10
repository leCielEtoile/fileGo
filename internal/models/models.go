package models

import "time"

type User struct {
	CreatedAt     time.Time `json:"created_at"`
	LastLogin     time.Time `json:"last_login"`
	DiscordID     string    `json:"discord_id"`
	Username      string    `json:"username"`
	Discriminator string    `json:"discriminator"`
	Avatar        string    `json:"avatar"`
}

// GetDirectoryName はユーザーのディレクトリ名（ユーザー名）を返します。
func (u *User) GetDirectoryName() string {
	return u.Username
}

type Session struct {
	ExpiresAt           time.Time `json:"expires_at"`
	CreatedAt           time.Time `json:"created_at"`
	SessionToken        string    `json:"session_token"`
	UserID              string    `json:"user_id"`
	DiscordAccessToken  string    `json:"-"`
	DiscordRefreshToken string    `json:"-"`
}

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

type DiscordUser struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Discriminator string `json:"discriminator"`
	Avatar        string `json:"avatar"`
}

type DiscordGuildMember struct {
	User  DiscordUser `json:"user"`
	Roles []string    `json:"roles"`
}

type FileInfo struct {
	ModifiedAt   time.Time `json:"modified_at"`   // 24 bytes (wall: uint64, ext: int64, loc: *Location)
	Filename     string    `json:"filename"`      // 16 bytes
	OriginalName string    `json:"original_name"` // 16 bytes
	Uploader     string    `json:"uploader"`      // 16 bytes
	Hash         string    `json:"hash"`          // 16 bytes
	Size         int64     `json:"size"`          // 8 bytes
}

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
