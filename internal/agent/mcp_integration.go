package agent

import (
	"context"
	"log/slog"
	"time"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/mcpcfg"
)

// ConnectMCPServers 加载 MCP 配置并连接所有 auto_start 的服务器，
// 将发现的工具注册到 ToolRegistry 中。
// 返回 MCPRegistry 实例 + 启动过程中累积的警告（F-04 修复：
// 之前只 slog.Warn，用户看不见；现在收集到 warnings 切片供调用方展示）。
// 返回 MCPRegistry 实例，供关闭时调用 StopAll。
func ConnectMCPServers(registry *ap.ToolRegistry) (*ap.MCPRegistry, []MCPWarning, error) {
	cfg, loadErr := mcpcfg.Load()
	if loadErr != nil {
		slog.Warn("MCP 配置加载失败", "error", loadErr)
		// 配置错误不阻塞 Agent，但必须告知用户
		return nil, []MCPWarning{{Server: "<config>", Err: loadErr.Error()}}, nil
	}
	if len(cfg.Servers) == 0 {
		return nil, nil, nil
	}

	var warnings []MCPWarning
	mcpRegistry := ap.NewMCPRegistry()

	// 注册所有 auto_start 的服务器
	for name, srv := range cfg.Servers {
		if !srv.AutoStart {
			continue
		}
		if srv.Command == "" && srv.BaseURL == "" {
			slog.Warn("MCP 服务器配置无效，跳过", "name", name, "reason", "command 和 base_url 均为空")
			warnings = append(warnings, MCPWarning{
				Server: name,
				Err:    "command 和 base_url 均为空",
			})
			continue
		}

		mcpRegistry.Register(ap.MCPClientConfig{
			Name:      name,
			Command:   srv.Command,
			Args:      srv.Args,
			BaseURL:   srv.BaseURL,
			AutoStart: true,
		})
	}

	// 启动所有已注册的服务器（10 秒超时）
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := mcpRegistry.StartAll(ctx); err != nil {
		// 部分服务器启动失败：把错误暴露给用户，不再静默
		slog.Warn("部分 MCP 服务器启动失败", "error", err)
		warnings = append(warnings, MCPWarning{
			Server: "<startall>",
			Err:    err.Error(),
		})
	}

	// 将所有运行中的 MCP 工具注册到 ToolRegistry
	if err := mcpRegistry.RegisterIntoRegistry(registry); err != nil {
		slog.Warn("MCP 工具注册失败", "error", err)
		warnings = append(warnings, MCPWarning{
			Server: "<register>",
			Err:    err.Error(),
		})
	}

	return mcpRegistry, warnings, nil
}

// MCPWarning 描述一次 MCP 启动/注册的非致命错误（F-04）。
type MCPWarning struct {
	Server string
	Err    string
}
