// Package storage はファイルストレージ管理機能を提供します。
// ファイルのアップロード、ダウンロード、ディレクトリ管理、ファイルメタデータを処理します。
package storage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"fileserver/internal/config"
	"fileserver/internal/models"

	"github.com/google/uuid"
)

// Manager はディレクトリ作成とファイル管理を含むファイルストレージ操作を処理します。
type Manager struct {
	config *config.Config
	db     *sql.DB
}

// SavedFile は正常に保存されたファイルとそのメタデータを表します。
type SavedFile struct {
	Filename string
	Path     string
	Size     int64
}

// NewManager は提供された設定で新しいストレージマネージャーインスタンスを作成します。
func NewManager(cfg *config.Config, db *sql.DB) *Manager {
	return &Manager{
		config: cfg,
		db:     db,
	}
}

// InitializeDirectories は設定ファイルで定義されたすべてのディレクトリを作成します。
// ルートアップロードディレクトリと、安全な権限を持つすべての設定されたサブディレクトリを作成します。
func (m *Manager) InitializeDirectories() error {
	// アップロードルートディレクトリ作成
	if err := os.MkdirAll(m.config.Storage.UploadPath, 0750); err != nil {
		return fmt.Errorf("アップロードディレクトリの作成に失敗しました: %w", err)
	}

	// 設定ファイルで定義された各ディレクトリを作成
	for _, dir := range m.config.Storage.Directories {
		// user_private タイプの場合は親ディレクトリのみ作成
		// ユーザー個別ディレクトリは初回アクセス時に作成
		dirPath := filepath.Join(m.config.Storage.UploadPath, dir.Path)
		if err := os.MkdirAll(dirPath, 0750); err != nil {
			return fmt.Errorf("ディレクトリ '%s' の作成に失敗しました: %w", dir.Path, err)
		}
		slog.Info("ディレクトリを作成しました", "path", dirPath)
	}

	return nil
}

// EnsureUserDirectory はユーザー専用ディレクトリが存在しない場合に作成します。
// これは、ユーザーが個人ディレクトリに初めてアップロードする際にオンデマンドで呼び出されます。
// directoryName はユーザーのディレクトリ名（例: "username"）である必要があります。
func (m *Manager) EnsureUserDirectory(directoryName string) error {
	userDir := filepath.Join(m.config.Storage.UploadPath, "user", directoryName)
	if err := os.MkdirAll(userDir, 0750); err != nil {
		return fmt.Errorf("ユーザーディレクトリの作成に失敗しました: %w", err)
	}
	return nil
}

// SaveFile はファイルを一意のUUIDベースのファイル名で指定されたディレクトリに保存します。
// 生成されたファイル名、パス、サイズを含む保存されたファイルのメタデータを返します。
func (m *Manager) SaveFile(file io.Reader, filename, directory string) (*SavedFile, error) {
	// ファイル名生成（UUID + 元のファイル名）
	fileID := uuid.New().String()
	savedFilename := fmt.Sprintf("%s_%s", fileID, sanitizeFilename(filename))

	// 保存先パス
	destPath := filepath.Join(m.config.Storage.UploadPath, directory, savedFilename)

	// ファイル作成
	// #nosec G304 - destPath is constructed from sanitized inputs
	destFile, err := os.Create(destPath)
	if err != nil {
		return nil, fmt.Errorf("ファイル作成エラー: %w", err)
	}
	defer func() {
		if closeErr := destFile.Close(); closeErr != nil {
			slog.Error("ファイルのクローズに失敗しました", "error", closeErr)
		}
	}()

	// データコピー
	written, err := io.Copy(destFile, file)
	if err != nil {
		if removeErr := os.Remove(destPath); removeErr != nil {
			slog.Error("一時ファイルの削除に失敗しました", "error", removeErr)
		}
		return nil, fmt.Errorf("ファイル書き込みエラー: %w", err)
	}

	return &SavedFile{
		Filename: savedFilename,
		Path:     filepath.Join(directory, savedFilename),
		Size:     written,
	}, nil
}

// ListFiles は指定されたディレクトリ内のすべてのファイルのリストを返します。
// 一時ファイル（.temp）とメタデータファイル（.meta）はリストから除外されます。
func (m *Manager) ListFiles(directory string) ([]models.FileInfo, error) {
	dirPath := filepath.Join(m.config.Storage.UploadPath, directory)

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("ディレクトリ読み込みエラー: %w", err)
	}

	files := make([]models.FileInfo, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// .temp, .metaファイルは除外
		if strings.HasSuffix(entry.Name(), ".temp") || strings.HasSuffix(entry.Name(), ".meta") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// 元のファイル名を抽出
		originalName := extractOriginalFilename(entry.Name())

		// メタデータを取得
		uploader, hash, err := m.GetFileMetadata(directory, entry.Name())
		if err != nil {
			slog.Warn("メタデータの取得に失敗しました", "filename", entry.Name(), "error", err)
		}

		files = append(files, models.FileInfo{
			Filename:     entry.Name(),
			OriginalName: originalName,
			Size:         info.Size(),
			ModifiedAt:   info.ModTime(),
			Uploader:     uploader,
			Hash:         hash,
		})
	}

	return files, nil
}

// DeleteFile は指定されたディレクトリからファイルを削除します。
func (m *Manager) DeleteFile(directory, filename string) error {
	filePath := filepath.Join(m.config.Storage.UploadPath, directory, filename)
	return os.Remove(filePath)
}

// sanitizeFilename ファイル名のサニタイズ
func sanitizeFilename(filename string) string {
	// パストラバーサル対策
	filename = filepath.Base(filename)

	// 危険な文字を置換
	replacer := strings.NewReplacer(
		"..", "_",
		"/", "_",
		"\\", "_",
	)

	return replacer.Replace(filename)
}

// extractOriginalFilename UUID付きファイル名から元のファイル名を抽出
func extractOriginalFilename(filename string) string {
	parts := strings.SplitN(filename, "_", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return filename
}

// SaveFileMetadata はファイルのメタデータをデータベースに保存します。
func (m *Manager) SaveFileMetadata(directory, filename, uploaderID, uploaderName string) error {
	if m.db == nil {
		return fmt.Errorf("データベース接続が設定されていません")
	}

	// ファイルのハッシュ値を計算
	hash, err := m.calculateFileHash(directory, filename)
	if err != nil {
		slog.Warn("ファイルハッシュの計算に失敗しました", "error", err)
		hash = "" // ハッシュ計算失敗時は空文字列
	}

	query := `
		INSERT INTO file_metadata (directory, filename, uploader_id, uploader_name, hash)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(directory, filename) DO UPDATE SET
			uploader_id = excluded.uploader_id,
			uploader_name = excluded.uploader_name,
			hash = excluded.hash,
			created_at = CURRENT_TIMESTAMP
	`

	_, err = m.db.Exec(query, directory, filename, uploaderID, uploaderName, hash)
	if err != nil {
		return fmt.Errorf("メタデータの保存に失敗しました: %w", err)
	}

	return nil
}

// GetFileMetadata はファイルのメタデータをデータベースから取得します。
func (m *Manager) GetFileMetadata(directory, filename string) (uploader string, hash string, err error) {
	if m.db == nil {
		return "", "", nil
	}

	query := `SELECT uploader_name, hash FROM file_metadata WHERE directory = ? AND filename = ?`
	err = m.db.QueryRow(query, directory, filename).Scan(&uploader, &hash)
	if err == sql.ErrNoRows {
		return "", "", nil // データが存在しない場合はエラーではなく空文字列を返す
	}
	if err != nil {
		return "", "", fmt.Errorf("メタデータの取得に失敗しました: %w", err)
	}

	return uploader, hash, nil
}

// calculateFileHash はファイルのSHA256ハッシュ値を計算します。
func (m *Manager) calculateFileHash(directory, filename string) (string, error) {
	filePath := filepath.Join(m.config.Storage.UploadPath, directory, filename)

	// #nosec G304 - filePath is constructed from controlled inputs
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("ファイルのオープンに失敗しました: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("ハッシュ計算に失敗しました: %w", err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
