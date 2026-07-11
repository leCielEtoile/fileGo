// Package permission はファイル操作のためのユーザー権限チェック機能を提供します。
// 認証プロバイダーのロール、および設定のディレクトリ付与（grants）と統合して、
// 異なるディレクトリへのユーザーアクセスを決定します。
package permission

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	"fileserver/internal/authprovider"
	"fileserver/internal/config"
	"fileserver/internal/storage"
)

// Checker は権限チェッカーを表します。
type Checker struct {
	config   *config.Config
	provider authprovider.Provider
	storage  *storage.Manager
	db       *sql.DB
}

// AccessibleDirectory はユーザーがアクセスできるディレクトリと、その実効権限を表します。
type AccessibleDirectory struct {
	Path        string
	Type        string
	Permissions []string
}

// NewChecker は新しい権限チェッカーインスタンスを作成します。
// ロールベースの権限チェックのために認証プロバイダーが、
// ユーザーディレクトリの存在確認のためにストレージマネージャーが必要です。
func NewChecker(cfg *config.Config, provider authprovider.Provider, sm *storage.Manager, db *sql.DB) *Checker {
	return &Checker{
		config:   cfg,
		provider: provider,
		storage:  sm,
		db:       db,
	}
}

// CheckPermission はユーザーがディレクトリに対して指定された権限を持っているかを検証します。
// userID: ユーザーID（＝プロバイダー内のsubject。ロール取得とユーザー指定grantの照合に使用）
// directory: ディレクトリパス（例: "user", "user/alice", "public", "admin"）
// permission: 権限タイプ（"read", "write", "delete"）
func (pc *Checker) CheckPermission(userID, directory, permission string) (bool, error) {
	pathParts := strings.Split(directory, "/")
	rootDir := pathParts[0]

	dirConfig := pc.config.GetDirectoryConfig(rootDir)
	if dirConfig == nil {
		return false, fmt.Errorf("ディレクトリ '%s' の設定が見つかりません", rootDir)
	}

	// user_private は本人と管理者のみアクセスできるため個別に判定する
	if dirConfig.Type == "user_private" {
		return pc.checkUserPrivatePermission(userID, permission, pathParts)
	}

	// ロールに依存しない付与（"*" / ユーザー指定）を先に評価する。
	// これによりロール取得の失敗に影響されず公開・個人指定を許可できる。
	if dirConfig.StaticPermissions(userID)[permission] {
		return true, nil
	}

	userRoles, err := pc.provider.GetUserRoles(context.Background(), userID)
	if err != nil {
		slog.Error("ユーザーロール取得エラー", "user_id", userID, "error", err)
		return false, fmt.Errorf("ユーザーロールの取得に失敗しました: %w", err)
	}

	// 管理者ロールは全ディレクトリ・全操作を許可する。
	if pc.config.HasAdminRole(userRoles) {
		slog.Debug("管理者権限によるアクセス許可", "user_id", userID, "directory", directory)
		return true, nil
	}

	return dirConfig.RolePermissions(toSet(userRoles))[permission], nil
}

// checkUserPrivatePermission はuser_privateディレクトリへの権限を判定します。
// /user 直下は一覧のみ許可し、/user/{name} は本人または管理者にのみアクセスを許可します。
func (pc *Checker) checkUserPrivatePermission(userID, permission string, pathParts []string) (bool, error) {
	// /user 直下は一覧表示（read）のみを許可する。
	// 共有の親ディレクトリへ直接書き込み・削除させないため、read以外は拒否する。
	if len(pathParts) == 1 {
		return permission == "read", nil
	}

	// /user/{targetUserDirName}: 本人か管理者のみ許可し、それ以外は拒否する。
	if len(pathParts) >= 2 {
		targetUserDirName := pathParts[1]

		myDirName, err := pc.getUserDirectoryName(userID)
		if err != nil {
			return false, err
		}
		if targetUserDirName == myDirName {
			return pc.allowUserDirectoryAccess(myDirName, permission), nil
		}

		isAdmin, err := pc.isAdmin(userID)
		if err != nil {
			return false, err
		}
		if isAdmin {
			return pc.allowUserDirectoryAccess(targetUserDirName, permission), nil
		}

		return false, nil
	}

	return false, nil
}

// allowUserDirectoryAccess はユーザー個別ディレクトリへのアクセス可否を返します。
// 書き込みは初回アップロードでディレクトリを作成するため常に許可します。
// 読み取り・削除は、ディレクトリが未作成（未アップロード）の場合は拒否します。
// これにより、Discord以外のOIDCユーザーはアップロードするまで
// 自分のディレクトリを閲覧できません。
func (pc *Checker) allowUserDirectoryAccess(dirName, permission string) bool {
	if permission == "write" {
		return true
	}
	return pc.storage.UserDirectoryExists(dirName)
}

// isAdmin はユーザーが管理者ロールを保有しているかを返します。
func (pc *Checker) isAdmin(userID string) (bool, error) {
	if pc.config.Storage.AdminRoleID == "" {
		return false, nil
	}

	userRoles, err := pc.provider.GetUserRoles(context.Background(), userID)
	if err != nil {
		slog.Error("ユーザーロール取得エラー", "user_id", userID, "error", err)
		return false, fmt.Errorf("ユーザーロールの取得に失敗しました: %w", err)
	}

	return pc.config.HasAdminRole(userRoles), nil
}

// getUserDirectoryName はデータベースからユーザーのディレクトリ名（ユーザー名）を取得します。
func (pc *Checker) getUserDirectoryName(userID string) (string, error) {
	var username string
	err := pc.db.QueryRowContext(context.Background(),
		"SELECT username FROM users WHERE id = ?", userID).Scan(&username)
	if err != nil {
		return "", fmt.Errorf("ユーザー名の取得に失敗しました: %w", err)
	}
	return username, nil
}

// GetAccessibleDirectories はユーザーがアクセスできるディレクトリと実効権限のリストを返します。
// ロールをディレクトリの付与（grants）と照合し、user_privateなどの特殊なケースを処理します。
// ロール取得に失敗した場合でも、ロールに依存しない付与（公開・個人指定）と
// user_privateディレクトリは引き続き列挙します。
func (pc *Checker) GetAccessibleDirectories(userID string) ([]AccessibleDirectory, error) {
	accessible := []AccessibleDirectory{}

	// ユーザーのロールを取得（失敗しても致命的にはせず、ロール非依存の付与は返す）
	userRoles, err := pc.provider.GetUserRoles(context.Background(), userID)
	if err != nil {
		slog.Warn("ユーザーロール取得に失敗しました。ロール非依存の付与のみで一覧します", "user_id", userID, "error", err)
		userRoles = nil
	}
	roleSet := toSet(userRoles)
	isAdmin := pc.config.HasAdminRole(userRoles)

	for _, dirConfig := range pc.config.Storage.Directories {
		// user_private タイプの場合は、ユーザー個別ディレクトリパスに変換
		if dirConfig.Type == "user_private" {
			userDirName, err := pc.getUserDirectoryName(userID)
			if err != nil {
				slog.Error("ユーザーディレクトリ名取得エラー", "user_id", userID, "error", err)
				continue
			}
			// ディレクトリが未作成（未アップロード）の場合は一覧に表示しない。
			// Discordユーザーはログイン時に事前作成されるため常に表示される。
			if !pc.storage.UserDirectoryExists(userDirName) {
				continue
			}
			accessible = append(accessible, AccessibleDirectory{
				Path:        fmt.Sprintf("user/%s", userDirName),
				Type:        dirConfig.Type,
				Permissions: []string{"read", "write", "delete"},
			})
			continue
		}

		// 実効権限を算出（管理者は全操作）
		perms := map[string]bool{"read": true, "write": true, "delete": true}
		if !isAdmin {
			perms = dirConfig.EffectivePermissions(userID, roleSet)
		}

		if len(perms) > 0 {
			accessible = append(accessible, AccessibleDirectory{
				Path:        dirConfig.Path,
				Type:        dirConfig.Type,
				Permissions: permissionList(perms),
			})
		}
	}

	return accessible, nil
}

// toSet は文字列スライスを集合に変換します。
func toSet(items []string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, item := range items {
		set[item] = true
	}
	return set
}

// permissionList は権限集合を安定した順序のスライスに変換します。
func permissionList(set map[string]bool) []string {
	order := []string{"read", "write", "delete"}
	result := make([]string, 0, len(set))
	for _, p := range order {
		if set[p] {
			result = append(result, p)
		}
	}
	// 既知の3種以外の権限も取りこぼさない
	for p := range set {
		if p != "read" && p != "write" && p != "delete" {
			result = append(result, p)
		}
	}
	return result
}
