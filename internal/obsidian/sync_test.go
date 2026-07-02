package obsidian

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestSyncerWritesVaultInBackground(t *testing.T) {
	st := seededStore(t)
	dir := t.TempDir()
	sy := NewSyncer(st, dir, discardLog())
	if !sy.Enabled() {
		t.Fatal("syncer with a dir should be enabled")
	}
	sy.Sync("src1")
	sy.Drain()
	if _, err := os.Stat(filepath.Join(dir, "my-paper", "My Paper.md")); err != nil {
		t.Fatalf("hub note not written: %v", err)
	}
}

func TestSyncerDisabledWithoutDir(t *testing.T) {
	st := seededStore(t)
	sy := NewSyncer(st, "", discardLog())
	if sy.Enabled() {
		t.Fatal("syncer without a dir should be disabled")
	}
	sy.Sync("src1") // must not panic or spawn work
	sy.Drain()      // returns immediately
}

// Unknown sources log and drop; the syncer must stay usable.
func TestSyncerUnknownSourceIsLoggedNotFatal(t *testing.T) {
	st := seededStore(t)
	dir := t.TempDir()
	sy := NewSyncer(st, dir, discardLog())
	sy.Sync("missing")
	sy.Drain()
	sy.Sync("src1")
	sy.Drain()
	if _, err := os.Stat(filepath.Join(dir, "my-paper", "My Paper.md")); err != nil {
		t.Fatalf("syncer unusable after unknown source: %v", err)
	}
}
