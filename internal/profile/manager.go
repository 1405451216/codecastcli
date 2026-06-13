package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Profile 配置档案
type Profile struct {
	Name         string `json:"name"`
	Model        string `json:"model,omitempty"`
	Provider     string `json:"provider,omitempty"`
	BaseURL      string `json:"base_url,omitempty"`
	APIKey       string `json:"api_key,omitempty"`
	Permission   string `json:"permission,omitempty"`
	SafeMode     bool   `json:"safe_mode,omitempty"`
	SummaryModel string `json:"summary_model,omitempty"`
}

// Manager Profile 管理器
type Manager struct {
	profilesDir string
	active      string
	mu          sync.RWMutex
}

// NewManager 创建 Profile 管理器
func NewManager(configDir string) *Manager {
	return &Manager{
		profilesDir: filepath.Join(configDir, "profiles"),
		active:      "default",
	}
}

// Save 保存 Profile
func (m *Manager) Save(profile Profile) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(m.profilesDir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(m.profilesDir, profile.Name+".json")
	return os.WriteFile(path, data, 0644)
}

// Load 加载 Profile
func (m *Manager) Load(name string) (*Profile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	path := filepath.Join(m.profilesDir, name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("Profile %q 不存在", name)
	}

	var profile Profile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("解析 Profile 失败: %w", err)
	}

	return &profile, nil
}

// List 列出所有 Profile
func (m *Manager) List() ([]Profile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := os.MkdirAll(m.profilesDir, 0755); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(m.profilesDir)
	if err != nil {
		return nil, err
	}

	var profiles []Profile
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(m.profilesDir, entry.Name()))
		if err != nil {
			continue
		}

		var profile Profile
		if err := json.Unmarshal(data, &profile); err != nil {
			continue
		}

		profiles = append(profiles, profile)
	}

	return profiles, nil
}

// Delete 删除 Profile
func (m *Manager) Delete(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	path := filepath.Join(m.profilesDir, name+".json")
	return os.Remove(path)
}

// SetActive 设置活跃 Profile
func (m *Manager) SetActive(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active = name
}

// GetActive 获取活跃 Profile 名称
func (m *Manager) GetActive() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}
