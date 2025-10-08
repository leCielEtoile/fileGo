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

// SSEEvent イベントの種類
type SSEEvent struct {
	Data interface{}
	Type string
}

// SSEHandler Server-Sent Eventsハンドラー
type SSEHandler struct {
	clients map[chan SSEEvent]bool
	mu      sync.RWMutex
}

// NewSSEHandler SSEハンドラーを作成
func NewSSEHandler() *SSEHandler {
	return &SSEHandler{
		clients: make(map[chan SSEEvent]bool),
	}
}

// HandleSSE SSEエンドポイント
func (h *SSEHandler) HandleSSE(w http.ResponseWriter, r *http.Request) {
	// SSE用のヘッダー設定
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	// Cloudflare対策: X-Accel-Buffering を無効化してバッファリングを防ぐ
	w.Header().Set("X-Accel-Buffering", "no")

	// クライアントチャンネル作成
	clientChan := make(chan SSEEvent, 10)

	// クライアント登録
	h.mu.Lock()
	h.clients[clientChan] = true
	clientCount := len(h.clients)
	h.mu.Unlock()

	slog.Info("SSE client connected", "total_clients", clientCount)

	// 接続確立メッセージ
	if _, err := fmt.Fprintf(w, "data: {\"message\": \"connected\"}\n\n"); err != nil {
		slog.Error("SSE write error", "error", err)
		return
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// コンテキストのキャンセルを監視
	ctx := r.Context()

	// ハートビート（15秒ごと - Cloudflareの100秒タイムアウト対策）
	// Cloudflareは100秒で接続を切断するため、それより短い間隔で送信
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	// クライアントにイベントを送信
	for {
		select {
		case <-ctx.Done():
			// クライアント切断
			h.mu.Lock()
			delete(h.clients, clientChan)
			clientCount := len(h.clients)
			h.mu.Unlock()
			close(clientChan)
			slog.Info("SSE client disconnected", "total_clients", clientCount)
			return

		case event := <-clientChan:
			// イベント送信
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
			// ハートビート送信
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

// BroadcastFileUpload ファイルアップロードイベントをブロードキャスト
func (h *SSEHandler) BroadcastFileUpload(user *models.User, directory, filename string, size int64) {
	h.broadcast(SSEEvent{
		Type: "file_upload",
		Data: map[string]interface{}{
			"username":  user.Username,
			"user_id":   user.DiscordID,
			"directory": directory,
			"filename":  filename,
			"size":      size,
			"timestamp": time.Now().Format(time.RFC3339),
		},
	})
}

// BroadcastFileDownload ファイルダウンロードイベントをブロードキャスト
func (h *SSEHandler) BroadcastFileDownload(user *models.User, directory, filename string) {
	h.broadcast(SSEEvent{
		Type: "file_download",
		Data: map[string]interface{}{
			"username":  user.Username,
			"user_id":   user.DiscordID,
			"directory": directory,
			"filename":  filename,
			"timestamp": time.Now().Format(time.RFC3339),
		},
	})
}

// BroadcastFileDelete ファイル削除イベントをブロードキャスト
func (h *SSEHandler) BroadcastFileDelete(user *models.User, directory, filename string) {
	h.broadcast(SSEEvent{
		Type: "file_delete",
		Data: map[string]interface{}{
			"username":  user.Username,
			"user_id":   user.DiscordID,
			"directory": directory,
			"filename":  filename,
			"timestamp": time.Now().Format(time.RFC3339),
		},
	})
}

// BroadcastUserLogin ユーザーログインイベントをブロードキャスト
func (h *SSEHandler) BroadcastUserLogin(user *models.User) {
	h.broadcast(SSEEvent{
		Type: "user_login",
		Data: map[string]interface{}{
			"username":  user.Username,
			"user_id":   user.DiscordID,
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
			// 送信成功
		default:
			// チャンネルがいっぱいの場合はスキップ
			slog.Warn("SSE client channel full, skipping event", "event_type", event.Type)
		}
	}

	slog.Debug("SSE event broadcasted", "type", event.Type, "clients", len(h.clients))
}

// GetClientCount 接続中のクライアント数を取得
func (h *SSEHandler) GetClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
