// Package handler はファイルサーバーのHTTPリクエストハンドラーを提供します。
// このファイルはチャンク分割されたファイルアップロード操作のハンドラーを含みます。
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"fileserver/internal/permission"
	"fileserver/internal/storage"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// validUploadID は upload_id が正規のUUIDであることを検証します。
// 未検証のIDは loadSessionFromMeta の filepath.Glob に渡り得るため、
// グロブメタ文字やパス片の混入をハンドラ入口で弾きます。不正時は400を書き込みます。
func validUploadID(w http.ResponseWriter, uploadID string) bool {
	if _, err := uuid.Parse(uploadID); err != nil {
		http.Error(w, "無効なアップロードIDです", http.StatusBadRequest)
		return false
	}
	return true
}

// writeChunkError はストレージ層のエラーを適切なHTTPステータスに変換して応答します。
func writeChunkError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storage.ErrSessionNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, storage.ErrPermissionDenied):
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, storage.ErrInvalidChunk),
		errors.Is(err, storage.ErrIncompleteUpload),
		errors.Is(err, storage.ErrSizeMismatch),
		errors.Is(err, storage.ErrMaxConcurrentUploads):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "内部エラーが発生しました", http.StatusInternalServerError)
	}
}

// ChunkHandler はチャンク分割されたファイルアップロードのHTTPリクエストを処理します。
type ChunkHandler struct {
	storageManager    *storage.Manager
	uploadManager     *storage.UploadManager
	permissionChecker *permission.Checker
}

// NewChunkHandler は新しいチャンクアップロードハンドラーを作成します。
func NewChunkHandler(sm *storage.Manager, um *storage.UploadManager, pc *permission.Checker) *ChunkHandler {
	return &ChunkHandler{
		storageManager:    sm,
		uploadManager:     um,
		permissionChecker: pc,
	}
}

// InitChunkUpload は新しいチャンク分割アップロードセッションを初期化します。
// 権限を検証し、アップロードセッションを作成し、アップロードIDを返します。
func (h *ChunkHandler) InitChunkUpload(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(w, r)
	if !ok {
		return
	}

	var req struct {
		Filename  string `json:"filename"`
		Directory string `json:"directory"`
		FileSize  int64  `json:"file_size"`
		ChunkSize int64  `json:"chunk_size"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "リクエストのパースに失敗しました", http.StatusBadRequest)
		return
	}

	if req.Filename == "" || req.Directory == "" || req.FileSize <= 0 || req.ChunkSize <= 0 {
		http.Error(w, "必須パラメータが不足しています", http.StatusBadRequest)
		return
	}

	req.Directory, ok = cleanDir(w, req.Directory)
	if !ok {
		return
	}

	hasPermission, err := h.permissionChecker.CheckPermission(user.ID, req.Directory, "write")
	if err != nil {
		slog.ErrorContext(r.Context(), "権限チェックエラー", "error", err)
		http.Error(w, "権限の確認に失敗しました", http.StatusInternalServerError)
		return
	}
	if !hasPermission {
		http.Error(w, "書き込み権限がありません", http.StatusForbidden)
		return
	}

	// user配下は初回アップロード時に個別ディレクトリを作る（事前作成しない方針）。
	if strings.HasPrefix(req.Directory, "user/") {
		if ensureErr := h.storageManager.EnsureUserDirectory(user.GetDirectoryName()); ensureErr != nil {
			slog.ErrorContext(r.Context(), "ユーザーディレクトリ作成エラー", "error", ensureErr)
			http.Error(w, "ユーザーディレクトリの作成に失敗しました", http.StatusInternalServerError)
			return
		}
	}

	// 切り上げ除算でチャンク数を求める。
	totalChunks := int((req.FileSize + req.ChunkSize - 1) / req.ChunkSize)
	session, err := h.uploadManager.CreateUploadSession(
		user.ID,
		req.Filename,
		req.Directory,
		req.FileSize,
		req.ChunkSize,
		totalChunks,
	)
	if err != nil {
		slog.ErrorContext(r.Context(), "アップロード初期化エラー", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	uploadID := session.UploadID

	slog.InfoContext(r.Context(), "チャンクアップロード初期化", "upload_id", uploadID, "user_id", user.ID, "filename", req.Filename, "directory", req.Directory)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":      true,
		"upload_id":    uploadID,
		"total_chunks": totalChunks,
		"chunk_size":   req.ChunkSize,
	})
}

// UploadChunk は進行中のアップロードのための単一のチャンクデータを受信して保存します。
func (h *ChunkHandler) UploadChunk(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(w, r)
	if !ok {
		return
	}

	uploadID := chi.URLParam(r, "upload_id")
	if !validUploadID(w, uploadID) {
		return
	}
	chunkIndexStr := r.URL.Query().Get("chunk_index")

	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil {
		http.Error(w, "無効なチャンクインデックスです", http.StatusBadRequest)
		return
	}

	// ボディ読み取り前にセッションを引く（所有者照合とMaxBytesReaderの上限決定のため）。
	session, err := h.uploadManager.GetUploadSession(uploadID)
	if err != nil {
		writeChunkError(w, err)
		return
	}
	if session.UserID != user.ID {
		http.Error(w, "このアップロードセッションを操作する権限がありません", http.StatusForbidden)
		return
	}

	// ReadAllで全体を読むため、1チャンク分でボディを打ち切りメモリ枯渇を防ぐ。
	r.Body = http.MaxBytesReader(w, r.Body, session.ChunkSize)

	chunkData, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "チャンクデータが大きすぎるか読み取りに失敗しました", http.StatusBadRequest)
		return
	}

	if err := h.uploadManager.SaveChunk(uploadID, user.ID, chunkIndex, chunkData); err != nil {
		slog.ErrorContext(r.Context(), "チャンク保存エラー", "upload_id", uploadID, "chunk_index", chunkIndex, "error", err)
		writeChunkError(w, err)
		return
	}

	slog.DebugContext(r.Context(), "チャンク保存成功", "upload_id", uploadID, "chunk_index", chunkIndex, "size", len(chunkData))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":     true,
		"chunk_index": chunkIndex,
	})
}

// GetChunkStatus は進捗を含むチャンク分割アップロードの現在のステータスを返します。
func (h *ChunkHandler) GetChunkStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(w, r)
	if !ok {
		return
	}

	uploadID := chi.URLParam(r, "upload_id")
	if !validUploadID(w, uploadID) {
		return
	}

	session, err := h.uploadManager.GetUploadSession(uploadID)
	if err != nil {
		writeChunkError(w, err)
		return
	}

	if session.UserID != user.ID {
		http.Error(w, "このアップロードセッションを操作する権限がありません", http.StatusForbidden)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":         true,
		"upload_id":       uploadID,
		"filename":        session.Filename,
		"directory":       session.Directory,
		"total_chunks":    session.TotalChunks,
		"uploaded_chunks": session.UploadedChunks,
		"file_size":       session.TotalSize,
		"uploaded_size":   session.UploadedSize,
		"created_at":      session.CreatedAt,
		"updated_at":      session.UpdatedAt,
	})
}

// CompleteChunkUpload はすべてのチャンクが受信された後、チャンク分割アップロードを完了します。
func (h *ChunkHandler) CompleteChunkUpload(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(w, r)
	if !ok {
		return
	}

	uploadID := chi.URLParam(r, "upload_id")
	if !validUploadID(w, uploadID) {
		return
	}

	savedFile, err := h.uploadManager.CompleteUpload(uploadID, user.ID)
	if err != nil {
		slog.ErrorContext(r.Context(), "アップロード完了エラー", "upload_id", uploadID, "error", err)
		writeChunkError(w, err)
		return
	}

	directory := filepath.Dir(savedFile.Path)

	// メタデータ保存の失敗は完了を失敗させない（本体は保存済み）。
	if err := h.storageManager.SaveFileMetadata(directory, savedFile.Filename, user.ID, user.Username); err != nil {
		slog.WarnContext(r.Context(), "メタデータの保存に失敗しました", "error", err)
	}

	slog.InfoContext(r.Context(), "チャンクアップロード完了", "upload_id", uploadID, "final_path", savedFile.Path)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"message":  "アップロードが完了しました",
		"path":     savedFile.Path,
		"filename": savedFile.Filename,
		"size":     savedFile.Size,
	})
}

// CancelChunkUpload は進行中のチャンク分割アップロードを中止し、一時ファイルをクリーンアップします。
func (h *ChunkHandler) CancelChunkUpload(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(w, r)
	if !ok {
		return
	}

	uploadID := chi.URLParam(r, "upload_id")
	if !validUploadID(w, uploadID) {
		return
	}

	if err := h.uploadManager.CancelUpload(uploadID, user.ID); err != nil {
		slog.ErrorContext(r.Context(), "アップロードキャンセルエラー", "upload_id", uploadID, "error", err)
		writeChunkError(w, err)
		return
	}

	slog.InfoContext(r.Context(), "チャンクアップロードキャンセル", "upload_id", uploadID)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "アップロードをキャンセルしました",
	})
}
