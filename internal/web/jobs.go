package web

import (
	"log/slog"
	"sync"
	"time"
)

// Job is the status of a background operation.
type Job struct {
	Key      string `json:"key"`
	Kind     string `json:"kind"`
	SourceID string `json:"sourceId"`
	Lang     string `json:"lang"`
	Status   string `json:"status"` // running | done | error
	Progress string `json:"progress"`
	Err      string `json:"error"`
	Done     bool   `json:"done"`
}

// JobManager runs long operations in the background and tracks their status.
type JobManager struct {
	mu   sync.Mutex
	jobs map[string]*Job
	log  *slog.Logger
}

// NewJobManager returns an empty JobManager.
func NewJobManager(log *slog.Logger) *JobManager {
	return &JobManager{jobs: map[string]*Job{}, log: log}
}

// Start launches fn in a goroutine unless a job for key is already running.
// It returns true if a new job was started.
func (m *JobManager) Start(key, kind, sourceID, lang string, fn func(progress func(string)) error) bool {
	m.mu.Lock()
	if j, ok := m.jobs[key]; ok && j.Status == "running" {
		m.mu.Unlock()
		return false
	}
	m.jobs[key] = &Job{Key: key, Kind: kind, SourceID: sourceID, Lang: lang, Status: "running"}
	m.mu.Unlock()
	m.log.Info("job.start", "kind", kind, "source", sourceID, "lang", lang)

	progress := func(s string) {
		m.mu.Lock()
		if j := m.jobs[key]; j != nil {
			j.Progress = s
		}
		m.mu.Unlock()
		m.log.Info("job.progress", "kind", kind, "source", sourceID, "step", s)
	}
	start := time.Now()
	go func() {
		err := fn(progress)
		m.mu.Lock()
		j := m.jobs[key]
		if j != nil {
			j.Done = true
			if err != nil {
				j.Status = "error"
				j.Err = err.Error()
			} else {
				j.Status = "done"
			}
		}
		m.mu.Unlock()
		if err != nil {
			m.log.Error("job.error", "kind", kind, "source", sourceID, "err", err)
		} else {
			m.log.Info("job.done", "kind", kind, "source", sourceID, "dur", time.Since(start))
		}
	}()
	return true
}

// Get returns a copy of the job for key.
func (m *JobManager) Get(key string) (Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[key]
	if !ok {
		return Job{}, false
	}
	return *j, true
}

// Snapshot returns copies of all known jobs.
func (m *JobManager) Snapshot() []Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		out = append(out, *j)
	}
	return out
}
