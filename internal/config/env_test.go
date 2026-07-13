package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 不正な値は黙って無視せず、起動時にエラーにすること。
// 旧実装は解釈に失敗すると項目を無視していたため、運用者は
// 「設定したつもり」のまま既定値で動いていることに気付けなかった。
func TestInvalidEnvValueFailsFast(t *testing.T) {
	cases := map[string]string{
		"FILEGO_DATABASE_MAX_CONNECTIONS":       "abc",
		"FILEGO_STORAGE_MAX_FILE_SIZE":          "1MB",
		"FILEGO_STORAGE_CLEANUP_INTERVAL":       "60",
		"FILEGO_SERVER_BEHIND_PROXY":            "yes",
		"FILEGO_STORAGE_CHUNK_UPLOAD_ENABLED":   "on",
		"FILEGO_STORAGE_MAX_CONCURRENT_UPLOADS": "many",
	}
	for name, bad := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, bad)
			_, err := loadFrom(t, minimalYAML)
			if err == nil {
				t.Fatalf("%s=%q は不正なのに起動できてしまう（黙って無視されている）", name, bad)
			}
			if !strings.Contains(err.Error(), name) {
				t.Errorf("どの環境変数が悪いのか分からないエラー: %v", err)
			}
		})
	}
}

// bool は "1" / "TRUE" / "True" なども正しく解釈すること。
// 旧実装は `== "true"` の比較だったため、"1" は黙って false になっていた。
func TestBoolEnvAcceptsCommonForms(t *testing.T) {
	for _, v := range []string{"1", "true", "TRUE", "True", "t"} {
		t.Run("真:"+v, func(t *testing.T) {
			t.Setenv("FILEGO_SERVER_BEHIND_PROXY", v)
			cfg, err := loadFrom(t, minimalYAML)
			if err != nil {
				t.Fatal(err)
			}
			if !cfg.Server.BehindProxy {
				t.Errorf("%q が真として解釈されていない", v)
			}
		})
	}
	for _, v := range []string{"0", "false", "FALSE", "f"} {
		t.Run("偽:"+v, func(t *testing.T) {
			t.Setenv("FILEGO_SERVER_BEHIND_PROXY", v)
			cfg, err := loadFrom(t, minimalYAML)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Server.BehindProxy {
				t.Errorf("%q が偽として解釈されていない", v)
			}
		})
	}
}

// 環境変数は config.yaml より優先されること。
func TestEnvOverridesConfigFile(t *testing.T) {
	t.Setenv("FILEGO_SERVER_PORT", "19999")
	t.Setenv("FILEGO_LOG_LEVEL", "debug")
	cfg, err := loadFrom(t, minimalYAML+`
server:
  port: "8080"
log_level: "info"
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != "19999" {
		t.Errorf("環境変数がconfig.yamlを上書きしていない: %s", cfg.Server.Port)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("log_level が環境変数で上書きされていない: %s", cfg.LogLevel)
	}
}

// secure_cookie も環境変数で設定できること（以前は環境変数が存在しなかった）。
func TestSecureCookieEnv(t *testing.T) {
	t.Setenv("FILEGO_SERVER_SECURE_COOKIE", "false")
	cfg, err := loadFrom(t, minimalYAML+`
server:
  secure_cookie: true
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.SecureCookie {
		t.Error("FILEGO_SERVER_SECURE_COOKIE=false が反映されていない")
	}
}

// 秘密情報は「値」ではなく「ファイルのパス」を環境変数で渡す（*_FILE 規約）。
// Docker secrets / K8s Secret ボリュームを素直に使えるようにするため。
func TestSecretFileEnv(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "bot_token")
	// 末尾改行があっても正しく読めること（ファイル配布では付きやすい）
	if err := os.WriteFile(tokenPath, []byte("secret-token-from-file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FILEGO_BOT_TOKEN_FILE", tokenPath)

	cfg, err := loadFrom(t, minimalYAML)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.Provider.BotToken != "secret-token-from-file" {
		t.Errorf("ファイルから秘密情報を読めていない: %q", cfg.Auth.Provider.BotToken)
	}
}

// 指定されたファイルが無い/空なら、黙って既定値で動かずエラーにすること。
func TestSecretFileMissingFails(t *testing.T) {
	t.Setenv("FILEGO_BOT_TOKEN_FILE", filepath.Join(t.TempDir(), "nope"))
	if _, err := loadFrom(t, minimalYAML); err == nil {
		t.Fatal("存在しない秘密情報ファイルを指定したのに起動できてしまう")
	}

	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(empty, []byte("  \n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FILEGO_BOT_TOKEN_FILE", empty)
	if _, err := loadFrom(t, minimalYAML); err == nil {
		t.Fatal("空の秘密情報ファイルなのに起動できてしまう")
	}
}

// 接頭辞なしの旧名は読み込まれないこと（衝突を避けるため）。
func TestLegacyEnvNamesAreIgnored(t *testing.T) {
	t.Setenv("SERVER_PORT", "12345")
	cfg, err := loadFrom(t, minimalYAML)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port == "12345" {
		t.Error("接頭辞なしの旧名が読み込まれている（衝突の温床になる）")
	}
}
