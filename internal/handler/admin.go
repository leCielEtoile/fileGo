// Package handler は管理者用のHTTPハンドラーを提供します。
package handler

import (
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"

	"fileserver/internal/config"
	"fileserver/internal/storage"
)

// AdminHandler は管理者機能のHTTPハンドラーです。
type AdminHandler struct {
	config        *config.Config
	uploadManager *storage.UploadManager
}

// NewAdminHandler は新しい管理者ハンドラーを作成します。
func NewAdminHandler(cfg *config.Config, uploadManager *storage.UploadManager) *AdminHandler {
	return &AdminHandler{
		config:        cfg,
		uploadManager: uploadManager,
	}
}

// AdminPage は管理者ページを表示します。
func (h *AdminHandler) AdminPage(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("web/templates/admin.html")
	if err != nil {
		slog.Error("テンプレートの読み込みに失敗しました", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"ServiceName": h.config.Server.ServiceName,
	}

	if err := tmpl.Execute(w, data); err != nil {
		slog.Error("テンプレートのレンダリングに失敗しました", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// UploadSessionInfo はAPIレスポンス用のアップロードセッション情報です。
type UploadSessionInfo struct {
	UploadID       string  `json:"upload_id"`
	UserID         string  `json:"user_id"`
	Filename       string  `json:"filename"`
	Directory      string  `json:"directory"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	ExpiresAt      string  `json:"expires_at"`
	TotalSize      int64   `json:"total_size"`
	ChunkSize      int64   `json:"chunk_size"`
	Progress       float64 `json:"progress"`
	TotalChunks    int     `json:"total_chunks"`
	UploadedChunks int     `json:"uploaded_chunks"`
}

// GetUploadSessions は現在進行中のアップロードセッション一覧を返します。
func (h *AdminHandler) GetUploadSessions(w http.ResponseWriter, r *http.Request) {
	sessions := h.uploadManager.GetAllUploadSessions()

	// レスポンス用の形式に変換
	infos := make([]UploadSessionInfo, 0, len(sessions))
	for _, session := range sessions {
		progress := float64(len(session.UploadedChunks)) / float64(session.TotalChunks) * 100

		info := UploadSessionInfo{
			UploadID:       session.UploadID,
			UserID:         session.UserID,
			Filename:       session.Filename,
			Directory:      session.Directory,
			TotalSize:      session.TotalSize,
			ChunkSize:      session.ChunkSize,
			TotalChunks:    session.TotalChunks,
			UploadedChunks: len(session.UploadedChunks),
			Progress:       progress,
			CreatedAt:      session.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt:      session.UpdatedAt.Format("2006-01-02 15:04:05"),
			ExpiresAt:      session.ExpiresAt.Format("2006-01-02 15:04:05"),
		}
		infos = append(infos, info)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(infos); err != nil {
		slog.Error("JSONエンコードエラー", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// GetUploadStats はアップロード統計情報を返します。
func (h *AdminHandler) GetUploadStats(w http.ResponseWriter, r *http.Request) {
	sessions := h.uploadManager.GetAllUploadSessions()

	// ユーザー別のアップロード数を集計
	userUploads := make(map[string]int)
	var totalSize int64
	var totalUploadedSize int64

	for _, session := range sessions {
		userUploads[session.UserID]++
		totalSize += session.TotalSize

		// アップロード済みサイズを計算
		uploadedSize := int64(len(session.UploadedChunks)) * session.ChunkSize
		if uploadedSize > session.TotalSize {
			uploadedSize = session.TotalSize
		}
		totalUploadedSize += uploadedSize
	}

	stats := map[string]interface{}{
		"total_sessions":      len(sessions),
		"total_users":         len(userUploads),
		"total_size":          totalSize,
		"total_uploaded_size": totalUploadedSize,
		"max_concurrent":      h.config.Storage.MaxConcurrentUploads,
		"user_upload_counts":  userUploads,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		slog.Error("JSONエンコードエラー", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
