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

	"fileserver/internal/config"
	"fileserver/internal/discord"
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
func AuthMiddleware(cfg *config.Config, db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get session token
			cookie, err := r.Cookie("session_token")
			if err != nil {
				slog.Debug("認証Cookie未検出", "path", r.URL.Path, "error", err)
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}

			slog.Debug("認証Cookie検出", "path", r.URL.Path, "token_prefix", cookie.Value[:10]+"...")

			// Validate session
			var session models.Session
			err = db.QueryRowContext(r.Context(), `
				SELECT session_token, user_id, expires_at
				FROM sessions
				WHERE session_token = ? AND expires_at > CURRENT_TIMESTAMP
			`, cookie.Value).Scan(&session.SessionToken, &session.UserID, &session.ExpiresAt)

			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					slog.Debug("無効または期限切れセッション", "token_prefix", cookie.Value[:10]+"...")
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
				SELECT discord_id, username, discriminator, avatar, created_at, last_login
				FROM users
				WHERE discord_id = ?
			`, session.UserID).Scan(
				&user.DiscordID,
				&user.Username,
				&user.Discriminator,
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

			// Add user info to context
			ctx := context.WithValue(r.Context(), models.UserContextKey, &user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AdminMiddleware は管理者権限チェックミドルウェアです。
func AdminMiddleware(cfg *config.Config, discordClient *discord.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// コンテキストからユーザー情報を取得
			user, ok := r.Context().Value(models.UserContextKey).(*models.User)
			if !ok {
				slog.Warn("管理者チェック: ユーザー情報が見つかりません")
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			// Discordから最新のロール情報を取得
			member, err := discordClient.GetGuildMember(cfg.Discord.GuildID, user.DiscordID)
			if err != nil {
				slog.Error("管理者チェック: Discordメンバー情報取得エラー", "error", err, "user_id", user.DiscordID)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			// 管理者ロールを持っているかチェック
			isAdmin := false
			for _, roleID := range member.Roles {
				if roleID == cfg.Storage.AdminRoleID {
					isAdmin = true
					break
				}
			}

			if !isAdmin {
				slog.Warn("管理者権限がありません", "user_id", user.DiscordID, "username", user.Username)
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			slog.Debug("管理者権限確認成功", "user_id", user.DiscordID)
			next.ServeHTTP(w, r)
		})
	}
}
