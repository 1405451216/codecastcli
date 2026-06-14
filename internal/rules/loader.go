package rules

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// RuleLevel represents the source level of a rule
type RuleLevel int

const (
	LevelGlobal  RuleLevel = iota // ~/.codecast/rules.md
	LevelProject                  // .codecast/rules.md (project root)
	LevelLocal                    // .codecast/rules.local.md (project root, gitignored)
)

// SubModuleRule holds a single sub-module rule loaded from .codecast/rules/*.md
type SubModuleRule struct {
	Filename string // e.g. "backend.md"
	Content  string
	Size     int // original size in bytes before truncation
}

// RuleSet holds all loaded rules
type RuleSet struct {
	Global     string
	Project    string
	Local      string
	SubModules []SubModuleRule // rules from .codecast/rules/*.md
	Merged     string          // merged result
}

// Loader loads rules from multiple levels
type Loader struct {
	homeDir         string
	projectRoot     string
	maxSize         int // max size in bytes per file
	maxSubModuleSize int // max total size in bytes for all sub-module rules combined
}

// NewLoader creates a rules loader
func NewLoader(projectRoot string) *Loader {
	homeDir, _ := os.UserHomeDir()
	return &Loader{
		homeDir:          homeDir,
		projectRoot:      projectRoot,
		maxSize:          32 * 1024, // 32KB per file
		maxSubModuleSize: 16 * 1024, // 16KB total for sub-module rules
	}
}

// WithMaxSize sets the max size per rules file
func (l *Loader) WithMaxSize(maxSize int) *Loader {
	l.maxSize = maxSize
	return l
}

// Load loads rules from all levels and merges them
func (l *Loader) Load() (*RuleSet, error) {
	rs := &RuleSet{}

	// Load global rules
	globalPath := filepath.Join(l.homeDir, ".codecast", "rules.md")
	rs.Global = l.loadFile(globalPath)

	// Load project rules
	projectPath := filepath.Join(l.projectRoot, ".codecast", "rules.md")
	rs.Project = l.loadFile(projectPath)

	// Load local rules
	localPath := filepath.Join(l.projectRoot, ".codecast", "rules.local.md")
	rs.Local = l.loadFile(localPath)

	// Load sub-module rules from .codecast/rules/*.md
	rs.SubModules = l.loadSubModules()

	// Merge
	rs.Merged = l.merge(rs)

	return rs, nil
}

// loadFile reads a rules file with size limit
func (l *Loader) loadFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(data) > l.maxSize {
		data = data[:l.maxSize]
	}
	return string(data)
}

// loadSubModules loads all .md files from .codecast/rules/ directory in alphabetical order
func (l *Loader) loadSubModules() []SubModuleRule {
	rulesDir := filepath.Join(l.projectRoot, ".codecast", "rules")

	entries, err := os.ReadDir(rulesDir)
	if err != nil {
		// Directory doesn't exist or can't be read — silently skip
		return nil
	}

	// Filter and sort .md files alphabetically by filename
	var mdFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".md") {
			mdFiles = append(mdFiles, entry.Name())
		}
	}
	sort.Strings(mdFiles)

	var result []SubModuleRule
	totalSize := 0

	for _, name := range mdFiles {
		path := filepath.Join(rulesDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		origSize := len(data)

		// Check total size limit
		if totalSize+origSize > l.maxSubModuleSize {
			remaining := l.maxSubModuleSize - totalSize
			if remaining > 0 {
				data = data[:remaining]
				log.Printf("警告: 子模块规则总大小超过 %dKB 限制，%s 已被截断", l.maxSubModuleSize/1024, name)
			} else {
				log.Printf("警告: 子模块规则总大小超过 %dKB 限制，%s 已被跳过", l.maxSubModuleSize/1024, name)
				continue
			}
		}
		totalSize += len(data)

		result = append(result, SubModuleRule{
			Filename: name,
			Content:  string(data),
			Size:     origSize,
		})
	}

	return result
}

// merge combines rules from all levels
func (l *Loader) merge(rs *RuleSet) string {
	var parts []string

	if rs.Global != "" {
		parts = append(parts, fmt.Sprintf("[全局规则]\n%s", rs.Global))
	}
	if rs.Project != "" {
		parts = append(parts, fmt.Sprintf("[项目规则]\n%s", rs.Project))
	}
	for _, sm := range rs.SubModules {
		parts = append(parts, fmt.Sprintf("=== 项目子模块规则: %s ===\n%s", sm.Filename, sm.Content))
	}
	if rs.Local != "" {
		parts = append(parts, fmt.Sprintf("[本地规则]\n%s", rs.Local))
	}

	return strings.Join(parts, "\n\n")
}

// ApplyTemplateVariables replaces template variables in rules
// Supported: {{.ProjectName}}, {{.CWD}}, {{.Date}}, {{.OS}}, {{.Arch}}, {{PROJECT_ROOT}}, {{HOME}}, {{OS}}
func ApplyTemplateVariables(rules string, projectRoot, homeDir, goos string) string {
	r := strings.ReplaceAll(rules, "{{PROJECT_ROOT}}", projectRoot)
	r = strings.ReplaceAll(r, "{{HOME}}", homeDir)
	r = strings.ReplaceAll(r, "{{OS}}", goos)

	// New-style template variables
	r = strings.ReplaceAll(r, "{{.ProjectName}}", filepath.Base(projectRoot))
	r = strings.ReplaceAll(r, "{{.CWD}}", projectRoot)
	r = strings.ReplaceAll(r, "{{.Date}}", time.Now().Format("2006-01-02"))
	r = strings.ReplaceAll(r, "{{.OS}}", runtime.GOOS)
	r = strings.ReplaceAll(r, "{{.Arch}}", runtime.GOARCH)

	return r
}

// InitProject creates .codecast directory and rules.md template
func InitProject(projectRoot string) error {
	codecastDir := filepath.Join(projectRoot, ".codecast")
	if err := os.MkdirAll(codecastDir, 0755); err != nil {
		return fmt.Errorf("创建 .codecast 目录失败: %w", err)
	}

	rulesPath := filepath.Join(codecastDir, "rules.md")
	if _, err := os.Stat(rulesPath); err == nil {
		return fmt.Errorf("rules.md 已存在，跳过创建")
	}

	template := `# 项目规则

这些规则会在每次对话中自动注入到系统提示词中。

## 代码风格
- 遵循项目现有的代码风格
- 优先使用标准库

## 测试
- 修改代码后运行相关测试

## 禁止事项
- 不要删除现有测试
- 不要修改 .codecast/ 目录外的配置文件
`

	if err := os.WriteFile(rulesPath, []byte(template), 0644); err != nil {
		return fmt.Errorf("写入 rules.md 失败: %w", err)
	}

	// Create .gitignore for local rules
	gitignorePath := filepath.Join(codecastDir, ".gitignore")
	gitignoreContent := "rules.local.md\n"
	if err := os.WriteFile(gitignorePath, []byte(gitignoreContent), 0644); err != nil {
		// non-fatal
	}

	return nil
}

// GetRulesPath returns the path to the project rules file
func GetRulesPath(projectRoot string) string {
	return filepath.Join(projectRoot, ".codecast", "rules.md")
}
