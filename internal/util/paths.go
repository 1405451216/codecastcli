package util

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

// HasUnsafePathSegment 检查路径是否不安全。
// S-04 修复：从 tools 包提取为公共函数，供 read_file / edit_file / multi_edit / grep 共用。
// SEC-11 修复：增加系统关键目录拒绝和空字节检查。
//
// 用于阻止工具误删/误读系统关键文件：
//   - 拒绝 ".."，任何含 ".." 段的相对路径都会逃逸出预期目录
//   - 拒绝 Linux 根 "/" 以及 Windows 驱动器根 "C:\" / "C:/"
//   - 拒绝 UNC 根 "\\server\share" / "//server/share"
//   - 拒绝系统关键目录（/etc, /usr, /bin, C:\Windows 等）
//   - 拒绝空字节
func HasUnsafePathSegment(p string) bool {
	// 拒绝空字节注入
	if strings.ContainsRune(p, 0) {
		return true
	}
	// 拒绝 POSIX 根目录
	if p == "/" {
		return true
	}
	// 拒绝 Windows 驱动器根（含 "C:" 无尾部分隔符的情况）
	if len(p) == 2 && p[1] == ':' {
		return true
	}
	if len(p) == 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/') {
		return true
	}
	// 拒绝 UNC 根
	if strings.HasPrefix(p, "\\\\") || strings.HasPrefix(p, "//") {
		return true
	}
	// SEC-11 修复：拒绝系统关键目录
	normalized := filepath.ToSlash(p)
	systemPrefixes := []string{
		// Unix 系统目录
		"/etc/", "/usr/", "/bin/", "/sbin/", "/lib/", "/lib64/",
		"/boot/", "/dev/", "/proc/", "/sys/", "/root/",
		// Windows 系统目录
		"c:/windows/", "c:/program files/", "c:/program files (x86)/",
		"c:/programdata/",
	}
	lowerNorm := strings.ToLower(normalized)
	for _, prefix := range systemPrefixes {
		if strings.HasPrefix(lowerNorm, prefix) {
			return true
		}
	}
	// 也检查 Windows 短路径格式（如 C:\PROGRA~1）
	if runtime.GOOS == "windows" {
		shortPathPrefixes := []string{
			"c:/progra~1/", "c:/progra~2/",
			"c:/window~1/",
		}
		for _, prefix := range shortPathPrefixes {
			if strings.HasPrefix(lowerNorm, prefix) {
				return true
			}
		}
	}
	// 拒绝任何 ".." 路径段
	for _, seg := range strings.Split(normalized, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// FormatSize 格式化文件大小为人类可读字符串（M-05 修复：从 indexer/tools 提取为公共函数）
func FormatSize(size int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case size >= GB:
		return fmt.Sprintf("%.1f GB", float64(size)/float64(GB))
	case size >= MB:
		return fmt.Sprintf("%.1f MB", float64(size)/float64(MB))
	case size >= KB:
		return fmt.Sprintf("%.1f KB", float64(size)/float64(KB))
	default:
		return fmt.Sprintf("%d B", size)
	}
}
