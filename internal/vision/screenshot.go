package vision

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
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
	// 使用 PowerShell 截图
	script := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.Screen]::PrimaryScreen | ForEach-Object { $bmp = New-Object System.Drawing.Bitmap($_.Bounds.Width, $_.Bounds.Height); $g = [System.Drawing.Graphics]::FromImage($bmp); $g.CopyFromScreen($_.Bounds.Location, [System.Drawing.Point]::Empty, $_.Bounds.Size); $bmp.Save('%s'); $g.Dispose(); $bmp.Dispose() }`, path)
	cmd := exec.Command("powershell", "-Command", script)
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
