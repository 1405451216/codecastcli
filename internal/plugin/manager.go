package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	ap "agentprimordia/pkg"
)

// Manager 插件管理器
type Manager struct {
	loader     *ap.PluginLoader
	pluginsDir string
	mu         sync.RWMutex
}

// NewManager 创建插件管理器
func NewManager(registry *ap.ToolRegistry, pluginsDir string) *Manager {
	return &Manager{
		loader:     ap.NewPluginLoader(registry),
		pluginsDir: pluginsDir,
	}
}

// RegisterPlugin 注册内置插件（进程内）
func (m *Manager) RegisterPlugin(plugin ap.ToolPlugin, config map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loader.LoadWithConfig(plugin, config)
}

// UnloadPlugin 卸载插件
func (m *Manager) UnloadPlugin(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loader.Unload(name)
}

// ListPlugins 列出所有已加载的插件
func (m *Manager) ListPlugins() []ap.PluginInfo {
	return m.loader.List()
}

// ListAvailable 列出可用但未加载的插件
func (m *Manager) ListAvailable() []PluginMeta {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var metas []PluginMeta
	entries, err := os.ReadDir(m.pluginsDir)
	if err != nil {
		return metas
	}

	loaded := make(map[string]bool)
	for _, info := range m.loader.List() {
		loaded[info.Name] = true
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		metaPath := filepath.Join(m.pluginsDir, entry.Name(), "plugin.json")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}

		var meta PluginMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}

		meta.Loaded = loaded[meta.Name]
		metas = append(metas, meta)
	}

	return metas
}

// InstallPlugin 安装插件（从目录复制）
func (m *Manager) InstallPlugin(sourceDir string) error {
	metaPath := filepath.Join(sourceDir, "plugin.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return fmt.Errorf("读取插件元数据失败: %w", err)
	}

	var meta PluginMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return fmt.Errorf("解析插件元数据失败: %w", err)
	}

	destDir := filepath.Join(m.pluginsDir, meta.Name)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("创建插件目录失败: %w", err)
	}

	// 复制 plugin.json
	jsonDest := filepath.Join(destDir, "plugin.json")
	if err := os.WriteFile(jsonDest, data, 0644); err != nil {
		return fmt.Errorf("写入插件元数据失败: %w", err)
	}

	return nil
}

// PluginMeta 插件元数据
type PluginMeta struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      string `json:"author,omitempty"`
	Loaded      bool   `json:"loaded,omitempty"`
}
