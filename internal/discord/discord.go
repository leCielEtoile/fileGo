// Package discord provides Discord API client functionality for role-based access control.
package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

const (
	discordAPIBase = "https://discord.com/api/v10"
)

// Client Discord APIクライアント
type Client struct {
	httpClient *http.Client           // 8 bytes
	cache      map[string]*cacheEntry // 8 bytes
	botToken   string                 // 16 bytes
	guildID    string                 // 16 bytes
	cacheMutex sync.RWMutex           // 24 bytes
	cacheTTL   time.Duration          // 8 bytes
}

// cacheEntry キャッシュエントリ
type cacheEntry struct {
	expiresAt time.Time
	roles     []string
}

// GuildMember Discord APIレスポンス
type GuildMember struct {
	User  User     `json:"user"`
	Roles []string `json:"roles"`
}

// User ユーザー情報
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

// ErrorResponse Discord APIエラーレスポンス
type ErrorResponse struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// NewClient Discordクライアントを作成
func NewClient(botToken, guildID string) *Client {
	return &Client{
		botToken:   botToken,
		guildID:    guildID,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		cache:      make(map[string]*cacheEntry),
		cacheTTL:   5 * time.Minute, // 5分間キャッシュ
	}
}

// GetMemberRoles ユーザーのロールIDリストを取得
func (c *Client) GetMemberRoles(userID string) ([]string, error) {
	// キャッシュチェック
	c.cacheMutex.RLock()
	if entry, exists := c.cache[userID]; exists {
		if time.Now().Before(entry.expiresAt) {
			roles := make([]string, len(entry.roles))
			copy(roles, entry.roles)
			c.cacheMutex.RUnlock()
			return roles, nil
		}
	}
	c.cacheMutex.RUnlock()

	// Discord APIからロール取得
	url := fmt.Sprintf("%s/guilds/%s/members/%s", discordAPIBase, c.guildID, userID)

	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("リクエスト作成エラー: %w", err)
	}

	req.Header.Set("Authorization", "Bot "+c.botToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discord API呼び出しエラー: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			slog.Error("レスポンスボディのクローズに失敗", "error", closeErr)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("レスポンス読み取りエラー: %w", err)
	}

	// ステータスコードチェック
	switch resp.StatusCode {
	case http.StatusOK:
		// 正常処理
	case http.StatusNotFound:
		return nil, fmt.Errorf("ユーザーがギルドに存在しません (user_id: %s)", userID)
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("discord Bot認証エラー (status: %d)", resp.StatusCode)
	case http.StatusTooManyRequests:
		// レート制限
		var errResp ErrorResponse
		if err := json.Unmarshal(body, &errResp); err == nil {
			return nil, fmt.Errorf("discord APIレート制限: %s", errResp.Message)
		}
		return nil, fmt.Errorf("discord APIレート制限 (status: 429)")
	default:
		var errResp ErrorResponse
		if err := json.Unmarshal(body, &errResp); err == nil {
			return nil, fmt.Errorf("discord APIエラー (code: %d, message: %s)", errResp.Code, errResp.Message)
		}
		return nil, fmt.Errorf("discord APIエラー (status: %d)", resp.StatusCode)
	}

	// JSON解析
	var member GuildMember
	if err := json.Unmarshal(body, &member); err != nil {
		return nil, fmt.Errorf("JSONパースエラー: %w", err)
	}

	// キャッシュに保存
	c.cacheMutex.Lock()
	c.cache[userID] = &cacheEntry{
		roles:     member.Roles,
		expiresAt: time.Now().Add(c.cacheTTL),
	}
	c.cacheMutex.Unlock()

	return member.Roles, nil
}

// ClearCache キャッシュをクリア（テスト用）
func (c *Client) ClearCache() {
	c.cacheMutex.Lock()
	defer c.cacheMutex.Unlock()
	c.cache = make(map[string]*cacheEntry)
}

// SetCacheTTL キャッシュTTLを設定（テスト用）
func (c *Client) SetCacheTTL(ttl time.Duration) {
	c.cacheTTL = ttl
}
