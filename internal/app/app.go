// Package app holds shared runtime helpers (paths, ids).
package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// DataDir returns ~/.tanaka, creating it if needed.
func DataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	dir := filepath.Join(home, ".tanaka")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create data dir: %w", err)
	}
	return dir, nil
}

// DBPath returns the SQLite database path.
func DBPath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", fmt.Errorf("db path: %w", err)
	}
	return filepath.Join(dir, "tanaka.db"), nil
}

// NewID returns a random 16-hex-character identifier.
func NewID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b[:])
}
