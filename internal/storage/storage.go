// Package storage provides file storage management functionality.
// It handles file uploads, downloads, directory management, and file metadata.
package storage

import (
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

// Manager handles file storage operations including directory creation and file management.
type Manager struct {
	config *config.Config
}

// SavedFile represents a successfully saved file with its metadata.
type SavedFile struct {
	Filename string
	Path     string
	Size     int64
}

// NewManager creates a new storage manager instance with the provided configuration.
func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		config: cfg,
	}
}

// InitializeDirectories creates all directories defined in the configuration file.
// It creates the root upload directory and all configured subdirectories with secure permissions.
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

// EnsureUserDirectory creates a user-specific directory if it doesn't exist.
// This is called on-demand when a user first uploads to their personal directory.
// directoryName should be the user's directory name (e.g., "@username")
func (m *Manager) EnsureUserDirectory(directoryName string) error {
	userDir := filepath.Join(m.config.Storage.UploadPath, "user", directoryName)
	if err := os.MkdirAll(userDir, 0750); err != nil {
		return fmt.Errorf("ユーザーディレクトリの作成に失敗しました: %w", err)
	}
	return nil
}

// SaveFile saves a file to the specified directory with a unique UUID-based filename.
// It returns metadata about the saved file including the generated filename, path, and size.
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

// ListFiles returns a list of all files in the specified directory.
// It excludes temporary files (.temp) and metadata files (.meta) from the listing.
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

		files = append(files, models.FileInfo{
			Filename:     entry.Name(),
			OriginalName: originalName,
			Size:         info.Size(),
			ModifiedAt:   info.ModTime(),
		})
	}

	return files, nil
}

// DeleteFile removes a file from the specified directory.
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
