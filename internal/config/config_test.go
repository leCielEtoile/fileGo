package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 必須項目だけ書いた最小の config.yaml。既定値が効けばこれで起動できる。
const minimalYAML = `
auth:
  provider:
    type: discord
    bot_token: "x.y.z"
    client_id: "1"
    client_secret: "s"
    guild_id: "g"
    redirect_url: "http://localhost:8080/auth/callback"
storage:
  directories:
    - path: "public"
      grants:
        - role: "*"
          permissions: ["read"]
`

func loadFrom(t *testing.T, yaml string) (*Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}
	return Load(path, nil)
}

// 最小構成でも既定値が入り、壊れる値（0）が残らないこと。
// 特に cleanup_interval=0 は time.NewTicker をパニックさせ、
// max_concurrent_uploads=0 は全アップロードを拒否させる。
func TestMinimalConfigGetsSafeDefaults(t *testing.T) {
	cfg, err := loadFrom(t, minimalYAML)
	if err != nil {
		t.Fatalf("最小構成が読み込めない: %v", err)
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"server.port", cfg.Server.Port, defaultPort},
		{"server.service_name", cfg.Server.ServiceName, defaultServiceName},
		{"database.path", cfg.Database.Path, defaultDatabasePath},
		{"database.max_connections", cfg.Database.MaxConnections, defaultMaxConnections},
		{"storage.upload_path", cfg.Storage.UploadPath, defaultUploadPath},
		{"storage.max_file_size", cfg.Storage.MaxFileSize, int64(defaultMaxFileSize)},
		{"storage.chunk_size", cfg.Storage.ChunkSize, int64(defaultChunkSize)},
		{"storage.max_chunk_file_size", cfg.Storage.MaxChunkFileSize, int64(defaultMaxChunkFileSize)},
		{"storage.max_concurrent_uploads", cfg.Storage.MaxConcurrentUploads, defaultMaxConcurrentUploads},
		{"storage.upload_session_ttl", cfg.Storage.UploadSessionTTL, time.Duration(defaultUploadSessionTTL)},
		{"storage.cleanup_interval", cfg.Storage.CleanupInterval, time.Duration(defaultCleanupInterval)},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, 既定値 %v が入るべき", c.name, c.got, c.want)
		}
	}

	// 未指定のチャンクアップロードは「有効」扱い（省略で黙って無効化されない）
	if !cfg.Storage.ChunkUploadOn() {
		t.Error("chunk_upload_enabled 未指定なら有効であるべき")
	}
}

// 明示した値は既定値で上書きされないこと。
func TestExplicitValuesArePreserved(t *testing.T) {
	cfg, err := loadFrom(t, minimalYAML+`
server:
  port: "9999"
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != "9999" {
		t.Errorf("明示した port が既定値で潰された: %s", cfg.Server.Port)
	}
}

// chunk_upload_enabled: false は「無効」として尊重されること（未指定のnilと区別できる）。
func TestChunkUploadCanBeDisabledExplicitly(t *testing.T) {
	cfg, err := loadFrom(t, `
auth:
  provider:
    type: discord
    bot_token: "x.y.z"
    client_id: "1"
    client_secret: "s"
    guild_id: "g"
    redirect_url: "http://localhost:8080/auth/callback"
storage:
  chunk_upload_enabled: false
  directories:
    - path: "public"
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Storage.ChunkUploadOn() {
		t.Error("chunk_upload_enabled: false は無効として扱うべき")
	}
}

// 必須項目の欠落は起動時に、何が足りないか分かる形で失敗すること。
func TestValidateReportsMissingFields(t *testing.T) {
	_, err := loadFrom(t, `
auth:
  provider:
    type: discord
    bot_token: "x.y.z"
storage:
  directories:
    - path: "public"
`)
	if err == nil {
		t.Fatal("必須項目が欠けているのに起動できてしまう")
	}
	for _, want := range []string{"client_id", "client_secret", "guild_id", "redirect_url"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("エラーに %q が含まれず、何を直せばよいか分からない: %v", want, err)
		}
	}
}

func TestValidateRejectsBadProviderType(t *testing.T) {
	if _, err := loadFrom(t, `
auth:
  provider:
    type: ""
storage:
  directories:
    - path: "public"
`); err == nil {
		t.Error("type 未設定を検出できていない")
	}

	if _, err := loadFrom(t, `
auth:
  provider:
    type: saml
storage:
  directories:
    - path: "public"
`); err == nil {
		t.Error("未対応の type を検出できていない")
	}
}

func TestValidateRequiresDirectories(t *testing.T) {
	_, err := loadFrom(t, `
auth:
  provider:
    type: discord
    bot_token: "x.y.z"
    client_id: "1"
    client_secret: "s"
    guild_id: "g"
    redirect_url: "http://localhost/auth/callback"
`)
	if err == nil || !strings.Contains(err.Error(), "storage.directories") {
		t.Errorf("directories が空なのに検出できていない: %v", err)
	}
}

// OIDC は issuer が必須（bot_token/guild_id は不要）。
func TestValidateOIDCRequiresIssuer(t *testing.T) {
	_, err := loadFrom(t, `
auth:
  provider:
    type: oidc
    client_id: "1"
    client_secret: "s"
    redirect_url: "http://localhost/auth/callback"
storage:
  directories:
    - path: "public"
`)
	if err == nil || !strings.Contains(err.Error(), "issuer") {
		t.Errorf("OIDCのissuer未設定を検出できていない: %v", err)
	}
}
