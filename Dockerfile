# syntax=docker/dockerfile:1

# ベースイメージは ARG を介さず literal に記述する。
# Dependabot の docker エコシステムは ARG 経由の FROM を解決できず、
# ARG 化するとベースイメージが更新対象から外れてしまうため。
#
# 別のベースイメージ（例: Docker Hardened Images）でビルドしたい場合は
# Dockerfile を変更せず --build-context で上書きできる。名前は下記 FROM の
# 記述と完全一致させる必要がある。手順は docs/DEPLOYMENT.md を参照。

# ---- CSSビルドステージ ----
# Tailwind をランタイムのCDNで実行せず、ビルド時に静的CSSへコンパイルする。
# テンプレートとJSを走査し、実際に使用されているクラスのみを含む最小CSSを生成する。
FROM node:26-alpine AS webbuilder
WORKDIR /web
COPY package.json tailwind.config.js ./
COPY web ./web
RUN npm install --no-audit --no-fund \
    && npx tailwindcss -c ./tailwind.config.js -i ./web/tailwind/input.css -o ./web/static/css/tailwind.css --minify

# ---- ビルドステージ ----
FROM golang:1.26-alpine AS builder

# CIから渡されるビルド情報。バイナリへ -ldflags -X で埋め込む。
ARG VERSION=dev
ARG BUILD_DATE=unknown
ARG VCS_REF=unknown

WORKDIR /src

# 依存だけ先に解決し、ソース変更時もモジュール層のキャッシュを再利用する
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# ソースをコピーして静的バイナリをビルド
#   CGO_ENABLED=0 : 完全静的リンク（static/distroless上で動作）
#   -trimpath     : ビルドパスを除去し再現性を向上
#   -buildvcs=false: VCSスタンプを無効化しgitを不要にする
#   -ldflags -s -w: デバッグ情報を除去してサイズ削減
COPY . .
# CDN実行をやめた分、埋め込むCSSはCSSビルドステージで生成した最新版で上書きする。
COPY --from=webbuilder /web/web/static/css/tailwind.css ./web/static/css/tailwind.css
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -buildvcs=false \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.BuildDate=${BUILD_DATE} -X main.GitCommit=${VCS_REF}" \
    -o /out/fileserver .

# 非rootで書き込むデータ用ディレクトリの雛形を用意する。
# 実行ステージにはシェルが無くRUN mkdirできないため、
# ビルド側で作成し所有者を指定してコピーする。
RUN mkdir -p /out/rootfs/config /out/rootfs/data

# ---- 実行ステージ ----
# 非root(UID 65532)・シェル無しの最小イメージ。CA証明書を同梱する。
# :nonroot はバージョンを持たないローリングタグのため、再現性と
# Dependabot による digest 更新の両立のためダイジェストで固定する。
FROM gcr.io/distroless/static-debian12:nonroot@sha256:b7bb25d9f7c31d2bdd1982feb4dafcaf137703c7075dbe2febb41c24212b946f AS runtime

# 空のデータディレクトリとバイナリを非rootユーザー(65532)所有で配置
COPY --from=builder --chown=65532:65532 /out/rootfs /app
COPY --from=builder --chown=65532:65532 /out/fileserver /app/fileserver

WORKDIR /app
USER 65532:65532

# 設定ファイルの場所とタイムゾーン（tzdataはバイナリに埋め込み済み）。
# DB/アップロード先はコンテナのボリューム構成に合わせて固定する。設定ひな型の
# 既定値は「バイナリ直接実行でそのまま動く」相対パスにしてあるため、コンテナでは
# ここで絶対パスを与えて上書きする（環境変数は config.yaml より優先される）。
# 別の場所に置きたい場合は compose 等でこの環境変数を上書きする。
ENV FILEGO_CONFIG_PATH=/app/config/config.yaml \
    FILEGO_DATABASE_PATH=/app/config/fileserver.db \
    FILEGO_STORAGE_UPLOAD_PATH=/app/data/uploads \
    TZ=Asia/Tokyo

EXPOSE 8080

# シェルやwgetを持たない最小イメージのため、バイナリ自身でヘルスチェックする
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/app/fileserver", "-healthcheck"]

ENTRYPOINT ["/app/fileserver"]
