// Package config はアプリケーション設定の読み込みと管理を提供します。
package config

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config はアプリケーション設定を表します。
type Config struct {
	Discord  DiscordConfig  `yaml:"discord"`
	Database DatabaseConfig `yaml:"database"`
	Server   ServerConfig   `yaml:"server"`
	Storage  StorageConfig  `yaml:"storage"`
}

// ServerConfig はサーバー設定を表します。
type ServerConfig struct {
	Port           string   `yaml:"port"`            // 16 bytes
	ServiceName    string   `yaml:"service_name"`    // 16 bytes
	TrustedProxies []string `yaml:"trusted_proxies"` // 24 bytes
	BehindProxy    bool     `yaml:"behind_proxy"`    // 1 byte
	SecureCookie   bool     `yaml:"secure_cookie"`   // 1 byte (HTTPS環境でのみtrue)
}

// DiscordConfig はDiscord設定を表します。
type DiscordConfig struct {
	BotToken     string `yaml:"bot_token"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	GuildID      string `yaml:"guild_id"`
	RedirectURL  string `yaml:"redirect_url"`
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
	Path          string   `yaml:"path"`
	Type          string   `yaml:"type"`
	RequiredRoles []string `yaml:"required_roles"`
	Permissions   []string `yaml:"permissions"`
}

// Load は設定ファイルを読み込み、環境変数で上書きします。
// 設定ファイルが存在しない場合、config.yaml.exampleからコピーを試みます。
func Load(path string) (*Config, error) {
	// 設定ファイルが存在するか確認
	// #nosec G304 - Configuration file path is intentionally provided by the application
	data, err := os.ReadFile(path)
	if err != nil {
		// ファイルが存在しない場合、config.yaml.exampleからコピーを試みる
		if os.IsNotExist(err) {
			if copyErr := copyConfigFromExample(path); copyErr != nil {
				return nil, fmt.Errorf("設定ファイルが見つかりません。config.yaml.exampleからのコピーにも失敗しました: %w", copyErr)
			}
			// コピー後に再度読み込み
			// #nosec G304 - Configuration file path is intentionally provided by the application
			data, err = os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("コピーした設定ファイルの読み込みに失敗しました: %w", err)
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

// copyConfigFromExample はconfig.yaml.exampleを指定されたパスにコピーします。
// まずローカルのファイルを探し、見つからない場合はGitHubリポジトリからダウンロードします。
func copyConfigFromExample(destPath string) error {
	// 1. ローカルのconfig.yaml.exampleを探す
	examplePaths := []string{
		"config.yaml.example",
		"../config.yaml.example",
		"../../config.yaml.example",
	}

	var exampleData []byte
	foundExample := false

	for _, examplePath := range examplePaths {
		// #nosec G304 - Configuration file path is intentionally provided by the application
		data, err := os.ReadFile(examplePath)
		if err == nil {
			exampleData = data
			foundExample = true
			break
		}
	}

	// 2. ローカルに見つからない場合、GitHubからダウンロード
	if !foundExample {
		data, err := downloadConfigFromGitHub()
		if err != nil {
			return fmt.Errorf("config.yaml.exampleが見つかりません。GitHubからのダウンロードにも失敗しました: %w", err)
		}
		exampleData = data
	}

	// 3. 設定ファイルを書き込み
	// #nosec G306 - Configuration file permissions are intentionally set
	if err := os.WriteFile(destPath, exampleData, 0644); err != nil {
		return fmt.Errorf("設定ファイルの書き込みに失敗しました: %w", err)
	}

	return nil
}

// downloadConfigFromGitHub はGitHubリポジトリからconfig.yaml.exampleをダウンロードします。
func downloadConfigFromGitHub() ([]byte, error) {
	// Git リモートURLを取得
	remoteURL, branch, err := getGitRemoteInfo()
	if err != nil {
		return nil, fmt.Errorf("git リモート情報の取得に失敗しました: %w", err)
	}

	// GitHub URLをraw.githubusercontent.com形式に変換
	rawURL := convertToRawGitHubURL(remoteURL, branch)
	if rawURL == "" {
		return nil, fmt.Errorf("GitHubのraw URLへの変換に失敗しました。リポジトリURL: %s", remoteURL)
	}

	// HTTPリクエストでダウンロード
	// #nosec G107 - URL is constructed from git remote, not user input
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("GitHubからのダウンロードリクエストの作成に失敗しました: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GitHubからのダウンロードに失敗しました: %w", err)
	}
	defer func() {
		_ = resp.Body.Close() //nolint:errcheck // Body close error is intentionally ignored
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHubからのダウンロードに失敗しました。ステータスコード: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("レスポンスの読み込みに失敗しました: %w", err)
	}

	return data, nil
}

// getGitRemoteInfo はGitリポジトリのリモートURLとブランチを取得します。
func getGitRemoteInfo() (string, string, error) {
	ctx := context.Background()

	// リモートURL取得
	cmd := exec.CommandContext(ctx, "git", "config", "--get", "remote.origin.url")
	output, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("git remote URLの取得に失敗しました: %w", err)
	}
	remoteURL := strings.TrimSpace(string(output))

	// ブランチ取得
	cmd = exec.CommandContext(ctx, "git", "branch", "--show-current")
	output, err = cmd.Output()
	if err != nil {
		// ブランチ取得に失敗した場合はmainをデフォルトにする
		return remoteURL, "main", nil
	}
	branch := strings.TrimSpace(string(output))
	if branch == "" {
		branch = "main"
	}

	return remoteURL, branch, nil
}

// convertToRawGitHubURL はGitHubのリポジトリURLをraw.githubusercontent.com形式に変換します。
func convertToRawGitHubURL(repoURL, branch string) string {
	// https://github.com/user/repo.git -> https://raw.githubusercontent.com/user/repo/branch/config.yaml.example
	// git@github.com:user/repo.git -> https://raw.githubusercontent.com/user/repo/branch/config.yaml.example

	var owner, repo string

	if strings.HasPrefix(repoURL, "https://github.com/") {
		// HTTPS形式
		path := strings.TrimPrefix(repoURL, "https://github.com/")
		path = strings.TrimSuffix(path, ".git")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) != 2 {
			return ""
		}
		owner, repo = parts[0], parts[1]
	} else if strings.HasPrefix(repoURL, "git@github.com:") {
		// SSH形式
		path := strings.TrimPrefix(repoURL, "git@github.com:")
		path = strings.TrimSuffix(path, ".git")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) != 2 {
			return ""
		}
		owner, repo = parts[0], parts[1]
	} else {
		return ""
	}

	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/config.yaml.example", owner, repo, branch)
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

// HasPermission は指定された権限を持っているか確認します。
func (d *DirectoryConfig) HasPermission(permission string) bool {
	for _, p := range d.Permissions {
		if p == permission {
			return true
		}
	}
	return false
}
