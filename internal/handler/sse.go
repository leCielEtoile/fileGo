package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"fileserver/internal/models"
	"fileserver/internal/permission"
)

// SSEEvent はイベントの種類を表します。
// Directory が非空のイベントは、そのディレクトリへの read 権限を持つ
// クライアントにのみ配信します（空の場合は全ログインユーザーへ配信）。
type SSEEvent struct {
	Data      interface{}
	Type      string
	Directory string
}

// sseClient は接続中のSSEクライアント1件を表します。
// userID はイベント毎のディレクトリ権限フィルタに使用します。
type sseClient struct {
	ch     chan SSEEvent
	userID string
}

// SSEHandler はServer-Sent Eventsハンドラーを表します。
type SSEHandler struct {
	clients           map[*sseClient]bool
	permissionChecker *permission.Checker
	mu                sync.RWMutex
}

// NewSSEHandler はSSEハンドラーを作成します。
// permissionChecker はファイルイベントをディレクトリ権限で絞り込むために使用します。
func NewSSEHandler(pc *permission.Checker) *SSEHandler {
	return &SSEHandler{
		clients:           make(map[*sseClient]bool),
		permissionChecker: pc,
	}
}

// HandleSSE はSSEエンドポイントを処理します。
func (h *SSEHandler) HandleSSE(w http.ResponseWriter, r *http.Request) {
	// SSE用のヘッダー設定
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// 認証必須の同一オリジンendpointのためワイルドカードCORSは付与しない。
	// Cloudflare対策: X-Accel-Buffering を無効化してバッファリングを防ぐ
	w.Header().Set("X-Accel-Buffering", "no")

	// 権限フィルタのため接続ユーザーを取り出す（本ルートはAuthMiddleware配下）。
	user, ok := r.Context().Value(models.UserContextKey).(*models.User)
	if !ok {
		http.Error(w, "認証情報が見つかりません", http.StatusUnauthorized)
		return
	}

	// バッファ付きにして、遅い/切断済みクライアントがbroadcastをブロックしないようにする。
	client := &sseClient{ch: make(chan SSEEvent, 10), userID: user.ID}

	h.mu.Lock()
	h.clients[client] = true
	clientCount := len(h.clients)
	h.mu.Unlock()

	// 切断でも書き込みエラーでも確実に登録解除する（チャネル/ゴルーチンのリークを防ぐ）。
	// 解除は排他ロック下で行うため、broadcast(共有ロック)がクローズ済みチャネルへ
	// 送信することはない。
	defer func() {
		h.mu.Lock()
		delete(h.clients, client)
		remaining := len(h.clients)
		h.mu.Unlock()
		close(client.ch)
		slog.Info("SSE client disconnected", "total_clients", remaining)
	}()

	slog.Info("SSE client connected", "total_clients", clientCount)

	if _, err := fmt.Fprintf(w, "data: {\"message\": \"connected\"}\n\n"); err != nil {
		slog.Error("SSE write error", "error", err)
		return
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	ctx := r.Context()

	// Cloudflareは100秒で無通信接続を切るため、それより短い間隔でハートビートを送る。
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case event := <-client.ch:
			jsonData, err := json.Marshal(event.Data)
			if err != nil {
				slog.Error("JSON marshal error", "error", err)
				continue
			}

			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, jsonData); err != nil {
				slog.Error("SSE write error", "error", err)
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}

		case <-ticker.C:
			if _, err := fmt.Fprintf(w, ": heartbeat\n\n"); err != nil {
				slog.Error("SSE write error", "error", err)
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}
}

// BroadcastFileUpload はファイルアップロードイベントをブロードキャストします。
func (h *SSEHandler) BroadcastFileUpload(user *models.User, directory, filename string, size int64) {
	h.broadcast(SSEEvent{
		Type:      "file_upload",
		Directory: directory,
		Data: map[string]interface{}{
			"username":  user.Username,
			"user_id":   user.ID,
			"directory": directory,
			"filename":  filename,
			"size":      size,
			"timestamp": time.Now().Format(time.RFC3339),
		},
	})
}

// BroadcastFileDownload はファイルダウンロードイベントをブロードキャストします。
func (h *SSEHandler) BroadcastFileDownload(user *models.User, directory, filename string) {
	h.broadcast(SSEEvent{
		Type:      "file_download",
		Directory: directory,
		Data: map[string]interface{}{
			"username":  user.Username,
			"user_id":   user.ID,
			"directory": directory,
			"filename":  filename,
			"timestamp": time.Now().Format(time.RFC3339),
		},
	})
}

// BroadcastFileDelete はファイル削除イベントをブロードキャストします。
func (h *SSEHandler) BroadcastFileDelete(user *models.User, directory, filename string) {
	h.broadcast(SSEEvent{
		Type:      "file_delete",
		Directory: directory,
		Data: map[string]interface{}{
			"username":  user.Username,
			"user_id":   user.ID,
			"directory": directory,
			"filename":  filename,
			"timestamp": time.Now().Format(time.RFC3339),
		},
	})
}

// BroadcastUserLogin はユーザーログインイベントをブロードキャストします。
func (h *SSEHandler) BroadcastUserLogin(user *models.User) {
	h.broadcast(SSEEvent{
		Type: "user_login",
		Data: map[string]interface{}{
			"username":  user.Username,
			"user_id":   user.ID,
			"timestamp": time.Now().Format(time.RFC3339),
		},
	})
}

// broadcast はイベントを、そのディレクトリへの閲覧権限を持つクライアントにのみ配信します。
// Directory が空のイベント（ログイン通知など）は全クライアントへ配信します。
func (h *SSEHandler) broadcast(event SSEEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		if !h.canReceive(client, event) {
			continue
		}
		select {
		case client.ch <- event:
		default:
			// 詰まっているクライアントで全体をブロックしないよう、送れなければ捨てる。
			slog.Warn("SSE client channel full, skipping event", "event_type", event.Type)
		}
	}

	slog.Debug("SSE event broadcasted", "type", event.Type, "clients", len(h.clients))
}

// canReceive はクライアントがイベントを受信してよいかを判定します。
// ディレクトリ付きイベントは read 権限を要求し、権限確認に失敗した場合は
// 情報漏えいを避けるため配信しません（フェイルクローズ）。
func (h *SSEHandler) canReceive(client *sseClient, event SSEEvent) bool {
	if event.Directory == "" {
		return true
	}
	if h.permissionChecker == nil {
		return false
	}
	allowed, err := h.permissionChecker.CheckPermission(client.userID, event.Directory, "read")
	if err != nil {
		slog.Debug("SSE権限チェックに失敗したためイベントを抑止しました", "user_id", client.userID, "directory", event.Directory, "error", err)
		return false
	}
	return allowed
}

// GetClientCount は接続中のクライアント数を取得します。
func (h *SSEHandler) GetClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
