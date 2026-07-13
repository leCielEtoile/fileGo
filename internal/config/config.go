// Package config はアプリケーション設定の読み込みと管理を提供します。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ResolvePath は読み込む設定ファイルのパスを決定します。
// FILEGO_CONFIG_PATH が設定されている場合はそれを優先し、
// 未設定の場合は実行ファイルと同じディレクトリの "config.yaml" を既定値とします。
func ResolvePath() string {
	if envPath := os.Getenv(EnvPrefix + "CONFIG_PATH"); envPath != "" {
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
	// LogLevel はログレベル（debug / info / warn / error）。既定は info。
	LogLevel string         `yaml:"log_level"`
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

	// RequiredRoles はログインに必要なロールです（いずれか1つを保有していればよい）。
	// 未設定なら在籍しているだけでログインできます（従来動作）。
	// Discordはロール、OIDCは groups_claim の値と照合します。
	RequiredRoles []string `yaml:"required_roles,omitempty"`

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
	// ChunkUploadEnabled は未指定(nil)を「有効」として扱うためポインタにしています。
	// boolのままだと省略時にゼロ値(false)となり、チャンクアップロードが黙って無効化される。
	ChunkUploadEnabled *bool `yaml:"chunk_upload_enabled"`
}

// ChunkUploadOn はチャンクアップロードを有効にすべきかを返します（未指定は有効）。
func (s *StorageConfig) ChunkUploadOn() bool {
	return s.ChunkUploadEnabled == nil || *s.ChunkUploadEnabled
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

	// 省略された項目に既定値を入れる（環境変数より先。環境変数が最優先）。
	applyDefaults(&cfg)

	// 環境変数で上書き。値が不正なら黙って無視せずエラーにする。
	if err := applyEnvOverrides(&cfg); err != nil {
		return nil, err
	}

	// 誤った設定は起動時に検出する（実行時の不可解な失敗を避ける）。
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// 省略時に適用する既定値。ゼロ値のまま使うと壊れる項目があるため必ず埋める。
// 例: cleanup_interval が0だと time.NewTicker(0) がパニックする。
// max_concurrent_uploads が0だと「同時数 >= 0」が常に真になり全アップロードが拒否される。
const (
	defaultLogLevel             = "info"
	defaultPort                 = "8080"
	defaultServiceName          = "Discord File Server"
	defaultDatabasePath         = "./data/fileserver.db"
	defaultMaxConnections       = 10
	defaultUploadPath           = "./data/uploads"
	defaultMaxFileSize          = 100 * 1024 * 1024        // 100MB
	defaultChunkSize            = 20 * 1024 * 1024         // 20MB
	defaultMaxChunkFileSize     = 500 * 1024 * 1024 * 1024 // 500GB
	defaultMaxConcurrentUploads = 3
	defaultUploadSessionTTL     = 48 * time.Hour
	defaultCleanupInterval      = time.Hour
)

// applyDefaults は未設定（ゼロ値）の項目に既定値を入れます。
// これにより config.yaml は「既定から変えたい項目だけ」書けばよくなります。
func applyDefaults(cfg *Config) {
	if cfg.LogLevel == "" {
		cfg.LogLevel = defaultLogLevel
	}
	if cfg.Server.Port == "" {
		cfg.Server.Port = defaultPort
	}
	if cfg.Server.ServiceName == "" {
		cfg.Server.ServiceName = defaultServiceName
	}

	if cfg.Database.Path == "" {
		cfg.Database.Path = defaultDatabasePath
	}
	if cfg.Database.MaxConnections <= 0 {
		cfg.Database.MaxConnections = defaultMaxConnections
	}

	if cfg.Storage.UploadPath == "" {
		cfg.Storage.UploadPath = defaultUploadPath
	}
	if cfg.Storage.MaxFileSize <= 0 {
		cfg.Storage.MaxFileSize = defaultMaxFileSize
	}
	if cfg.Storage.ChunkSize <= 0 {
		cfg.Storage.ChunkSize = defaultChunkSize
	}
	if cfg.Storage.MaxChunkFileSize <= 0 {
		cfg.Storage.MaxChunkFileSize = defaultMaxChunkFileSize
	}
	if cfg.Storage.MaxConcurrentUploads <= 0 {
		cfg.Storage.MaxConcurrentUploads = defaultMaxConcurrentUploads
	}
	if cfg.Storage.UploadSessionTTL <= 0 {
		cfg.Storage.UploadSessionTTL = defaultUploadSessionTTL
	}
	if cfg.Storage.CleanupInterval <= 0 {
		cfg.Storage.CleanupInterval = defaultCleanupInterval
	}
}

// Validate は設定の不備を起動時に検出します。
// 実行時に不可解なエラーとして現れるより、何をどう直せばよいかを起動時に示します。
func (c *Config) Validate() error {
	p := c.Auth.Provider

	switch p.Type {
	case "discord":
		if err := requireAll(map[string]string{
			"auth.provider.bot_token":     p.BotToken,
			"auth.provider.client_id":     p.ClientID,
			"auth.provider.client_secret": p.ClientSecret,
			"auth.provider.guild_id":      p.GuildID,
			"auth.provider.redirect_url":  p.RedirectURL,
		}); err != nil {
			return err
		}
	case "oidc":
		if err := requireAll(map[string]string{
			"auth.provider.issuer":        p.Issuer,
			"auth.provider.client_id":     p.ClientID,
			"auth.provider.client_secret": p.ClientSecret,
			"auth.provider.redirect_url":  p.RedirectURL,
		}); err != nil {
			return err
		}
	case "":
		return fmt.Errorf("auth.provider.type が未設定です（\"discord\" または \"oidc\" を指定してください）")
	default:
		return fmt.Errorf("auth.provider.type が不正です: %q（\"discord\" または \"oidc\" を指定してください）", p.Type)
	}

	if len(c.Storage.Directories) == 0 {
		return fmt.Errorf("storage.directories が空です（最低1つのディレクトリを定義してください）")
	}
	for i, d := range c.Storage.Directories {
		if d.Path == "" {
			return fmt.Errorf("storage.directories[%d].path が未設定です", i)
		}
	}

	return nil
}

// requireAll は空文字の項目があれば、どれが足りないかを示すエラーを返します。
func requireAll(fields map[string]string) error {
	missing := make([]string, 0, len(fields))
	for name, v := range fields {
		if strings.TrimSpace(v) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("必須の設定が未設定です: %s", strings.Join(missing, ", "))
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
