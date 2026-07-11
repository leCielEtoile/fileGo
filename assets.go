// Package main は fileGo ファイルサーバーのエントリポイントと、
// web資産・設定ひな型のバイナリ埋め込みを提供します。
package main

import "embed"

// webFS はHTMLテンプレートと静的アセット（CSS/JS）をバイナリに埋め込みます。
// これによりランタイムイメージへ web ディレクトリを同梱する必要がなくなり、
// distroless / DHI static のようなシェルの無い最小イメージでも動作します。
//
//go:embed web/templates web/static
var webFS embed.FS

// defaultConfigYAML は設定ファイルが存在しない初回起動時に書き出すひな型です。
// config.yaml.example をそのまま埋め込むことで、実行時に外部ファイルや
// ネットワーク取得へ依存せず自己完結して初期設定を生成できます。
//
//go:embed config.yaml.example
var defaultConfigYAML []byte
