package indexer

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Tag represents a code symbol (function, class, method, etc.) extracted from a source file.
//
// When CGO is enabled, tags are populated by tree-sitter AST parsing via
// extractTagsAST. Otherwise, regex-based extraction in extractTagsRegex serves
// as the fallback implementation.
type Tag struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`      // function, class, interface, method, variable, struct
	Line      int    `json:"line"`      // 1-based line number
	Signature string `json:"signature"` // full signature string
	Receiver  string `json:"receiver"`  // Go: receiver type; OOP: class name for methods
}

// extractTags extracts code tags from the given file content.
// It first tries AST-based extraction via tree-sitter (available when CGO is
// enabled). If that returns nil (unsupported language or CGO unavailable), it
// falls back to regex-based extraction.
func extractTags(path string, content []byte, language string) []Tag {
	// Try AST-based extraction first (available when CGO is enabled)
	if tags := extractTagsAST(path, content, language); tags != nil {
		return tags
	}
	// Fall back to regex-based extraction
	return extractTagsRegex(path, content, language)
}

// ExtractTags 公开导出的符号提取入口。
// 供 semantic 包等外部调用方使用，避免直接暴露内部 extractTags。
//
// 参数：
//   - path: 文件路径（用于语言推断，若 language 已知可传空）
//   - content: 文件内容
//   - language: 已知语言（空则按 path 扩展名推断）
func ExtractTags(path string, content []byte, language string) []Tag {
	if language == "" {
		language = detectLanguageByExt(path)
	}
	return extractTags(path, content, language)
}

// detectLanguageByExt 按扩展名推断语言（供 ExtractTags 使用）
func detectLanguageByExt(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js", ".jsx", ".mjs":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	default:
		return ""
	}
}

// extractTagsRegex extracts code tags from the given file content using
// regex-based parsing. The language parameter determines which extraction
// rules to apply.
func extractTagsRegex(path string, content []byte, language string) []Tag {
	src := string(content)
	switch language {
	case "go":
		return extractGoTagsRegex(src)
	case "python":
		return extractPythonTagsRegex(src)
	case "javascript", "typescript":
		return extractJSTagsRegex(src)
	default:
		return nil
	}
}

// ---------------------------------------------------------------------------
// Go tag extraction
// ---------------------------------------------------------------------------

var (
	// Go method: func (recv) Name(params) returns
	goMethodRe = regexp.MustCompile(`^func\s+\(\s*(\w+)\s+\*?(\w+)\s*\)\s+(\w+)\s*\(([^)]*)\)\s*(.*)$`)
	// Go function: func Name(params) returns
	goFuncRe = regexp.MustCompile(`^func\s+(\w+)\s*\(([^)]*)\)\s*(.*)$`)
	// Go interface type
	goInterfaceRe = regexp.MustCompile(`^type\s+(\w+)\s+interface\s*\{`)
	// Go struct type
	goStructRe = regexp.MustCompile(`^type\s+(\w+)\s+struct\s*\{`)
	// Go type alias / other type
	goTypeRe = regexp.MustCompile(`^type\s+(\w+)\s+`)
	// Go var block entry
	goVarRe = regexp.MustCompile(`^var\s+(\w+)\s+`)
)

func extractGoTagsRegex(src string) []Tag {
	var tags []Tag
	lines := strings.Split(src, "\n")

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip comments and blank lines
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		// Method with receiver
		if m := goMethodRe.FindStringSubmatch(trimmed); m != nil {
			recvName, recvType, name, params, ret := m[1], m[2], m[3], m[4], strings.TrimSpace(m[5])
			sig := fmt.Sprintf("func (%s *%s) %s(%s)", recvName, recvType, name, params)
			if ret != "" {
				sig += " " + ret
			}
			tags = append(tags, Tag{
				Name:      name,
				Kind:      "method",
				Line:      i + 1,
				Signature: sig,
				Receiver:  recvType,
			})
			continue
		}

		// Free function
		if m := goFuncRe.FindStringSubmatch(trimmed); m != nil {
			name, params, ret := m[1], m[2], strings.TrimSpace(m[3])
			// Skip "func (" which is a method handled above (shouldn't match, but guard)
			sig := fmt.Sprintf("func %s(%s)", name, params)
			if ret != "" {
				sig += " " + ret
			}
			tags = append(tags, Tag{
				Name:      name,
				Kind:      "function",
				Line:      i + 1,
				Signature: sig,
			})
			continue
		}

		// Interface
		if m := goInterfaceRe.FindStringSubmatch(trimmed); m != nil {
			tags = append(tags, Tag{
				Name:      m[1],
				Kind:      "interface",
				Line:      i + 1,
				Signature: fmt.Sprintf("type %s interface", m[1]),
			})
			continue
		}

		// Struct
		if m := goStructRe.FindStringSubmatch(trimmed); m != nil {
			tags = append(tags, Tag{
				Name:      m[1],
				Kind:      "struct",
				Line:      i + 1,
				Signature: fmt.Sprintf("type %s struct", m[1]),
			})
			continue
		}

		// Top-level var (only exported)
		if m := goVarRe.FindStringSubmatch(trimmed); m != nil {
			if len(m[1]) > 0 && m[1][0] >= 'A' && m[1][0] <= 'Z' {
				tags = append(tags, Tag{
					Name:      m[1],
					Kind:      "variable",
					Line:      i + 1,
					Signature: fmt.Sprintf("var %s", m[1]),
				})
			}
			continue
		}
	}

	return tags
}

// ---------------------------------------------------------------------------
// Python tag extraction
// ---------------------------------------------------------------------------

var (
	pyClassRe    = regexp.MustCompile(`^class\s+(\w+)(?:\(([^)]*)\))?\s*:`)
	pyFuncRe     = regexp.MustCompile(`^def\s+(\w+)\s*\(([^)]*)\)\s*:`)
	pyMethodRe   = regexp.MustCompile(`^\s+def\s+(\w+)\s*\(([^)]*)\)\s*:`)
	pyAsyncFuncRe = regexp.MustCompile(`^async\s+def\s+(\w+)\s*\(([^)]*)\)\s*:`)
)

func extractPythonTagsRegex(src string) []Tag {
	var tags []Tag
	lines := strings.Split(src, "\n")

	// Track the current class for method receiver
	currentClass := ""

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Detect indentation reset (leaving class scope)
		if line != "" && line[0] != ' ' && line[0] != '\t' {
			currentClass = ""
		}

		// Class definition
		if m := pyClassRe.FindStringSubmatch(trimmed); m != nil {
			name := m[1]
			base := strings.TrimSpace(m[2])
			sig := fmt.Sprintf("class %s", name)
			if base != "" {
				sig += fmt.Sprintf("(%s)", base)
			}
			tags = append(tags, Tag{
				Name:      name,
				Kind:      "class",
				Line:      i + 1,
				Signature: sig,
			})
			currentClass = name
			continue
		}

		// Async function
		if m := pyAsyncFuncRe.FindStringSubmatch(trimmed); m != nil {
			name, params := m[1], m[2]
			tags = append(tags, Tag{
				Name:      name,
				Kind:      "function",
				Line:      i + 1,
				Signature: fmt.Sprintf("async def %s(%s)", name, params),
			})
			continue
		}

		// Top-level function (no leading whitespace)
		if m := pyFuncRe.FindStringSubmatch(trimmed); m != nil {
			name, params := m[1], m[2]
			tags = append(tags, Tag{
				Name:      name,
				Kind:      "function",
				Line:      i + 1,
				Signature: fmt.Sprintf("def %s(%s)", name, params),
			})
			continue
		}

		// Method (indented def)
		if m := pyMethodRe.FindStringSubmatch(line); m != nil {
			name, params := m[1], m[2]
			kind := "method"
			receiver := currentClass
			// Classify: if first param is self or cls, it's a method
			sig := fmt.Sprintf("def %s(%s)", name, params)
			if currentClass != "" {
				sig = fmt.Sprintf("def %s(%s)", name, params)
			}
			tags = append(tags, Tag{
				Name:      name,
				Kind:      kind,
				Line:      i + 1,
				Signature: sig,
				Receiver:  receiver,
			})
			continue
		}
	}

	return tags
}

// ---------------------------------------------------------------------------
// JavaScript / TypeScript tag extraction
// ---------------------------------------------------------------------------

var (
	jsFuncDeclRe    = regexp.MustCompile(`^export\s+(?:async\s+)?function\s+(\w+)\s*\(([^)]*)\)`)
	jsFuncDeclNoExp = regexp.MustCompile(`^(?:async\s+)?function\s+(\w+)\s*\(([^)]*)\)`)
	jsArrowFuncRe   = regexp.MustCompile(`(?:export\s+(?:const|let|var)\s+|const\s+|let\s+|var\s+)(\w+)\s*=\s*(?:async\s+)?\(([^)]*)\)\s*=>`)
	jsArrowNoParens = regexp.MustCompile(`(?:export\s+(?:const|let|var)\s+|const\s+|let\s+|var\s+)(\w+)\s*=\s*(?:async\s+)?(\w+)\s*=>`)
	jsClassRe       = regexp.MustCompile(`^(?:export\s+(?:default\s+)?)?(?:abstract\s+)?class\s+(\w+)(?:\s+extends\s+\w+)?(?:\s+implements\s+[\w,\s]+)?\s*\{`)
	jsMethodRe      = regexp.MustCompile(`^\s+(?:(?:public|private|protected|static|async|readonly|abstract)\s+)*(\w+)\s*\(([^)]*)\)\s*(?::\s*[^{]+)?\{`)
	tsInterfaceRe   = regexp.MustCompile(`^(?:export\s+)?interface\s+(\w+)(?:\s+extends\s+[\w,\s]+)?\s*\{`)
	tsTypeAliasRe   = regexp.MustCompile(`^(?:export\s+)?type\s+(\w+)\s*=`)
)

func extractJSTagsRegex(src string) []Tag {
	var tags []Tag
	lines := strings.Split(src, "\n")

	currentClass := ""

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			continue
		}

		// Detect class exit (dedent)
		if line != "" && line[0] != ' ' && line[0] != '\t' {
			currentClass = ""
		}

		// Interface (TypeScript)
		if m := tsInterfaceRe.FindStringSubmatch(trimmed); m != nil {
			tags = append(tags, Tag{
				Name:      m[1],
				Kind:      "interface",
				Line:      i + 1,
				Signature: fmt.Sprintf("interface %s", m[1]),
			})
			continue
		}

		// Type alias (TypeScript)
		if m := tsTypeAliasRe.FindStringSubmatch(trimmed); m != nil {
			tags = append(tags, Tag{
				Name:      m[1],
				Kind:      "variable",
				Line:      i + 1,
				Signature: fmt.Sprintf("type %s =", m[1]),
			})
			continue
		}

		// Class
		if m := jsClassRe.FindStringSubmatch(trimmed); m != nil {
			name := m[1]
			tags = append(tags, Tag{
				Name:      name,
				Kind:      "class",
				Line:      i + 1,
				Signature: fmt.Sprintf("class %s", name),
			})
			currentClass = name
			continue
		}

		// Exported function declaration
		if m := jsFuncDeclRe.FindStringSubmatch(trimmed); m != nil {
			name, params := m[1], m[2]
			tags = append(tags, Tag{
				Name:      name,
				Kind:      "function",
				Line:      i + 1,
				Signature: fmt.Sprintf("function %s(%s)", name, params),
			})
			continue
		}

		// Non-exported function declaration
		if m := jsFuncDeclNoExp.FindStringSubmatch(trimmed); m != nil {
			name, params := m[1], m[2]
			tags = append(tags, Tag{
				Name:      name,
				Kind:      "function",
				Line:      i + 1,
				Signature: fmt.Sprintf("function %s(%s)", name, params),
			})
			continue
		}

		// Arrow function with parentheses: const name = (params) =>
		if m := jsArrowFuncRe.FindStringSubmatch(trimmed); m != nil {
			name, params := m[1], m[2]
			tags = append(tags, Tag{
				Name:      name,
				Kind:      "function",
				Line:      i + 1,
				Signature: fmt.Sprintf("%s(%s) =>", name, params),
			})
			continue
		}

		// Arrow function without parentheses: const name = x =>
		if m := jsArrowNoParens.FindStringSubmatch(trimmed); m != nil {
			name, param := m[1], m[2]
			tags = append(tags, Tag{
				Name:      name,
				Kind:      "function",
				Line:      i + 1,
				Signature: fmt.Sprintf("%s(%s) =>", name, param),
			})
			continue
		}

		// Method inside class (indented)
		if currentClass != "" {
			if m := jsMethodRe.FindStringSubmatch(line); m != nil {
				name, params := m[1], m[2]
				// Skip constructor (already covered by class)
				if name == "constructor" {
					continue
				}
				tags = append(tags, Tag{
					Name:      name,
					Kind:      "method",
					Line:      i + 1,
					Signature: fmt.Sprintf("%s(%s)", name, params),
					Receiver:  currentClass,
				})
				continue
			}
		}
	}

	return tags
}

// ---------------------------------------------------------------------------
// RepoMap — generate a codebase structure summary suitable for LLM context
// ---------------------------------------------------------------------------

// RepoMap generates a structured summary of the codebase, grouped by directory,
// showing extracted tags (functions, classes, methods, etc.) with signatures and line numbers.
// The output is truncated to approximately maxTokens tokens (1 token ≈ 4 characters).
func (idx *Indexer) RepoMap(maxTokens int) string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// tagWithFile pairs a tag with its source file name for directory grouping.
	type tagWithFile struct {
		file string
		tag  Tag
	}

	dirMap := make(map[string][]tagWithFile)
	for _, entry := range idx.index.Files {
		if len(entry.Tags) == 0 {
			continue
		}
		dir := filepath.Dir(entry.Path)
		if dir == "." {
			dir = "(root)"
		}
		for _, t := range entry.Tags {
			dirMap[dir] = append(dirMap[dir], tagWithFile{
				file: filepath.Base(entry.Path),
				tag:  t,
			})
		}
	}

	// Sort directories
	dirs := make([]string, 0, len(dirMap))
	for d := range dirMap {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)

	// Build output
	var sb strings.Builder
	maxChars := maxTokens * 4
	if maxChars <= 0 {
		maxChars = 4000 // default ~1000 tokens
	}

	for _, dir := range dirs {
		if sb.Len() > maxChars {
			break
		}

		sb.WriteString(dir)
		sb.WriteString(":\n")

		items := dirMap[dir]
		// Sort by file name then line number
		sort.Slice(items, func(i, j int) bool {
			if items[i].file != items[j].file {
				return items[i].file < items[j].file
			}
			return items[i].tag.Line < items[j].tag.Line
		})

		prevFile := ""
		for _, item := range items {
			if sb.Len() > maxChars {
				break
			}

			// Show file separator when file changes
			if item.file != prevFile {
				sb.WriteString(fmt.Sprintf("  // %s\n", item.file))
				prevFile = item.file
			}

			sb.WriteString(fmt.Sprintf("  %s (L%d)\n", item.tag.Signature, item.tag.Line))
		}
		sb.WriteString("\n")
	}

	// Truncate if needed
	result := sb.String()
	if len(result) > maxChars {
		result = result[:maxChars]
		// Find last newline to avoid cutting mid-line
		if idx := strings.LastIndex(result, "\n"); idx > 0 {
			result = result[:idx+1]
		}
		result += "... (truncated)\n"
	}

	return result
}
