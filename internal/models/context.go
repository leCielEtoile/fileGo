package models

// contextKey is a custom type for context keys to avoid collisions
type ContextKey string

// UserContextKey is the key for storing user information in context
const UserContextKey ContextKey = "user"
