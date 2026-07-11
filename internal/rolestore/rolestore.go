// Package rolestore はOIDCロールの永続化ストアを提供します。
// OIDCのロールはログイン時にしか取得できないため、再起動後も復元できるよう
// データベースに保存します（Discordはサーバー側で随時再取得できるため対象外）。
package rolestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// Store は *sql.DB を用いたロールストアの実装です。
// authprovider.RoleStore インターフェースを満たします。
type Store struct {
	db *sql.DB
}

// New は Store を作成します。
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// SaveRoles はプロバイダー/subjectごとのロール一覧を保存（upsert）します。
func (s *Store) SaveRoles(ctx context.Context, provider, subject string, roles []string) error {
	if roles == nil {
		roles = []string{}
	}
	encoded, err := json.Marshal(roles)
	if err != nil {
		return fmt.Errorf("ロールのJSONエンコードに失敗しました: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO oidc_user_roles (provider, subject, roles, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(provider, subject) DO UPDATE SET
			roles = excluded.roles,
			updated_at = CURRENT_TIMESTAMP
	`, provider, subject, string(encoded))
	if err != nil {
		return fmt.Errorf("ロールの保存に失敗しました: %w", err)
	}
	return nil
}

// LoadRoles は保存済みのロール一覧を取得します。
// レコードが存在しない場合は found=false を返します（エラーにはしません）。
func (s *Store) LoadRoles(ctx context.Context, provider, subject string) (roles []string, found bool, err error) {
	var encoded string
	err = s.db.QueryRowContext(ctx,
		"SELECT roles FROM oidc_user_roles WHERE provider = ? AND subject = ?",
		provider, subject).Scan(&encoded)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("ロールの取得に失敗しました: %w", err)
	}
	if err := json.Unmarshal([]byte(encoded), &roles); err != nil {
		return nil, false, fmt.Errorf("ロールのJSONデコードに失敗しました: %w", err)
	}
	return roles, true, nil
}
