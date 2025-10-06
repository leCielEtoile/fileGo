package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config アプリケーション設定
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Discord  DiscordConfig  `yaml:"discord"`
	Database DatabaseConfig `yaml:"database"`
	Storage  StorageConfig  `yaml:"storage"`
}

// ServerConfig サーバー設定
type ServerConfig struct {
	Port           string   `yaml:"port"`            // 16 bytes
	TrustedProxies []string `yaml:"trusted_proxies"` // 24 bytes
	BehindProxy    bool     `yaml:"behind_proxy"`    // 1 byte
	SecureCookie   bool     `yaml:"secure_cookie"`   // 1 byte (HTTPS環境でのみtrue)
}

// DiscordConfig Discord設定
type DiscordConfig struct {
	BotToken     string `yaml:"bot_token"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	GuildID      string `yaml:"guild_id"`
	RedirectURL  string `yaml:"redirect_url"`
}

// DatabaseConfig データベース設定
type DatabaseConfig struct {
	Path           string `yaml:"path"`
	MaxConnections int    `yaml:"max_connections"`
}

// StorageConfig ストレージ設定
type StorageConfig struct {
	UploadPath           string            `yaml:"upload_path"`            // 16 bytes
	Directories          []DirectoryConfig `yaml:"directories"`            // 24 bytes
	UploadSessionTTL     time.Duration     `yaml:"upload_session_ttl"`     // 8 bytes
	CleanupInterval      time.Duration     `yaml:"cleanup_interval"`       // 8 bytes
	MaxFileSize          int64             `yaml:"max_file_size"`          // 8 bytes
	ChunkSize            int64             `yaml:"chunk_size"`             // 8 bytes
	MaxChunkFileSize     int64             `yaml:"max_chunk_file_size"`    // 8 bytes
	MaxConcurrentUploads int               `yaml:"max_concurrent_uploads"` // 8 bytes
	ChunkUploadEnabled   bool              `yaml:"chunk_upload_enabled"`   // 1 byte
}

// DirectoryConfig ディレクトリ設定
type DirectoryConfig struct {
	Path          string   `yaml:"path"`
	RequiredRoles []string `yaml:"required_roles"`
	Permissions   []string `yaml:"permissions"`
}

// Load 設定ファイルを読み込み、環境変数で上書きする
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("設定ファイルの読み込みに失敗しました: %w", err)
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
	if behindProxy := os.Getenv("SERVER_BEHIND_PROXY"); behindProxy != "" {
		cfg.Server.BehindProxy = behindProxy == "true"
	}
	if trustedProxies := os.Getenv("SERVER_TRUSTED_PROXIES"); trustedProxies != "" {
		cfg.Server.TrustedProxies = strings.Split(trustedProxies, ",")
	}

	// Discord設定
	if botToken := os.Getenv("DISCORD_BOT_TOKEN"); botToken != "" {
		cfg.Discord.BotToken = botToken
	}
	if clientID := os.Getenv("DISCORD_CLIENT_ID"); clientID != "" {
		cfg.Discord.ClientID = clientID
	}
	if clientSecret := os.Getenv("DISCORD_CLIENT_SECRET"); clientSecret != "" {
		cfg.Discord.ClientSecret = clientSecret
	}
	if guildID := os.Getenv("DISCORD_GUILD_ID"); guildID != "" {
		cfg.Discord.GuildID = guildID
	}
	if redirectURL := os.Getenv("DISCORD_REDIRECT_URL"); redirectURL != "" {
		cfg.Discord.RedirectURL = redirectURL
	}

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
}

// GetDirectoryConfig ディレクトリパスから設定を取得
func (c *Config) GetDirectoryConfig(path string) *DirectoryConfig {
	for i := range c.Storage.Directories {
		if c.Storage.Directories[i].Path == path {
			return &c.Storage.Directories[i]
		}
	}
	return nil
}

// HasPermission 指定された権限を持っているか確認
func (d *DirectoryConfig) HasPermission(permission string) bool {
	for _, p := range d.Permissions {
		if p == permission {
			return true
		}
	}
	return false
}
