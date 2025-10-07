package permission

import (
	"fmt"
	"log/slog"
	"strings"

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
// directory: ディレクトリパス（例: "user", "user/123456789", "public", "admin"）
// permission: 権限種別 ("read", "write", "delete")
func (pc *Checker) CheckPermission(userID, directory, permission string) (bool, error) {
	// ディレクトリパスを分解
	pathParts := strings.Split(directory, "/")
	rootDir := pathParts[0]

	// ディレクトリ設定を取得
	dirConfig := pc.config.GetDirectoryConfig(rootDir)
	if dirConfig == nil {
		return false, fmt.Errorf("ディレクトリ '%s' の設定が見つかりません", rootDir)
	}

	// 指定された権限がディレクトリ設定に含まれているか確認
	if !dirConfig.HasPermission(permission) {
		return false, nil
	}

	// ユーザー個別ディレクトリの場合の特殊処理
	if dirConfig.Type == "user_private" {
		return pc.checkUserPrivatePermission(userID, directory, pathParts)
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

// checkUserPrivatePermission ユーザー個別ディレクトリの権限チェック
func (pc *Checker) checkUserPrivatePermission(userID, fullPath string, pathParts []string) (bool, error) {
	// /user 直下の場合は、自分のディレクトリのみ見える
	if len(pathParts) == 1 {
		return true, nil
	}

	// /user/{targetUserID} の場合
	if len(pathParts) >= 2 {
		targetUserID := pathParts[1]

		// 本人の場合はアクセス許可
		if targetUserID == userID {
			return true, nil
		}

		// 管理者の場合はすべてのユーザーディレクトリにアクセス許可
		isAdmin, err := pc.isAdmin(userID)
		if err != nil {
			return false, err
		}
		if isAdmin {
			return true, nil
		}

		// 本人でも管理者でもない場合はアクセス拒否
		return false, nil
	}

	return false, nil
}

// isAdmin 管理者ロールを持っているか確認
func (pc *Checker) isAdmin(userID string) (bool, error) {
	if pc.config.Storage.AdminRoleID == "" {
		return false, nil
	}

	userRoles, err := pc.discordClient.GetMemberRoles(userID)
	if err != nil {
		slog.Error("ユーザーロール取得エラー", "user_id", userID, "error", err)
		return false, fmt.Errorf("ユーザーロールの取得に失敗しました: %w", err)
	}

	for _, role := range userRoles {
		if role == pc.config.Storage.AdminRoleID {
			return true, nil
		}
	}

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
		// user_private タイプの場合は、ユーザー個別ディレクトリパスに変換
		if dirConfig.Type == "user_private" {
			userDirConfig := dirConfig
			userDirConfig.Path = fmt.Sprintf("user/%s", userID)
			accessible = append(accessible, userDirConfig)
			continue
		}

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
