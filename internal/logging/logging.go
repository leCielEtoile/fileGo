// Package logging はアプリ共通のロギング設定を提供します。
// JSON構造化ログ・レベルのenv制御・リクエストID自動付与を担います。
package logging

import (
	"context"
	"log/slog"
	"strings"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// ParseLevel はレベル文字列を slog.Level に変換します。未知の値は Info を返します。
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// ContextHandler はリクエストコンテキストの request_id を全ログ行へ自動付与する
// slog.Handler ラッパーです。ハンドラ内で *Context 版のログ関数
// （InfoContext 等）に r.Context() を渡すと、アクセスログと同じ request_id で
// 突き合わせられるようになります。
type ContextHandler struct {
	slog.Handler
}

// NewContextHandler は基盤ハンドラをラップした ContextHandler を返します。
func NewContextHandler(base slog.Handler) *ContextHandler {
	return &ContextHandler{Handler: base}
}

// Handle は request_id が取得できればログレコードへ付与してから委譲します。
func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := chimw.GetReqID(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs はラップを維持したまま属性を追加したハンドラを返します。
func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

// WithGroup はラップを維持したままグループを追加したハンドラを返します。
func (h *ContextHandler) WithGroup(name string) slog.Handler {
	return &ContextHandler{Handler: h.Handler.WithGroup(name)}
}
