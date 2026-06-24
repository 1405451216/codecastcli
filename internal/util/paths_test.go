package util

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestHasUnsafePathSegment(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "safe relative path",
			path:     "src/main.go",
			expected: false,
		},
		{
			name:     "safe absolute path",
			path:     filepath.FromSlash("/home/user/project"),
			expected: false,
		},
		{
			name:     "parent directory escape",
			path:     "../etc/passwd",
			expected: true,
		},
		{
			name:     "parent directory in middle",
			path:     "src/../config",
			expected: true,
		},
		{
			name:     "root path",
			path:     "/",
			expected: true,
		},
		{
			name:     "windows drive root",
			path:     "C:\\",
			expected: runtime.GOOS == "windows",
		},
		{
			name:     "windows drive root no slash",
			path:     "C:",
			expected: runtime.GOOS == "windows",
		},
		{
			name:     "UNC path",
			path:     "\\\\server\\share",
			expected: true,
		},
		{
			name:     "UNC path forward slash",
			path:     "//server/share",
			expected: true,
		},
		{
			name:     "etc directory",
			path:     "/etc/passwd",
			expected: true,
		},
		{
			name:     "usr directory",
			path:     "/usr/bin",
			expected: true,
		},
		{
			name:     "windows system directory",
			path:     "C:\\Windows\\System32",
			expected: true,
		},
		{
			name:     "null byte injection",
			path:     "file\x00.txt",
			expected: true,
		},
		{
			name:     "safe nested path",
			path:     "a/b/c/d/e",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasUnsafePathSegment(tt.path)
			if result != tt.expected {
				t.Errorf("HasUnsafePathSegment(%q) = %v; want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name     string
		size     int64
		expected string
	}{
		{
			name:     "bytes",
			size:     512,
			expected: "512 B",
		},
		{
			name:     "kilobytes",
			size:     1024,
			expected: "1.0 KB",
		},
		{
			name:     "kilobytes with decimals",
			size:     1536,
			expected: "1.5 KB",
		},
		{
			name:     "megabytes",
			size:     1024 * 1024,
			expected: "1.0 MB",
		},
		{
			name:     "megabytes with decimals",
			size:     1536 * 1024,
			expected: "1.5 MB",
		},
		{
			name:     "gigabytes",
			size:     1024 * 1024 * 1024,
			expected: "1.0 GB",
		},
		{
			name:     "gigabytes with decimals",
			size:     1536 * 1024 * 1024,
			expected: "1.5 GB",
		},
		{
			name:     "zero bytes",
			size:     0,
			expected: "0 B",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatSize(tt.size)
			if result != tt.expected {
				t.Errorf("FormatSize(%d) = %q; want %q", tt.size, result, tt.expected)
			}
		})
	}
}
