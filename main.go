package main

import (
	"context"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fileserver/internal/config"
	"fileserver/internal/database"
	"fileserver/internal/discord"
	"fileserver/internal/handler"
	"fileserver/internal/middleware"
	"fileserver/internal/permission"
	"fileserver/internal/storage"

	"github.com/go-chi/chi/v5"
)

func main() {
	// ロガー初期化
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// 設定読み込み
	cfg, err := config.Load("/root/config/config.yaml")
	if err != nil {
		slog.Error("設定ファイルの読み込みに失敗しました", "error", err)
		os.Exit(1)
	}

	// データベース初期化
	db, err := database.Initialize(cfg.Database.Path)
	if err != nil {
		slog.Error("データベースの初期化に失敗しました", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("データベースのクローズに失敗しました", "error", err)
		}
	}()

	// ストレージ初期化（ディレクトリ作成）
	storageManager := storage.NewManager(cfg)
	if err := storageManager.InitializeDirectories(); err != nil {
		slog.Error("ストレージディレクトリの初期化に失敗しました", "error", err)
		os.Exit(1)
	}

	// アップロードマネージャー初期化
	uploadManager := storage.NewUploadManager(cfg)

	// Discordクライアント初期化
	discordClient := discord.NewClient(cfg.Discord.BotToken, cfg.Discord.GuildID)

	// 権限チェッカー初期化
	permissionChecker := permission.NewChecker(cfg, discordClient, db)

	// SSEハンドラー初期化
	sseHandler := handler.NewSSEHandler()

	// ハンドラー初期化
	authHandler := handler.NewAuthHandler(cfg, db)
	fileHandler := handler.NewFileHandler(cfg, storageManager, uploadManager, permissionChecker)
	chunkHandler := handler.NewChunkHandler(storageManager, uploadManager, permissionChecker)

	// SSEハンドラーをファイルハンドラーに注入
	fileHandler.SetSSEHandler(sseHandler)
	authHandler.SetSSEHandler(sseHandler)

	// ルーター設定
	r := chi.NewRouter()

	// ミドルウェア
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP(cfg.Server.BehindProxy, cfg.Server.TrustedProxies))

	// 静的ファイル
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

	// ルートパス（Webインターフェース）
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.ParseFiles("web/templates/index.html")
		if err != nil {
			slog.Error("テンプレートの読み込みに失敗しました", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		data := map[string]interface{}{
			"ServiceName": cfg.Server.ServiceName,
		}

		if err := tmpl.Execute(w, data); err != nil {
			slog.Error("テンプレートのレンダリングに失敗しました", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// ヘルスチェック
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("OK")); err != nil {
			slog.Error("ヘルスチェックレスポンスの書き込みに失敗しました", "error", err)
		}
	})

	// 認証エンドポイント
	r.Get("/auth/login", authHandler.Login)
	r.Get("/auth/callback", authHandler.Callback)
	r.Get("/auth/logout", authHandler.Logout)

	// 認証が必要なエンドポイント
	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddleware(cfg, db))

		// ユーザー情報
		r.Get("/api/user", authHandler.GetCurrentUser)

		// Server-Sent Events
		r.Get("/api/events", sseHandler.HandleSSE)

		// ファイル操作
		r.Post("/files/upload", fileHandler.Upload)
		r.Get("/files", fileHandler.ListFiles)
		r.Get("/files/directories", fileHandler.ListDirectories)
		r.Get("/files/download/{directory}/{filename}", fileHandler.Download)
		r.Delete("/files/{directory}/{filename}", fileHandler.DeleteFile)

		// チャンクアップロード
		r.Post("/files/chunk/init", chunkHandler.InitChunkUpload)
		r.Post("/files/chunk/upload/{upload_id}", chunkHandler.UploadChunk)
		r.Get("/files/chunk/status/{upload_id}", chunkHandler.GetChunkStatus)
		r.Post("/files/chunk/complete/{upload_id}", chunkHandler.CompleteChunkUpload)
		r.Delete("/files/chunk/cancel/{upload_id}", chunkHandler.CancelChunkUpload)
	})

	// HTTPサーバー起動
	addr := ":" + cfg.Server.Port
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// グレースフルシャットダウン
	go func() {
		slog.Info("サーバーを起動しました", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("サーバーエラー", "error", err)
			os.Exit(1)
		}
	}()

	// シグナル待機
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("サーバーをシャットダウンしています...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("シャットダウンエラー", "error", err)
	}

	slog.Info("サーバーを停止しました")
}
