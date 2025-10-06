package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"fileserver/internal/config"
	"fileserver/internal/models"
	"fileserver/internal/permission"
	"fileserver/internal/storage"

	"github.com/go-chi/chi/v5"
)

// contextKey is a custom type for context keys to avoid staticcheck issues
type contextKey string

const userContextKey contextKey = "user"

type FileHandler struct {
	config            *config.Config
	storageManager    *storage.Manager
	uploadManager     *storage.UploadManager
	permissionChecker *permission.Checker
	sseHandler        *SSEHandler
}

func NewFileHandler(cfg *config.Config, sm *storage.Manager, um *storage.UploadManager, pc *permission.Checker) *FileHandler {
	return &FileHandler{
		config:            cfg,
		storageManager:    sm,
		uploadManager:     um,
		permissionChecker: pc,
	}
}

func (h *FileHandler) SetSSEHandler(sse *SSEHandler) {
	h.sseHandler = sse
}

// Upload 通常のファイルアップロード（最大100MB）
func (h *FileHandler) Upload(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(userContextKey).(*models.User)
	if !ok {
		http.Error(w, "ユーザー情報の取得に失敗しました", http.StatusInternalServerError)
		return
	}

	// ディレクトリパス取得
	directory := r.FormValue("directory")
	if directory == "" {
		http.Error(w, "ディレクトリが指定されていません", http.StatusBadRequest)
		return
	}

	// パストラバーサル対策
	directory = filepath.Clean(directory)
	if strings.Contains(directory, "..") {
		http.Error(w, "無効なディレクトリパスです", http.StatusBadRequest)
		return
	}

	// 権限チェック
	hasPermission, err := h.permissionChecker.CheckPermission(user.DiscordID, directory, "write")
	if err != nil {
		slog.Error("権限チェックエラー", "error", err)
		http.Error(w, "権限の確認に失敗しました", http.StatusInternalServerError)
		return
	}
	if !hasPermission {
		http.Error(w, "書き込み権限がありません", http.StatusForbidden)
		return
	}

	// ファイル取得
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "ファイルの取得に失敗しました", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// ファイルサイズチェック（100MB制限）
	if header.Size > h.config.Storage.MaxFileSize {
		http.Error(w, fmt.Sprintf("ファイルサイズが制限を超えています（最大: %d MB）", h.config.Storage.MaxFileSize/(1024*1024)), http.StatusBadRequest)
		return
	}

	// ファイル保存
	savedFile, err := h.storageManager.SaveFile(file, header.Filename, directory)
	if err != nil {
		slog.Error("ファイル保存エラー", "error", err)
		http.Error(w, "ファイルの保存に失敗しました", http.StatusInternalServerError)
		return
	}

	slog.Info("ファイルアップロード成功", "user_id", user.DiscordID, "filename", header.Filename, "directory", directory, "size", header.Size)

	// SSEでブロードキャスト
	if h.sseHandler != nil {
		h.sseHandler.BroadcastFileUpload(user, directory, savedFile.Filename, savedFile.Size)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"filename": savedFile.Filename,
		"size":     savedFile.Size,
		"path":     savedFile.Path,
	}); err != nil {
		slog.Error("JSONエンコードに失敗しました", "error", err)
	}
}

// ListFiles ファイル一覧取得
func (h *FileHandler) ListFiles(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(userContextKey).(*models.User)
	if !ok {
		http.Error(w, "ユーザー情報の取得に失敗しました", http.StatusInternalServerError)
		return
	}

	// ディレクトリパス取得
	directory := r.URL.Query().Get("directory")
	if directory == "" {
		http.Error(w, "ディレクトリが指定されていません", http.StatusBadRequest)
		return
	}

	// パストラバーサル対策
	directory = filepath.Clean(directory)
	if strings.Contains(directory, "..") {
		http.Error(w, "無効なディレクトリパスです", http.StatusBadRequest)
		return
	}

	// 権限チェック
	hasPermission, err := h.permissionChecker.CheckPermission(user.DiscordID, directory, "read")
	if err != nil {
		slog.Error("権限チェックエラー", "error", err)
		http.Error(w, "権限の確認に失敗しました", http.StatusInternalServerError)
		return
	}
	if !hasPermission {
		http.Error(w, "読み取り権限がありません", http.StatusForbidden)
		return
	}

	// ファイル一覧取得
	files, err := h.storageManager.ListFiles(directory)
	if err != nil {
		slog.Error("ファイル一覧取得エラー", "error", err)
		http.Error(w, "ファイル一覧の取得に失敗しました", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"directory": directory,
		"files":     files,
	}); err != nil {
		slog.Error("JSONエンコードに失敗しました", "error", err)
	}
}

// Download ファイルダウンロード（Range Request対応）
func (h *FileHandler) Download(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(userContextKey).(*models.User)
	if !ok {
		http.Error(w, "ユーザー情報の取得に失敗しました", http.StatusInternalServerError)
		return
	}

	directory := chi.URLParam(r, "directory")
	filename := chi.URLParam(r, "filename")

	// パストラバーサル対策
	directory = filepath.Clean(directory)
	if strings.Contains(directory, "..") || strings.Contains(filename, "..") {
		http.Error(w, "無効なパスです", http.StatusBadRequest)
		return
	}

	// 権限チェック
	hasPermission, err := h.permissionChecker.CheckPermission(user.DiscordID, directory, "read")
	if err != nil {
		slog.Error("権限チェックエラー", "error", err)
		http.Error(w, "権限の確認に失敗しました", http.StatusInternalServerError)
		return
	}
	if !hasPermission {
		http.Error(w, "読み取り権限がありません", http.StatusForbidden)
		return
	}

	// ファイルパス構築
	filePath := filepath.Join(h.config.Storage.UploadPath, directory, filename)

	// ファイル存在チェック
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "ファイルが見つかりません", http.StatusNotFound)
			return
		}
		slog.Error("ファイル情報取得エラー", "error", err)
		http.Error(w, "ファイル情報の取得に失敗しました", http.StatusInternalServerError)
		return
	}

	// ファイルオープン
	file, err := os.Open(filePath)
	if err != nil {
		slog.Error("ファイルオープンエラー", "error", err)
		http.Error(w, "ファイルのオープンに失敗しました", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Range Requestのサポート
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	rangeHeader := r.Header.Get("Range")
	if rangeHeader != "" {
		// Range Requestの処理
		ranges, err := parseRange(rangeHeader, fileInfo.Size())
		if err != nil || len(ranges) != 1 {
			http.Error(w, "無効なRangeヘッダーです", http.StatusRequestedRangeNotSatisfiable)
			return
		}

		start, end := ranges[0][0], ranges[0][1]
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileInfo.Size()))
		w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
		w.WriteHeader(http.StatusPartialContent)

		if _, err := file.Seek(start, 0); err != nil {
			slog.Error("ファイルシークに失敗しました", "error", err)
			return
		}
		if _, err := io.CopyN(w, file, end-start+1); err != nil {
			slog.Error("ファイル転送に失敗しました", "error", err)
		}
	} else {
		// 通常のダウンロード
		w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))
		if _, err := io.Copy(w, file); err != nil {
			slog.Error("ファイル転送に失敗しました", "error", err)
		}
	}

	slog.Info("ファイルダウンロード", "user_id", user.DiscordID, "filename", filename, "directory", directory)

	// SSEでブロードキャスト
	if h.sseHandler != nil {
		h.sseHandler.BroadcastFileDownload(user, directory, filename)
	}
}

// DeleteFile ファイル削除
func (h *FileHandler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(userContextKey).(*models.User)
	if !ok {
		http.Error(w, "ユーザー情報の取得に失敗しました", http.StatusInternalServerError)
		return
	}

	directory := chi.URLParam(r, "directory")
	filename := chi.URLParam(r, "filename")

	// パストラバーサル対策
	directory = filepath.Clean(directory)
	if strings.Contains(directory, "..") || strings.Contains(filename, "..") {
		http.Error(w, "無効なパスです", http.StatusBadRequest)
		return
	}

	// 権限チェック
	hasPermission, err := h.permissionChecker.CheckPermission(user.DiscordID, directory, "delete")
	if err != nil {
		slog.Error("権限チェックエラー", "error", err)
		http.Error(w, "権限の確認に失敗しました", http.StatusInternalServerError)
		return
	}
	if !hasPermission {
		http.Error(w, "削除権限がありません", http.StatusForbidden)
		return
	}

	// ファイル削除
	if err := h.storageManager.DeleteFile(directory, filename); err != nil {
		slog.Error("ファイル削除エラー", "error", err)
		http.Error(w, "ファイルの削除に失敗しました", http.StatusInternalServerError)
		return
	}

	slog.Info("ファイル削除成功", "user_id", user.DiscordID, "filename", filename, "directory", directory)

	// SSEでブロードキャスト
	if h.sseHandler != nil {
		h.sseHandler.BroadcastFileDelete(user, directory, filename)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "ファイルを削除しました",
	}); err != nil {
		slog.Error("JSONエンコードに失敗しました", "error", err)
	}
}

// ListDirectories ユーザーがアクセス可能なディレクトリ一覧を取得
func (h *FileHandler) ListDirectories(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(userContextKey).(*models.User)
	if !ok {
		http.Error(w, "ユーザー情報の取得に失敗しました", http.StatusInternalServerError)
		return
	}

	// ユーザーがアクセス可能なディレクトリを取得
	accessibleDirs, err := h.permissionChecker.GetAccessibleDirectories(user.DiscordID)
	if err != nil {
		slog.Error("アクセス可能ディレクトリ取得エラー", "error", err)
		http.Error(w, "ディレクトリ一覧の取得に失敗しました", http.StatusInternalServerError)
		return
	}

	// レスポンス用のディレクトリ情報を構築
	directories := make([]map[string]interface{}, 0, len(accessibleDirs))
	for _, dir := range accessibleDirs {
		directories = append(directories, map[string]interface{}{
			"path":        dir.Path,
			"permissions": dir.Permissions,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"directories": directories,
	}); err != nil {
		slog.Error("JSONエンコードに失敗しました", "error", err)
	}
}

// parseRange Range ヘッダーをパースする
func parseRange(rangeHeader string, size int64) ([][2]int64, error) {
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		return nil, fmt.Errorf("無効なRange形式です")
	}

	rangeSpec := strings.TrimPrefix(rangeHeader, "bytes=")
	ranges := strings.Split(rangeSpec, ",")

	result := make([][2]int64, 0, len(ranges))
	for _, r := range ranges {
		parts := strings.Split(strings.TrimSpace(r), "-")
		if len(parts) != 2 {
			return nil, fmt.Errorf("無効なRange指定です")
		}

		var start, end int64
		var err error

		if parts[0] == "" {
			// 末尾からのバイト指定（例: -500）
			end = size - 1
			start, err = strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
				return nil, err
			}
			start = size - start
		} else if parts[1] == "" {
			// 開始位置からの指定（例: 500-）
			start, err = strconv.ParseInt(parts[0], 10, 64)
			if err != nil {
				return nil, err
			}
			end = size - 1
		} else {
			// 範囲指定（例: 500-999）
			start, err = strconv.ParseInt(parts[0], 10, 64)
			if err != nil {
				return nil, err
			}
			end, err = strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
				return nil, err
			}
		}

		if start < 0 || end >= size || start > end {
			return nil, fmt.Errorf("範囲外です")
		}

		result = append(result, [2]int64{start, end})
	}

	return result, nil
}
