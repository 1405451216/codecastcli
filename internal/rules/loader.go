package rules

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RuleLevel represents the source level of a rule
type RuleLevel int

const (
	LevelGlobal  RuleLevel = iota // ~/.codecast/rules.md
	LevelProject                  // .codecast/rules.md (project root)
	LevelLocal                    // .codecast/rules.local.md (project root, gitignored)
)

// RuleSet holds all loaded rules
type RuleSet struct {
	Global  string
	Project string
	Local   string
	Merged  string // merged result
}

// Loader loads rules from multiple levels
type Loader struct {
	homeDir     string
	projectRoot string
	maxSize     int // max size in bytes per file
}

// NewLoader creates a rules loader
func NewLoader(projectRoot string) *Loader {
	homeDir, _ := os.UserHomeDir()
	return &Loader{
		homeDir:     homeDir,
		projectRoot: projectRoot,
		maxSize:     32 * 1024, // 32KB per file
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

// merge combines rules from all levels
func (l *Loader) merge(rs *RuleSet) string {
	var parts []string

	if rs.Global != "" {
		parts = append(parts, fmt.Sprintf("[全局规则]\n%s", rs.Global))
	}
	if rs.Project != "" {
		parts = append(parts, fmt.Sprintf("[项目规则]\n%s", rs.Project))
	}
	if rs.Local != "" {
		parts = append(parts, fmt.Sprintf("[本地规则]\n%s", rs.Local))
	}

	return strings.Join(parts, "\n\n")
}

// ApplyTemplateVariables replaces template variables in rules
// Supported: {{PROJECT_ROOT}}, {{HOME}}, {{OS}}
func ApplyTemplateVariables(rules string, projectRoot, homeDir, goos string) string {
	r := strings.ReplaceAll(rules, "{{PROJECT_ROOT}}", projectRoot)
	r = strings.ReplaceAll(r, "{{HOME}}", homeDir)
	r = strings.ReplaceAll(r, "{{OS}}", goos)
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
