package storage

import (
	"time"

	"github.com/langgenius/dify-sandbox/internal/utils/log"
)

// CleanupWorker handles periodic cleanup of expired files
type CleanupWorker struct {
	storage    *LocalStorage
	interval   time.Duration
	defaultTTL int
	stopCh     chan struct{}
}

// NewCleanupWorker creates a new cleanup worker
func NewCleanupWorker(storage *LocalStorage, interval time.Duration, defaultTTL int) *CleanupWorker {
	return &CleanupWorker{
		storage:    storage,
		interval:   interval,
		defaultTTL: defaultTTL,
		stopCh:     make(chan struct{}),
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

	log.Info("cleanup worker started with interval %v, default TTL %d seconds", w.interval, w.defaultTTL)

	// Run cleanup immediately on start
	w.cleanup()

	for {
		select {
		case <-ticker.C:
			w.cleanup()
		case <-w.stopCh:
			log.Info("cleanup worker stopped")
			return
		}
	}
}

func (w *CleanupWorker) cleanup() {
	fileIds, err := w.storage.List()
	if err != nil {
		log.Error("cleanup worker failed to list files: %v", err)
		return
	}

	var deletedCount int
	var errorCount int

	for _, fileId := range fileIds {
		expired, err := w.storage.IsExpired(fileId, w.defaultTTL)
		if err != nil {
			log.Error("cleanup worker failed to check expiration for %s: %v", fileId, err)
			errorCount++
			continue
		}

		if expired {
			if err := w.storage.Delete(fileId); err != nil {
				log.Error("cleanup worker failed to delete %s: %v", fileId, err)
				errorCount++
			} else {
				deletedCount++
			}
		}
	}

	if deletedCount > 0 || errorCount > 0 {
		log.Info("cleanup worker: checked %d files, deleted %d expired files, %d errors",
			len(fileIds), deletedCount, errorCount)
	}
}
