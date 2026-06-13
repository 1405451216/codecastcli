package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ServerTemplate MCP 服务器模板
type ServerTemplate struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Command     string            `json:"command"`
	Args        []string          `json:"args"`
	Env         map[string]string `json:"env,omitempty"`
	Category    string            `json:"category"`
	AutoStart   bool              `json:"auto_start"`
}

// BuiltInTemplates 内置 MCP 服务器模板
var BuiltInTemplates = []ServerTemplate{
	{
		Name:        "filesystem",
		Description: "文件系统 MCP 服务器（提供文件读写能力）",
		Command:     "npx",
		Args:        []string{"-y", "@modelcontextprotocol/server-filesystem", "."},
		Category:    "core",
		AutoStart:   false,
	},
	{
		Name:        "github",
		Description: "GitHub MCP 服务器（PR/Issue/代码搜索）",
		Command:     "npx",
		Args:        []string{"-y", "@modelcontextprotocol/server-github"},
		Env:         map[string]string{"GITHUB_PERSONAL_ACCESS_TOKEN": ""},
		Category:    "devops",
		AutoStart:   false,
	},
	{
		Name:        "gitlab",
		Description: "GitLab MCP 服务器",
		Command:     "npx",
		Args:        []string{"-y", "@modelcontextprotocol/server-gitlab"},
		Env:         map[string]string{"GITLAB_PERSONAL_ACCESS_TOKEN": ""},
		Category:    "devops",
		AutoStart:   false,
	},
	{
		Name:        "postgres",
		Description: "PostgreSQL 数据库 MCP 服务器",
		Command:     "npx",
		Args:        []string{"-y", "@modelcontextprotocol/server-postgres"},
		Env:         map[string]string{"POSTGRES_CONNECTION_STRING": ""},
		Category:    "database",
		AutoStart:   false,
	},
	{
		Name:        "sqlite",
		Description: "SQLite 数据库 MCP 服务器",
		Command:     "npx",
		Args:        []string{"-y", "@modelcontextprotocol/server-sqlite"},
		Category:    "database",
		AutoStart:   false,
	},
	{
		Name:        "brave-search",
		Description: "Brave 搜索 MCP 服务器",
		Command:     "npx",
		Args:        []string{"-y", "@modelcontextprotocol/server-brave-search"},
		Env:         map[string]string{"BRAVE_API_KEY": ""},
		Category:    "search",
		AutoStart:   false,
	},
	{
		Name:        "puppeteer",
		Description: "Puppeteer 浏览器自动化 MCP 服务器",
		Command:     "npx",
		Args:        []string{"-y", "@modelcontextprotocol/server-puppeteer"},
		Category:    "browser",
		AutoStart:   false,
	},
	{
		Name:        "slack",
		Description: "Slack MCP 服务器",
		Command:     "npx",
		Args:        []string{"-y", "@modelcontextprotocol/server-slack"},
		Env:         map[string]string{"SLACK_BOT_TOKEN": "", "SLACK_TEAM_ID": ""},
		Category:    "communication",
		AutoStart:   false,
	},
	{
		Name:        "memory",
		Description: "记忆存储 MCP 服务器",
		Command:     "npx",
		Args:        []string{"-y", "@modelcontextprotocol/server-memory"},
		Category:    "core",
		AutoStart:   false,
	},
	{
		Name:        "sequential-thinking",
		Description: "顺序思考 MCP 服务器（增强推理能力）",
		Command:     "npx",
		Args:        []string{"-y", "@modelcontextprotocol/server-sequential-thinking"},
		Category:    "reasoning",
		AutoStart:   false,
	},
}

// TemplateManager 模板管理器
type TemplateManager struct {
	templatesDir string
}

// NewTemplateManager 创建模板管理器
func NewTemplateManager(configDir string) *TemplateManager {
	return &TemplateManager{
		templatesDir: filepath.Join(configDir, "mcp-templates"),
	}
}

// ListTemplates 列出可用模板
func ListTemplates() []ServerTemplate {
	return BuiltInTemplates
}

// ListTemplatesByCategory 按类别列出模板
func ListTemplatesByCategory(category string) []ServerTemplate {
	var result []ServerTemplate
	for _, t := range BuiltInTemplates {
		if t.Category == category {
			result = append(result, t)
		}
	}
	return result
}

// GetTemplate 获取指定模板
func GetTemplate(name string) (*ServerTemplate, bool) {
	for _, t := range BuiltInTemplates {
		if t.Name == name {
			return &t, true
		}
	}
	return nil, false
}

// GetCategories 获取所有类别
func GetCategories() []string {
	seen := make(map[string]bool)
	var categories []string
	for _, t := range BuiltInTemplates {
		if !seen[t.Category] {
			seen[t.Category] = true
			categories = append(categories, t.Category)
		}
	}
	return categories
}

// SaveTemplateConfig 保存模板配置到 .codecast/mcp.json
func SaveTemplateConfig(configDir string, templates []ServerTemplate) error {
	mcpDir := filepath.Join(configDir, ".codecast")
	if err := os.MkdirAll(mcpDir, 0755); err != nil {
		return err
	}

	config := map[string]any{
		"mcpServers": templates,
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(mcpDir, "mcp.json"), data, 0644)
}
