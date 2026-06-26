package web

import (
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

func quietJM() *JobManager {
	return NewJobManager(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func waitJob(t *testing.T, m *JobManager, key string) Job {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if j, ok := m.Get(key); ok && j.Done {
			return j
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("job %s did not finish in time", key)
	return Job{}
}

func TestJobRunsToDone(t *testing.T) {
	m := quietJM()
	got := ""
	started := m.Start("prepare:a", "prepare", "a", "", func(progress func(string)) error {
		progress("section 1/2")
		got = "ran"
		return nil
	})
	if !started {
		t.Fatal("Start should return true for a fresh key")
	}
	j := waitJob(t, m, "prepare:a")
	if j.Status != "done" || got != "ran" {
		t.Fatalf("job = %+v, got=%q", j, got)
	}
}

func TestJobError(t *testing.T) {
	m := quietJM()
	m.Start("build:a:go", "build", "a", "go", func(progress func(string)) error {
		return errors.New("boom")
	})
	j := waitJob(t, m, "build:a:go")
	if j.Status != "error" || j.Err != "boom" {
		t.Fatalf("job = %+v, want error/boom", j)
	}
}

func TestJobDedupWhileRunning(t *testing.T) {
	m := quietJM()
	release := make(chan struct{})
	m.Start("prepare:a", "prepare", "a", "", func(progress func(string)) error {
		<-release
		return nil
	})
	if m.Start("prepare:a", "prepare", "a", "", func(progress func(string)) error { return nil }) {
		t.Fatal("second Start while running should return false")
	}
	close(release)
	waitJob(t, m, "prepare:a")
	// After completion a new Start is allowed again.
	if !m.Start("prepare:a", "prepare", "a", "", func(progress func(string)) error { return nil }) {
		t.Fatal("Start after completion should return true")
	}
}

func TestSnapshotAndProgress(t *testing.T) {
	m := quietJM()
	release := make(chan struct{})
	m.Start("prepare:a", "prepare", "a", "", func(progress func(string)) error {
		progress("section 2/3")
		<-release
		return nil
	})
	// Give the goroutine a moment to set progress.
	time.Sleep(20 * time.Millisecond)
	snap := m.Snapshot()
	if len(snap) != 1 || snap[0].Key != "prepare:a" || snap[0].Progress != "section 2/3" {
		t.Fatalf("snapshot = %+v", snap)
	}
	close(release)
	waitJob(t, m, "prepare:a")
}
