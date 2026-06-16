package vision

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ScreenshotCapture 截图工具
type ScreenshotCapture struct {
	tmpDir string
}

// NewScreenshotCapture 创建截图工具
func NewScreenshotCapture() *ScreenshotCapture {
	return &ScreenshotCapture{
		tmpDir: os.TempDir(),
	}
}

// Capture 截取屏幕截图
func (c *ScreenshotCapture) Capture() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return c.captureMacOS()
	case "windows":
		return c.captureWindows()
	case "linux":
		return c.captureLinux()
	default:
		return "", fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

func (c *ScreenshotCapture) captureMacOS() (string, error) {
	path := fmt.Sprintf("%s/codecast_screenshot_%d.png", c.tmpDir, time.Now().Unix())
	cmd := exec.Command("screencapture", "-x", path)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("macOS 截图失败: %w", err)
	}
	return path, nil
}

func (c *ScreenshotCapture) captureWindows() (string, error) {
	path := fmt.Sprintf("%s\\codecast_screenshot_%d.png", os.TempDir(), time.Now().Unix())
	// SEC-22 修复：通过环境变量传递路径，避免 PowerShell 命令注入
	script := `$path = $env:CODECAST_SCREENSHOT_PATH
Add-Type -AssemblyName System.Windows.Forms
[System.Windows.Forms.Screen]::PrimaryScreen | ForEach-Object {
    $bmp = New-Object System.Drawing.Bitmap($_.Bounds.Width, $_.Bounds.Height)
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.CopyFromScreen($_.Bounds.Location, [System.Drawing.Point]::Empty, $_.Bounds.Size)
    $bmp.Save($path)
    $g.Dispose()
    $bmp.Dispose()
}`
	cmd := exec.Command("powershell", "-NoProfile", "-Command", script)
	// High 修复：清理环境变量，防止注入
	cmd.Env = sanitizeEnv(os.Environ())
	cmd.Env = append(cmd.Env, fmt.Sprintf("CODECAST_SCREENSHOT_PATH=%s", path))
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("Windows 截图失败: %w", err)
	}
	return path, nil
}

func (c *ScreenshotCapture) captureLinux() (string, error) {
	path := fmt.Sprintf("%s/codecast_screenshot_%d.png", c.tmpDir, time.Now().Unix())
	// 尝试使用 gnome-screenshot
	cmd := exec.Command("gnome-screenshot", "-f", path)
	if err := cmd.Run(); err != nil {
		// 回退到 scrot
		cmd = exec.Command("scrot", path)
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("Linux 截图失败（需要 gnome-screenshot 或 scrot）: %w", err)
		}
	}
	return path, nil
}

// sanitizeEnv 清理环境变量，移除包含危险字符的变量值
func sanitizeEnv(env []string) []string {
	sanitized := make([]string, 0, len(env))
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, value := parts[0], parts[1]
		// 移除可能包含命令注入的环境变量
		if strings.Contains(value, ";") || strings.Contains(value, "|") ||
			strings.Contains(value, "&") || strings.Contains(value, "`") ||
			strings.Contains(value, "\n") || strings.Contains(value, "\r") ||
			strings.Contains(value, "$(") {
			continue
		}
		sanitized = append(sanitized, fmt.Sprintf("%s=%s", key, value))
	}
	return sanitized
}
