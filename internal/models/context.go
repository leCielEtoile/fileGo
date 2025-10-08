package models

// ContextKey は衝突を避けるためのコンテキストキー用のカスタム型です。
type ContextKey string

// UserContextKey はコンテキストにユーザー情報を保存するためのキーです。
const UserContextKey ContextKey = "user"
