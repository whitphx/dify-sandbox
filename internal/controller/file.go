package controller

import (
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/langgenius/dify-sandbox/internal/storage"
	"github.com/langgenius/dify-sandbox/internal/types"
)

// sanitizeFilename removes or replaces characters that could cause header injection
// or other security issues in Content-Disposition headers
func sanitizeFilename(filename string) string {
	// Remove path components - only keep the base name
	filename = filepath.Base(filename)

	// Remove or replace dangerous characters for Content-Disposition
	// This includes: newlines, carriage returns, quotes, backslashes, and other control chars
	dangerous := regexp.MustCompile(`[\x00-\x1f\x7f"\\]`)
	filename = dangerous.ReplaceAllString(filename, "_")

	// Trim spaces and dots from edges (Windows safety)
	filename = strings.Trim(filename, " .")

	// If filename is empty after sanitization, use a default
	if filename == "" {
		filename = "download"
	}

	return filename
}

func UploadFile(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, types.ErrorResponse(400, "file is required"))
		return
	}

	// Parse optional TTL parameter (in seconds)
	// 0 or empty means use server default
	var ttlSeconds int
	ttlStr := c.PostForm("ttl")
	if ttlStr != "" {
		ttlSeconds, err = strconv.Atoi(ttlStr)
		if err != nil || ttlSeconds < 0 {
			c.JSON(http.StatusBadRequest, types.ErrorResponse(400, "ttl must be a non-negative integer"))
			return
		}
	}

	f, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, types.ErrorResponse(500, "failed to open file"))
		return
	}
	defer f.Close()

	store := storage.GetStorage()
	fileId, err := store.PutWithTTL(f, fileHeader.Filename, ttlSeconds)
	if err != nil {
		c.JSON(http.StatusInternalServerError, types.ErrorResponse(500, fmt.Sprintf("failed to save file: %v", err)))
		return
	}

	c.JSON(http.StatusOK, types.SuccessResponse(gin.H{
		"file_id": fileId,
	}))
}

func DownloadFile(c *gin.Context) {
	fileId := c.Param("file_id")
	// Clean up leading slash if using wildcard
	if len(fileId) > 0 && fileId[0] == '/' {
		fileId = fileId[1:]
	}
	if fileId == "" {
		c.JSON(http.StatusBadRequest, types.ErrorResponse(400, "file_id is required"))
		return
	}

	store := storage.GetStorage()
	reader, err := store.Get(fileId)
	if err != nil {
		// Could be 404 or 500
		c.JSON(http.StatusNotFound, types.ErrorResponse(404, "file not found"))
		return
	}
	defer reader.Close()

	// Try to get original filename from metadata, fall back to fileId base name
	filename := filepath.Base(fileId) // default fallback
	if metadata, err := store.GetMetadata(fileId); err == nil && metadata != nil && metadata.Filename != "" {
		filename = metadata.Filename
	}

	// Sanitize filename to prevent header injection attacks
	safeFilename := sanitizeFilename(filename)

	c.Header("Content-Disposition", "attachment; filename=\""+safeFilename+"\"")
	c.Header("Content-Type", "application/octet-stream")

	_, err = io.Copy(c.Writer, reader)
	if err != nil {
		// Can't write JSON error if streaming started, but we can log
		// gin logs handled errors usually
		return
	}
}
