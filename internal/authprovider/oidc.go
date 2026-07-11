package authprovider

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCConfig は汎用OIDCプロバイダーの設定を表します。
type OIDCConfig struct {
	Name         string
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
	// GroupsClaim はロール/グループ一覧を含むクレーム名です（例: "groups", "roles"）。
	// 空の場合は "groups" が使われます。
	GroupsClaim string

	// AllowedEmailDomains / AllowedEmails は任意のアクセス許可リストです。
	// いずれも空の場合は認証できた全ユーザーを許可します。
	// いずれかが設定されている場合は、ユーザーのメールがドメイン一致または
	// 完全一致したときのみアクセスを許可します。
	AllowedEmailDomains []string
	AllowedEmails       []string
}

// oidcClaims はID Token / UserInfoから読み取る標準クレームです。
type oidcClaims struct {
	Subject           string `json:"sub"`
	Email             string `json:"email"`
	PreferredUsername string `json:"preferred_username"`
	Name              string `json:"name"`
	Picture           string `json:"picture"`
}

// RoleStore はOIDCロールを永続化するためのストアです。
// OIDCのロールはログイン時のID Tokenからしか得られないため、再起動後も
// 復元できるよう永続化します。nil の場合はメモリキャッシュのみで動作します。
type RoleStore interface {
	SaveRoles(ctx context.Context, provider, subject string, roles []string) error
	LoadRoles(ctx context.Context, provider, subject string) (roles []string, found bool, err error)
}

// OIDCProvider は .well-known/openid-configuration によるディスカバリと
// ID Token検証を行う汎用OIDCプロバイダー実装です。
type OIDCProvider struct {
	oauthConfig *oauth2.Config
	provider    *oidc.Provider
	verifier    *oidc.IDTokenVerifier
	store       RoleStore
	name        string
	groupsClaim string

	// allowlist が空の場合は認証できた全ユーザーを許可します。
	allowedEmailDomains []string
	allowedEmails       []string

	// roleCache はログイン時にID Tokenから取得したロール一覧を保持します。
	// OIDCのロールはID Token発行時にしか得られずサーバー側で再取得できないため、
	// 再起動後も復元できるよう store にも永続化します。
	roleMutex sync.RWMutex
	roleCache map[string]roleCacheRecord
}

type roleCacheRecord struct {
	roles     []string
	updatedAt time.Time
}

// NewOIDCProvider はディスカバリを行いOIDCProviderを作成します。
// ディスカバリはネットワークアクセスを伴うため起動時に一度だけ行われます。
// store が非nilの場合、ログイン時に取得したロールを永続化し、再起動後も復元します。
func NewOIDCProvider(ctx context.Context, cfg OIDCConfig, store RoleStore) (*OIDCProvider, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("oidcプロバイダー名が指定されていません")
	}

	p, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("OIDCプロバイダー '%s' のディスカバリに失敗しました: %w", cfg.Name, err)
	}

	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}

	groupsClaim := cfg.GroupsClaim
	if groupsClaim == "" {
		groupsClaim = "groups"
	}

	return &OIDCProvider{
		name:     cfg.Name,
		provider: p,
		store:    store,
		verifier: p.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		oauthConfig: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Scopes:       scopes,
			Endpoint:     p.Endpoint(),
		},
		groupsClaim:         groupsClaim,
		allowedEmailDomains: normalizeStrings(cfg.AllowedEmailDomains),
		allowedEmails:       normalizeStrings(cfg.AllowedEmails),
		roleCache:           make(map[string]roleCacheRecord),
	}, nil
}

// Name はプロバイダー名を返します。
func (p *OIDCProvider) Name() string {
	return p.name
}

// AuthCodeURL は認可URLを返します。
func (p *OIDCProvider) AuthCodeURL(state string) string {
	return p.oauthConfig.AuthCodeURL(state)
}

// Exchange は認可コードをトークンと交換します。
func (p *OIDCProvider) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	return p.oauthConfig.Exchange(ctx, code)
}

// FetchUserInfo はID Tokenを検証し、標準クレームからユーザー情報を組み立てます。
// 併せてGroupsClaimに指定されたロール一覧をキャッシュします
// （OIDCのロール情報はID Token発行時にしか確実に得られないため）。
func (p *OIDCProvider) FetchUserInfo(ctx context.Context, token *oauth2.Token) (*UserInfo, error) {
	claims, rawClaims, err := p.verifiedClaims(ctx, token)
	if err != nil {
		return nil, err
	}

	username := claims.PreferredUsername
	if username == "" {
		username = claims.Name
	}
	if username == "" {
		username = claims.Subject
	}

	roles := extractStringSlice(rawClaims[p.groupsClaim])

	p.roleMutex.Lock()
	p.roleCache[claims.Subject] = roleCacheRecord{
		roles:     roles,
		updatedAt: time.Now(),
	}
	p.roleMutex.Unlock()

	// 再起動後もロールを復元できるよう永続化する（失敗してもログインは継続）。
	if p.store != nil {
		if err := p.store.SaveRoles(ctx, p.name, claims.Subject, roles); err != nil {
			slog.Error("OIDCロールの永続化に失敗しました", "error", err, "subject", claims.Subject)
		}
	}

	return &UserInfo{
		Provider: p.name,
		Subject:  claims.Subject,
		Username: username,
		Avatar:   claims.Picture,
		Email:    claims.Email,
	}, nil
}

// IsMember は allowlist（許可メールドメイン/メール）でアクセス可否を判定します。
// allowlistが未設定の場合は、認証できた全ユーザーを許可します
// （Discordのギルドメンバーシップに相当する概念がないため）。
func (p *OIDCProvider) IsMember(_ context.Context, _ *oauth2.Token, info *UserInfo) (bool, error) {
	if len(p.allowedEmailDomains) == 0 && len(p.allowedEmails) == 0 {
		return true, nil
	}
	if info == nil {
		return false, nil
	}
	return p.isEmailAllowed(info.Email), nil
}

// isEmailAllowed はメールアドレスが allowlist に含まれるかを判定します。
func (p *OIDCProvider) isEmailAllowed(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		// allowlist設定時にメールが取得できない場合は fail-closed で拒否する
		// （OIDCの "email" スコープが必要）。
		return false
	}

	for _, allowed := range p.allowedEmails {
		if allowed == email {
			return true
		}
	}

	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	domain := email[at+1:]
	for _, allowed := range p.allowedEmailDomains {
		if allowed == domain {
			return true
		}
	}
	return false
}

// VerifyMembership は常にtrueを返します。
// OIDCにはログイン後にユーザートークン無しで在籍を再検証する手段がないためです。
// アクセスを制限する場合はログイン時のallowlist（allowed_email_domains等）を使用してください。
func (p *OIDCProvider) VerifyMembership(_ context.Context, _ string) (bool, error) {
	return true, nil
}

// PrecreateUserDirectory はOIDCではfalseを返します。
// 誰でもログインできるため、初回アップロードまでユーザーディレクトリを作成しません。
func (p *OIDCProvider) PrecreateUserDirectory() bool {
	return false
}

// normalizeStrings は前後空白の除去と小文字化を行い、空要素を取り除きます。
func normalizeStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// GetUserRoles はログイン時にID Tokenから取得したロール一覧を返します。
// メモリキャッシュに無ければ永続ストアから復元します（再起動後の復元）。
// いずれにも無い場合（ログイン未実施等）はエラーを返すため、呼び出し側は再ログインを促すべきです。
func (p *OIDCProvider) GetUserRoles(ctx context.Context, subject string) ([]string, error) {
	p.roleMutex.RLock()
	record, ok := p.roleCache[subject]
	p.roleMutex.RUnlock()

	if !ok {
		// メモリに無い場合は永続ストアから復元を試みる。
		if p.store != nil {
			stored, found, err := p.store.LoadRoles(ctx, p.name, subject)
			if err != nil {
				return nil, fmt.Errorf("ロールの復元に失敗しました (subject: %s): %w", subject, err)
			}
			if found {
				p.roleMutex.Lock()
				p.roleCache[subject] = roleCacheRecord{roles: stored, updatedAt: time.Now()}
				p.roleMutex.Unlock()
				record = roleCacheRecord{roles: stored}
				ok = true
			}
		}
	}

	if !ok {
		return nil, fmt.Errorf("ロール情報がありません。再ログインが必要です (subject: %s)", subject)
	}

	roles := make([]string, len(record.roles))
	copy(roles, record.roles)
	return roles, nil
}

func (p *OIDCProvider) verifiedClaims(ctx context.Context, token *oauth2.Token) (*oidcClaims, map[string]interface{}, error) {
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return nil, nil, fmt.Errorf("oidcレスポンスにid_tokenが含まれていません")
	}

	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, nil, fmt.Errorf("id_tokenの検証に失敗しました: %w", err)
	}

	var claims oidcClaims
	if err := idToken.Claims(&claims); err != nil {
		return nil, nil, fmt.Errorf("id_tokenのクレーム解析に失敗しました: %w", err)
	}

	var raw map[string]interface{}
	if err := idToken.Claims(&raw); err != nil {
		return nil, nil, fmt.Errorf("id_tokenのクレーム解析に失敗しました: %w", err)
	}

	return &claims, raw, nil
}

func extractStringSlice(v interface{}) []string {
	switch val := v.(type) {
	case []string:
		return val
	case []interface{}:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case string:
		// 一部のIdPはグループ/ロールが1件の場合に配列ではなく
		// 単一の文字列で返すため、その場合も1要素として扱う。
		if val == "" {
			return nil
		}
		return []string{val}
	default:
		return nil
	}
}
