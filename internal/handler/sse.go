package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"fileserver/internal/models"
)

// SSEEvent はイベントの種類を表します。
type SSEEvent struct {
	Data interface{}
	Type string
}

// SSEHandler はServer-Sent Eventsハンドラーを表します。
type SSEHandler struct {
	clients map[chan SSEEvent]bool
	mu      sync.RWMutex
}

// NewSSEHandler はSSEハンドラーを作成します。
func NewSSEHandler() *SSEHandler {
	return &SSEHandler{
		clients: make(map[chan SSEEvent]bool),
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

	// バッファ付きにして、遅い/切断済みクライアントがbroadcastをブロックしないようにする。
	clientChan := make(chan SSEEvent, 10)

	h.mu.Lock()
	h.clients[clientChan] = true
	clientCount := len(h.clients)
	h.mu.Unlock()

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
			h.mu.Lock()
			delete(h.clients, clientChan)
			clientCount := len(h.clients)
			h.mu.Unlock()
			close(clientChan)
			slog.Info("SSE client disconnected", "total_clients", clientCount)
			return

		case event := <-clientChan:
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
		Type: "file_upload",
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
		Type: "file_download",
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
		Type: "file_delete",
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

// broadcast 全クライアントにイベントをブロードキャスト
func (h *SSEHandler) broadcast(event SSEEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for clientChan := range h.clients {
		select {
		case clientChan <- event:
		default:
			// 詰まっているクライアントで全体をブロックしないよう、送れなければ捨てる。
			slog.Warn("SSE client channel full, skipping event", "event_type", event.Type)
		}
	}

	slog.Debug("SSE event broadcasted", "type", event.Type, "clients", len(h.clients))
}

// GetClientCount は接続中のクライアント数を取得します。
func (h *SSEHandler) GetClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
