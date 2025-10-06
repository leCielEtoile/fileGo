# ビルドステージ
FROM golang:1.23-alpine AS builder

# 必要なパッケージをインストール
RUN apk add --no-cache git gcc musl-dev

WORKDIR /build

# 依存関係をコピーしてダウンロード
COPY go.mod go.sum ./
RUN go mod download

# ソースコードをコピー
COPY . .

# バイナリをビルド
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o fileserver .

# 実行ステージ
FROM alpine:latest

# 必要なパッケージをインストール
RUN apk --no-cache add ca-certificates tzdata

# タイムゾーン設定
ENV TZ=Asia/Tokyo

WORKDIR /root

# ビルドしたバイナリをコピー
COPY --from=builder /build/fileserver .

# Webファイルをコピー
COPY --from=builder /build/web ./web

# エントリーポイントスクリプトをコピー
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

# ディレクトリ作成
RUN mkdir -p /root/config /root/data/uploads /root/logs

# ポート公開
EXPOSE 8080

# ヘルスチェック
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# エントリーポイント設定
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]

# 実行
CMD ["./fileserver"]
