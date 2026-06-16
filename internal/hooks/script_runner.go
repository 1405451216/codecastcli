package hooks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ScriptRunner 执行 .codecast/hooks/ 目录下的脚本
type ScriptRunner struct {
	hooksDir string
}

// NewScriptRunner 创建脚本执行器
// S-03 修复：对 hooksDir 做符号链接解析和存在性校验
func NewScriptRunner(hooksDir string) *ScriptRunner {
	// 解析符号链接，防止指向敏感目录
	if resolved, err := filepath.EvalSymlinks(hooksDir); err == nil {
		hooksDir = resolved
	}
	return &ScriptRunner{hooksDir: hooksDir}
}

// RunBeforeTool 执行 before_tool 目录下的所有脚本
func (r *ScriptRunner) RunBeforeTool(ctx context.Context, toolName, args string) error {
	return r.runScriptsInDir(ctx, "before_tool", map[string]string{
		"CODECAST_TOOL_NAME": toolName,
		"CODECAST_TOOL_ARGS": args,
	})
}

// RunAfterTool 执行 after_tool 目录下的所有脚本
func (r *ScriptRunner) RunAfterTool(ctx context.Context, toolName, result string) error {
	return r.runScriptsInDir(ctx, "after_tool", map[string]string{
		"CODECAST_TOOL_NAME":   toolName,
		"CODECAST_TOOL_RESULT": result,
	})
}

// runScriptsInDir 执行目录下的所有可执行脚本
func (r *ScriptRunner) runScriptsInDir(ctx context.Context, subDir string, env map[string]string) error {
	dir := filepath.Join(r.hooksDir, subDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取 hooks 目录失败: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		scriptPath := filepath.Join(dir, entry.Name())

		// S-03 修复：解析符号链接，确保脚本路径仍在 hooksDir 下
		resolvedPath, err := filepath.EvalSymlinks(scriptPath)
		if err != nil {
			continue
		}
		resolvedDir, err := filepath.EvalSymlinks(dir)
		if err != nil {
			resolvedDir = dir
		}
		if !strings.HasPrefix(resolvedPath, resolvedDir+string(filepath.Separator)) {
			continue // 跳过逃逸出 hooksDir 的符号链接
		}

		// 检查是否可执行
		info, err := entry.Info()
		if err != nil {
			continue
		}
		// COR-25 修复：Windows 上不支持 Unix 权限位，改用扩展名判断可执行性
		if runtime.GOOS != "windows" {
			if info.Mode()&0111 == 0 {
				continue // 不可执行
			}
		} else {
			// Windows 上通过扩展名判断
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if ext != ".exe" && ext != ".bat" && ext != ".cmd" && ext != ".ps1" {
				continue // 非 Windows 可执行文件
			}
		}

		// 跳过以 . 开头的文件和 ~ 结尾的文件
		if strings.HasPrefix(entry.Name(), ".") || strings.HasSuffix(entry.Name(), "~") {
			continue
		}

		cmd := r.buildCommand(ctx, scriptPath, env)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("脚本 %s 执行失败: %w\n输出: %s", entry.Name(), err, string(output))
		}
	}

	return nil
}

// buildCommand 构建执行命令
// High 修复：清理环境变量，防止注入攻击
func (r *ScriptRunner) buildCommand(ctx context.Context, scriptPath string, env map[string]string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, scriptPath)
	// 清理环境变量，移除可能包含命令注入的变量（sanitizeEnvironment 定义在 manager.go）
	cmd.Env = sanitizeEnvironment(os.Environ())
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	return cmd
}
