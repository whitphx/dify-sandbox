package storage

import (
	"log/slog"
	"time"
)

// CleanupWorker handles periodic cleanup of expired files
type CleanupWorker struct {
	storage     *LocalStorage
	interval    time.Duration
	defaultTTL  int
	gracePeriod int
	stopCh      chan struct{}
}

// NewCleanupWorker creates a new cleanup worker
//
// The gracePeriod parameter is crucial for multi-container deployments (e.g., K8s).
// See IsExpiredForCleanup() in storage.go for the full explanation of the
// coordination strategy between readers and cleanup workers.
func NewCleanupWorker(storage *LocalStorage, interval time.Duration, defaultTTL int, gracePeriod int) *CleanupWorker {
	return &CleanupWorker{
		storage:     storage,
		interval:    interval,
		defaultTTL:  defaultTTL,
		gracePeriod: gracePeriod,
		stopCh:      make(chan struct{}),
	}
}

// Start begins the cleanup worker
func (w *CleanupWorker) Start() {
	go w.run()
}

// Stop stops the cleanup worker
func (w *CleanupWorker) Stop() {
	close(w.stopCh)
}

func (w *CleanupWorker) run() {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	slog.Info("cleanup worker started",
		"interval", w.interval, "default_ttl", w.defaultTTL, "grace_period", w.gracePeriod)

	// Run cleanup immediately on start
	w.cleanup()

	for {
		select {
		case <-ticker.C:
			w.cleanup()
		case <-w.stopCh:
			slog.Info("cleanup worker stopped")
			return
		}
	}
}

func (w *CleanupWorker) cleanup() {
	fileIds, err := w.storage.List()
	if err != nil {
		slog.Error("cleanup worker failed to list files", "err", err)
		return
	}

	var deletedCount int
	var errorCount int

	for _, fileId := range fileIds {
		// Use IsExpiredForCleanup which adds grace period.
		// This ensures files are only deleted after TTL + gracePeriod,
		// giving readers time to finish before deletion.
		expired, err := w.storage.IsExpiredForCleanup(fileId, w.defaultTTL, w.gracePeriod)
		if err != nil {
			slog.Error("cleanup worker failed to check expiration", "file_id", fileId, "err", err)
			errorCount++
			continue
		}

		if expired {
			if err := w.storage.Delete(fileId); err != nil {
				slog.Error("cleanup worker failed to delete file", "file_id", fileId, "err", err)
				errorCount++
			} else {
				deletedCount++
			}
		}
	}

	if deletedCount > 0 || errorCount > 0 {
		slog.Info("cleanup worker completed",
			"checked", len(fileIds), "deleted", deletedCount, "errors", errorCount)
	}
}
