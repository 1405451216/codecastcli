package vision

import (
	"path/filepath"
	"testing"
)

func TestDetectMIME(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{".jpg", "image/jpeg"},
		{".jpeg", "image/jpeg"},
		{".png", "image/png"},
		{".gif", "image/gif"},
		{".webp", "image/webp"},
		{".bmp", "image/bmp"},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			path := filepath.Join("some", "dir", "image"+tt.ext)
			got := detectMIME(path)
			if got != tt.want {
				t.Errorf("detectMIME(%q) = %q, want %q", path, got, tt.want)
			}
		})
	}

	t.Run("uppercase extension", func(t *testing.T) {
		got := detectMIME("photo.JPG")
		if got != "image/jpeg" {
			t.Errorf("detectMIME(%q) = %q, want %q", "photo.JPG", got, "image/jpeg")
		}
	})

	t.Run("mixed case extension", func(t *testing.T) {
		got := detectMIME("photo.PnG")
		if got != "image/png" {
			t.Errorf("detectMIME(%q) = %q, want %q", "photo.PnG", got, "image/png")
		}
	})
}

func TestDetectMIME_Unsupported(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"txt file", "document.txt"},
		{"pdf file", "report.pdf"},
		{"svg file", "icon.svg"},
		{"no extension", "README"},
		{"tiff file", "scan.tiff"},
		{"empty extension", "file."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectMIME(tt.path)
			if got != "" {
				t.Errorf("detectMIME(%q) = %q, want empty string", tt.path, got)
			}
		})
	}
}

func TestIsImageFile(t *testing.T) {
	t.Run("supported images", func(t *testing.T) {
		supported := []string{
			"photo.jpg",
			"photo.jpeg",
			"photo.png",
			"photo.gif",
			"photo.webp",
			"photo.bmp",
			"photo.PNG",
		}
		for _, path := range supported {
			if !IsImageFile(path) {
				t.Errorf("IsImageFile(%q) = false, want true", path)
			}
		}
	})

	t.Run("unsupported files", func(t *testing.T) {
		unsupported := []string{
			"document.txt",
			"report.pdf",
			"icon.svg",
			"script.go",
			"README",
		}
		for _, path := range unsupported {
			if IsImageFile(path) {
				t.Errorf("IsImageFile(%q) = true, want false", path)
			}
		}
	})
}
