package plugin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// RegistryEntry 注册表条目
type RegistryEntry struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Author      string   `json:"author"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
	DownloadURL string   `json:"download_url"`
	Homepage    string   `json:"homepage"`
	Stars       int      `json:"stars"`
	Downloads   int      `json:"downloads"`
	UpdatedAt   string   `json:"updated_at"`
}

// Registry 插件注册表
type Registry struct {
	baseURL  string
	client   *http.Client
	cacheDir string
	cacheTTL time.Duration
}

// NewRegistry 创建插件注册表
func NewRegistry(cacheDir string) *Registry {
	return &Registry{
		baseURL: "https://registry.codecast.dev/api/v1",
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		cacheDir: cacheDir,
		cacheTTL: 1 * time.Hour,
	}
}

// WithBaseURL 设置注册表 URL
func (r *Registry) WithBaseURL(url string) *Registry {
	r.baseURL = url
	return r
}

// Search 搜索插件
func (r *Registry) Search(query string) ([]RegistryEntry, error) {
	// 先尝试从缓存加载
	if entries, err := r.loadCache("search_" + query); err == nil {
		return entries, nil
	}

	// 从远程获取
	resp, err := r.client.Get(fmt.Sprintf("%s/plugins/search?q=%s", r.baseURL, query))
	if err != nil {
		// 回退到本地内置列表
		return r.builtinEntries(), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return r.builtinEntries(), nil
	}

	var entries []RegistryEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("解析搜索结果失败: %w", err)
	}

	// 缓存结果
	r.saveCache("search_"+query, entries)

	return entries, nil
}

// GetPlugin 获取插件详情
func (r *Registry) GetPlugin(name string) (*RegistryEntry, error) {
	resp, err := r.client.Get(fmt.Sprintf("%s/plugins/%s", r.baseURL, name))
	if err != nil {
		return nil, fmt.Errorf("获取插件信息失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("插件 %q 不存在", name)
	}

	var entry RegistryEntry
	if err := json.NewDecoder(resp.Body).Decode(&entry); err != nil {
		return nil, fmt.Errorf("解析插件信息失败: %w", err)
	}

	return &entry, nil
}

// ListPopular 列出热门插件
func (r *Registry) ListPopular() ([]RegistryEntry, error) {
	if entries, err := r.loadCache("popular"); err == nil {
		return entries, nil
	}

	resp, err := r.client.Get(fmt.Sprintf("%s/plugins/popular", r.baseURL))
	if err != nil {
		return r.builtinEntries(), nil
	}
	defer resp.Body.Close()

	var entries []RegistryEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return r.builtinEntries(), nil
	}

	r.saveCache("popular", entries)
	return entries, nil
}

// Download 下载插件
func (r *Registry) Download(entry *RegistryEntry, destDir string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	resp, err := r.client.Get(entry.DownloadURL)
	if err != nil {
		return fmt.Errorf("下载插件失败: %w", err)
	}
	defer resp.Body.Close()

	destPath := filepath.Join(destDir, entry.Name, "plugin.json")
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	// 保存元数据
	meta, _ := json.MarshalIndent(entry, "", "  ")
	if err := os.WriteFile(destPath, meta, 0644); err != nil {
		return err
	}

	// 保存下载内容
	dataPath := filepath.Join(destDir, entry.Name, "plugin_data.tar.gz")
	f, err := os.Create(dataPath)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("写入插件数据失败: %w", err)
	}

	return nil
}

// loadCache 从缓存加载
func (r *Registry) loadCache(key string) ([]RegistryEntry, error) {
	if r.cacheDir == "" {
		return nil, fmt.Errorf("no cache dir")
	}

	cachePath := filepath.Join(r.cacheDir, key+".json")
	info, err := os.Stat(cachePath)
	if err != nil {
		return nil, err
	}

	if time.Since(info.ModTime()) > r.cacheTTL {
		return nil, fmt.Errorf("cache expired")
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err
	}

	var entries []RegistryEntry
	return entries, json.Unmarshal(data, &entries)
}

// saveCache 保存到缓存
func (r *Registry) saveCache(key string, entries []RegistryEntry) {
	if r.cacheDir == "" {
		return
	}

	os.MkdirAll(r.cacheDir, 0755)
	data, _ := json.MarshalIndent(entries, "", "  ")
	os.WriteFile(filepath.Join(r.cacheDir, key+".json"), data, 0644)
}

// builtinEntries 内置插件列表（离线回退）
func (r *Registry) builtinEntries() []RegistryEntry {
	return []RegistryEntry{
		{Name: "code-formatter", Version: "1.0.0", Description: "代码格式化工具", Category: "formatting", Stars: 42, Downloads: 1200},
		{Name: "security-scanner", Version: "1.2.0", Description: "安全扫描工具", Category: "security", Stars: 89, Downloads: 3400},
		{Name: "test-runner", Version: "2.0.0", Description: "测试运行器", Category: "testing", Stars: 156, Downloads: 8900},
		{Name: "doc-generator", Version: "1.1.0", Description: "文档生成器", Category: "documentation", Stars: 67, Downloads: 2300},
		{Name: "api-client", Version: "1.3.0", Description: "API 客户端工具", Category: "network", Stars: 34, Downloads: 980},
	}
}
