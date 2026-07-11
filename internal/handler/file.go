// Package handler はファイルサーバーのHTTPリクエストハンドラーを提供します。
// このファイルはファイル操作（アップロード、ダウンロード、削除、一覧表示）のハンドラーを含みます。
package handler

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"fileserver/internal/config"
	"fileserver/internal/permission"
	"fileserver/internal/storage"
)

// FileHandler はファイル操作のHTTPリクエストを処理します。
type FileHandler struct {
	config            *config.Config
	storageManager    *storage.Manager
	uploadManager     *storage.UploadManager
	permissionChecker *permission.Checker
	sseHandler        *SSEHandler
}

// NewFileHandler は指定された依存関係で新しいファイルハンドラーを作成します。
func NewFileHandler(cfg *config.Config, sm *storage.Manager, um *storage.UploadManager, pc *permission.Checker) *FileHandler {
	return &FileHandler{
		config:            cfg,
		storageManager:    sm,
		uploadManager:     um,
		permissionChecker: pc,
	}
}

// SetSSEHandler はファイルイベントをブロードキャストするためのSSEハンドラーを設定します。
func (h *FileHandler) SetSSEHandler(sse *SSEHandler) {
	h.sseHandler = sse
}

// Upload は設定された最大ファイルサイズまでの通常のファイルアップロードを処理します。
// 権限を検証し、ファイルを保存し、SSE経由でアップロードイベントをブロードキャストします。
func (h *FileHandler) Upload(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(w, r)
	if !ok {
		return
	}

	// FormValue/FormFile がボディをパースする前に上限を掛ける必要がある。
	// マルチパート境界・フォームフィールド分の余裕を加える。
	const multipartOverhead = 1 << 20
	r.Body = http.MaxBytesReader(w, r.Body, h.config.Storage.MaxFileSize+multipartOverhead)

	directory := r.FormValue("directory")
	if directory == "" {
		http.Error(w, "ディレクトリが指定されていません", http.StatusBadRequest)
		return
	}

	directory, ok = cleanDir(w, directory)
	if !ok {
		return
	}

	hasPermission, err := h.permissionChecker.CheckPermission(user.ID, directory, "write")
	if err != nil {
		slog.Error("権限チェックエラー", "error", err)
		http.Error(w, "権限の確認に失敗しました", http.StatusInternalServerError)
		return
	}
	if !hasPermission {
		http.Error(w, "書き込み権限がありません", http.StatusForbidden)
		return
	}

	// user配下は初回アップロード時に個別ディレクトリを作る（事前作成しない方針）。
	if strings.HasPrefix(directory, "user/") {
		if ensureErr := h.storageManager.EnsureUserDirectory(user.GetDirectoryName()); ensureErr != nil {
			slog.Error("ユーザーディレクトリ作成エラー", "error", ensureErr)
			http.Error(w, "ユーザーディレクトリの作成に失敗しました", http.StatusInternalServerError)
			return
		}
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "ファイルの取得に失敗しました", http.StatusBadRequest)
		return
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			slog.Error("ファイルのクローズに失敗しました", "error", closeErr)
		}
	}()

	if header.Size > h.config.Storage.MaxFileSize {
		http.Error(w, fmt.Sprintf("ファイルサイズが制限を超えています（最大: %d MB）", h.config.Storage.MaxFileSize/(1024*1024)), http.StatusBadRequest)
		return
	}

	savedFile, err := h.storageManager.SaveFile(file, header.Filename, directory)
	if err != nil {
		slog.Error("ファイル保存エラー", "error", err)
		http.Error(w, "ファイルの保存に失敗しました", http.StatusInternalServerError)
		return
	}

	// メタデータ保存の失敗はアップロード自体を失敗させない（本体は保存済み）。
	if err := h.storageManager.SaveFileMetadata(directory, savedFile.Filename, user.ID, user.Username); err != nil {
		slog.Warn("メタデータの保存に失敗しました", "error", err)
	}

	slog.Info("ファイルアップロード成功", "user_id", user.ID, "filename", header.Filename, "directory", directory, "size", header.Size)

	if h.sseHandler != nil {
		h.sseHandler.BroadcastFileUpload(user, directory, savedFile.Filename, savedFile.Size)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"filename": savedFile.Filename,
		"size":     savedFile.Size,
		"path":     savedFile.Path,
	})
}

// ListFiles は指定されたディレクトリ内のファイル一覧を返します。
// ファイル一覧を取得する前にユーザー権限を検証します。
func (h *FileHandler) ListFiles(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(w, r)
	if !ok {
		return
	}

	directory := r.URL.Query().Get("directory")
	if directory == "" {
		http.Error(w, "ディレクトリが指定されていません", http.StatusBadRequest)
		return
	}

	directory, ok = cleanDir(w, directory)
	if !ok {
		return
	}

	hasPermission, err := h.permissionChecker.CheckPermission(user.ID, directory, "read")
	if err != nil {
		slog.Error("権限チェックエラー", "error", err)
		http.Error(w, "権限の確認に失敗しました", http.StatusInternalServerError)
		return
	}
	if !hasPermission {
		http.Error(w, "読み取り権限がありません", http.StatusForbidden)
		return
	}

	files, err := h.storageManager.ListFiles(directory)
	if err != nil {
		slog.Error("ファイル一覧取得エラー", "error", err)
		http.Error(w, "ファイル一覧の取得に失敗しました", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"directory": directory,
		"files":     files,
	})
}

// Download はHTTP Rangeリクエストをサポートしたファイルダウンロードを処理します。
// これにより再開可能なダウンロードと部分的なコンテンツ配信が可能になります。
func (h *FileHandler) Download(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(w, r)
	if !ok {
		return
	}

	directory, filename, ok := decodePathParams(w, r)
	if !ok {
		return
	}

	hasPermission, err := h.permissionChecker.CheckPermission(user.ID, directory, "read")
	if err != nil {
		slog.Error("権限チェックエラー", "error", err)
		http.Error(w, "権限の確認に失敗しました", http.StatusInternalServerError)
		return
	}
	if !hasPermission {
		http.Error(w, "読み取り権限がありません", http.StatusForbidden)
		return
	}

	filePath := filepath.Join(h.config.Storage.UploadPath, directory, filename)

	// #nosec G703 - directory/filename は decodePathParams で ".." を除去済み
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

	// #nosec G304,G703 - filePath は decodePathParams で検証済みの入力から構築される
	file, err := os.Open(filePath)
	if err != nil {
		slog.Error("ファイルオープンエラー", "error", err)
		http.Error(w, "ファイルのオープンに失敗しました", http.StatusInternalServerError)
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Error("ファイルのクローズに失敗しました", "error", err)
		}
	}()

	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	rangeHeader := r.Header.Get("Range")
	if rangeHeader != "" {
		// 単一レンジのみ対応する（複数レンジ/multipartは未サポート）。
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
		w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))
		if _, err := io.Copy(w, file); err != nil {
			slog.Error("ファイル転送に失敗しました", "error", err)
		}
	}

	slog.Info("ファイルダウンロード", "user_id", user.ID, "filename", filename, "directory", directory)

	if h.sseHandler != nil {
		h.sseHandler.BroadcastFileDownload(user, directory, filename)
	}
}

// DeleteFile は指定されたディレクトリからファイルを削除します。
// ファイルを削除する前に削除権限を検証します。
func (h *FileHandler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(w, r)
	if !ok {
		return
	}

	directory, filename, ok := decodePathParams(w, r)
	if !ok {
		return
	}

	hasPermission, err := h.permissionChecker.CheckPermission(user.ID, directory, "delete")
	if err != nil {
		slog.Error("権限チェックエラー", "error", err)
		http.Error(w, "権限の確認に失敗しました", http.StatusInternalServerError)
		return
	}
	if !hasPermission {
		http.Error(w, "削除権限がありません", http.StatusForbidden)
		return
	}

	if err := h.storageManager.DeleteFile(directory, filename); err != nil {
		slog.Error("ファイル削除エラー", "error", err)
		http.Error(w, "ファイルの削除に失敗しました", http.StatusInternalServerError)
		return
	}

	slog.Info("ファイル削除成功", "user_id", user.ID, "filename", filename, "directory", directory)

	// SSEでブロードキャスト
	if h.sseHandler != nil {
		h.sseHandler.BroadcastFileDelete(user, directory, filename)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "ファイルを削除しました",
	})
}

// ListDirectories は認証されたユーザーがアクセス可能なすべてのディレクトリを返します。
// 各ディレクトリの権限情報を含みます。
func (h *FileHandler) ListDirectories(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(w, r)
	if !ok {
		return
	}

	accessibleDirs, err := h.permissionChecker.GetAccessibleDirectories(user.ID)
	if err != nil {
		slog.Error("アクセス可能ディレクトリ取得エラー", "error", err)
		http.Error(w, "ディレクトリ一覧の取得に失敗しました", http.StatusInternalServerError)
		return
	}

	directories := make([]map[string]interface{}, 0, len(accessibleDirs))
	for _, dir := range accessibleDirs {
		directories = append(directories, map[string]interface{}{
			"path":        dir.Path,
			"permissions": dir.Permissions,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":     true,
		"directories": directories,
	})
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
