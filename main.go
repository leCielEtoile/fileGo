package main

import (
	"context"
	"flag"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	// time/tzdata をブランクインポートし、タイムゾーンDBをバイナリへ埋め込む。
	// これにより zoneinfo を持たない distroless / DHI static 上でも TZ が解決できる。
	_ "time/tzdata"

	"fileserver/internal/authprovider"
	"fileserver/internal/config"
	"fileserver/internal/database"
	"fileserver/internal/handler"
	"fileserver/internal/middleware"
	"fileserver/internal/permission"
	"fileserver/internal/rolestore"
	"fileserver/internal/storage"

	"github.com/go-chi/chi/v5"
)

// ビルド時に -ldflags "-X main.Version=..." 等で注入されるビルド情報。
// 未注入の場合（go run / go build 単体）は既定値のままとなる。
var (
	Version   = "dev"
	BuildDate = "unknown"
	GitCommit = "unknown"
)

// isMobile はUser-Agentヘッダーからモバイルデバイスを判定します
func isMobile(userAgent string) bool {
	ua := strings.ToLower(userAgent)
	mobileKeywords := []string{
		"mobile", "android", "iphone", "ipad", "ipod",
		"blackberry", "windows phone", "webos",
	}
	for _, keyword := range mobileKeywords {
		if strings.Contains(ua, keyword) {
			return true
		}
	}
	return false
}

// loadTemplate は埋め込みFSからテンプレートをパースします。
// 失敗した場合は起動を継続できないため、ログを出力してプロセスを終了します。
func loadTemplate(name string) *template.Template {
	tmpl, err := template.ParseFS(webFS, name)
	if err != nil {
		slog.Error("テンプレートの読み込みに失敗しました", "error", err, "template", name)
		os.Exit(1)
	}
	return tmpl
}

// runHealthcheck は /health エンドポイントへ問い合わせ、その結果を終了コードで返します。
// コンテナのHEALTHCHECKから `fileserver -healthcheck` として呼び出す用途で、
// シェルやwgetを持たない最小イメージ（distroless / DHI static）でも動作します。
func runHealthcheck() int {
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// #nosec G704 - 固定の 127.0.0.1 に対するローカルヘルスチェックであり外部入力ではない
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+port+"/health", nil)
	if err != nil {
		return 1
	}
	// #nosec G704 - 固定の 127.0.0.1 に対するローカルヘルスチェックであり外部入力ではない
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 1
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // ヘルスチェックのレスポンス、close失敗は無視してよい
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func main() {
	// コンテナのHEALTHCHECK用モード。サーバーを起動せず疎通確認のみ行う。
	healthcheck := flag.Bool("healthcheck", false, "ヘルスチェックを実行して終了する（コンテナHEALTHCHECK用）")
	flag.Parse()
	if *healthcheck {
		os.Exit(runHealthcheck())
	}

	// ロガー初期化
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("fileserver を起動します", "version", Version, "build_date", BuildDate, "git_commit", GitCommit)

	// 設定読み込み
	// CONFIG_PATH環境変数が設定されていればそれを使用し、
	// 未設定の場合は実行ファイルと同じディレクトリのconfig.yamlを既定値とする。
	configPath := config.ResolvePath()
	slog.Info("設定ファイルを読み込みます", "path", configPath)
	cfg, err := config.Load(configPath, defaultConfigYAML)
	if err != nil {
		slog.Error("設定ファイルの読み込みに失敗しました", "error", err)
		os.Exit(1)
	}

	// 初回起動でひな型を生成した直後などは認証情報が未設定のため、編集を促す。
	if strings.Contains(cfg.Auth.Provider.BotToken, "YOUR_") ||
		strings.Contains(cfg.Auth.Provider.ClientID, "YOUR_") ||
		strings.Contains(cfg.Auth.Provider.ClientSecret, "YOUR_") {
		slog.Warn("認証情報がプレースホルダのままです。config.yaml の auth.provider を編集してください", "path", configPath)
	}

	// データベース初期化
	db, err := database.Initialize(cfg.Database.Path, cfg.Database.MaxConnections)
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
	storageManager := storage.NewManager(cfg, db)
	if err := storageManager.InitializeDirectories(); err != nil {
		slog.Error("ストレージディレクトリの初期化に失敗しました", "error", err)
		os.Exit(1)
	}

	// アップロードマネージャー初期化
	uploadManager := storage.NewUploadManager(cfg)

	// 認証プロバイダー初期化（単一プロバイダー）
	// OIDCのロールは再起動後も復元できるようDBへ永続化する（Discordでは未使用）。
	roleStore := rolestore.New(db)
	authProvider, err := authprovider.New(context.Background(), cfg.Auth.Provider, roleStore)
	if err != nil {
		slog.Error("認証プロバイダーの初期化に失敗しました", "error", err)
		os.Exit(1)
	}

	// 権限チェッカー初期化
	permissionChecker := permission.NewChecker(cfg, authProvider, storageManager, db)

	// SSEハンドラー初期化
	sseHandler := handler.NewSSEHandler()

	// テンプレートを起動時に一度だけパースする（リクエスト毎の再パースを避ける）。
	indexTmpl := loadTemplate("web/templates/index.html")
	indexMobileTmpl := loadTemplate("web/templates/index_mobile.html")
	adminTmpl := loadTemplate("web/templates/admin.html")

	// ハンドラー初期化
	authHandler := handler.NewAuthHandler(cfg, db, authProvider, storageManager)
	fileHandler := handler.NewFileHandler(cfg, storageManager, uploadManager, permissionChecker)
	chunkHandler := handler.NewChunkHandler(storageManager, uploadManager, permissionChecker)
	adminHandler := handler.NewAdminHandler(cfg, uploadManager, adminTmpl)

	// SSEハンドラーをファイルハンドラーに注入
	fileHandler.SetSSEHandler(sseHandler)
	authHandler.SetSSEHandler(sseHandler)

	// ルーター設定
	r := chi.NewRouter()

	// ミドルウェア
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP(cfg.Server.BehindProxy, cfg.Server.TrustedProxies))

	// 静的ファイル（バイナリ埋め込みのFSから配信）
	staticFS, err := fs.Sub(webFS, "web/static")
	if err != nil {
		slog.Error("静的ファイルFSの初期化に失敗しました", "error", err)
		os.Exit(1)
	}
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// ログイン画面に表示する認証プロバイダー（単一）。
	loginLabel := cfg.Auth.Provider.Name
	if cfg.Auth.Provider.Type == "discord" {
		loginLabel = "Discord"
	}

	// ルートパス（Webインターフェース）。テンプレートは起動時にパース済み。
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		// Vary: User-Agentヘッダーを追加（SEO対策）
		w.Header().Set("Vary", "User-Agent")

		// User-Agentを判定してテンプレートを切り替え
		tmpl := indexTmpl
		if isMobile(r.Header.Get("User-Agent")) {
			tmpl = indexMobileTmpl
		}

		data := map[string]interface{}{
			"ServiceName": cfg.Server.ServiceName,
			"LoginLabel":  loginLabel,
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

	// 認証エンドポイント（認証プロバイダーは1つに限定）
	r.Get("/auth/login", authHandler.Login)
	r.Get("/auth/callback", authHandler.Callback)
	r.Get("/auth/logout", authHandler.Logout)

	// 認証が必要なエンドポイント
	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddleware(cfg, db, authProvider))

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

		// チャンクアップロード（設定で有効化されている場合のみ登録）
		if cfg.Storage.ChunkUploadEnabled {
			r.Post("/files/chunk/init", chunkHandler.InitChunkUpload)
			r.Post("/files/chunk/upload/{upload_id}", chunkHandler.UploadChunk)
			r.Get("/files/chunk/status/{upload_id}", chunkHandler.GetChunkStatus)
			r.Post("/files/chunk/complete/{upload_id}", chunkHandler.CompleteChunkUpload)
			r.Delete("/files/chunk/cancel/{upload_id}", chunkHandler.CancelChunkUpload)
		} else {
			slog.Info("チャンクアップロードは無効化されています（storage.chunk_upload_enabled=false）")
		}

		// 管理者専用エンドポイント
		r.Group(func(r chi.Router) {
			r.Use(middleware.AdminMiddleware(cfg, authProvider))

			r.Get("/admin", adminHandler.AdminPage)
			r.Get("/api/admin/uploads", adminHandler.GetUploadSessions)
			r.Get("/api/admin/stats", adminHandler.GetUploadStats)
		})
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
