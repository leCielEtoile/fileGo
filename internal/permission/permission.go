package permission

import (
	"fmt"
	"log/slog"

	"fileserver/internal/config"
	"fileserver/internal/discord"
)

// Checker 権限チェッカー
type Checker struct {
	config        *config.Config
	discordClient *discord.Client
}

// NewChecker 権限チェッカーを作成
func NewChecker(cfg *config.Config, dc *discord.Client) *Checker {
	return &Checker{
		config:        cfg,
		discordClient: dc,
	}
}

// CheckPermission 権限チェック
// userID: DiscordユーザーID
// directory: ディレクトリパス
// permission: 権限種別 ("read", "write", "delete")
func (pc *Checker) CheckPermission(userID, directory, permission string) (bool, error) {
	// ディレクトリ設定を取得
	dirConfig := pc.config.GetDirectoryConfig(directory)
	if dirConfig == nil {
		return false, fmt.Errorf("ディレクトリ '%s' の設定が見つかりません", directory)
	}

	// 指定された権限がディレクトリ設定に含まれているか確認
	if !dirConfig.HasPermission(permission) {
		return false, nil
	}

	// required_rolesが空配列の場合は全員アクセス可能
	if len(dirConfig.RequiredRoles) == 0 {
		return true, nil
	}

	// ユーザーのロールを取得
	userRoles, err := pc.discordClient.GetMemberRoles(userID)
	if err != nil {
		slog.Error("ユーザーロール取得エラー", "user_id", userID, "error", err)
		return false, fmt.Errorf("ユーザーロールの取得に失敗しました: %w", err)
	}

	// ユーザーのロールとrequired_rolesを照合
	for _, userRole := range userRoles {
		for _, requiredRole := range dirConfig.RequiredRoles {
			if userRole == requiredRole {
				return true, nil
			}
		}
	}

	// いずれのロールもマッチしない場合は権限なし
	return false, nil
}

// GetAccessibleDirectories ユーザーがアクセス可能なディレクトリ一覧を取得
func (pc *Checker) GetAccessibleDirectories(userID string) ([]config.DirectoryConfig, error) {
	accessible := []config.DirectoryConfig{}

	// ユーザーのロールを取得
	userRoles, err := pc.discordClient.GetMemberRoles(userID)
	if err != nil {
		slog.Error("ユーザーロール取得エラー", "user_id", userID, "error", err)
		return nil, fmt.Errorf("ユーザーロールの取得に失敗しました: %w", err)
	}

	// 各ディレクトリをチェック
	for _, dirConfig := range pc.config.Storage.Directories {
		// required_rolesが空配列の場合は全員アクセス可能
		if len(dirConfig.RequiredRoles) == 0 {
			accessible = append(accessible, dirConfig)
			continue
		}

		// ユーザーのロールとrequired_rolesを照合
		hasAccess := false
		for _, userRole := range userRoles {
			for _, requiredRole := range dirConfig.RequiredRoles {
				if userRole == requiredRole {
					hasAccess = true
					break
				}
			}
			if hasAccess {
				break
			}
		}

		if hasAccess {
			accessible = append(accessible, dirConfig)
		}
	}

	return accessible, nil
}
