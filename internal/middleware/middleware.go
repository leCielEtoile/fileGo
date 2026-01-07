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
	"sync"
	"time"

	"fileserver/internal/config"
	"fileserver/internal/models"

	"golang.org/x/time/rate"
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

// visitor はIP単位のレート制限を管理します。
type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter はレート制限を管理する構造体です。
type RateLimiter struct {
	visitors map[string]*visitor
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
}

// NewRateLimiter は新しいRateLimiterを作成します。
// rps: 秒あたりのリクエスト数、burst: バースト許可数
func NewRateLimiter(rps float64, burst int) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate.Limit(rps),
		burst:    burst,
	}

	// 古いエントリのクリーンアップ（5分ごと）
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			rl.cleanupVisitors()
		}
	}()

	return rl
}

// getVisitor はIPアドレスのvisitorを取得または作成します。
func (rl *RateLimiter) getVisitor(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[ip]
	if !exists {
		limiter := rate.NewLimiter(rl.rate, rl.burst)
		rl.visitors[ip] = &visitor{limiter: limiter, lastSeen: time.Now()}
		return limiter
	}

	v.lastSeen = time.Now()
	return v.limiter
}

// cleanupVisitors は3分以上アクセスのないvisitorを削除します。
func (rl *RateLimiter) cleanupVisitors() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	for ip, v := range rl.visitors {
		if time.Since(v.lastSeen) > 3*time.Minute {
			delete(rl.visitors, ip)
		}
	}
}

// Middleware はレート制限ミドルウェアを返します。
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		limiter := rl.getVisitor(ip)

		if !limiter.Allow() {
			slog.Warn("レート制限超過", "ip", ip, "path", r.URL.Path)
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// SecurityHeaders はセキュリティヘッダーを設定するミドルウェアです。
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// XSS対策
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Content Security Policy
		csp := "default-src 'self'; " +
			"script-src 'self' 'unsafe-inline'; " +
			"style-src 'self' 'unsafe-inline'; " +
			"img-src 'self' data: https:; " +
			"font-src 'self'; " +
			"connect-src 'self'; " +
			"frame-ancestors 'none'"
		w.Header().Set("Content-Security-Policy", csp)

		// HTTPS強制（プロキシ経由の場合）
		if r.Header.Get("X-Forwarded-Proto") == "https" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
		}

		// その他のセキュリティヘッダー
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		next.ServeHTTP(w, r)
	})
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
