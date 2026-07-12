// Package config はアプリケーション設定の読み込みと管理を提供します。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// configPathEnvVar は設定ファイルパスを明示的に指定するための環境変数名です。
const configPathEnvVar = "CONFIG_PATH"

// ResolvePath は読み込む設定ファイルのパスを決定します。
// CONFIG_PATH環境変数が設定されている場合はそれを優先し、
// 未設定の場合は実行ファイルと同じディレクトリの "config.yaml" を既定値とします。
func ResolvePath() string {
	if envPath := os.Getenv(configPathEnvVar); envPath != "" {
		return envPath
	}

	exePath, err := os.Executable()
	if err != nil {
		return "config.yaml"
	}

	return filepath.Join(filepath.Dir(exePath), "config.yaml")
}

// Config はアプリケーション設定を表します。
type Config struct {
	Auth     AuthConfig     `yaml:"auth"`
	Database DatabaseConfig `yaml:"database"`
	Server   ServerConfig   `yaml:"server"`
	Storage  StorageConfig  `yaml:"storage"`
}

// ServerConfig はサーバー設定を表します。
type ServerConfig struct {
	Port           string   `yaml:"port"`
	ServiceName    string   `yaml:"service_name"`
	TrustedProxies []string `yaml:"trusted_proxies"`
	BehindProxy    bool     `yaml:"behind_proxy"`
	SecureCookie   bool     `yaml:"secure_cookie"` // HTTPS環境でのみ有効化する
}

// AuthConfig は認証プロバイダーの設定を表します。
// 認証プロバイダーは1つに限定します（Discordでの利用を主眼とした設計）。
type AuthConfig struct {
	Provider ProviderConfig `yaml:"provider"`
}

// ProviderConfig は認証プロバイダー1件分の設定を表します。
// Type には "discord"（Discord OAuth2） または "oidc"（汎用OIDC）を指定します。
type ProviderConfig struct {
	Name         string   `yaml:"name"`
	Type         string   `yaml:"type"`
	ClientID     string   `yaml:"client_id"`
	ClientSecret string   `yaml:"client_secret"`
	RedirectURL  string   `yaml:"redirect_url"`
	Scopes       []string `yaml:"scopes"`

	// Discord専用
	GuildID  string `yaml:"guild_id,omitempty"`
	BotToken string `yaml:"bot_token,omitempty"`

	// Discord専用: ゲートウェイ常時接続によるロールのリアルタイム同期の有効化。
	// 未指定(nil)は有効扱い。Server Members Intent が未許可の環境では、
	// 起動時に自動検出してREST方式へフォールバックする。
	GatewayEnabled *bool `yaml:"gateway_enabled,omitempty"`

	// 汎用OIDC専用
	Issuer      string `yaml:"issuer,omitempty"`
	GroupsClaim string `yaml:"groups_claim,omitempty"`

	// 汎用OIDC専用の任意アクセス許可リスト。
	// いずれも未設定の場合は認証できた全ユーザーを許可します。
	AllowedEmailDomains []string `yaml:"allowed_email_domains,omitempty"`
	AllowedEmails       []string `yaml:"allowed_emails,omitempty"`
}

// GatewayOn はゲートウェイ同期を試みるべきかを返します（未指定は有効）。
func (p *ProviderConfig) GatewayOn() bool {
	return p.GatewayEnabled == nil || *p.GatewayEnabled
}

// DatabaseConfig はデータベース設定を表します。
type DatabaseConfig struct {
	Path           string `yaml:"path"`
	MaxConnections int    `yaml:"max_connections"`
}

// StorageConfig はストレージ設定を表します。
type StorageConfig struct {
	UploadPath           string            `yaml:"upload_path"`
	AdminRoleID          string            `yaml:"admin_role_id"`
	Directories          []DirectoryConfig `yaml:"directories"`
	MaxFileSize          int64             `yaml:"max_file_size"`
	ChunkSize            int64             `yaml:"chunk_size"`
	MaxChunkFileSize     int64             `yaml:"max_chunk_file_size"`
	UploadSessionTTL     time.Duration     `yaml:"upload_session_ttl"`
	CleanupInterval      time.Duration     `yaml:"cleanup_interval"`
	MaxConcurrentUploads int               `yaml:"max_concurrent_uploads"`
	ChunkUploadEnabled   bool              `yaml:"chunk_upload_enabled"`
}

// DirectoryConfig はディレクトリ設定を表します。
type DirectoryConfig struct {
	Path string `yaml:"path"`
	Type string `yaml:"type"`
	// Grants はこのディレクトリへのアクセス付与一覧です。
	// ロール単位・メンバー単位で、それぞれに許可する操作を個別に指定できます。
	Grants []GrantConfig `yaml:"grants"`
}

// GrantConfig はディレクトリへのアクセス付与1件を表します。
// Role または User のいずれか一方を指定します。
//   - Role: Discordのロールid（"*" を指定すると全メンバーを対象）
//   - User: Discordのユーザーid（特定メンバー個人への付与）
//
// Permissions には "read" / "write" / "delete" のうち許可する操作を列挙します。
type GrantConfig struct {
	Role        string   `yaml:"role,omitempty"`
	User        string   `yaml:"user,omitempty"`
	Permissions []string `yaml:"permissions"`
}

// Load は設定ファイルを読み込み、環境変数で上書きします。
// 設定ファイルが存在しない場合、defaultTemplate（呼び出し側が埋め込むひな型）から生成します。
func Load(path string, defaultTemplate []byte) (*Config, error) {
	// 設定ファイルが存在するか確認
	// #nosec G304 - Configuration file path is intentionally provided by the application
	data, err := os.ReadFile(path)
	if err != nil {
		// ファイルが存在しない場合、埋め込みのひな型から生成する
		if os.IsNotExist(err) {
			if writeErr := writeDefaultConfig(path, defaultTemplate); writeErr != nil {
				return nil, fmt.Errorf("設定ファイルが見つかりません。既定のひな型からの生成にも失敗しました: %w", writeErr)
			}
			// 生成後に再度読み込み
			// #nosec G304 - Configuration file path is intentionally provided by the application
			data, err = os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("生成した設定ファイルの読み込みに失敗しました: %w", err)
			}
		} else {
			return nil, fmt.Errorf("設定ファイルの読み込みに失敗しました: %w", err)
		}
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("設定ファイルのパースに失敗しました: %w", err)
	}

	// 環境変数で上書き
	overrideFromEnv(&cfg)

	return &cfg, nil
}

// overrideFromEnv 環境変数で設定を上書きする
func overrideFromEnv(cfg *Config) {
	// Server設定
	if port := os.Getenv("SERVER_PORT"); port != "" {
		cfg.Server.Port = port
	}
	if serviceName := os.Getenv("SERVER_SERVICE_NAME"); serviceName != "" {
		cfg.Server.ServiceName = serviceName
	}
	if behindProxy := os.Getenv("SERVER_BEHIND_PROXY"); behindProxy != "" {
		cfg.Server.BehindProxy = behindProxy == "true"
	}
	if trustedProxies := os.Getenv("SERVER_TRUSTED_PROXIES"); trustedProxies != "" {
		cfg.Server.TrustedProxies = strings.Split(trustedProxies, ",")
	}

	// 認証プロバイダー（Discord含む）の設定は環境変数では上書きせず、
	// config.yaml のみで管理する。

	// Database設定
	if dbPath := os.Getenv("DATABASE_PATH"); dbPath != "" {
		cfg.Database.Path = dbPath
	}
	if maxConn := os.Getenv("DATABASE_MAX_CONNECTIONS"); maxConn != "" {
		if val, err := strconv.Atoi(maxConn); err == nil {
			cfg.Database.MaxConnections = val
		}
	}

	// Storage設定
	if uploadPath := os.Getenv("STORAGE_UPLOAD_PATH"); uploadPath != "" {
		cfg.Storage.UploadPath = uploadPath
	}
	if maxFileSize := os.Getenv("STORAGE_MAX_FILE_SIZE"); maxFileSize != "" {
		if val, err := strconv.ParseInt(maxFileSize, 10, 64); err == nil {
			cfg.Storage.MaxFileSize = val
		}
	}
	if chunkSize := os.Getenv("STORAGE_CHUNK_SIZE"); chunkSize != "" {
		if val, err := strconv.ParseInt(chunkSize, 10, 64); err == nil {
			cfg.Storage.ChunkSize = val
		}
	}
	if maxChunkFileSize := os.Getenv("STORAGE_MAX_CHUNK_FILE_SIZE"); maxChunkFileSize != "" {
		if val, err := strconv.ParseInt(maxChunkFileSize, 10, 64); err == nil {
			cfg.Storage.MaxChunkFileSize = val
		}
	}
	if maxConcurrent := os.Getenv("STORAGE_MAX_CONCURRENT_UPLOADS"); maxConcurrent != "" {
		if val, err := strconv.Atoi(maxConcurrent); err == nil {
			cfg.Storage.MaxConcurrentUploads = val
		}
	}
	if chunkEnabled := os.Getenv("STORAGE_CHUNK_UPLOAD_ENABLED"); chunkEnabled != "" {
		cfg.Storage.ChunkUploadEnabled = chunkEnabled == "true"
	}
	if sessionTTL := os.Getenv("STORAGE_UPLOAD_SESSION_TTL"); sessionTTL != "" {
		if val, err := time.ParseDuration(sessionTTL); err == nil {
			cfg.Storage.UploadSessionTTL = val
		}
	}
	if cleanupInterval := os.Getenv("STORAGE_CLEANUP_INTERVAL"); cleanupInterval != "" {
		if val, err := time.ParseDuration(cleanupInterval); err == nil {
			cfg.Storage.CleanupInterval = val
		}
	}
	if adminRoleID := os.Getenv("STORAGE_ADMIN_ROLE_ID"); adminRoleID != "" {
		cfg.Storage.AdminRoleID = adminRoleID
	}
}

// GetDirectoryConfig はディレクトリパスから設定を取得します。
func (c *Config) GetDirectoryConfig(path string) *DirectoryConfig {
	for i := range c.Storage.Directories {
		if c.Storage.Directories[i].Path == path {
			return &c.Storage.Directories[i]
		}
	}
	return nil
}

// HasAdminRole は与えられたロール集合に管理者ロールが含まれるかを返します。
// 管理者ロール（admin_role_id）が未設定の場合は常にfalseを返します。
func (c *Config) HasAdminRole(roles []string) bool {
	if c.Storage.AdminRoleID == "" {
		return false
	}
	for _, r := range roles {
		if r == c.Storage.AdminRoleID {
			return true
		}
	}
	return false
}

// StaticPermissions はロールに依存しない付与（"*" と 指定ユーザー）から
// 得られる許可操作の集合を返します。ロール取得の前に評価でき、
// 公開ディレクトリや個人指定はロール取得の失敗に影響されません。
func (d *DirectoryConfig) StaticPermissions(userID string) map[string]bool {
	result := make(map[string]bool)
	for _, g := range d.Grants {
		if g.Role == "*" || (g.User != "" && g.User == userID) {
			for _, p := range g.Permissions {
				result[p] = true
			}
		}
	}
	return result
}

// RolePermissions は保有ロール集合にマッチする付与から得られる許可操作の集合を返します。
func (d *DirectoryConfig) RolePermissions(roleSet map[string]bool) map[string]bool {
	result := make(map[string]bool)
	for _, g := range d.Grants {
		if g.Role != "" && g.Role != "*" && roleSet[g.Role] {
			for _, p := range g.Permissions {
				result[p] = true
			}
		}
	}
	return result
}

// EffectivePermissions はユーザーID（"*"・個人指定）と保有ロールの双方を考慮した、
// このディレクトリでの実効的な許可操作の集合を返します。
func (d *DirectoryConfig) EffectivePermissions(userID string, roleSet map[string]bool) map[string]bool {
	perms := d.StaticPermissions(userID)
	for p := range d.RolePermissions(roleSet) {
		perms[p] = true
	}
	return perms
}
