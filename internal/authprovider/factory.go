package authprovider

import (
	"context"
	"fmt"

	"fileserver/internal/config"
)

// New は設定から単一のProviderを構築します。
// 本プロジェクトは認証プロバイダーを1つに限定します。
// store はOIDCのロール永続化に使用します（Discordでは未使用のためnilでも可）。
// OIDCタイプはディスカバリのためネットワークアクセスを行うため、
// ここでのエラーは起動時に処理される想定です。
func New(ctx context.Context, c config.ProviderConfig, store RoleStore) (Provider, error) {
	switch c.Type {
	case "discord":
		return NewDiscordProvider(DiscordConfig{
			Name:         c.Name,
			ClientID:     c.ClientID,
			ClientSecret: c.ClientSecret,
			RedirectURL:  c.RedirectURL,
			GuildID:      c.GuildID,
			BotToken:     c.BotToken,
		}), nil
	case "oidc":
		return NewOIDCProvider(ctx, OIDCConfig{
			Name:                c.Name,
			Issuer:              c.Issuer,
			ClientID:            c.ClientID,
			ClientSecret:        c.ClientSecret,
			RedirectURL:         c.RedirectURL,
			Scopes:              c.Scopes,
			GroupsClaim:         c.GroupsClaim,
			AllowedEmailDomains: c.AllowedEmailDomains,
			AllowedEmails:       c.AllowedEmails,
		}, store)
	default:
		return nil, fmt.Errorf("未対応のプロバイダータイプです: %q (name: %q)", c.Type, c.Name)
	}
}
