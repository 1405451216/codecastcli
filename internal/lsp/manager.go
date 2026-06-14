// Package lsp 管理 LSP (Language Server Protocol) 服务器实例的启动、检测和生命周期。
// 当前为 stub/skeleton 实现：能够检测和启动 gopls/pyright/typescript-language-server，
// 但实际的 LSP 协议通信尚未实现。
package lsp

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// languageConfig 描述一种语言对应的 LSP 服务器配置
type languageConfig struct {
	ServerCmd    string   // LSP 服务器可执行文件名
	LanguageIDs  []string // LSP Language IDs
	Extensions   []string // 识别该语言的文件扩展名
	ConfigFiles  []string // 项目根目录中标识该语言的配置文件
}

// supportedLanguages 定义支持的语言及其 LSP 服务器配置
var supportedLanguages = map[string]languageConfig{
	"go": {
		ServerCmd:   "gopls",
		LanguageIDs: []string{"go"},
		Extensions:  []string{".go"},
		ConfigFiles: []string{"go.mod", "go.sum"},
	},
	"python": {
		ServerCmd:   "pyright-langserver",
		LanguageIDs: []string{"python"},
		Extensions:  []string{".py", ".pyi", ".pyw"},
		ConfigFiles: []string{"pyproject.toml", "setup.py", "setup.cfg", "requirements.txt", "Pipfile"},
	},
	"typescript": {
		ServerCmd:   "typescript-language-server",
		LanguageIDs: []string{"typescript", "typescriptreact", "javascript", "javascriptreact"},
		Extensions:  []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"},
		ConfigFiles: []string{"tsconfig.json", "package.json", "jsconfig.json"},
	},
}

// Server 表示一个运行中的 LSP 服务器进程
type Server struct {
	Language string
	CmdPath  string
	Process  *os.Process
	Running  bool
	stdin    io.WriteCloser
	stdout   io.ReadCloser
	cmd      *exec.Cmd
	client   *Client
	mu       sync.Mutex // protects stdin writes
}

// Manager 管理 LSP 服务器实例的启动、停止和查询
type Manager struct {
	rootDir  string
	servers  map[string]*Server
	mu       sync.RWMutex
	detected map[string]bool // 已检测到的可用 LSP 服务器
}

// NewManager 创建 LSP Manager 实例，并检测项目语言和可用的 LSP 服务器
func NewManager(rootDir string) *Manager {
	m := &Manager{
		rootDir:  rootDir,
		servers:  make(map[string]*Server),
		detected: make(map[string]bool),
	}
	m.detectAvailableServers()
	return m
}

// detectAvailableServers 使用 exec.LookPath 检测系统中可用的 LSP 服务器
func (m *Manager) detectAvailableServers() {
	for lang, cfg := range supportedLanguages {
		if path, err := exec.LookPath(cfg.ServerCmd); err == nil {
			m.mu.Lock()
			m.detected[lang] = true
			m.mu.Unlock()
			_ = path // 存储路径供后续使用
		}
	}
}

// detectProjectLanguages 检测项目根目录中使用的编程语言
func (m *Manager) detectProjectLanguages() []string {
	var languages []string
	seen := make(map[string]bool)

	for lang, cfg := range supportedLanguages {
		// 检查配置文件
		for _, cf := range cfg.ConfigFiles {
			if _, err := os.Stat(filepath.Join(m.rootDir, cf)); err == nil {
				if !seen[lang] {
					languages = append(languages, lang)
					seen[lang] = true
				}
				break
			}
		}

		// 如果配置文件未检测到，检查源文件扩展名（扫描根目录，不递归）
		if !seen[lang] {
			entries, err := os.ReadDir(m.rootDir)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				ext := filepath.Ext(entry.Name())
				for _, supportedExt := range cfg.Extensions {
					if ext == supportedExt {
						if !seen[lang] {
							languages = append(languages, lang)
							seen[lang] = true
						}
						break
					}
				}
				if seen[lang] {
					break
				}
			}
		}
	}

	return languages
}

// StartServer 启动指定语言的 LSP 服务器并完成 initialize 握手
func (m *Manager) StartServer(language string) error {
	lang := strings.ToLower(language)

	cfg, ok := supportedLanguages[lang]
	if !ok {
		return fmt.Errorf("不支持的语言: %q（支持: go, python, typescript）", language)
	}

	// 检查服务器是否已在运行
	m.mu.RLock()
	if srv, exists := m.servers[lang]; exists && srv.Running {
		m.mu.RUnlock()
		return nil
	}
	m.mu.RUnlock()

	// 检测 LSP 服务器是否安装
	cmdPath, err := exec.LookPath(cfg.ServerCmd)
	if err != nil {
		return fmt.Errorf("LSP 服务器 %q 未安装或不在 PATH 中。请先安装: %s",
			cfg.ServerCmd, installHint(lang))
	}

	// 启动 LSP 服务器进程（stdio 模式）
	cmd := exec.Command(cmdPath, "--stdio")
	cmd.Dir = m.rootDir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("创建 LSP stdin 管道失败: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("创建 LSP stdout 管道失败: %w", err)
	}
	cmd.Stderr = nil // 丢弃 stderr 噪音

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 LSP 服务器 %q 失败: %w", cfg.ServerCmd, err)
	}

	srv := &Server{
		Language: lang,
		CmdPath:  cmdPath,
		Process:  cmd.Process,
		Running:  true,
		stdin:    stdin,
		stdout:   stdout,
		cmd:      cmd,
	}

	// 创建 Client 并执行 LSP initialize 握手
	client := newClient(srv)
	rootURI := FileURI(m.rootDir)
	if err := client.Start(rootURI); err != nil {
		// 握手失败，清理进程
		_ = cmd.Process.Kill()
		srv.Running = false
		return fmt.Errorf("LSP initialize 握手失败: %w", err)
	}
	srv.client = client

	m.mu.Lock()
	m.servers[lang] = srv
	m.mu.Unlock()

	return nil
}

// StopAll 停止所有运行中的 LSP 服务器
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for lang, srv := range m.servers {
		if srv.client != nil {
			srv.client.Shutdown()
			srv.client.Stop()
		}
		if srv.Running && srv.Process != nil {
			_ = srv.Process.Kill()
			srv.Running = false
		}
		delete(m.servers, lang)
	}
}

// IsAvailable 检查指定语言的 LSP 服务器是否正在运行
func (m *Manager) IsAvailable(language string) bool {
	lang := strings.ToLower(language)
	m.mu.RLock()
	defer m.mu.RUnlock()

	srv, exists := m.servers[lang]
	return exists && srv.Running
}

// IsInstalled 检查指定语言的 LSP 服务器是否已安装
func (m *Manager) IsInstalled(language string) bool {
	lang := strings.ToLower(language)
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.detected[lang]
}

// AvailableServers 返回已安装的 LSP 服务器列表
func (m *Manager) AvailableServers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var available []string
	for lang := range m.detected {
		available = append(available, lang)
	}
	return available
}

// ProjectLanguages 返回项目检测到的编程语言列表
func (m *Manager) ProjectLanguages() []string {
	return m.detectProjectLanguages()
}

// ServerInfo 返回指定语言 LSP 服务器的信息
func (m *Manager) ServerInfo(language string) (cmdPath string, running bool, found bool) {
	lang := strings.ToLower(language)
	m.mu.RLock()
	defer m.mu.RUnlock()

	srv, exists := m.servers[lang]
	if !exists {
		return "", false, false
	}
	return srv.CmdPath, srv.Running, true
}

// installHint 返回 LSP 服务器的安装提示
func installHint(language string) string {
	switch language {
	case "go":
		return "go install golang.org/x/tools/gopls@latest"
	case "python":
		return "npm install -g pyright"
	case "typescript":
		return "npm install -g typescript-language-server typescript"
	default:
		return ""
	}
}

// GetClient 返回指定语言的 LSP Client，如果服务器未运行则返回 nil
func (m *Manager) GetClient(language string) *Client {
	lang := strings.ToLower(language)
	m.mu.RLock()
	defer m.mu.RUnlock()

	srv, exists := m.servers[lang]
	if !exists || !srv.Running {
		return nil
	}
	return srv.client
}

// RestartServer 重启指定语言的 LSP 服务器
func (m *Manager) RestartServer(language string) error {
	lang := strings.ToLower(language)

	// 停止现有服务器
	m.mu.Lock()
	if srv, exists := m.servers[lang]; exists {
		if srv.client != nil {
			srv.client.Shutdown()
			srv.client.Stop()
		}
		if srv.Running && srv.Process != nil {
			_ = srv.Process.Kill()
			srv.Running = false
		}
		delete(m.servers, lang)
	}
	m.mu.Unlock()

	// 重新启动
	return m.StartServer(language)
}
