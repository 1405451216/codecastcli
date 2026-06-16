package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"codecast/cli/internal/agent"
	"codecast/cli/internal/config"
	"codecast/cli/internal/output"

	"github.com/spf13/cobra"
)

// Exit codes
const (
	ExitSuccess       = 0
	ExitError         = 1
	ExitTimeout       = 2
	ExitConfigError   = 3
	ExitInterrupted   = 130
)

// ExitCodeError 是 cobra 可以识别的退出错误，返回非 0 退出码。
//
// 之前实现使用 os.Exit(...) 直接终止进程，绕过了 cobra 的错误流，
// 在 CI/脚本/测试中难以捕获。改用 cobra 的 RunE 返回错误后，
// 由 main.go 中的 cmd.Execute() 统一处理退出码。
type ExitCodeError struct {
	Code int
	Err  error
}

func (e *ExitCodeError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("exit code %d", e.Code)
}

func (e *ExitCodeError) Unwrap() error { return e.Err }

// newExit 创建一个 ExitCodeError
func newExit(code int, err error) *ExitCodeError {
	return &ExitCodeError{Code: code, Err: err}
}

var (
	execFormat  string
	execModel   string
	execTimeout time.Duration
	execTools   []string
	execNoTools bool
	execWorkDir string
	execQuiet   bool
)

// execCmd represents the exec command for headless mode
var execCmd = &cobra.Command{
	Use:   "exec [prompt]",
	Short: "Headless 模式执行单次提示",
	Long: `以非交互模式执行单次提示，适合 CI/CD 集成和脚本调用。

支持三种输出格式：
  --format text         纯文本输出（默认）
  --format json         单个 JSON 对象输出
  --format stream-json  NDJSON 流式输出

示例：
  codecast exec "解释 main.go 的功能"
  codecast exec --format json "修复 bug"
  cat error.log | codecast exec "分析这个错误"
  codecast exec --format stream-json "重构代码"
  codecast exec --tools read_file,edit_file "修改代码"
  codecast exec --no-tools "纯对话"
  codecast exec --work-dir /path/to/project "分析项目"
  codecast exec --quiet "只看结果"
  codecast exec --timeout 5m "限时执行"`,
	Args: cobra.MaximumNArgs(1),
	RunE: runExec,
}

func init() {
	rootCmd.AddCommand(execCmd)
	execCmd.Flags().StringVarP(&execFormat, "format", "f", "text", "输出格式 (text/json/stream-json)")
	execCmd.Flags().StringVarP(&execModel, "model", "m", "", "覆盖模型")
	execCmd.Flags().DurationVarP(&execTimeout, "timeout", "t", 10*time.Minute, "总超时时间（如 30s, 5m, 1h）")
	execCmd.Flags().StringSliceVar(&execTools, "tools", nil, "允许的工具列表（逗号分隔）")
	execCmd.Flags().BoolVar(&execNoTools, "no-tools", false, "禁用所有工具（纯对话模式）")
	execCmd.Flags().StringVar(&execWorkDir, "work-dir", "", "工作目录（执行前切换到此目录）")
	execCmd.Flags().BoolVarP(&execQuiet, "quiet", "q", false, "安静模式，仅输出最终结果")
}

func runExec(cmd *cobra.Command, args []string) error {
	// 解析输出格式
	format, err := output.ParseFormat(execFormat)
	if err != nil {
		return newExit(ExitConfigError, err)
	}
	formatter := output.NewFormatter(format)

	// --no-tools 和 --tools 互斥检查
	if execNoTools && len(execTools) > 0 {
		err := fmt.Errorf("--no-tools 和 --tools 不能同时使用")
		formatter.WriteError(err)
		return newExit(ExitConfigError, err)
	}

	// --work-dir: 切换工作目录
	if execWorkDir != "" {
		if err := os.Chdir(execWorkDir); err != nil {
			wrapped := fmt.Errorf("切换工作目录失败: %w", err)
			formatter.WriteError(wrapped)
			return newExit(ExitConfigError, wrapped)
		}
	}

	// 加载配置
	cfg := config.Load()
	if execModel != "" {
		cfg.Model = execModel
	}

	// 读取输入
	input, err := readExecInput(args)
	if err != nil {
		formatter.WriteError(err)
		return newExit(ExitConfigError, err)
	}

	if input == "" {
		err := fmt.Errorf("未提供输入提示，请通过参数或 stdin 提供")
		formatter.WriteError(err)
		return newExit(ExitError, err)
	}

	// 展开 @file 引用
	expanded := expandFileReferences(input)

	// 创建 Agent
	ag, err := agent.New(cfg)
	if err != nil {
		formatter.WriteError(err)
		return newExit(ExitError, err)
	}
	defer ag.Close()

	// --no-tools: 移除所有工具（纯对话模式）
	if execNoTools {
		registry := ag.GetRegistry()
		if registry != nil {
			for _, name := range registry.List() {
				registry.Unregister(name)
			}
		}
	}

	// --tools: 只保留指定工具
	if len(execTools) > 0 {
		allowed := make(map[string]bool, len(execTools))
		for _, t := range execTools {
			allowed[strings.TrimSpace(t)] = true
		}
		registry := ag.GetRegistry()
		if registry != nil {
			for _, name := range registry.List() {
				if !allowed[name] {
					registry.Unregister(name)
				}
			}
		}
	}

	// 设置上下文和超时
	ctx, cancel := context.WithTimeout(context.Background(), execTimeout)
	defer cancel()

	// 信号处理
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	// High 修复：使用 context 控制 goroutine 生命周期，防止泄漏
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
			// context 已取消，退出 goroutine
		}
	}()

	// 执行
	result, err := ag.ProcessWithResult(ctx, expanded)
	if err != nil {
		if ctx.Err() == context.Canceled {
			wrapped := fmt.Errorf("操作被用户中断")
			formatter.WriteError(wrapped)
			return newExit(ExitInterrupted, wrapped)
		}
		if ctx.Err() == context.DeadlineExceeded {
			wrapped := fmt.Errorf("操作超时（%s）", execTimeout)
			fmt.Fprintf(os.Stderr, "错误: %v\n", wrapped)
			formatter.WriteError(wrapped)
			return newExit(ExitTimeout, wrapped)
		}
		if strings.Contains(err.Error(), "拒绝执行") || strings.Contains(err.Error(), "已被安全模式禁止") {
			formatter.WriteError(err)
			return newExit(ExitError, err)
		}
		formatter.WriteError(err)
		return newExit(ExitError, err)
	}

	// 输出结果
	if format == output.FormatStreamJSON {
		// stream-json: 输出 complete 事件
		completeData := output.CompleteEventData{
			Content:   result.Content,
			ToolsUsed: result.ToolsUsed,
		}
		formatter.WriteEvent("complete", completeData)
	} else if execQuiet {
		// --quiet: 只输出最终结果文本
		fmt.Print(result.Content)
	} else {
		if writeErr := formatter.WriteResult(result); writeErr != nil {
			return newExit(ExitError, writeErr)
		}
	}

	return nil
}

// readExecInput reads input from args or stdin
func readExecInput(args []string) (string, error) {
	// 优先使用命令行参数
	if len(args) > 0 && args[0] != "" {
		return args[0], nil
	}

	// 检查 stdin 是否有数据（管道或重定向）
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", nil
	}
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		// 终端模式，没有管道输入
		return "", nil
	}

	// 从 stdin 读取
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("读取 stdin 失败: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}
