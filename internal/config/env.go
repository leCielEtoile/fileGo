package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// EnvPrefix は環境変数の接頭辞です。
// `SERVER_PORT` や `DATABASE_PATH` のような一般的な名前は、共有環境（Kubernetes・PaaS・CI）で
// 他コンポーネントの変数と衝突し得るため、アプリ固有の接頭辞を付けます。
const EnvPrefix = "FILEGO_"

// legacyEnvNames は接頭辞導入前に使われていた名前です。
// 黙って無視すると「設定したのに効かない」事故になるため、検出したら警告します。
var legacyEnvNames = []string{
	"CONFIG_PATH", "LOG_LEVEL",
	"SERVER_PORT", "SERVER_SERVICE_NAME", "SERVER_BEHIND_PROXY", "SERVER_TRUSTED_PROXIES",
	"DATABASE_PATH", "DATABASE_MAX_CONNECTIONS",
	"STORAGE_UPLOAD_PATH", "STORAGE_MAX_FILE_SIZE", "STORAGE_CHUNK_SIZE",
	"STORAGE_MAX_CHUNK_FILE_SIZE", "STORAGE_MAX_CONCURRENT_UPLOADS",
	"STORAGE_CHUNK_UPLOAD_ENABLED", "STORAGE_UPLOAD_SESSION_TTL",
	"STORAGE_CLEANUP_INTERVAL", "STORAGE_ADMIN_ROLE_ID",
}

// WarnLegacyEnv は接頭辞なしの旧名が設定されていれば警告します。
// 旧名は読まないため、黙って無視されるとデバッグ困難な事故になります。
func WarnLegacyEnv() {
	for _, name := range legacyEnvNames {
		if os.Getenv(name) == "" {
			continue
		}
		// 接頭辞付きが設定済みなら、旧名は単なる残骸か他コンポーネントの変数。
		if os.Getenv(EnvPrefix+name) != "" {
			continue
		}
		slog.Warn("接頭辞なしの環境変数は読み込まれません。名前を変更してください",
			"検出した名前", name, "使うべき名前", EnvPrefix+name)
	}
}

// env は接頭辞付き環境変数の値を返します（未設定なら空文字）。
func env(name string) string {
	return os.Getenv(EnvPrefix + name)
}

// envString は設定されていれば dst を上書きします。
func envString(name string, dst *string) {
	if v := env(name); v != "" {
		*dst = v
	}
}

// envBool は真偽値を解釈します。"1" / "TRUE" / "True" なども正しく扱います
// （旧実装は `== "true"` の比較だったため、`1` を指定すると黙って false になっていました）。
func envBool(name string, dst *bool) error {
	v := env(name)
	if v == "" {
		return nil
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return fmt.Errorf("環境変数 %s%s の値が真偽値として不正です: %q（true/false/1/0 を指定してください）", EnvPrefix, name, v)
	}
	*dst = parsed
	return nil
}

// envBoolPtr は未指定(nil)と明示的なfalseを区別する項目のための版です。
func envBoolPtr(name string, dst **bool) error {
	v := env(name)
	if v == "" {
		return nil
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return fmt.Errorf("環境変数 %s%s の値が真偽値として不正です: %q（true/false/1/0 を指定してください）", EnvPrefix, name, v)
	}
	*dst = &parsed
	return nil
}

func envInt(name string, dst *int) error {
	v := env(name)
	if v == "" {
		return nil
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("環境変数 %s%s の値が整数として不正です: %q", EnvPrefix, name, v)
	}
	*dst = parsed
	return nil
}

func envInt64(name string, dst *int64) error {
	v := env(name)
	if v == "" {
		return nil
	}
	parsed, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fmt.Errorf("環境変数 %s%s の値が整数として不正です: %q", EnvPrefix, name, v)
	}
	*dst = parsed
	return nil
}

func envDuration(name string, dst *time.Duration) error {
	v := env(name)
	if v == "" {
		return nil
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return fmt.Errorf("環境変数 %s%s の値が期間として不正です: %q（例: 48h, 1h30m）", EnvPrefix, name, v)
	}
	*dst = parsed
	return nil
}

// envSecretFile は `*_FILE` 規約で秘密情報をファイルから読み込みます。
// 秘密情報そのものを環境変数へ入れると `docker inspect` やプロセス一覧、
// ログ経由で漏れるため、環境変数には「ファイルのパス」だけを渡します。
// Docker secrets / Kubernetes の Secret ボリューム（/run/secrets/...）を素直に使えます。
func envSecretFile(name string, dst *string) error {
	path := env(name + "_FILE")
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path) // #nosec G304 - 運用者が明示的に指定した秘密情報のパス
	if err != nil {
		return fmt.Errorf("環境変数 %s%s_FILE が指すファイルを読み込めません: %w", EnvPrefix, name, err)
	}
	secret := strings.TrimSpace(string(data))
	if secret == "" {
		return fmt.Errorf("環境変数 %s%s_FILE が指すファイルが空です: %s", EnvPrefix, name, path)
	}
	*dst = secret
	return nil
}

// applyEnvOverrides は環境変数で設定を上書きします（config.yaml より優先）。
// 値が不正な場合は黙って無視せずエラーにします。旧実装は解釈に失敗すると
// その項目を無視していたため、運用者は「設定したつもり」で気付けませんでした。
//
// 認証情報の「値」は環境変数から取りません（docker inspect 等で漏れるため）。
// 代わりに `*_FILE` でファイルパスを渡し、中身をファイルから読みます。
func applyEnvOverrides(cfg *Config) error {
	envString("LOG_LEVEL", &cfg.LogLevel)

	// Server
	envString("SERVER_PORT", &cfg.Server.Port)
	envString("SERVER_SERVICE_NAME", &cfg.Server.ServiceName)
	if err := envBool("SERVER_BEHIND_PROXY", &cfg.Server.BehindProxy); err != nil {
		return err
	}
	if err := envBool("SERVER_SECURE_COOKIE", &cfg.Server.SecureCookie); err != nil {
		return err
	}
	if v := env("SERVER_TRUSTED_PROXIES"); v != "" {
		cfg.Server.TrustedProxies = splitAndTrim(v)
	}

	// Database
	envString("DATABASE_PATH", &cfg.Database.Path)
	if err := envInt("DATABASE_MAX_CONNECTIONS", &cfg.Database.MaxConnections); err != nil {
		return err
	}

	// Storage
	envString("STORAGE_UPLOAD_PATH", &cfg.Storage.UploadPath)
	envString("STORAGE_ADMIN_ROLE_ID", &cfg.Storage.AdminRoleID)
	if err := envInt64("STORAGE_MAX_FILE_SIZE", &cfg.Storage.MaxFileSize); err != nil {
		return err
	}
	if err := envInt64("STORAGE_CHUNK_SIZE", &cfg.Storage.ChunkSize); err != nil {
		return err
	}
	if err := envInt64("STORAGE_MAX_CHUNK_FILE_SIZE", &cfg.Storage.MaxChunkFileSize); err != nil {
		return err
	}
	if err := envInt("STORAGE_MAX_CONCURRENT_UPLOADS", &cfg.Storage.MaxConcurrentUploads); err != nil {
		return err
	}
	if err := envBoolPtr("STORAGE_CHUNK_UPLOAD_ENABLED", &cfg.Storage.ChunkUploadEnabled); err != nil {
		return err
	}
	if err := envDuration("STORAGE_UPLOAD_SESSION_TTL", &cfg.Storage.UploadSessionTTL); err != nil {
		return err
	}
	if err := envDuration("STORAGE_CLEANUP_INTERVAL", &cfg.Storage.CleanupInterval); err != nil {
		return err
	}

	// 認証情報（値は環境変数から取らず、ファイル経由のみ）
	if err := envSecretFile("BOT_TOKEN", &cfg.Auth.Provider.BotToken); err != nil {
		return err
	}
	return envSecretFile("CLIENT_SECRET", &cfg.Auth.Provider.ClientSecret)
}

// splitAndTrim はカンマ区切りを分割し、各要素の空白を除去します。
func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
