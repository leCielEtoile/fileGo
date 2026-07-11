// Package database はデータベースの初期化と管理機能を提供します。
package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	_ "modernc.org/sqlite"
)

// Initialize はデータベース接続を初期化し、テーブルが存在しない場合は作成します。
// maxConns は最大接続数（0以下の場合は既定値を使用）です。
func Initialize(dbPath string, maxConns int) (*sql.DB, error) {
	// WAL と busy_timeout を有効化し、同時アクセス時の "database is locked" を軽減する。
	// DSN の pragma はコネクションごとに適用される。
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)",
		url.PathEscape(dbPath))

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("データベースオープンエラー: %w", err)
	}

	// 接続数の上限を設定する。未設定（0以下）の場合は安全側の既定値を使う。
	if maxConns <= 0 {
		maxConns = 10
	}
	db.SetMaxOpenConns(maxConns)

	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("データベース接続エラー: %w", err)
	}

	if err := createTables(db); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, fmt.Errorf("テーブル作成エラー: %w (クローズエラー: %w)", err, closeErr)
		}
		return nil, fmt.Errorf("テーブル作成エラー: %w", err)
	}

	return db, nil
}

func createTables(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		provider TEXT NOT NULL,
		subject TEXT NOT NULL,
		username TEXT NOT NULL,
		avatar TEXT,
		email TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_login DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(provider, subject)
	);

	CREATE TABLE IF NOT EXISTS sessions (
		session_token TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		provider TEXT NOT NULL,
		expires_at DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
	CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

	CREATE TABLE IF NOT EXISTS access_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id TEXT,
		action TEXT NOT NULL,
		filename TEXT,
		filepath TEXT,
		filesize INTEGER,
		ip_address TEXT,
		user_agent TEXT,
		status_code INTEGER,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL
	);

	CREATE INDEX IF NOT EXISTS idx_access_logs_user_id ON access_logs(user_id);
	CREATE INDEX IF NOT EXISTS idx_access_logs_timestamp ON access_logs(timestamp);
	CREATE INDEX IF NOT EXISTS idx_access_logs_action ON access_logs(action);

	CREATE TABLE IF NOT EXISTS file_metadata (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		directory TEXT NOT NULL,
		filename TEXT NOT NULL,
		uploader_id TEXT,
		uploader_name TEXT,
		hash TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(directory, filename),
		FOREIGN KEY (uploader_id) REFERENCES users(id) ON DELETE SET NULL
	);

	CREATE INDEX IF NOT EXISTS idx_file_metadata_directory ON file_metadata(directory);
	CREATE INDEX IF NOT EXISTS idx_file_metadata_filename ON file_metadata(filename);
	CREATE INDEX IF NOT EXISTS idx_file_metadata_uploader_id ON file_metadata(uploader_id);

	-- OIDCプロバイダーのロールを永続化する。
	-- OIDCのロールはログイン時のID Tokenからしか得られず、Discordのように
	-- サーバー側で随時再取得できないため、再起動後もロールを復元できるよう保存する。
	CREATE TABLE IF NOT EXISTS oidc_user_roles (
		provider TEXT NOT NULL,
		subject TEXT NOT NULL,
		roles TEXT NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (provider, subject)
	);
	`

	ctx := context.Background()
	_, err := db.ExecContext(ctx, schema)
	return err
}
