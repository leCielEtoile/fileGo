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
	"fileserver/internal/models"
)

// contextKey is a custom type for context keys to avoid staticcheck issues
type contextKey string

const userContextKey contextKey = "user"

// Logger logging middleware
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

// responseWriter custom response writer for status code tracking
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Recoverer panic recovery middleware
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

// RealIP middleware to get real IP address
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

// isTrustedProxy check if IP is trusted proxy
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

// AuthMiddleware authentication middleware
func AuthMiddleware(cfg *config.Config, db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get session token
			cookie, err := r.Cookie("session_token")
			if err != nil {
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}

			// Validate session
			var session models.Session
			err = db.QueryRow(`
				SELECT session_token, user_id, expires_at
				FROM sessions
				WHERE session_token = ? AND expires_at > CURRENT_TIMESTAMP
			`, cookie.Value).Scan(&session.SessionToken, &session.UserID, &session.ExpiresAt)

			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					http.Error(w, "invalid session", http.StatusUnauthorized)
					return
				}
				slog.Error("session validation error", "error", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			// Get user information
			var user models.User
			err = db.QueryRow(`
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
					http.Error(w, "user not found", http.StatusUnauthorized)
					return
				}
				slog.Error("user info retrieval error", "error", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			// Add user info to context
			ctx := context.WithValue(r.Context(), userContextKey, &user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
