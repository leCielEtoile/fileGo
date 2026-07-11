// Package handler はファイルサーバーのHTTPリクエストハンドラーを提供します。
// このファイルはチャンク分割されたファイルアップロード操作のハンドラーを含みます。
package handler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"fileserver/internal/permission"
	"fileserver/internal/storage"

	"github.com/go-chi/chi/v5"
)

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

	// バリデーション
	if req.Filename == "" || req.Directory == "" || req.FileSize <= 0 || req.ChunkSize <= 0 {
		http.Error(w, "必須パラメータが不足しています", http.StatusBadRequest)
		return
	}

	// パストラバーサル対策
	req.Directory, ok = cleanDir(w, req.Directory)
	if !ok {
		return
	}

	// 権限チェック
	hasPermission, err := h.permissionChecker.CheckPermission(user.ID, req.Directory, "write")
	if err != nil {
		slog.Error("権限チェックエラー", "error", err)
		http.Error(w, "権限の確認に失敗しました", http.StatusInternalServerError)
		return
	}
	if !hasPermission {
		http.Error(w, "書き込み権限がありません", http.StatusForbidden)
		return
	}

	// userディレクトリの場合、ユーザー個別ディレクトリを自動作成
	if strings.HasPrefix(req.Directory, "user/") {
		if ensureErr := h.storageManager.EnsureUserDirectory(user.GetDirectoryName()); ensureErr != nil {
			slog.Error("ユーザーディレクトリ作成エラー", "error", ensureErr)
			http.Error(w, "ユーザーディレクトリの作成に失敗しました", http.StatusInternalServerError)
			return
		}
	}

	// アップロードセッション初期化
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
		slog.Error("アップロード初期化エラー", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	uploadID := session.UploadID

	slog.Info("チャンクアップロード初期化", "upload_id", uploadID, "user_id", user.ID, "filename", req.Filename, "directory", req.Directory)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":      true,
		"upload_id":    uploadID,
		"total_chunks": totalChunks,
		"chunk_size":   req.ChunkSize,
	})
}

// UploadChunk は進行中のアップロードのための単一のチャンクデータを受信して保存します。
func (h *ChunkHandler) UploadChunk(w http.ResponseWriter, r *http.Request) {
	uploadID := chi.URLParam(r, "upload_id")
	chunkIndexStr := r.URL.Query().Get("chunk_index")

	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil {
		http.Error(w, "無効なチャンクインデックスです", http.StatusBadRequest)
		return
	}

	// チャンクデータを読み取り
	chunkData, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "チャンクデータの読み取りに失敗しました", http.StatusBadRequest)
		return
	}

	// チャンクを保存
	if err := h.uploadManager.SaveChunk(uploadID, chunkIndex, chunkData); err != nil {
		slog.Error("チャンク保存エラー", "upload_id", uploadID, "chunk_index", chunkIndex, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	slog.Debug("チャンク保存成功", "upload_id", uploadID, "chunk_index", chunkIndex, "size", len(chunkData))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":     true,
		"chunk_index": chunkIndex,
	})
}

// GetChunkStatus は進捗を含むチャンク分割アップロードの現在のステータスを返します。
func (h *ChunkHandler) GetChunkStatus(w http.ResponseWriter, r *http.Request) {
	uploadID := chi.URLParam(r, "upload_id")

	session, err := h.uploadManager.GetUploadSession(uploadID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
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

	savedFile, err := h.uploadManager.CompleteUpload(uploadID)
	if err != nil {
		slog.Error("アップロード完了エラー", "upload_id", uploadID, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// ディレクトリとファイル名を抽出
	directory := filepath.Dir(savedFile.Path)

	// メタデータを保存
	if err := h.storageManager.SaveFileMetadata(directory, savedFile.Filename, user.ID, user.Username); err != nil {
		slog.Warn("メタデータの保存に失敗しました", "error", err)
	}

	slog.Info("チャンクアップロード完了", "upload_id", uploadID, "final_path", savedFile.Path)

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
	uploadID := chi.URLParam(r, "upload_id")

	if err := h.uploadManager.CancelUpload(uploadID); err != nil {
		slog.Error("アップロードキャンセルエラー", "upload_id", uploadID, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	slog.Info("チャンクアップロードキャンセル", "upload_id", uploadID)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "アップロードをキャンセルしました",
	})
}
