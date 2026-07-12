// Package middleware はファイルサーバー用のHTTPミドルウェア関数を提供します。
package middleware

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"fileserver/internal/authprovider"
	"fileserver/internal/config"
	"fileserver/internal/models"
)

// Logger はアクセスログを出力するミドルウェアです。
// ステータスに応じてレベルを出し分け（5xx=Error / 4xx=Warn / それ以外=Info）、
// ヘルスチェックと静的アセットは平常時ノイズになるため Debug に落とします。
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(wrapped, r)

		level := statusLevel(wrapped.statusCode)
		// /health や /static は正常時は情報価値が低いためDebugへ（異常時は本来のレベルを維持）。
		if isNoisePath(r.URL.Path) && level < slog.LevelWarn {
			level = slog.LevelDebug
		}

		slog.LogAttrs(r.Context(), level, "HTTP request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", wrapped.statusCode),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			slog.Int("bytes", wrapped.written),
			slog.String("ip", r.RemoteAddr),
			slog.String("user_agent", r.UserAgent()),
		)
	})
}

// statusLevel はHTTPステータスから対応するログレベルを決めます。
func statusLevel(status int) slog.Level {
	switch {
	case status >= 500:
		return slog.LevelError
	case status >= 400:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

// isNoisePath はアクセスログを常時Infoで残す価値が低いパスかを返します。
func isNoisePath(path string) bool {
	return path == "/health" || strings.HasPrefix(path, "/static/")
}

// responseWriter はステータスコードと応答バイト数を追跡するレスポンスライターです。
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.written += n
	return n, err
}

// Flush はSSE等のストリーミング応答のため http.Flusher を透過させます。
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Recoverer はパニックを回復し、原因調査のためスタックトレース付きで記録します。
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.LogAttrs(r.Context(), slog.LevelError, "panic recovered",
					slog.Any("error", err),
					slog.String("stack", string(debug.Stack())),
				)
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
				if clientIP := clientIPFromXFF(r, trustedProxies); clientIP != "" {
					r.RemoteAddr = clientIP
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// clientIPFromXFF は X-Forwarded-For から実クライアントIPを求めます。
// 直前のホップ(RemoteAddr)が信頼済みプロキシの場合にのみXFFを参照し、
// 右端（最も手前のプロキシが付与した値）から信頼済みプロキシを剥がして、
// 最初に現れる非信頼IPをクライアントとみなします。左端を無条件に信頼すると、
// クライアントがXFFを注入して任意IPを詐称できてしまうため、それを防ぎます。
// 採用すべき値が無い場合は空文字を返します（呼び出し側はRemoteAddrを維持）。
func clientIPFromXFF(r *http.Request, trustedProxies []string) string {
	remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteIP = r.RemoteAddr
	}
	if !isTrustedProxy(remoteIP, trustedProxies) {
		return ""
	}

	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return ""
	}

	ips := strings.Split(xff, ",")
	for i := len(ips) - 1; i >= 0; i-- {
		ip := strings.TrimSpace(ips[i])
		if ip == "" {
			continue
		}
		if isTrustedProxy(ip, trustedProxies) {
			continue
		}
		return ip
	}
	return ""
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
			cookie, err := r.Cookie("session_token")
			if err != nil {
				slog.DebugContext(r.Context(), "認証Cookie未検出", "path", r.URL.Path, "error", err)
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}

			slog.DebugContext(r.Context(), "認証Cookie検出", "path", r.URL.Path, "token_prefix", tokenPrefix(cookie.Value))

			var session models.Session
			err = db.QueryRowContext(r.Context(), `
				SELECT session_token, user_id, expires_at
				FROM sessions
				WHERE session_token = ? AND expires_at > CURRENT_TIMESTAMP
			`, cookie.Value).Scan(&session.SessionToken, &session.UserID, &session.ExpiresAt)

			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					slog.DebugContext(r.Context(), "無効または期限切れセッション", "token_prefix", tokenPrefix(cookie.Value))
					http.Error(w, "invalid session", http.StatusUnauthorized)
					return
				}
				slog.ErrorContext(r.Context(), "session validation error", "error", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			slog.DebugContext(r.Context(), "セッション検証成功", "user_id", session.UserID)

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
					slog.WarnContext(r.Context(), "ユーザー情報が見つかりません", "user_id", session.UserID)
					http.Error(w, "user not found", http.StatusUnauthorized)
					return
				}
				slog.ErrorContext(r.Context(), "user info retrieval error", "error", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			slog.DebugContext(r.Context(), "ユーザー情報取得成功", "username", user.Username)

			// 在籍の継続確認（Discordはギルド在籍。5分キャッシュでレート制限を回避）。
			// エラー時は一時的障害の可能性があるため弾かず500とする。
			isMember, memberErr := provider.VerifyMembership(r.Context(), user.Subject)
			if memberErr != nil {
				slog.ErrorContext(r.Context(), "在籍確認エラー", "error", memberErr, "user_id", user.ID)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			if !isMember {
				slog.WarnContext(r.Context(), "在籍が確認できないためアクセスを拒否しました", "user_id", user.ID, "username", user.Username)
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

			ctx := context.WithValue(r.Context(), models.UserContextKey, &user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AdminMiddleware は管理者権限チェックミドルウェアです。
func AdminMiddleware(cfg *config.Config, provider authprovider.Provider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := r.Context().Value(models.UserContextKey).(*models.User)
			if !ok {
				slog.WarnContext(r.Context(), "管理者チェック: ユーザー情報が見つかりません")
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			// ロールはキャッシュではなくプロバイダーから都度取得し、降格を即時反映する。
			roles, err := provider.GetUserRoles(r.Context(), user.Subject)
			if err != nil {
				slog.ErrorContext(r.Context(), "管理者チェック: ロール情報取得エラー", "error", err, "user_id", user.ID)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			if !cfg.HasAdminRole(roles) {
				slog.WarnContext(r.Context(), "管理者権限がありません", "user_id", user.ID, "username", user.Username)
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			slog.DebugContext(r.Context(), "管理者権限確認成功", "user_id", user.ID)
			next.ServeHTTP(w, r)
		})
	}
}
