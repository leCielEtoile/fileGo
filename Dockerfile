# ビルドステージ
FROM golang:1.26-alpine AS builder

# 必要なパッケージをインストール
RUN apk add --no-cache git

WORKDIR /build

# 依存関係をコピーしてダウンロード
COPY go.mod go.sum ./
RUN go mod download

# ソースコードをコピー
COPY . .

# バイナリをビルド
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o fileserver .

# 実行ステージ
FROM alpine:latest

# 必要なパッケージをインストール
RUN apk --no-cache add ca-certificates tzdata

# タイムゾーン設定
ENV TZ=Asia/Tokyo

# 非rootユーザーとグループを作成
RUN addgroup -g 1000 -S appgroup && \
    adduser -u 1000 -S appuser -G appgroup

# アプリケーションディレクトリを作成
RUN mkdir -p /app/config /app/data /app/logs && \
    chown -R appuser:appgroup /app

WORKDIR /app

# ビルドしたバイナリをコピー
COPY --from=builder --chown=appuser:appgroup /build/fileserver .

# Webファイルをコピー
COPY --from=builder --chown=appuser:appgroup /build/web ./web

# エントリーポイントスクリプトをコピー
COPY --chown=appuser:appgroup entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

# 非rootユーザーに切り替え
USER appuser

# ポート公開
EXPOSE 8080

# ヘルスチェック
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# エントリーポイント設定
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]

# 実行
CMD ["./fileserver"]
