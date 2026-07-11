// Package handler は認証、ファイル操作、SSE用のHTTPハンドラーを提供します。
package handler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"fileserver/internal/authprovider"
	"fileserver/internal/config"
	"fileserver/internal/models"
	"fileserver/internal/storage"
)

// AuthHandler は認証関連のHTTPリクエストを処理します。
type AuthHandler struct {
	config         *config.Config
	db             *sql.DB
	provider       authprovider.Provider
	sseHandler     *SSEHandler
	storageManager *storage.Manager
}

// NewAuthHandler は新しいAuthHandlerインスタンスを作成します。
func NewAuthHandler(cfg *config.Config, db *sql.DB, provider authprovider.Provider, sm *storage.Manager) *AuthHandler {
	return &AuthHandler{
		config:         cfg,
		db:             db,
		provider:       provider,
		storageManager: sm,
	}
}

// SetSSEHandler はイベントのブロードキャスト用にSSEハンドラーを設定します。
func (h *AuthHandler) SetSSEHandler(sse *SSEHandler) {
	h.sseHandler = sse
}

// Login はOAuth2/OIDCログインの開始を処理します。
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	p := h.provider

	state := generateRandomString(32)

	// CSRF対策のstateをクッキーに保存し、コールバックで照合する
	// #nosec G124 - Secureは設定(SecureCookie)由来。HttpOnly/SameSiteも設定済み
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   300,
		HttpOnly: true,
		Secure:   h.config.Server.SecureCookie,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, p.AuthCodeURL(state), http.StatusTemporaryRedirect)
}

// Callback はプロバイダーからのOAuth2/OIDCコールバックを処理します。
func (h *AuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	p := h.provider
	providerName := p.Name()

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
	token, err := p.Exchange(r.Context(), code)
	if err != nil {
		slog.Error("トークン交換エラー", "error", err, "provider", providerName)
		http.Error(w, "認証に失敗しました", http.StatusInternalServerError)
		return
	}

	// ユーザー情報取得
	userInfo, err := p.FetchUserInfo(r.Context(), token)
	if err != nil {
		slog.Error("ユーザー情報取得エラー", "error", err, "provider", providerName)
		http.Error(w, "ユーザー情報の取得に失敗しました", http.StatusInternalServerError)
		return
	}

	// メンバーシップ確認（Discordのギルドメンバー確認、OIDCはallowlist判定等）
	isMember, err := p.IsMember(r.Context(), token, userInfo)
	if err != nil {
		slog.Error("メンバーシップ確認エラー", "error", err, "provider", providerName)
		http.Error(w, "メンバーシップの確認に失敗しました", http.StatusInternalServerError)
		return
	}
	if !isMember {
		http.Error(w, "このサービスへのアクセスが許可されていません", http.StatusForbidden)
		return
	}

	// 認証プロバイダーは1つに限定しているため、ユーザーIDはsubject単体で一意になる。
	userID := userInfo.Subject

	// ユーザー登録/更新
	if upsertErr := h.upsertUser(userID, userInfo); upsertErr != nil {
		slog.Error("ユーザー登録エラー", "error", upsertErr)
		http.Error(w, "ユーザー登録に失敗しました", http.StatusInternalServerError)
		return
	}

	// ユーザーディレクトリを事前作成（プロバイダーが事前作成を要求する場合のみ）。
	// OIDCなど誰でもログインできるプロバイダーでは事前作成せず、
	// 初回アップロード時に作成する（それまで自分のディレクトリは閲覧できない）。
	if h.storageManager != nil && p.PrecreateUserDirectory() {
		if ensureErr := h.storageManager.EnsureUserDirectory(userInfo.Username); ensureErr != nil {
			slog.Error("ユーザーディレクトリ作成エラー", "error", ensureErr, "username", userInfo.Username)
			// エラーが発生してもログインは継続（次回アップロード時に再試行される）
		} else {
			slog.Info("ユーザーディレクトリを作成しました", "username", userInfo.Username)
		}
	}

	// セッション作成
	sessionToken := generateRandomString(64)
	expiresAt := time.Now().Add(7 * 24 * time.Hour) // 7日間有効

	// プロバイダーのアクセストークン/リフレッシュトークンはログイン処理内でのみ使用し、
	// このアプリはログイン後に再利用しないためDBには保存しない。
	ctx := context.Background()
	_, err = h.db.ExecContext(ctx, `
		INSERT INTO sessions (session_token, user_id, provider, expires_at)
		VALUES (?, ?, ?, ?)
	`, sessionToken, userID, userInfo.Provider, expiresAt)
	if err != nil {
		slog.Error("セッション作成エラー", "error", err)
		http.Error(w, "セッション作成に失敗しました", http.StatusInternalServerError)
		return
	}

	// セッションクッキー設定
	// #nosec G124 - Secureは設定(SecureCookie)由来。HttpOnly/SameSiteも設定済み
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    sessionToken,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   h.config.Server.SecureCookie, // HTTPS環境でのみ有効化
		SameSite: http.SameSiteLaxMode,         // Strictは外部IdPからのリダイレクトでクッキーが送出されない
	})

	slog.Info("ユーザーがログインしました", "user_id", userID, "username", userInfo.Username, "provider", providerName)

	// SSEでブロードキャスト
	if h.sseHandler != nil {
		user := &models.User{
			ID:       userID,
			Provider: userInfo.Provider,
			Subject:  userInfo.Subject,
			Username: userInfo.Username,
		}
		h.sseHandler.BroadcastUserLogin(user)
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Logout はセッションを無効化してユーザーのログアウトを処理します。
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session_token")
	if err == nil {
		// セッション削除
		ctx := context.Background()
		if _, err := h.db.ExecContext(ctx, "DELETE FROM sessions WHERE session_token = ?", cookie.Value); err != nil {
			slog.Error("セッション削除に失敗しました", "error", err)
		}
	}

	// クッキー削除
	// #nosec G124 - Secureは設定(SecureCookie)由来。HttpOnly/SameSiteも設定済み
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

// GetCurrentUser は現在認証されているユーザー情報を返します。
func (h *AuthHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(w, r)
	if !ok {
		return
	}

	writeJSON(w, http.StatusOK, user)
}

func (h *AuthHandler) upsertUser(userID string, info *authprovider.UserInfo) error {
	ctx := context.Background()
	_, err := h.db.ExecContext(ctx, `
		INSERT INTO users (id, provider, subject, username, avatar, email, last_login)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			username = excluded.username,
			avatar = excluded.avatar,
			email = excluded.email,
			last_login = CURRENT_TIMESTAMP
	`, userID, info.Provider, info.Subject, info.Username, info.Avatar, info.Email)
	return err
}

func generateRandomString(length int) string {
	// 必要な文字数を確実に満たすだけのランダムバイトを生成する。
	// base64は3バイト→4文字に符号化するため、length分の文字を得るには
	// ceil(length*3/4) バイトあれば足りるが、余裕を持って length バイト読む。
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		// フォールバック: タイムスタンプベースの文字列生成。
		// 短い入力を[:length]で切り出すとパニックするため、必要長まで繰り返して埋める。
		slog.Error("暗号乱数の生成に失敗しました。フォールバックを使用します", "error", err)
		seed := base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
		for len(seed) < length {
			seed += seed
		}
		return seed[:length]
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length]
}
