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

type Manager struct {
	config *config.Config
}

type SavedFile struct {
	Filename string
	Path     string
	Size     int64
}

func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		config: cfg,
	}
}

// InitializeDirectories は設定ファイルで定義されたディレクトリを作成
func (m *Manager) InitializeDirectories() error {
	// アップロードルートディレクトリ作成
	if err := os.MkdirAll(m.config.Storage.UploadPath, 0755); err != nil {
		return fmt.Errorf("アップロードディレクトリの作成に失敗しました: %w", err)
	}

	// 設定ファイルで定義された各ディレクトリを作成
	for _, dir := range m.config.Storage.Directories {
		dirPath := filepath.Join(m.config.Storage.UploadPath, dir.Path)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return fmt.Errorf("ディレクトリ '%s' の作成に失敗しました: %w", dir.Path, err)
		}
		slog.Info("ディレクトリを作成しました", "path", dirPath)
	}

	return nil
}

// SaveFile 通常ファイル保存
func (m *Manager) SaveFile(file io.Reader, filename, directory string) (*SavedFile, error) {
	// ファイル名生成（UUID + 元のファイル名）
	fileID := uuid.New().String()
	savedFilename := fmt.Sprintf("%s_%s", fileID, sanitizeFilename(filename))

	// 保存先パス
	destPath := filepath.Join(m.config.Storage.UploadPath, directory, savedFilename)

	// ファイル作成
	destFile, err := os.Create(destPath)
	if err != nil {
		return nil, fmt.Errorf("ファイル作成エラー: %w", err)
	}
	defer destFile.Close()

	// データコピー
	written, err := io.Copy(destFile, file)
	if err != nil {
		os.Remove(destPath)
		return nil, fmt.Errorf("ファイル書き込みエラー: %w", err)
	}

	return &SavedFile{
		Filename: savedFilename,
		Path:     filepath.Join(directory, savedFilename),
		Size:     written,
	}, nil
}

// ListFiles ディレクトリ内のファイル一覧取得
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

// DeleteFile ファイル削除
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
