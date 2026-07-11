// Package handler はファイルサーバーのHTTPリクエストハンドラーを提供します。
// このファイルは各ハンドラーで共通して使うヘルパー関数をまとめます。
package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"fileserver/internal/models"

	"github.com/go-chi/chi/v5"
)

// userFromContext はAuthMiddlewareがコンテキストに格納したユーザーを取り出します。
// 取得できない場合は401を書き込み、ok=falseを返します（呼び出し側はreturnするだけでよい）。
func userFromContext(w http.ResponseWriter, r *http.Request) (*models.User, bool) {
	user, ok := r.Context().Value(models.UserContextKey).(*models.User)
	if !ok {
		http.Error(w, "認証情報が見つかりません", http.StatusUnauthorized)
		return nil, false
	}
	return user, true
}

// writeJSON はJSONレスポンスを書き込みます。
// エンコード失敗時はヘッダー送出済みのためログのみ行います。
//
//nolint:unparam // status は将来の非200レスポンスのため引数として保持する
func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("JSONエンコードに失敗しました", "error", err)
	}
}

// cleanDir はディレクトリパスを正規化し、パストラバーサルを検出します。
// 不正な場合は400を書き込み、ok=falseを返します。
func cleanDir(w http.ResponseWriter, dir string) (string, bool) {
	dir = filepath.Clean(dir)
	if strings.Contains(dir, "..") {
		http.Error(w, "無効なディレクトリパスです", http.StatusBadRequest)
		return "", false
	}
	return dir, true
}

// decodePathParams はURLパスの {directory}/{filename} をデコード・正規化し、
// パストラバーサルを検出します。不正な場合は400を書き込み、ok=falseを返します。
func decodePathParams(w http.ResponseWriter, r *http.Request) (directory, filename string, ok bool) {
	directory = chi.URLParam(r, "directory")
	filename = chi.URLParam(r, "filename")

	var err error
	if directory, err = url.PathUnescape(directory); err != nil {
		http.Error(w, "無効なディレクトリパスです", http.StatusBadRequest)
		return "", "", false
	}
	if filename, err = url.PathUnescape(filename); err != nil {
		http.Error(w, "無効なファイル名です", http.StatusBadRequest)
		return "", "", false
	}

	directory = filepath.Clean(directory)
	if strings.Contains(directory, "..") || strings.Contains(filename, "..") {
		http.Error(w, "無効なパスです", http.StatusBadRequest)
		return "", "", false
	}
	return directory, filename, true
}
