// Package authprovider は複数の認証プロバイダー（Discord、汎用OIDC等）を
// 共通のインターフェースの下で扱うための抽象化レイヤーを提供します。
package authprovider

import (
	"context"

	"golang.org/x/oauth2"
)

// UserInfo はプロバイダーから取得したユーザー情報を表します。
type UserInfo struct {
	Provider string
	Subject  string // プロバイダー内でユーザーを一意に識別するID
	Username string
	Avatar   string
	Email    string
}

// Provider は認証プロバイダーが実装すべき共通インターフェースです。
// 非OIDCのDiscordと、Google/Keycloak等のOIDCプロバイダーを同一の抽象で扱います。
type Provider interface {
	// Name はプロバイダー名を返します（例: "discord", "google"）。
	Name() string

	// AuthCodeURL は認可コードを取得するためのリダイレクト先URLを返します。
	AuthCodeURL(state string) string

	// Exchange は認可コードをトークンと交換します。
	Exchange(ctx context.Context, code string) (*oauth2.Token, error)

	// FetchUserInfo は取得したトークンからユーザー情報を取得します。
	FetchUserInfo(ctx context.Context, token *oauth2.Token) (*UserInfo, error)

	// IsMember はログイン時点でアクセスを許可された対象かを確認します。
	// Discordはユーザートークンでギルドメンバーシップを確認します。
	// OIDCはallowlist（許可メールドメイン/メール）が設定されていればそれで判定し、
	// 未設定なら常にtrueを返します。
	IsMember(ctx context.Context, token *oauth2.Token, info *UserInfo) (bool, error)

	// VerifyMembership はログイン後のリクエストで在籍を継続確認します。
	// ユーザートークンを用いずサーバー側の資格情報のみで判定できる必要があります。
	// DiscordはBotトークンでギルド在籍を確認します（5分キャッシュ）。
	// OIDCは再検証手段がないため常にtrueを返します。
	VerifyMembership(ctx context.Context, subject string) (bool, error)

	// GetUserRoles はロールベースのアクセス制御に用いるロール一覧を返します。
	GetUserRoles(ctx context.Context, subject string) ([]string, error)

	// PrecreateUserDirectory はログイン時にユーザー専用ディレクトリを
	// 事前作成すべきかを返します。メンバーが限定されるDiscordはtrue、
	// 誰でもログインできるOIDCはfalse（初回アップロードまで自分のディレクトリを持たない）。
	PrecreateUserDirectory() bool
}

// hasRequiredRole はログイン要件のロールを満たすかを返します。
// required が空なら要件なし（在籍のみでログイン可）。指定がある場合は
// いずれか1つを保有していればよい（OR判定）。
func hasRequiredRole(required, roles []string) bool {
	if len(required) == 0 {
		return true
	}
	held := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		held[r] = struct{}{}
	}
	for _, r := range required {
		if _, ok := held[r]; ok {
			return true
		}
	}
	return false
}

// MembershipSyncer は、ロールのリアルタイム同期（Discordゲートウェイ）を任意で
// 提供するプロバイダーが実装します。対応しないプロバイダー（OIDC等）は実装しません。
type MembershipSyncer interface {
	// StartMembershipSync はリアルタイム同期を開始します。started=false は
	// フォールバック運用（従来のREST/都度取得）のままであることを表します。
	// onChange は対象ユーザーのロール変化時に呼ばれます。
	StartMembershipSync(ctx context.Context, onChange func(userID string)) (started bool, err error)
	// StopMembershipSync は同期を停止します。
	StopMembershipSync() error
}
