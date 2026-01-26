package controller

import "testing"

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Basic cases
		{
			name:     "normal filename",
			input:    "document.pdf",
			expected: "document.pdf",
		},
		{
			name:     "filename with spaces",
			input:    "my document.pdf",
			expected: "my document.pdf",
		},

		// Path traversal prevention
		{
			name:     "path traversal with slashes",
			input:    "../../../etc/passwd",
			expected: "passwd",
		},
		{
			name:     "absolute path",
			input:    "/etc/passwd",
			expected: "passwd",
		},
		// windows path - on Linux, filepath.Base doesn't parse Windows paths
		// but backslashes get sanitized, so it's still safe
		{
			name:     "windows path",
			input:    "C:\\Users\\test\\file.txt",
			expected: "C:_Users_test_file.txt",
		},

		// Header injection prevention
		// Note: newline acts as path separator, so filepath.Base
		// returns the part after - still safe
		{
			name:     "newline injection",
			input:    "file\nContent-Type: text/html",
			expected: "html",
		},
		{
			name:     "carriage return injection",
			input:    "file\r\nSet-Cookie: evil",
			expected: "file__Set-Cookie: evil",
		},
		{
			name:     "quote injection",
			input:    "file\"; filename=\"evil.exe",
			expected: "file_; filename=_evil.exe",
		},
		{
			name:     "backslash in filename",
			input:    "file\\name.txt",
			expected: "file_name.txt",
		},

		// Control characters
		{
			name:     "null byte",
			input:    "file\x00.txt",
			expected: "file_.txt",
		},
		{
			name:     "tab character",
			input:    "file\t.txt",
			expected: "file_.txt",
		},
		{
			name:     "DEL character",
			input:    "file\x7f.txt",
			expected: "file_.txt",
		},

		// Edge cases
		{
			name:     "empty string",
			input:    "",
			expected: "download",
		},
		{
			name:     "only dots",
			input:    "...",
			expected: "download",
		},
		{
			name:     "only spaces",
			input:    "   ",
			expected: "download",
		},
		{
			name:     "leading and trailing dots",
			input:    "...file.txt...",
			expected: "file.txt",
		},
		{
			name:     "leading and trailing spaces",
			input:    "  file.txt  ",
			expected: "file.txt",
		},
		// After sanitization, we get "____" (4 underscores), which is not empty
		{
			name:     "only dangerous characters",
			input:    "\"\\\n\r",
			expected: "____",
		},

		// Unicode (should be preserved)
		{
			name:     "unicode filename",
			input:    "ファイル.txt",
			expected: "ファイル.txt",
		},
		{
			name:     "emoji in filename",
			input:    "document📄.pdf",
			expected: "document📄.pdf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeFilename(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
