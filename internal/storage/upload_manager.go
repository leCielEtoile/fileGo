// Package storage はファイルストレージ管理機能を提供します。
// このファイルはチャンク分割されたファイルアップロードを処理するアップロードマネージャーを含みます。
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
	// ErrSessionNotFound はアップロードセッションが見つからない場合に返されます
	ErrSessionNotFound = errors.New("アップロードセッションが見つかりません")
	// ErrMaxConcurrentUploads はユーザーが同時アップロード制限を超えた場合に返されます
	ErrMaxConcurrentUploads = errors.New("同時アップロード数の上限に達しています")
	// ErrIncompleteUpload はチャンクが不足したままアップロードを完了しようとした場合に返されます
	ErrIncompleteUpload = errors.New("アップロードが完了していません")
	// ErrPermissionDenied は他ユーザーのアップロードセッションを操作しようとした場合に返されます
	ErrPermissionDenied = errors.New("このアップロードセッションを操作する権限がありません")
	// ErrInvalidChunk はチャンク番号やサイズが不正な場合に返されます
	ErrInvalidChunk = errors.New("チャンクの指定が不正です")
	// ErrSizeMismatch は完了時の実サイズが宣言サイズと一致しない場合に返されます
	ErrSizeMismatch = errors.New("アップロードされたファイルサイズが宣言と一致しません")
)

// UploadManager は同時アップロード制限とクリーンアップを備えたチャンク分割ファイルアップロードセッションを管理します。
type UploadManager struct {
	config      *config.Config
	sessions    map[string]*models.UploadSession
	userUploads map[string]int // ユーザーごとの同時アップロード数
	mu          sync.RWMutex
}

// NewUploadManager は新しいアップロードマネージャーを作成し、クリーンアップルーチンを開始します。
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

// CreateUploadSession はファイルのための新しいチャンク分割アップロードセッションを作成します。
// ファイルサイズの検証、同時アップロード制限のチェック、一時ファイルの作成を行います。
func (um *UploadManager) CreateUploadSession(userID, filename, directory string, totalSize, chunkSize int64, totalChunks int) (*models.UploadSession, error) {
	um.mu.Lock()
	defer um.mu.Unlock()

	if um.userUploads[userID] >= um.config.Storage.MaxConcurrentUploads {
		return nil, ErrMaxConcurrentUploads
	}

	if totalSize > um.config.Storage.MaxChunkFileSize {
		return nil, fmt.Errorf("ファイルサイズが上限を超えています")
	}

	// chunkSizeはクライアント指定のため、1チャンクの受信上限（MaxBytesReader）に
	// 直結する。上限を設けないと巨大チャンクや過大なチャンク数でメモリを枯渇できる。
	if chunkSize <= 0 || chunkSize > um.config.Storage.MaxChunkFileSize {
		return nil, fmt.Errorf("チャンクサイズが不正です")
	}
	const maxChunks = 100000
	if totalChunks <= 0 || totalChunks > maxChunks {
		return nil, fmt.Errorf("チャンク数が上限を超えています")
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

	um.sessions[uploadID] = session
	um.userUploads[userID]++

	// 総サイズ分を確保した空ファイルを先に作り、各チャンクをoffsetへ書き込む。
	tempPath := um.getTempFilePath(uploadID, filename, directory)
	if err := os.MkdirAll(filepath.Dir(tempPath), 0750); err != nil {
		return nil, err
	}

	// #nosec G304 - tempPath is constructed from sanitized inputs
	tempFile, err := os.Create(tempPath)
	if err != nil {
		return nil, err
	}
	if err := tempFile.Close(); err != nil {
		slog.Error("一時ファイルのクローズに失敗しました", "error", err)
	}

	// .metaが無いと再起動後にセッションを復元できないため、tempとセットで作る。
	if err := um.saveMetaFile(session); err != nil {
		if removeErr := os.Remove(tempPath); removeErr != nil {
			slog.Error("一時ファイルの削除に失敗しました", "error", removeErr)
		}
		return nil, err
	}

	return session, nil
}

// GetUploadSession はメモリまたはディスクからIDでアップロードセッションを取得します。
func (um *UploadManager) GetUploadSession(uploadID string) (*models.UploadSession, error) {
	um.mu.RLock()
	defer um.mu.RUnlock()

	return um.findSession(uploadID)
}

// SaveChunk はデータのチャンクを一時ファイルの適切なオフセットに保存します。
// アップロード済みチャンクを追跡し、セッションメタデータを更新します。
// userID はセッション所有者との照合に使用します（他ユーザーからの書き込みを拒否）。
func (um *UploadManager) SaveChunk(uploadID, userID string, chunkNumber int, data []byte) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	session, err := um.findSession(uploadID)
	if err != nil {
		return ErrSessionNotFound
	}
	um.sessions[uploadID] = session

	if session.UserID != userID {
		return ErrPermissionDenied
	}

	// chunkNumber/dataはクライアント任意入力。範囲外・過大サイズ・総サイズ超過を
	// 弾かないと、巨大offsetへの書き込みでスパースファイルを生成できてしまう。
	if chunkNumber < 0 || chunkNumber >= session.TotalChunks {
		return ErrInvalidChunk
	}
	if int64(len(data)) > session.ChunkSize {
		return ErrInvalidChunk
	}
	offset := int64(chunkNumber) * session.ChunkSize
	if offset+int64(len(data)) > session.TotalSize {
		return ErrInvalidChunk
	}

	// 冪等: 再送されたチャンクは成功として無視する。
	for _, uploaded := range session.UploadedChunks {
		if uploaded == chunkNumber {
			return nil
		}
	}

	tempPath := um.getTempFilePath(uploadID, session.Filename, session.Directory)
	// #nosec G304 - tempPath is constructed from sanitized inputs
	file, err := os.OpenFile(tempPath, os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Error("ファイルのクローズに失敗しました", "error", err)
		}
	}()

	if _, err := file.Seek(offset, 0); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}

	session.UploadedChunks = append(session.UploadedChunks, chunkNumber)
	session.UpdatedAt = time.Now()

	return um.saveMetaFile(session)
}

// GetUploadedChunks は正常にアップロードされたチャンク番号のリストを返します。
func (um *UploadManager) GetUploadedChunks(uploadID string) ([]int, error) {
	um.mu.RLock()
	defer um.mu.RUnlock()

	session, err := um.findSession(uploadID)
	if err != nil {
		return nil, ErrSessionNotFound
	}

	return session.UploadedChunks, nil
}

// CompleteUpload は一時ファイルをリネームしてメタデータをクリーンアップすることでアップロードを完了します。
// 完了前にすべてのチャンクがアップロード済みであることを検証します。
// userID はセッション所有者との照合に使用します。
func (um *UploadManager) CompleteUpload(uploadID, userID string) (*SavedFile, error) {
	um.mu.Lock()
	defer um.mu.Unlock()

	session, err := um.findSession(uploadID)
	if err != nil {
		return nil, ErrSessionNotFound
	}

	if session.UserID != userID {
		return nil, ErrPermissionDenied
	}

	if len(session.UploadedChunks) != session.TotalChunks {
		return nil, ErrIncompleteUpload
	}

	// チャンク数が揃っていても、各チャンクが規定サイズとは限らない。
	// 実サイズと宣言サイズの一致で完全性を担保する。
	tempPath := um.getTempFilePath(uploadID, session.Filename, session.Directory)
	if info, statErr := os.Stat(tempPath); statErr != nil {
		return nil, statErr
	} else if info.Size() != session.TotalSize {
		return nil, ErrSizeMismatch
	}

	fileID := uuid.New().String()
	finalFilename := fmt.Sprintf("%s_%s", fileID, sanitizeFilename(session.Filename))
	finalPath := filepath.Join(um.config.Storage.UploadPath, session.Directory, finalFilename)

	if err := os.Rename(tempPath, finalPath); err != nil {
		return nil, err
	}

	fileInfo, err := os.Stat(finalPath)
	if err != nil {
		return nil, err
	}

	metaPath := um.getMetaFilePath(uploadID, session.Filename, session.Directory)
	if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
		slog.Error("メタファイルの削除に失敗しました", "error", err)
	}

	delete(um.sessions, uploadID)
	um.releaseUploadSlot(session.UserID)

	return &SavedFile{
		Filename: finalFilename,
		Path:     filepath.Join(session.Directory, finalFilename),
		Size:     fileInfo.Size(),
	}, nil
}

// CancelUpload はアップロードセッションをキャンセルし、関連するすべてのファイルを削除します。
// userID はセッション所有者との照合に使用します。
func (um *UploadManager) CancelUpload(uploadID, userID string) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	session, err := um.findSession(uploadID)
	if err != nil {
		return ErrSessionNotFound
	}

	if session.UserID != userID {
		return ErrPermissionDenied
	}

	um.removeSession(uploadID, session)
	return nil
}

// startCleanupRoutine は一定間隔で期限切れセッションを掃除するゴルーチンです。
func (um *UploadManager) startCleanupRoutine() {
	// time.NewTicker は非正の間隔でパニックする。設定側で既定値を入れているが、
	// ここが落ちるとゴルーチン内のため回復できずプロセスごと死ぬので二重に守る。
	interval := um.config.Storage.CleanupInterval
	if interval <= 0 {
		interval = time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		um.cleanup()
	}
}

// cleanup はメモリ上の期限切れセッションを削除し、ディスク上の孤立ファイルも掃除します。
func (um *UploadManager) cleanup() {
	um.mu.Lock()
	defer um.mu.Unlock()

	now := time.Now()
	expiredSessions := make([]string, 0)

	// 走査中にmapを変更しないよう、対象を集めてから削除する。
	for uploadID, session := range um.sessions {
		if now.After(session.ExpiresAt) {
			expiredSessions = append(expiredSessions, uploadID)
		}
	}

	for _, uploadID := range expiredSessions {
		um.removeSession(uploadID, um.sessions[uploadID])
		slog.Info("期限切れセッションを削除しました", "upload_id", uploadID)
	}

	um.cleanupOrphanedFiles()
}

// cleanupOrphanedFiles は各ディレクトリを走査し、期限切れの.meta/.tempファイルを削除します。
// メモリにセッションが残っていない再起動後のケースを想定した後始末です。
func (um *UploadManager) cleanupOrphanedFiles() {
	for _, dir := range um.config.Storage.Directories {
		dirPath := filepath.Join(um.config.Storage.UploadPath, dir.Path)

		if err := filepath.Walk(dirPath, func(path string, info fs.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}

			if filepath.Ext(path) == ".meta" {
				// #nosec G304,G122 - path はアプリ所有のアップロードディレクトリのみを走査する filepath.Walk 由来
				data, err := os.ReadFile(path)
				if err != nil {
					return nil
				}

				var session models.UploadSession
				if err := json.Unmarshal(data, &session); err != nil {
					return nil
				}

				if time.Now().After(session.ExpiresAt) {
					// #nosec G122 - path はアプリ所有のアップロードディレクトリ内
					if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
						slog.Error("孤立ファイルの削除に失敗しました", "error", err)
					}

					// .metaと対になる.tempも消す（"..._X.meta" → "..._X.temp"）
					tempPath := path[:len(path)-5] + ".temp"
					// #nosec G122 - tempPath はアプリ所有のアップロードディレクトリ内
					if err := os.Remove(tempPath); err != nil && !os.IsNotExist(err) {
						slog.Error("孤立ファイルの削除に失敗しました", "error", err)
					}

					slog.Info("孤立ファイルを削除しました", "path", path)
				}
			}

			return nil
		}); err != nil {
			slog.Error("孤立ファイルのクリーンアップに失敗しました", "directory", dirPath, "error", err)
		}
	}
}

// saveMetaFile はセッションの状態を.metaファイルにJSONで永続化します。
func (um *UploadManager) saveMetaFile(session *models.UploadSession) error {
	metaPath := um.getMetaFilePath(session.UploadID, session.Filename, session.Directory)

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(metaPath, data, 0600)
}

// loadSessionFromMeta は全ディレクトリを検索し、.metaファイルからセッションを復元します。
// 見つからない場合は ErrSessionNotFound を返します。
func (um *UploadManager) loadSessionFromMeta(uploadID string) (*models.UploadSession, error) {
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

// findSession はメモリから、無ければ.metaファイルからセッションを取得します。
// 呼び出し側は um.mu を保持している必要があります（本関数はロックを取得しません）。
func (um *UploadManager) findSession(uploadID string) (*models.UploadSession, error) {
	if session, ok := um.sessions[uploadID]; ok {
		return session, nil
	}
	return um.loadSessionFromMeta(uploadID)
}

// removeSession は一時ファイルとメタファイルを削除し、セッション管理情報を破棄します。
// 呼び出し側は um.mu の書き込みロックを保持している必要があります。
func (um *UploadManager) removeSession(uploadID string, session *models.UploadSession) {
	tempPath := um.getTempFilePath(uploadID, session.Filename, session.Directory)
	if err := os.Remove(tempPath); err != nil && !os.IsNotExist(err) {
		slog.Error("一時ファイルの削除に失敗しました", "error", err)
	}

	metaPath := um.getMetaFilePath(uploadID, session.Filename, session.Directory)
	if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
		slog.Error("メタファイルの削除に失敗しました", "error", err)
	}

	delete(um.sessions, uploadID)
	um.releaseUploadSlot(session.UserID)
}

// releaseUploadSlot はユーザーの同時アップロード数を1減らします。
// .metaから復元したセッション（再起動でカウンタ未加算）を破棄しても
// カウンタが負値にならないよう下限を0で保護し、0になった要素は取り除きます。
func (um *UploadManager) releaseUploadSlot(userID string) {
	if um.userUploads[userID] <= 1 {
		delete(um.userUploads, userID)
		return
	}
	um.userUploads[userID]--
}

// sessionFilePath はセッションに紐づくファイル（.temp/.meta）のパスを組み立てます。
func (um *UploadManager) sessionFilePath(uploadID, filename, directory, ext string) string {
	name := fmt.Sprintf("%s_%s%s", uploadID, sanitizeFilename(filename), ext)
	return filepath.Join(um.config.Storage.UploadPath, directory, name)
}

// getTempFilePath .tempファイルパス取得
func (um *UploadManager) getTempFilePath(uploadID, filename, directory string) string {
	return um.sessionFilePath(uploadID, filename, directory, ".temp")
}

// getMetaFilePath .metaファイルパス取得
func (um *UploadManager) getMetaFilePath(uploadID, filename, directory string) string {
	return um.sessionFilePath(uploadID, filename, directory, ".meta")
}

// GetAllUploadSessions は現在進行中のすべてのアップロードセッション情報を取得します（管理者用）。
func (um *UploadManager) GetAllUploadSessions() []*models.UploadSession {
	um.mu.RLock()
	defer um.mu.RUnlock()

	sessions := make([]*models.UploadSession, 0, len(um.sessions))
	for _, session := range um.sessions {
		sessions = append(sessions, session)
	}

	return sessions
}
