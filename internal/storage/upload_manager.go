package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"fileserver/internal/config"
	"fileserver/internal/models"

	"github.com/google/uuid"
)

var (
	ErrSessionNotFound      = errors.New("アップロードセッションが見つかりません")
	ErrMaxConcurrentUploads = errors.New("同時アップロード数の上限に達しています")
	ErrIncompleteUpload     = errors.New("アップロードが完了していません")
)

type UploadManager struct {
	config      *config.Config
	sessions    map[string]*models.UploadSession
	userUploads map[string]int // ユーザーごとの同時アップロード数
	mu          sync.RWMutex
}

func NewUploadManager(cfg *config.Config) *UploadManager {
	um := &UploadManager{
		config:      cfg,
		sessions:    make(map[string]*models.UploadSession),
		userUploads: make(map[string]int),
	}

	// クリーンアップゴルーチン起動
	go um.startCleanupRoutine()

	return um
}

// CreateUploadSession アップロードセッション作成
func (um *UploadManager) CreateUploadSession(userID, filename, directory string, totalSize, chunkSize int64, totalChunks int) (*models.UploadSession, error) {
	um.mu.Lock()
	defer um.mu.Unlock()

	// 同時アップロード数チェック
	if um.userUploads[userID] >= um.config.Storage.MaxConcurrentUploads {
		return nil, ErrMaxConcurrentUploads
	}

	// ファイルサイズチェック
	if totalSize > um.config.Storage.MaxChunkFileSize {
		return nil, fmt.Errorf("ファイルサイズが上限を超えています")
	}

	uploadID := uuid.New().String()
	now := time.Now()

	session := &models.UploadSession{
		UploadID:       uploadID,
		UserID:         userID,
		Filename:       filename,
		Directory:      directory,
		TotalSize:      totalSize,
		ChunkSize:      chunkSize,
		TotalChunks:    totalChunks,
		UploadedChunks: make([]int, 0),
		CreatedAt:      now,
		UpdatedAt:      now,
		ExpiresAt:      now.Add(um.config.Storage.UploadSessionTTL),
	}

	// セッション保存
	um.sessions[uploadID] = session
	um.userUploads[userID]++

	// .tempファイル作成
	tempPath := um.getTempFilePath(uploadID, filename, directory)
	if err := os.MkdirAll(filepath.Dir(tempPath), 0755); err != nil {
		return nil, err
	}

	tempFile, err := os.Create(tempPath)
	if err != nil {
		return nil, err
	}
	tempFile.Close()

	// .metaファイル保存
	if err := um.saveMetaFile(session); err != nil {
		os.Remove(tempPath)
		return nil, err
	}

	return session, nil
}

// GetUploadSession セッション取得
func (um *UploadManager) GetUploadSession(uploadID string) (*models.UploadSession, error) {
	um.mu.RLock()
	defer um.mu.RUnlock()

	session, exists := um.sessions[uploadID]
	if !exists {
		// メモリになければ.metaファイルから復元
		return um.loadSessionFromMeta(uploadID)
	}

	return session, nil
}

// SaveChunk チャンク保存
func (um *UploadManager) SaveChunk(uploadID string, chunkNumber int, data []byte) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	session, exists := um.sessions[uploadID]
	if !exists {
		// メモリになければ.metaファイルから復元
		var err error
		session, err = um.loadSessionFromMeta(uploadID)
		if err != nil {
			return ErrSessionNotFound
		}
		um.sessions[uploadID] = session
	}

	// 既にアップロード済みかチェック
	for _, uploaded := range session.UploadedChunks {
		if uploaded == chunkNumber {
			return nil // 既にアップロード済み
		}
	}

	// .tempファイルに書き込み
	tempPath := um.getTempFilePath(uploadID, session.Filename, session.Directory)
	file, err := os.OpenFile(tempPath, os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	// チャンクの位置にシーク
	offset := int64(chunkNumber) * session.ChunkSize
	if _, err := file.Seek(offset, 0); err != nil {
		return err
	}

	// データ書き込み
	if _, err := file.Write(data); err != nil {
		return err
	}

	// アップロード済みチャンクリストに追加
	session.UploadedChunks = append(session.UploadedChunks, chunkNumber)
	session.UpdatedAt = time.Now()

	// .metaファイル更新
	return um.saveMetaFile(session)
}

// GetUploadedChunks アップロード済みチャンク番号取得
func (um *UploadManager) GetUploadedChunks(uploadID string) ([]int, error) {
	um.mu.RLock()
	defer um.mu.RUnlock()

	session, exists := um.sessions[uploadID]
	if !exists {
		var err error
		session, err = um.loadSessionFromMeta(uploadID)
		if err != nil {
			return nil, ErrSessionNotFound
		}
	}

	return session.UploadedChunks, nil
}

// CompleteUpload アップロード完了処理
func (um *UploadManager) CompleteUpload(uploadID string) (*SavedFile, error) {
	um.mu.Lock()
	defer um.mu.Unlock()

	session, exists := um.sessions[uploadID]
	if !exists {
		var err error
		session, err = um.loadSessionFromMeta(uploadID)
		if err != nil {
			return nil, ErrSessionNotFound
		}
	}

	// 全チャンクがアップロード済みか確認
	if len(session.UploadedChunks) != session.TotalChunks {
		return nil, ErrIncompleteUpload
	}

	// .tempファイルを最終ファイルにリネーム
	tempPath := um.getTempFilePath(uploadID, session.Filename, session.Directory)
	fileID := uuid.New().String()
	finalFilename := fmt.Sprintf("%s_%s", fileID, sanitizeFilename(session.Filename))
	finalPath := filepath.Join(um.config.Storage.UploadPath, session.Directory, finalFilename)

	if err := os.Rename(tempPath, finalPath); err != nil {
		return nil, err
	}

	// ファイルサイズ取得
	fileInfo, err := os.Stat(finalPath)
	if err != nil {
		return nil, err
	}

	// .metaファイル削除
	metaPath := um.getMetaFilePath(uploadID, session.Filename, session.Directory)
	os.Remove(metaPath)

	// セッション削除
	delete(um.sessions, uploadID)
	um.userUploads[session.UserID]--

	return &SavedFile{
		Filename: finalFilename,
		Path:     filepath.Join(session.Directory, finalFilename),
		Size:     fileInfo.Size(),
	}, nil
}

// CancelUpload アップロードキャンセル
func (um *UploadManager) CancelUpload(uploadID string) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	session, exists := um.sessions[uploadID]
	if !exists {
		var err error
		session, err = um.loadSessionFromMeta(uploadID)
		if err != nil {
			return ErrSessionNotFound
		}
	}

	// .tempファイル削除
	tempPath := um.getTempFilePath(uploadID, session.Filename, session.Directory)
	os.Remove(tempPath)

	// .metaファイル削除
	metaPath := um.getMetaFilePath(uploadID, session.Filename, session.Directory)
	os.Remove(metaPath)

	// セッション削除
	delete(um.sessions, uploadID)
	um.userUploads[session.UserID]--

	return nil
}

// startCleanupRoutine クリーンアップルーチン
func (um *UploadManager) startCleanupRoutine() {
	ticker := time.NewTicker(um.config.Storage.CleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		um.cleanup()
	}
}

// cleanup 期限切れセッションのクリーンアップ
func (um *UploadManager) cleanup() {
	um.mu.Lock()
	defer um.mu.Unlock()

	now := time.Now()
	expiredSessions := make([]string, 0)

	// メモリ内の期限切れセッションを収集
	for uploadID, session := range um.sessions {
		if now.After(session.ExpiresAt) {
			expiredSessions = append(expiredSessions, uploadID)
		}
	}

	// 期限切れセッションを削除
	for _, uploadID := range expiredSessions {
		session := um.sessions[uploadID]

		// ファイル削除
		tempPath := um.getTempFilePath(uploadID, session.Filename, session.Directory)
		os.Remove(tempPath)

		metaPath := um.getMetaFilePath(uploadID, session.Filename, session.Directory)
		os.Remove(metaPath)

		// セッション削除
		delete(um.sessions, uploadID)
		um.userUploads[session.UserID]--

		slog.Info("期限切れセッションを削除しました", "upload_id", uploadID)
	}

	// ディスク上の古い.temp/.metaファイルもチェック
	um.cleanupOrphanedFiles()
}

// cleanupOrphanedFiles 孤立した.temp/.metaファイルのクリーンアップ
func (um *UploadManager) cleanupOrphanedFiles() {
	for _, dir := range um.config.Storage.Directories {
		dirPath := filepath.Join(um.config.Storage.UploadPath, dir.Path)

		if err := filepath.Walk(dirPath, func(path string, info fs.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}

			// .metaファイルの場合
			if filepath.Ext(path) == ".meta" {
				// メタファイルから有効期限を読み取る
				data, err := os.ReadFile(path)
				if err != nil {
					return nil
				}

				var session models.UploadSession
				if err := json.Unmarshal(data, &session); err != nil {
					return nil
				}

				// 期限切れなら削除
				if time.Now().After(session.ExpiresAt) {
					os.Remove(path)

					// 対応する.tempファイルも削除
					tempPath := path[:len(path)-5] + ".temp"
					os.Remove(tempPath)

					slog.Info("孤立ファイルを削除しました", "path", path)
				}
			}

			return nil
		}); err != nil {
			slog.Error("孤立ファイルのクリーンアップに失敗しました", "directory", dirPath, "error", err)
		}
	}
}

// saveMetaFile メタファイル保存
func (um *UploadManager) saveMetaFile(session *models.UploadSession) error {
	metaPath := um.getMetaFilePath(session.UploadID, session.Filename, session.Directory)

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(metaPath, data, 0600)
}

// loadSessionFromMeta メタファイルからセッション復元
func (um *UploadManager) loadSessionFromMeta(uploadID string) (*models.UploadSession, error) {
	// 全ディレクトリを検索
	for _, dir := range um.config.Storage.Directories {
		metaPath := filepath.Join(um.config.Storage.UploadPath, dir.Path, uploadID+"_*.meta")
		matches, err := filepath.Glob(metaPath)
		if err != nil || len(matches) == 0 {
			continue
		}

		data, err := os.ReadFile(matches[0])
		if err != nil {
			continue
		}

		var session models.UploadSession
		if err := json.Unmarshal(data, &session); err != nil {
			continue
		}

		return &session, nil
	}

	return nil, ErrSessionNotFound
}

// getTempFilePath .tempファイルパス取得
func (um *UploadManager) getTempFilePath(uploadID, filename, directory string) string {
	return filepath.Join(um.config.Storage.UploadPath, directory, fmt.Sprintf("%s_%s.temp", uploadID, sanitizeFilename(filename)))
}

// getMetaFilePath .metaファイルパス取得
func (um *UploadManager) getMetaFilePath(uploadID, filename, directory string) string {
	return filepath.Join(um.config.Storage.UploadPath, directory, fmt.Sprintf("%s_%s.meta", uploadID, sanitizeFilename(filename)))
}
