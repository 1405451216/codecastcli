package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/lsp"
	"codecast/cli/internal/util"
)

// LSPTool 提供 LSP (Language Server Protocol) 集成，支持跳转定义、查找引用、
// 悬停类型信息和诊断等功能。
type LSPTool struct {
	manager *lsp.Manager
}

// NewLSPTool 创建 LSPTool 实例
// rootDir 为项目根目录，用于检测语言和启动 LSP 服务器
func NewLSPTool(rootDir string) *LSPTool {
	return &LSPTool{
		manager: lsp.NewManager(rootDir),
	}
}

// Name 返回工具名称
func (t *LSPTool) Name() string {
	return "lsp"
}

// Description 返回工具描述
func (t *LSPTool) Description() string {
	return "LSP 语言服务工具，支持跳转定义、查找引用、悬停类型信息和诊断。需要对应语言的 LSP 服务器已安装并运行。"
}

// lspParams 定义 lsp 工具的参数
type lspParams struct {
	Action    string `json:"action"`
	FilePath  string `json:"file_path,omitempty"`
	Line      int    `json:"line,omitempty"`
	Character int    `json:"character,omitempty"`
	Language  string `json:"language,omitempty"`
}

// Parameters 返回工具参数的 JSON Schema
func (t *LSPTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["goto_definition", "find_references", "hover", "diagnostics"],
				"description": "LSP 操作类型：goto_definition=跳转定义, find_references=查找引用, hover=悬停类型信息, diagnostics=获取诊断信息"
			},
			"file_path": {
				"type": "string",
				"description": "文件路径（goto_definition/find_references/hover/diagnostics 均需要）"
			},
			"line": {
				"type": "integer",
				"description": "行号（0-based），goto_definition/find_references/hover 需要"
			},
			"character": {
				"type": "integer",
				"description": "列号（0-based），goto_definition/find_references/hover 需要"
			},
			"language": {
				"type": "string",
				"description": "编程语言（go/python/typescript），用于选择 LSP 服务器。不提供时根据文件扩展名自动推断"
			}
		},
		"required": ["action", "file_path"]
	}`)
}

// Execute 执行 lsp 工具
func (t *LSPTool) Execute(ctx context.Context, args json.RawMessage) (*ap.ToolResult, error) {
	var params lspParams
	if err := json.Unmarshal(args, &params); err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("参数解析失败: %v", err)), nil
	}

	if params.Action == "" {
		return ap.NewToolErrorResult("action 不能为空，可选值: goto_definition, find_references, hover, diagnostics"), nil
	}

	if params.FilePath == "" {
		return ap.NewToolErrorResult("file_path 不能为空"), nil
	}

	// 路径遍历防护
	if util.HasUnsafePathSegment(params.FilePath) {
		return ap.NewToolErrorResult(fmt.Sprintf("路径不安全: %q 含 \"..\" 段或指向根目录", params.FilePath)), nil
	}

	// 推断语言
	language := params.Language
	if language == "" {
		language = inferLanguage(params.FilePath)
	}
	if language == "" {
		return ap.NewToolErrorResult("无法推断文件语言，请通过 language 参数指定（支持: go, python, typescript）"), nil
	}

	// 确保 LSP 服务器已启动
	client := t.ensureClient(language)
	if client == nil {
		available := t.manager.AvailableServers()
		availStr := strings.Join(available, ", ")
		if availStr == "" {
			availStr = "无"
		}
		return ap.NewToolErrorResult(fmt.Sprintf(
			"LSP 服务不可用（语言: %s）\n已安装的 LSP 服务器: %s",
			language, availStr,
		)), nil
	}

	// 将文件路径转为绝对路径和 URI
	absPath, err := filepath.Abs(params.FilePath)
	if err != nil {
		absPath = params.FilePath
	}
	uri := lsp.FileURI(absPath)

	// 根据 action 分发（带自动重启重试）
	switch params.Action {
	case "goto_definition":
		return t.executeWithRetry(language, func(c *lsp.Client) (*ap.ToolResult, error) {
			return t.executeGotoDefinition(c, uri, params)
		})
	case "find_references":
		return t.executeWithRetry(language, func(c *lsp.Client) (*ap.ToolResult, error) {
			return t.executeFindReferences(c, uri, params)
		})
	case "hover":
		return t.executeWithRetry(language, func(c *lsp.Client) (*ap.ToolResult, error) {
			return t.executeHover(c, uri, params)
		})
	case "diagnostics":
		return t.executeWithRetry(language, func(c *lsp.Client) (*ap.ToolResult, error) {
			return t.executeDiagnostics(c, uri, params)
		})
	default:
		return ap.NewToolErrorResult(fmt.Sprintf("不支持的操作: %q（支持: goto_definition, find_references, hover, diagnostics）", params.Action)), nil
	}
}

// ensureClient 确保指定语言的 LSP 客户端可用，返回 nil 表示不可用
func (t *LSPTool) ensureClient(language string) *lsp.Client {
	client := t.manager.GetClient(language)
	if client != nil {
		return client
	}
	// 尝试启动
	if err := t.manager.StartServer(language); err != nil {
		return nil
	}
	return t.manager.GetClient(language)
}

// executeWithRetry 执行 LSP 操作，如果失败（服务器可能崩溃），重启并重试一次
func (t *LSPTool) executeWithRetry(language string, fn func(*lsp.Client) (*ap.ToolResult, error)) (*ap.ToolResult, error) {
	client := t.manager.GetClient(language)
	if client == nil {
		return ap.NewToolErrorResult(fmt.Sprintf("LSP 客户端不可用（语言: %s）", language)), nil
	}

	result, err := fn(client)
	if err != nil {
		// 可能服务器崩溃，尝试重启并重试
		_ = t.manager.RestartServer(language)
		client = t.manager.GetClient(language)
		if client == nil {
			return ap.NewToolErrorResult(fmt.Sprintf("LSP 服务器重启失败（语言: %s）: %v", language, err)), nil
		}
		result, err = fn(client)
		if err != nil {
			return ap.NewToolErrorResult(fmt.Sprintf("LSP 操作失败（语言: %s）: %v", language, err)), nil
		}
	}
	return result, nil
}

// executeGotoDefinition 处理 goto_definition 操作
func (t *LSPTool) executeGotoDefinition(client *lsp.Client, uri string, params lspParams) (*ap.ToolResult, error) {
	if err := validatePosition(params); err != nil {
		return ap.NewToolErrorResult(err.Error()), nil
	}

	locations, err := client.GotoDefinition(uri, params.Line, params.Character)
	if err != nil {
		return nil, err
	}

	if len(locations) == 0 {
		return ap.NewToolResult("未找到定义"), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("找到 %d 个定义:\n", len(locations)))
	for i, loc := range locations {
		path := uriToPath(loc.URI)
		sb.WriteString(fmt.Sprintf("  %d. %s:%d:%d\n", i+1, path, loc.Range.Start.Line, loc.Range.Start.Character))
	}
	return ap.NewToolResult(sb.String()), nil
}

// executeFindReferences 处理 find_references 操作
func (t *LSPTool) executeFindReferences(client *lsp.Client, uri string, params lspParams) (*ap.ToolResult, error) {
	if err := validatePosition(params); err != nil {
		return ap.NewToolErrorResult(err.Error()), nil
	}

	locations, err := client.FindReferences(uri, params.Line, params.Character)
	if err != nil {
		return nil, err
	}

	if len(locations) == 0 {
		return ap.NewToolResult("未找到引用"), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("找到 %d 个引用:\n", len(locations)))
	for i, loc := range locations {
		path := uriToPath(loc.URI)
		sb.WriteString(fmt.Sprintf("  %d. %s:%d:%d\n", i+1, path, loc.Range.Start.Line, loc.Range.Start.Character))
	}
	return ap.NewToolResult(sb.String()), nil
}

// executeHover 处理 hover 操作
func (t *LSPTool) executeHover(client *lsp.Client, uri string, params lspParams) (*ap.ToolResult, error) {
	if err := validatePosition(params); err != nil {
		return ap.NewToolErrorResult(err.Error()), nil
	}

	text, err := client.Hover(uri, params.Line, params.Character)
	if err != nil {
		return nil, err
	}

	if text == "" {
		return ap.NewToolResult("无悬停信息"), nil
	}
	return ap.NewToolResult(text), nil
}

// executeDiagnostics 处理 diagnostics 操作
func (t *LSPTool) executeDiagnostics(client *lsp.Client, uri string, params lspParams) (*ap.ToolResult, error) {
	diagnostics, err := client.Diagnostics(uri)
	if err != nil {
		return nil, err
	}

	if len(diagnostics) == 0 {
		return ap.NewToolResult("无诊断信息"), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("找到 %d 条诊断:\n", len(diagnostics)))
	for i, d := range diagnostics {
		severity := severityString(d.Severity)
		sb.WriteString(fmt.Sprintf("  %d. [%s] %s:%d:%d - %s\n",
			i+1, severity, uriToPath(uri), d.Range.Start.Line, d.Range.Start.Character, d.Message))
	}
	return ap.NewToolResult(sb.String()), nil
}

// validatePosition 验证需要行号/列号的操作参数
func validatePosition(params lspParams) error {
	if params.Line < 0 {
		return fmt.Errorf("line 不能为负数")
	}
	if params.Character < 0 {
		return fmt.Errorf("character 不能为负数")
	}
	return nil
}

// inferLanguage 根据文件扩展名推断编程语言
func inferLanguage(filePath string) string {
	ext := strings.ToLower(filePath)
	// 处理多段扩展名
	for _, suffix := range []string{".tsx", ".jsx"} {
		if strings.HasSuffix(ext, suffix) {
			return "typescript"
		}
	}

	// 单段扩展名
	switch {
	case strings.HasSuffix(ext, ".go"):
		return "go"
	case strings.HasSuffix(ext, ".py") || strings.HasSuffix(ext, ".pyi") || strings.HasSuffix(ext, ".pyw"):
		return "python"
	case strings.HasSuffix(ext, ".ts") || strings.HasSuffix(ext, ".js") ||
		strings.HasSuffix(ext, ".mjs") || strings.HasSuffix(ext, ".cjs"):
		return "typescript"
	default:
		return ""
	}
}

// severityString 将 LSP 诊断严重级别转为可读字符串
func severityString(severity int) string {
	switch severity {
	case 1:
		return "Error"
	case 2:
		return "Warning"
	case 3:
		return "Information"
	case 4:
		return "Hint"
	default:
		return fmt.Sprintf("Unknown(%d)", severity)
	}
}

// uriToPath 将 file:// URI 转换为本地文件路径
func uriToPath(uri string) string {
	if strings.HasPrefix(uri, "file:///") {
		// Windows: file:///C:/... -> C:/...
		return uri[8:]
	}
	if strings.HasPrefix(uri, "file://") {
		// Unix: file:///path -> /path
		return uri[7:]
	}
	return uri
}
