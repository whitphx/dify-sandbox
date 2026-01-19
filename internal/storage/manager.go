package storage

import (
	"log"
	"sync"
	"time"
)

var (
	GlobalStorage       Storage
	globalLocalStorage  *LocalStorage
	globalCleanupWorker *CleanupWorker
	once                sync.Once
)

func InitStorage(baseDir string) {
	once.Do(func() {
		var err error
		globalLocalStorage, err = NewLocalStorage(baseDir)
		if err != nil {
			log.Fatalf("failed to init storage: %v", err)
		}
		GlobalStorage = globalLocalStorage
	})
}

func GetStorage() Storage {
	if GlobalStorage == nil {
		InitStorage("data/sandbox") // Default fallback
	}
	return GlobalStorage
}

// StartCleanupWorker starts the background cleanup worker
//
// The gracePeriod parameter enables safe operation in multi-container environments.
// Files are deleted only after TTL + gracePeriod, while readers reject expired files
// immediately at TTL. This prevents files from being deleted while being read.
func StartCleanupWorker(intervalStr string, defaultTTL int, gracePeriod int) {
	if globalLocalStorage == nil {
		log.Fatalf("storage not initialized, cannot start cleanup worker")
	}

	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		log.Printf("invalid cleanup interval %s, using default 5m: %v", intervalStr, err)
		interval = 5 * time.Minute
	}

	globalCleanupWorker = NewCleanupWorker(globalLocalStorage, interval, defaultTTL, gracePeriod)
	globalCleanupWorker.Start()
}

// StopCleanupWorker stops the cleanup worker (for graceful shutdown)
func StopCleanupWorker() {
	if globalCleanupWorker != nil {
		globalCleanupWorker.Stop()
	}
}
