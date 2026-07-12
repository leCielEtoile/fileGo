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

	state, err := generateRandomString(32)
	if err != nil {
		slog.ErrorContext(r.Context(), "state生成エラー", "error", err)
		http.Error(w, "内部エラーが発生しました", http.StatusInternalServerError)
		return
	}

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

	// stateはLoginで発行しCookieに保存した値と一致すること（CSRF対策）。
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

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "認証コードが見つかりません", http.StatusBadRequest)
		return
	}

	token, err := p.Exchange(r.Context(), code)
	if err != nil {
		slog.ErrorContext(r.Context(), "トークン交換エラー", "error", err, "provider", providerName)
		http.Error(w, "認証に失敗しました", http.StatusInternalServerError)
		return
	}

	userInfo, err := p.FetchUserInfo(r.Context(), token)
	if err != nil {
		slog.ErrorContext(r.Context(), "ユーザー情報取得エラー", "error", err, "provider", providerName)
		http.Error(w, "ユーザー情報の取得に失敗しました", http.StatusInternalServerError)
		return
	}

	// メンバーシップ確認（Discordのギルドメンバー確認、OIDCはallowlist判定等）
	isMember, err := p.IsMember(r.Context(), token, userInfo)
	if err != nil {
		slog.ErrorContext(r.Context(), "メンバーシップ確認エラー", "error", err, "provider", providerName)
		http.Error(w, "メンバーシップの確認に失敗しました", http.StatusInternalServerError)
		return
	}
	if !isMember {
		http.Error(w, "このサービスへのアクセスが許可されていません", http.StatusForbidden)
		return
	}

	// 認証プロバイダーは1つに限定しているため、ユーザーIDはsubject単体で一意になる。
	userID := userInfo.Subject

	if upsertErr := h.upsertUser(userID, userInfo); upsertErr != nil {
		slog.ErrorContext(r.Context(), "ユーザー登録エラー", "error", upsertErr)
		http.Error(w, "ユーザー登録に失敗しました", http.StatusInternalServerError)
		return
	}

	// ユーザーディレクトリを事前作成（プロバイダーが事前作成を要求する場合のみ）。
	// OIDCなど誰でもログインできるプロバイダーでは事前作成せず、
	// 初回アップロード時に作成する（それまで自分のディレクトリは閲覧できない）。
	if h.storageManager != nil && p.PrecreateUserDirectory() {
		dirName := models.SanitizeDirName(userInfo.Username)
		if ensureErr := h.storageManager.EnsureUserDirectory(dirName); ensureErr != nil {
			slog.ErrorContext(r.Context(), "ユーザーディレクトリ作成エラー", "error", ensureErr, "username", userInfo.Username)
			// エラーが発生してもログインは継続（次回アップロード時に再試行される）
		} else {
			slog.InfoContext(r.Context(), "ユーザーディレクトリを作成しました", "username", userInfo.Username)
		}
	}

	sessionToken, err := generateRandomString(64)
	if err != nil {
		slog.ErrorContext(r.Context(), "セッショントークン生成エラー", "error", err)
		http.Error(w, "内部エラーが発生しました", http.StatusInternalServerError)
		return
	}
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	// プロバイダーのアクセストークン/リフレッシュトークンはログイン後に再利用しないため
	// DBには保存しない（漏洩面を減らす）。
	ctx := context.Background()
	_, err = h.db.ExecContext(ctx, `
		INSERT INTO sessions (session_token, user_id, provider, expires_at)
		VALUES (?, ?, ?, ?)
	`, sessionToken, userID, userInfo.Provider, expiresAt)
	if err != nil {
		slog.ErrorContext(r.Context(), "セッション作成エラー", "error", err)
		http.Error(w, "セッション作成に失敗しました", http.StatusInternalServerError)
		return
	}

	// #nosec G124 - Secureは設定(SecureCookie)由来。HttpOnly/SameSiteも設定済み
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    sessionToken,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   h.config.Server.SecureCookie,
		SameSite: http.SameSiteLaxMode, // Strictだと外部IdPからのリダイレクトでCookieが送出されない
	})

	slog.InfoContext(r.Context(), "ユーザーがログインしました", "user_id", userID, "username", userInfo.Username, "provider", providerName)

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
		ctx := context.Background()
		if _, err := h.db.ExecContext(ctx, "DELETE FROM sessions WHERE session_token = ?", cookie.Value); err != nil {
			slog.ErrorContext(r.Context(), "セッション削除に失敗しました", "error", err)
		}
	}

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

// currentUserResponse は /api/user の応答です。
// models.User の各フィールドに加え、フロントが管理者用UI（adminリンク等）を
// 出し分けられるよう is_admin を含めます。
type currentUserResponse struct {
	*models.User
	IsAdmin bool `json:"is_admin"`
}

// GetCurrentUser は現在認証されているユーザー情報を返します。
func (h *AuthHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(w, r)
	if !ok {
		return
	}

	// 管理者判定はAdminMiddlewareと同じくロール取得＋設定照合で行う。
	// ロール取得失敗時は権限を広げないよう非管理者として扱う。
	isAdmin := false
	if roles, err := h.provider.GetUserRoles(r.Context(), user.Subject); err != nil {
		slog.WarnContext(r.Context(), "管理者判定のためのロール取得に失敗しました（非管理者として扱います）", "error", err, "user_id", user.ID)
	} else {
		isAdmin = h.config.HasAdminRole(roles)
	}

	writeJSON(w, http.StatusOK, currentUserResponse{User: user, IsAdmin: isAdmin})
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

// generateRandomString は length 文字の暗号学的乱数トークンを生成します。
// 予測可能な値をセッショントークン/stateに使わないため、乱数取得に失敗した場合は
// フォールバックせずエラーを返します（フェイルクローズ）。
func generateRandomString(length int) (string, error) {
	// base64符号化はバイト数より長い文字列になるため、length バイト読めば
	// 末尾を[:length]で切り出しても足りる。
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("暗号乱数の生成に失敗しました: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length], nil
}
