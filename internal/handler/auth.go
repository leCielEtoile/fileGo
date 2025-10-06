package handler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"fileserver/internal/config"
	"fileserver/internal/models"

	"golang.org/x/oauth2"
)

type AuthHandler struct {
	config      *config.Config
	db          *sql.DB
	oauthConfig *oauth2.Config
	sseHandler  *SSEHandler
}

func NewAuthHandler(cfg *config.Config, db *sql.DB) *AuthHandler {
	oauthConfig := &oauth2.Config{
		ClientID:     cfg.Discord.ClientID,
		ClientSecret: cfg.Discord.ClientSecret,
		RedirectURL:  cfg.Discord.RedirectURL,
		Scopes:       []string{"identify", "guilds.members.read"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://discord.com/api/oauth2/authorize",
			TokenURL: "https://discord.com/api/oauth2/token",
		},
	}

	return &AuthHandler{
		config:      cfg,
		db:          db,
		oauthConfig: oauthConfig,
	}
}

func (h *AuthHandler) SetSSEHandler(sse *SSEHandler) {
	h.sseHandler = sse
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	state := generateRandomString(32)

	// stateをセッションに保存（簡易実装：クッキー使用）
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   300,
		HttpOnly: true,
		Secure:   h.config.Server.SecureCookie,
		SameSite: http.SameSiteLaxMode,
	})

	url := h.oauthConfig.AuthCodeURL(state)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func (h *AuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	// state検証
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil {
		http.Error(w, "無効なリクエストです", http.StatusBadRequest)
		return
	}

	state := r.URL.Query().Get("state")
	if state != stateCookie.Value {
		http.Error(w, "無効なstateパラメータです", http.StatusBadRequest)
		return
	}

	// 認証コード取得
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "認証コードが見つかりません", http.StatusBadRequest)
		return
	}

	// トークン交換
	token, err := h.oauthConfig.Exchange(r.Context(), code)
	if err != nil {
		slog.Error("トークン交換エラー", "error", err)
		http.Error(w, "認証に失敗しました", http.StatusInternalServerError)
		return
	}

	// Discordユーザー情報取得
	discordUser, err := h.getDiscordUser(token.AccessToken)
	if err != nil {
		slog.Error("Discordユーザー情報取得エラー", "error", err)
		http.Error(w, "ユーザー情報の取得に失敗しました", http.StatusInternalServerError)
		return
	}

	// ギルドメンバー確認
	isMember, err := h.checkGuildMembership(token.AccessToken, discordUser.ID)
	if err != nil {
		slog.Error("ギルドメンバーシップ確認エラー", "error", err)
		http.Error(w, "サーバーメンバーの確認に失敗しました", http.StatusInternalServerError)
		return
	}

	if !isMember {
		http.Error(w, "指定されたDiscordサーバーのメンバーではありません", http.StatusForbidden)
		return
	}

	// ユーザー登録/更新
	if upsertErr := h.upsertUser(discordUser); upsertErr != nil {
		slog.Error("ユーザー登録エラー", "error", upsertErr)
		http.Error(w, "ユーザー登録に失敗しました", http.StatusInternalServerError)
		return
	}

	// セッション作成
	sessionToken := generateRandomString(64)
	expiresAt := time.Now().Add(7 * 24 * time.Hour) // 7日間有効

	refreshToken := ""
	if token.RefreshToken != "" {
		refreshToken = token.RefreshToken
	}

	_, err = h.db.Exec(`
		INSERT INTO sessions (session_token, user_id, discord_access_token, discord_refresh_token, expires_at)
		VALUES (?, ?, ?, ?, ?)
	`, sessionToken, discordUser.ID, token.AccessToken, refreshToken, expiresAt)
	if err != nil {
		slog.Error("セッション作成エラー", "error", err)
		http.Error(w, "セッション作成に失敗しました", http.StatusInternalServerError)
		return
	}

	// セッションクッキー設定
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    sessionToken,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   h.config.Server.SecureCookie, // HTTPSの場合のみtrue
		SameSite: http.SameSiteLaxMode,         // StrictだとOAuth redirectで問題が起きる可能性
	})

	slog.Info("ユーザーがログインしました", "user_id", discordUser.ID, "username", discordUser.Username)

	// SSEでブロードキャスト
	if h.sseHandler != nil {
		user := &models.User{
			DiscordID: discordUser.ID,
			Username:  discordUser.Username,
		}
		h.sseHandler.BroadcastUserLogin(user)
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session_token")
	if err == nil {
		// セッション削除
		if _, err := h.db.Exec("DELETE FROM sessions WHERE session_token = ?", cookie.Value); err != nil {
			slog.Error("セッション削除に失敗しました", "error", err)
		}
	}

	// クッキー削除
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.config.Server.SecureCookie,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *AuthHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(userContextKey).(*models.User)
	if !ok {
		http.Error(w, "ユーザー情報の取得に失敗しました", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(user); err != nil {
		slog.Error("JSONエンコードに失敗しました", "error", err)
		http.Error(w, "レスポンスの生成に失敗しました", http.StatusInternalServerError)
	}
}

func (h *AuthHandler) getDiscordUser(accessToken string) (*models.DiscordUser, error) {
	req, err := http.NewRequestWithContext(context.Background(), "GET", "https://discord.com/api/users/@me", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Discord APIエラー: %d", resp.StatusCode)
	}

	var user models.DiscordUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}

	return &user, nil
}

func (h *AuthHandler) checkGuildMembership(accessToken, _ string) (bool, error) {
	url := fmt.Sprintf("https://discord.com/api/users/@me/guilds/%s/member", h.config.Discord.GuildID)
	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return false, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

func (h *AuthHandler) upsertUser(discordUser *models.DiscordUser) error {
	_, err := h.db.Exec(`
		INSERT INTO users (discord_id, username, discriminator, avatar, last_login)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(discord_id) DO UPDATE SET
			username = excluded.username,
			discriminator = excluded.discriminator,
			avatar = excluded.avatar,
			last_login = CURRENT_TIMESTAMP
	`, discordUser.ID, discordUser.Username, discordUser.Discriminator, discordUser.Avatar)
	return err
}

func generateRandomString(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		// フォールバック: タイムスタンプベースの文字列生成
		return base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))[:length]
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length]
}
