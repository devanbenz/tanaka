package obsidian

import (
	"context"
	"log/slog"
	"path/filepath"
	"sync"

	"github.com/devandbenz/tanaka/internal/sheet"
	"github.com/devandbenz/tanaka/internal/store"
)

// Syncer regenerates a source's Obsidian folder in the background. It is
// fire-and-forget: Sync never blocks the caller; errors are logged. The
// mutex serializes writes, and each goroutine re-reads fresh store state
// under it, so the last write always reflects the most complete snapshot.
type Syncer struct {
	st  store.Store
	dir string // vault root; empty disables the syncer
	log *slog.Logger
	mu  sync.Mutex
	wg  sync.WaitGroup
}

// NewSyncer returns a Syncer writing under dir. An empty dir disables it.
func NewSyncer(st store.Store, dir string, log *slog.Logger) *Syncer {
	return &Syncer{st: st, dir: dir, log: log}
}

// Enabled reports whether syncing is configured.
func (s *Syncer) Enabled() bool { return s.dir != "" }

// Sync regenerates sourceID's folder in the background. No-op when disabled.
func (s *Syncer) Sync(sourceID string) {
	if !s.Enabled() {
		return
	}
	s.wg.Go(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		exp, err := Assemble(context.Background(), s.st, sourceID)
		if err != nil {
			s.log.Error("obsidian sync: assemble", "source", sourceID, "err", err)
			return
		}
		dir := filepath.Join(s.dir, sheet.Slug(exp.Source.Title))
		if err := Write(dir, exp); err != nil {
			s.log.Error("obsidian sync: write", "source", sourceID, "err", err)
		}
	})
}

// Drain blocks until all in-flight syncs finish. Called during shutdown so
// a sync isn't killed mid-write.
func (s *Syncer) Drain() { s.wg.Wait() }
