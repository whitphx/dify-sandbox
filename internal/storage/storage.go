package storage

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// FileMetadata stores metadata about an uploaded file
type FileMetadata struct {
	CreatedAt int64  `json:"created_at"` // Unix timestamp
	ExpiresAt int64  `json:"expires_at"` // Unix timestamp, 0 = use global default
	Filename  string `json:"filename"`   // Original filename
}

type Storage interface {
	Put(reader io.Reader, filename string) (string, error)
	PutWithTTL(reader io.Reader, filename string, ttlSeconds int) (string, error)
	Get(fileId string) (io.ReadCloser, error)
	GetMetadata(fileId string) (*FileMetadata, error)
	Delete(fileId string) error
	List() ([]string, error)
	IsExpired(fileId string, defaultTTL int) (bool, error)
	IsExpiredForCleanup(fileId string, defaultTTL int, gracePeriod int) (bool, error)
}

type LocalStorage struct {
	BaseDir string
}

func NewLocalStorage(baseDir string) (*LocalStorage, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, err
	}
	// Store absolute path for secure comparisons
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, err
	}
	return &LocalStorage{BaseDir: absBaseDir}, nil
}

// validatePath checks that a file ID doesn't escape the base directory
// Returns the safe absolute path or an error
func (s *LocalStorage) validatePath(fileId string) (string, error) {
	// Reject obviously malicious patterns
	if strings.Contains(fileId, "..") {
		return "", fmt.Errorf("invalid file id: contains path traversal")
	}

	cleanPath := filepath.Clean(fileId)
	if cleanPath == "." || cleanPath == "/" || cleanPath == "" {
		return "", fmt.Errorf("invalid file id")
	}

	fullPath := filepath.Join(s.BaseDir, cleanPath)

	// Get absolute path and verify it's within BaseDir
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("invalid file id: %w", err)
	}

	// Ensure the path is within BaseDir (with trailing separator to prevent prefix attacks)
	if !strings.HasPrefix(absPath, s.BaseDir+string(filepath.Separator)) && absPath != s.BaseDir {
		return "", fmt.Errorf("invalid file id: path escapes storage directory")
	}

	return absPath, nil
}

func (s *LocalStorage) Put(reader io.Reader, filename string) (string, error) {
	// Delegate to PutWithTTL with TTL=0 (use global default)
	return s.PutWithTTL(reader, filename, 0)
}

func (s *LocalStorage) PutWithTTL(reader io.Reader, filename string, ttlSeconds int) (string, error) {
	// Generate a unique ID for the file
	fileId := uuid.New().String()
	// Create a directory for the file (shard by date to avoid huge directories)
	dateDir := time.Now().Format("20060102")
	dirPath := filepath.Join(s.BaseDir, dateDir)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return "", err
	}

	// Stateless approach: Return the relative path as the ID.
	// ID = "20231027/uuid"
	relPath := filepath.Join(dateDir, fileId)
	fullPath := filepath.Join(s.BaseDir, relPath)

	f, err := os.Create(fullPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	_, err = io.Copy(f, reader)
	if err != nil {
		return "", err
	}

	// Create metadata sidecar file
	now := time.Now().Unix()
	var expiresAt int64
	if ttlSeconds > 0 {
		expiresAt = now + int64(ttlSeconds)
	}
	metadata := FileMetadata{
		CreatedAt: now,
		ExpiresAt: expiresAt,
		Filename:  filename,
	}

	metaPath := fullPath + ".meta.json"
	metaFile, err := os.Create(metaPath)
	if err != nil {
		// Clean up the data file if metadata creation fails
		os.Remove(fullPath)
		return "", err
	}
	defer metaFile.Close()

	if err := json.NewEncoder(metaFile).Encode(&metadata); err != nil {
		os.Remove(fullPath)
		os.Remove(metaPath)
		return "", err
	}

	return relPath, nil
}

func (s *LocalStorage) Get(fileId string) (io.ReadCloser, error) {
	fullPath, err := s.validatePath(fileId)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (s *LocalStorage) GetMetadata(fileId string) (*FileMetadata, error) {
	fullPath, err := s.validatePath(fileId)
	if err != nil {
		return nil, err
	}

	metaPath := fullPath + ".meta.json"
	metaFile, err := os.Open(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Legacy file without metadata - return nil metadata (not an error)
			return nil, nil
		}
		return nil, err
	}
	defer metaFile.Close()

	var metadata FileMetadata
	if err := json.NewDecoder(metaFile).Decode(&metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}

func (s *LocalStorage) Delete(fileId string) error {
	fullPath, err := s.validatePath(fileId)
	if err != nil {
		return err
	}

	// Delete the data file
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Delete the metadata file if it exists
	metaPath := fullPath + ".meta.json"
	os.Remove(metaPath) // Ignore error - metadata may not exist

	return nil
}

func (s *LocalStorage) List() ([]string, error) {
	var fileIds []string

	err := filepath.Walk(s.BaseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and metadata files
		if info.IsDir() || strings.HasSuffix(path, ".meta.json") {
			return nil
		}

		// Get relative path from base directory
		relPath, err := filepath.Rel(s.BaseDir, path)
		if err != nil {
			return err
		}

		fileIds = append(fileIds, relPath)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return fileIds, nil
}

func (s *LocalStorage) IsExpired(fileId string, defaultTTL int) (bool, error) {
	// GetMetadata already validates the path internally
	metadata, err := s.GetMetadata(fileId)
	if err != nil {
		return false, err
	}

	now := time.Now().Unix()

	if metadata == nil {
		// Legacy file without metadata
		// Check file modification time and apply default TTL
		fullPath, err := s.validatePath(fileId)
		if err != nil {
			return false, err
		}
		info, err := os.Stat(fullPath)
		if err != nil {
			return false, err
		}
		createdAt := info.ModTime().Unix()
		return now > createdAt+int64(defaultTTL), nil
	}

	// If ExpiresAt is set, use it
	if metadata.ExpiresAt > 0 {
		return now > metadata.ExpiresAt, nil
	}

	// Otherwise, use default TTL from creation time
	return now > metadata.CreatedAt+int64(defaultTTL), nil
}

// IsExpiredForCleanup checks if a file should be deleted by the cleanup worker.
//
// Multi-container coordination strategy:
// In K8s environments with shared storage (NFS, EFS, etc.), multiple containers
// may run cleanup workers simultaneously. Traditional file locking (flock) doesn't
// work reliably across NFS mounts.
//
// Instead, we use a "grace period" approach:
//   - Readers (Get) reject files immediately when TTL expires
//   - Cleanup workers only delete files after TTL + gracePeriod
//
// Timeline:
//
//	Created -----> TTL expires -----> TTL + grace -----> Deleted
//	                    |                  |
//	               Reads rejected    Cleanup deletes
//
// This ensures no file is deleted while being read, without requiring
// distributed locks. The grace period (default 60s) provides a buffer
// for any in-flight requests to complete.
func (s *LocalStorage) IsExpiredForCleanup(fileId string, defaultTTL int, gracePeriod int) (bool, error) {
	metadata, err := s.GetMetadata(fileId)
	if err != nil {
		return false, err
	}

	now := time.Now().Unix()

	if metadata == nil {
		// Legacy file without metadata
		fullPath, err := s.validatePath(fileId)
		if err != nil {
			return false, err
		}
		info, err := os.Stat(fullPath)
		if err != nil {
			return false, err
		}
		createdAt := info.ModTime().Unix()
		// Add grace period for cleanup
		return now > createdAt+int64(defaultTTL)+int64(gracePeriod), nil
	}

	// If ExpiresAt is set, use it + grace period
	if metadata.ExpiresAt > 0 {
		return now > metadata.ExpiresAt+int64(gracePeriod), nil
	}

	// Otherwise, use default TTL + grace period from creation time
	return now > metadata.CreatedAt+int64(defaultTTL)+int64(gracePeriod), nil
}
