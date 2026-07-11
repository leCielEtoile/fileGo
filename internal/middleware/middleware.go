// Package middleware はファイルサーバー用のHTTPミドルウェア関数を提供します。
package middleware

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"fileserver/internal/authprovider"
	"fileserver/internal/config"
	"fileserver/internal/models"
)

// Logger はロギングミドルウェアです。
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Custom response writer
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)

		slog.Info("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.statusCode,
			"duration_ms", duration.Milliseconds(),
			"ip", r.RemoteAddr,
		)
	})
}

// responseWriter はステータスコード追跡のためのカスタムレスポンスライターです。
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Recoverer はパニック回復ミドルウェアです。
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("panic occurred", "error", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// RealIP は実際のIPアドレスを取得するミドルウェアです。
func RealIP(behindProxy bool, trustedProxies []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if behindProxy {
				// Get real IP from X-Forwarded-For header
				xForwardedFor := r.Header.Get("X-Forwarded-For")
				if xForwardedFor != "" {
					ips := strings.Split(xForwardedFor, ",")
					if len(ips) > 0 {
						// Check if request is from trusted proxy
						remoteIP, _, _ := net.SplitHostPort(r.RemoteAddr) // nolint:errcheck
						if isTrustedProxy(remoteIP, trustedProxies) {
							r.RemoteAddr = strings.TrimSpace(ips[0])
						}
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// tokenPrefix はセッショントークンの先頭一部だけをログ用に安全に取り出します。
// トークンが短い場合でもスライス範囲外パニックを起こさないようにします。
func tokenPrefix(token string) string {
	const n = 10
	if len(token) <= n {
		return "..."
	}
	return token[:n] + "..."
}

// isTrustedProxy はIPが信頼されたプロキシかどうかをチェックします。
func isTrustedProxy(ip string, trustedProxies []string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	for _, cidr := range trustedProxies {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(parsedIP) {
			return true
		}
	}

	return false
}

// AuthMiddleware は認証ミドルウェアです。
// セッション検証に加え、プロバイダーの在籍を継続的に確認します
// （Discordは5分キャッシュ、退出者はここで弾かれます）。
func AuthMiddleware(cfg *config.Config, db *sql.DB, provider authprovider.Provider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get session token
			cookie, err := r.Cookie("session_token")
			if err != nil {
				slog.Debug("認証Cookie未検出", "path", r.URL.Path, "error", err)
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}

			slog.Debug("認証Cookie検出", "path", r.URL.Path, "token_prefix", tokenPrefix(cookie.Value))

			// Validate session
			var session models.Session
			err = db.QueryRowContext(r.Context(), `
				SELECT session_token, user_id, expires_at
				FROM sessions
				WHERE session_token = ? AND expires_at > CURRENT_TIMESTAMP
			`, cookie.Value).Scan(&session.SessionToken, &session.UserID, &session.ExpiresAt)

			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					slog.Debug("無効または期限切れセッション", "token_prefix", tokenPrefix(cookie.Value))
					http.Error(w, "invalid session", http.StatusUnauthorized)
					return
				}
				slog.Error("session validation error", "error", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			slog.Debug("セッション検証成功", "user_id", session.UserID)

			// Get user information
			var user models.User
			err = db.QueryRowContext(r.Context(), `
				SELECT id, provider, subject, username, avatar, created_at, last_login
				FROM users
				WHERE id = ?
			`, session.UserID).Scan(
				&user.ID,
				&user.Provider,
				&user.Subject,
				&user.Username,
				&user.Avatar,
				&user.CreatedAt,
				&user.LastLogin,
			)

			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					slog.Warn("ユーザー情報が見つかりません", "user_id", session.UserID)
					http.Error(w, "user not found", http.StatusUnauthorized)
					return
				}
				slog.Error("user info retrieval error", "error", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			slog.Debug("ユーザー情報取得成功", "username", user.Username)

			// 在籍の継続確認（Discordはギルド在籍。5分キャッシュでレート制限を回避）。
			// エラー時は一時的障害の可能性があるため弾かず500とする。
			isMember, memberErr := provider.VerifyMembership(r.Context(), user.Subject)
			if memberErr != nil {
				slog.Error("在籍確認エラー", "error", memberErr, "user_id", user.ID)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			if !isMember {
				slog.Warn("在籍が確認できないためアクセスを拒否しました", "user_id", user.ID, "username", user.Username)
				// 認証Cookieを失効させ、再ログインを促す（属性はログイン時と揃える）
				// #nosec G124 - Secureは設定(SecureCookie)由来。HttpOnly/SameSiteも設定済み
				http.SetCookie(w, &http.Cookie{
					Name:     "session_token",
					Value:    "",
					Path:     "/",
					MaxAge:   -1,
					HttpOnly: true,
					Secure:   cfg.Server.SecureCookie,
					SameSite: http.SameSiteLaxMode,
				})
				http.Error(w, "membership required", http.StatusForbidden)
				return
			}

			// Add user info to context
			ctx := context.WithValue(r.Context(), models.UserContextKey, &user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AdminMiddleware は管理者権限チェックミドルウェアです。
func AdminMiddleware(cfg *config.Config, provider authprovider.Provider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// コンテキストからユーザー情報を取得
			user, ok := r.Context().Value(models.UserContextKey).(*models.User)
			if !ok {
				slog.Warn("管理者チェック: ユーザー情報が見つかりません")
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			// プロバイダーから最新のロール情報を取得
			roles, err := provider.GetUserRoles(r.Context(), user.Subject)
			if err != nil {
				slog.Error("管理者チェック: ロール情報取得エラー", "error", err, "user_id", user.ID)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			// 管理者ロールを持っているかチェック
			if !cfg.HasAdminRole(roles) {
				slog.Warn("管理者権限がありません", "user_id", user.ID, "username", user.Username)
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			slog.Debug("管理者権限確認成功", "user_id", user.ID)
			next.ServeHTTP(w, r)
		})
	}
}
