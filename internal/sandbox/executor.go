package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Config 沙箱配置
type Config struct {
	Enabled      bool          `yaml:"enabled"`
	Timeout      time.Duration `yaml:"timeout"`
	MaxMemoryMB  int           `yaml:"max_memory_mb"`
	AllowNetwork bool          `yaml:"allow_network"`
	WorkDir      string        `yaml:"work_dir"`
}

// DefaultConfig 默认沙箱配置
func DefaultConfig() Config {
	return Config{
		Enabled:      false,
		Timeout:      30 * time.Second,
		MaxMemoryMB:  512,
		AllowNetwork: false,
	}
}

// Executor 沙箱执行器
type Executor struct {
	config Config
}

// NewExecutor 创建沙箱执行器
func NewExecutor(cfg Config) *Executor {
	return &Executor{config: cfg}
}

// Result 执行结果
type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

// Execute 在沙箱中执行命令
func (e *Executor) Execute(ctx context.Context, command string, args ...string) (*Result, error) {
	if !e.config.Enabled {
		// 非沙箱模式：直接执行
		return e.executeDirect(ctx, command, args...)
	}

	// 沙箱模式
	switch runtime.GOOS {
	case "linux":
		return e.executeDocker(ctx, command, args...)
	case "windows":
		return e.executeWindowsSandbox(ctx, command, args...)
	default:
		// macOS 和其他系统回退到直接执行
		return e.executeDirect(ctx, command, args...)
	}
}

// ExecuteScript 在沙箱中执行脚本
func (e *Executor) ExecuteScript(ctx context.Context, script string) (*Result, error) {
	// COR-27 修复：根据操作系统选择脚本解释器和文件扩展名
	if runtime.GOOS == "windows" {
		return e.executeWindowsScript(ctx, script)
	}
	return e.executeUnixScript(ctx, script)
}

// executeUnixScript 在 Unix 系统上执行脚本
func (e *Executor) executeUnixScript(ctx context.Context, script string) (*Result, error) {
	tmpFile, err := os.CreateTemp("", "codecast_script_*.sh")
	if err != nil {
		return nil, fmt.Errorf("创建临时脚本文件失败: %w", err)
	}
	scriptPath := tmpFile.Name()

	if err := os.Chmod(scriptPath, 0600); err != nil {
		tmpFile.Close()
		os.Remove(scriptPath)
		return nil, fmt.Errorf("设置脚本权限失败: %w", err)
	}

	if _, err := tmpFile.WriteString(script); err != nil {
		tmpFile.Close()
		os.Remove(scriptPath)
		return nil, fmt.Errorf("写入脚本失败: %w", err)
	}
	tmpFile.Close()
	defer os.Remove(scriptPath)

	return e.Execute(ctx, "/bin/sh", scriptPath)
}

// executeWindowsScript 在 Windows 上执行脚本
func (e *Executor) executeWindowsScript(ctx context.Context, script string) (*Result, error) {
	// C-08 修复：验证脚本内容，防止 PowerShell 注入
	if err := validatePowerShellScript(script); err != nil {
		return nil, fmt.Errorf("脚本验证失败: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "codecast_script_*.ps1")
	if err != nil {
		return nil, fmt.Errorf("创建临时脚本文件失败: %w", err)
	}
	scriptPath := tmpFile.Name()

	if err := os.Chmod(scriptPath, 0600); err != nil {
		tmpFile.Close()
		os.Remove(scriptPath)
		return nil, fmt.Errorf("设置脚本权限失败: %w", err)
	}

	if _, err := tmpFile.WriteString(script); err != nil {
		tmpFile.Close()
		os.Remove(scriptPath)
		return nil, fmt.Errorf("写入脚本失败: %w", err)
	}
	tmpFile.Close()
	defer os.Remove(scriptPath)

	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
	// C-08 修复：清理环境变量，防止注入
	cmd.Env = sanitizeEnvironment(os.Environ())
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	runErr := cmd.Run()
	duration := time.Since(start)

	result := &Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration,
	}
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
	}
	return result, nil
}

// validatePowerShellScript 验证 PowerShell 脚本安全性
func validatePowerShellScript(script string) error {
	scriptLower := strings.ToLower(script)
	// 禁止的危险模式
	dangerousPatterns := []string{
		"invoke-expression", "iex ", "start-process",
		"new-object net.webclient", "downloadstring",
		"downloadfile", "invoke-webrequest",
		"remove-item -recurse", "del /f /s",
		"format-volume", "format-disk",
	}
	for _, pattern := range dangerousPatterns {
		if strings.Contains(scriptLower, pattern) {
			return fmt.Errorf("脚本包含危险命令: %s", pattern)
		}
	}
	return nil
}

// sanitizeEnvironment 清理环境变量
func sanitizeEnvironment(env []string) []string {
	sanitized := make([]string, 0, len(env))
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, value := parts[0], parts[1]
		// 移除可能包含命令注入的环境变量
		if strings.Contains(value, ";") || strings.Contains(value, "|") ||
			strings.Contains(value, "&") || strings.Contains(value, "`") {
			continue
		}
		sanitized = append(sanitized, fmt.Sprintf("%s=%s", key, value))
	}
	return sanitized
}

// executeDirect 直接执行（无沙箱）
func (e *Executor) executeDirect(ctx context.Context, command string, args ...string) (*Result, error) {
	if e.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.config.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, command, args...)
	if e.config.WorkDir != "" {
		// R5-H19 修复：验证 WorkDir 安全性
		cleanWorkDir, err := filepath.Abs(filepath.Clean(e.config.WorkDir))
		if err != nil {
			return nil, fmt.Errorf("工作目录无效: %w", err)
		}
		if strings.Contains(cleanWorkDir, "..") {
			return nil, fmt.Errorf("工作目录包含不安全路径")
		}
		cmd.Dir = cleanWorkDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	result := &Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
	}

	return result, nil
}

// executeDocker 在 Docker 容器中执行
func (e *Executor) executeDocker(ctx context.Context, command string, args ...string) (*Result, error) {
	dockerArgs := []string{
		"run", "--rm",
		fmt.Sprintf("--memory=%dm", e.config.MaxMemoryMB),
		"--network=none", // 默认禁用网络
	}

	if e.config.AllowNetwork {
		dockerArgs = []string{"run", "--rm", fmt.Sprintf("--memory=%dm", e.config.MaxMemoryMB)}
	}

	// 挂载工作目录 — 验证路径安全性，防止路径遍历和注入
	workDir := e.config.WorkDir
	if workDir == "" {
		workDir, _ = os.Getwd()
	}
	// R5-C5 修复：验证 workDir 是安全的绝对路径，防止 Docker 挂载注入
	cleanWorkDir, err := filepath.Abs(filepath.Clean(workDir))
	if err != nil {
		return nil, fmt.Errorf("工作目录无效: %w", err)
	}
	// 确保路径没有不安全的分段（如 ..）
	if filepath.Base(cleanWorkDir) == ".." || strings.Contains(cleanWorkDir, "..") {
		return nil, fmt.Errorf("工作目录包含不安全路径")
	}
	dockerArgs = append(dockerArgs, "-v", fmt.Sprintf("%s:/workspace", cleanWorkDir))
	dockerArgs = append(dockerArgs, "-w", "/workspace")

	// 超时
	if e.config.Timeout > 0 {
		dockerArgs = append(dockerArgs, fmt.Sprintf("--stop-timeout=%d", int(e.config.Timeout.Seconds())))
	}

	// 使用沙箱镜像
	dockerArgs = append(dockerArgs, "codecast-sandbox:latest", command)
	dockerArgs = append(dockerArgs, args...)

	return e.executeDirect(ctx, "docker", dockerArgs...)
}

// executeWindowsSandbox 在 Windows 沙箱中执行
func (e *Executor) executeWindowsSandbox(ctx context.Context, command string, args ...string) (*Result, error) {
	// S-01 修复：通过环境变量传递参数，避免字符串拼接导致的 PowerShell 注入
	workDir := e.config.WorkDir
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	// 使用 -File 模式执行脚本，参数通过环境变量传递，完全避免注入
	psScript := `$workDir = $env:CODECAST_WORK_DIR
$cmd = $env:CODECAST_COMMAND
$cmdArgs = $env:CODECAST_COMMAND_ARGS
Set-Location $workDir
$proc = Start-Process -FilePath $cmd -ArgumentList $cmdArgs -NoNewWindow -Wait -PassThru
$proc.ExitCode`

	tmpDir := os.TempDir()
	tmpFile, err := os.CreateTemp(tmpDir, "codecast_script_*.ps1")
	if err != nil {
		return nil, fmt.Errorf("创建临时脚本失败: %w", err)
	}
	tmpPath := tmpFile.Name()
	// N-06 修复：显式设置 0600 权限，防止多用户环境下其他用户读取脚本
	if err := os.Chmod(tmpPath, 0600); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("设置脚本权限失败: %w", err)
	}
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(psScript); err != nil {
		tmpFile.Close()
		return nil, fmt.Errorf("写入脚本失败: %w", err)
	}
	tmpFile.Close()

	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", tmpPath)
	// High 修复：清理基础环境变量，防止注入
	cmd.Env = sanitizeEnvironment(os.Environ())
	// N-05 修复：清理环境变量值中的换行符，防止 PowerShell 命令逃逸
	safeWorkDir := strings.ReplaceAll(workDir, "\n", "")
	safeWorkDir = strings.ReplaceAll(safeWorkDir, "\r", "")
	safeCommand := strings.ReplaceAll(command, "\n", "")
	safeCommand = strings.ReplaceAll(safeCommand, "\r", "")
	safeArgs := strings.ReplaceAll(strings.Join(args, " "), "\n", "")
	safeArgs = strings.ReplaceAll(safeArgs, "\r", "")
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("CODECAST_WORK_DIR=%s", safeWorkDir),
		fmt.Sprintf("CODECAST_COMMAND=%s", safeCommand),
		fmt.Sprintf("CODECAST_COMMAND_ARGS=%s", safeArgs),
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	runErr := cmd.Run()
	duration := time.Since(start)

	result := &Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration,
	}
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
	}
	return result, nil
}

// IsDockerAvailable 检查 Docker 是否可用
func IsDockerAvailable() bool {
	cmd := exec.Command("docker", "info")
	return cmd.Run() == nil
}

// BuildSandboxImage 构建 Docker 沙箱镜像
func BuildSandboxImage(ctx context.Context) error {
	// S-02 修复：使用 CreateTemp 创建不可预测的临时 Dockerfile
	tmpFile, err := os.CreateTemp("", "Dockerfile.codecast-sandbox-*")
	if err != nil {
		return fmt.Errorf("创建临时 Dockerfile 失败: %w", err)
	}
	dockerfilePath := tmpFile.Name()

	dockerfile := `FROM alpine:latest
RUN apk add --no-cache bash curl git python3 nodejs npm
WORKDIR /workspace
`

	if _, err := tmpFile.WriteString(dockerfile); err != nil {
		tmpFile.Close()
		os.Remove(dockerfilePath)
		return fmt.Errorf("写入 Dockerfile 失败: %w", err)
	}
	tmpFile.Close()
	defer os.Remove(dockerfilePath)

	cmd := exec.CommandContext(ctx, "docker", "build", "-t", "codecast-sandbox:latest", "-f", dockerfilePath, filepath.Dir(dockerfilePath))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("构建沙箱镜像失败: %w\n%s", err, string(output))
	}

	return nil
}
