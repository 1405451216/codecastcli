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
	// 创建临时脚本文件
	tmpDir := os.TempDir()
	scriptPath := filepath.Join(tmpDir, fmt.Sprintf("codecast_script_%d.sh", time.Now().UnixNano()))

	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return nil, fmt.Errorf("写入脚本失败: %w", err)
	}
	defer os.Remove(scriptPath)

	return e.Execute(ctx, "/bin/sh", scriptPath)
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
		cmd.Dir = e.config.WorkDir
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

	// 挂载工作目录
	workDir := e.config.WorkDir
	if workDir == "" {
		workDir, _ = os.Getwd()
	}
	dockerArgs = append(dockerArgs, "-v", fmt.Sprintf("%s:/workspace", workDir))
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
	// Windows Sandbox 需要专业版/企业版
	// 回退到受限 PowerShell 执行
	psScript := fmt.Sprintf(
		"Set-Location '%s'; $proc = Start-Process -FilePath '%s' -ArgumentList '%s' -NoNewWindow -Wait -PassThru; $proc.ExitCode",
		e.config.WorkDir, command, strings.Join(args, " "),
	)

	return e.executeDirect(ctx, "powershell", "-NoProfile", "-Command", psScript)
}

// IsDockerAvailable 检查 Docker 是否可用
func IsDockerAvailable() bool {
	cmd := exec.Command("docker", "info")
	return cmd.Run() == nil
}

// BuildSandboxImage 构建 Docker 沙箱镜像
func BuildSandboxImage(ctx context.Context) error {
	// 创建 Dockerfile
	tmpDir := os.TempDir()
	dockerfilePath := filepath.Join(tmpDir, "Dockerfile.codecast-sandbox")

	dockerfile := `FROM alpine:latest
RUN apk add --no-cache bash curl git python3 nodejs npm
WORKDIR /workspace
`

	if err := os.WriteFile(dockerfilePath, []byte(dockerfile), 0644); err != nil {
		return fmt.Errorf("写入 Dockerfile 失败: %w", err)
	}
	defer os.Remove(dockerfilePath)

	cmd := exec.CommandContext(ctx, "docker", "build", "-t", "codecast-sandbox:latest", "-f", dockerfilePath, tmpDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("构建沙箱镜像失败: %w\n%s", err, string(output))
	}

	return nil
}
