// Package config の本ファイルは、設定ファイルが存在しない場合に
// 呼び出し側から渡された埋め込みのひな型を書き出すブートストラップ処理を提供します。
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// writeDefaultConfig は埋め込みのひな型 template を destPath へ書き出します。
// 親ディレクトリが存在しない場合は作成します。設定には秘密情報が含まれ得るため
// パーミッションは 0600 とします。
func writeDefaultConfig(destPath string, template []byte) error {
	if len(template) == 0 {
		return fmt.Errorf("既定の設定テンプレートが指定されていません")
	}

	if dir := filepath.Dir(destPath); dir != "" {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return fmt.Errorf("設定ディレクトリの作成に失敗しました: %w", err)
		}
	}

	// #nosec G306 - 設定ファイルは秘密情報を含むため 0600 で保存する
	if err := os.WriteFile(destPath, template, 0600); err != nil {
		return fmt.Errorf("設定ファイルの書き込みに失敗しました: %w", err)
	}

	return nil
}
