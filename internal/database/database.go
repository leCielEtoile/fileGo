// Package database はデータベースの初期化と管理機能を提供します。
package database

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Initialize はデータベース接続を初期化し、テーブルが存在しない場合は作成します。
func Initialize(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("データベースオープンエラー: %w", err)
	}

	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("データベース接続エラー: %w", err)
	}

	// テーブル作成
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
		discord_id TEXT PRIMARY KEY,
		username TEXT NOT NULL,
		discriminator TEXT NOT NULL,
		avatar TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_login DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS sessions (
		session_token TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		discord_access_token TEXT NOT NULL,
		discord_refresh_token TEXT,
		expires_at DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(discord_id) ON DELETE CASCADE
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
		FOREIGN KEY (user_id) REFERENCES users(discord_id) ON DELETE SET NULL
	);

	CREATE INDEX IF NOT EXISTS idx_access_logs_user_id ON access_logs(user_id);
	CREATE INDEX IF NOT EXISTS idx_access_logs_timestamp ON access_logs(timestamp);
	CREATE INDEX IF NOT EXISTS idx_access_logs_action ON access_logs(action);
	`

	ctx := context.Background()
	_, err := db.ExecContext(ctx, schema)
	return err
}
