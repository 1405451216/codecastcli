package semantic

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Chunk 代码块：语义检索单元。
// 一个 Chunk 对应一个可被检索的代码片段（函数/类/方法/类型等）。
type Chunk struct {
	// ID 唯一标识，格式：file:startLine-endLine
	ID string `json:"id"`
	// File 源文件路径（相对项目根）
	File string `json:"file"`
	// Language 编程语言
	Language string `json:"language"`
	// Symbol 符号名（函数名/类名等）
	Symbol string `json:"symbol"`
	// Kind 符号类型（function/class/method/variable/struct/interface）
	Kind string `json:"kind"`
	// StartLine 起始行（1-based）
	StartLine int `json:"start_line"`
	// EndLine 结束行（1-based）
	EndLine int `json:"end_line"`
	// Content 代码内容
	Content string `json:"content"`
	// Signature 符号签名（如有）
	Signature string `json:"signature,omitempty"`
}

// Chunker 代码分块器。
// 按符号边界切块，每个符号一个 Chunk；
// 无符号的文件按固定行数切块（fallback）。
type Chunker struct {
	// MaxChunkLines 单个 chunk 最大行数（超过则截断）
	MaxChunkLines int
	// MinChunkLines 单个 chunk 最小行数（小于则合并到上一个）
	MinChunkLines int
}

// NewChunker 创建分块器
func NewChunker() *Chunker {
	return &Chunker{
		MaxChunkLines: 100,
		MinChunkLines: 3,
	}
}

// SymbolInfo 符号信息（由外部 indexer 提取）
type SymbolInfo struct {
	Name      string
	Kind      string
	Line      int // 1-based 起始行
	Signature string
}

// ChunkFile 把文件内容按符号切块。
// symbols 是该文件的符号列表（已按行排序）。
// 若 symbols 为空，按 MaxChunkLines 固定切块。
func (c *Chunker) ChunkFile(file, language, content string, symbols []SymbolInfo) []Chunk {
	lines := strings.Split(content, "\n")
	totalLines := len(lines)

	var chunks []Chunk

	if len(symbols) == 0 {
		// Fallback：固定行数切块
		for start := 0; start < totalLines; start += c.MaxChunkLines {
			end := start + c.MaxChunkLines
			if end > totalLines {
				end = totalLines
			}
			chunk := Chunk{
				ID:        fmtChunkID(file, start+1, end),
				File:      file,
				Language:  language,
				Symbol:    filepath.Base(file),
				Kind:      "file",
				StartLine: start + 1,
				EndLine:   end,
				Content:   strings.Join(lines[start:end], "\n"),
			}
			chunks = append(chunks, chunk)
		}
		return chunks
	}

	// 按符号切块
	for i, sym := range symbols {
		start := sym.Line - 1 // 转为 0-based
		if start < 0 {
			start = 0
		}
		var end int
		if i+1 < len(symbols) {
			end = symbols[i+1].Line - 1
		} else {
			end = totalLines
		}
		// 限制最大行数
		if end-start > c.MaxChunkLines {
			end = start + c.MaxChunkLines
		}
		if start >= totalLines {
			break
		}
		if end > totalLines {
			end = totalLines
		}
		if end <= start {
			continue
		}

		chunk := Chunk{
			ID:        fmtChunkID(file, start+1, end),
			File:      file,
			Language:  language,
			Symbol:    sym.Name,
			Kind:      sym.Kind,
			StartLine: start + 1,
			EndLine:   end,
			Content:   strings.Join(lines[start:end], "\n"),
			Signature: sym.Signature,
		}
		chunks = append(chunks, chunk)
	}

	// 合并过小的 chunk（可选优化，这里简化跳过）
	return chunks
}

// fmtChunkID 生成 chunk ID
func fmtChunkID(file string, start, end int) string {
	return file + ":" + itoa(start) + "-" + itoa(end)
}

// itoa 简单整数转字符串（避免 strconv 导入）
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// ExtractSymbolsFromTags 从 indexer.Tag 列表提取 SymbolInfo。
// 这是适配层，把 indexer.Tag 转为 semantic.SymbolInfo。
func ExtractSymbolsFromTags(tags []TagLike) []SymbolInfo {
	out := make([]SymbolInfo, 0, len(tags))
	for _, t := range tags {
		out = append(out, SymbolInfo{
			Name:      t.GetName(),
			Kind:      t.GetKind(),
			Line:      t.GetLine(),
			Signature: t.GetSignature(),
		})
	}
	return out
}

// TagLike 适配 indexer.Tag 的接口（避免直接依赖 indexer 包）
type TagLike interface {
	GetName() string
	GetKind() string
	GetLine() int
	GetSignature() string
}

// SymbolExtractor 符号提取器接口。
// 由外部注入实现（如 indexer.ExtractTags 的适配器），
// 让 semantic 包不直接依赖 indexer，避免循环依赖。
type SymbolExtractor interface {
	// Extract 从文件内容提取符号。
	// path 用于语言推断；language 已知则直接用。
	Extract(path string, content []byte, language string) []SymbolInfo
}

// SymbolExtractorFunc 函数适配器
type SymbolExtractorFunc func(path string, content []byte, language string) []SymbolInfo

// Extract 实现 SymbolExtractor
func (f SymbolExtractorFunc) Extract(path string, content []byte, language string) []SymbolInfo {
	return f(path, content, language)
}

// NoopSymbolExtractor 空实现，返回 nil（退化为固定行数切块）
type NoopSymbolExtractor struct{}

// Extract 返回 nil
func (NoopSymbolExtractor) Extract(string, []byte, string) []SymbolInfo { return nil }

// SimpleTag 简单实现，用于测试和独立使用
type SimpleTag struct {
	Name      string
	Kind      string
	Line      int
	Signature string
}

func (t SimpleTag) GetName() string      { return t.Name }
func (t SimpleTag) GetKind() string      { return t.Kind }
func (t SimpleTag) GetLine() int         { return t.Line }
func (t SimpleTag) GetSignature() string { return t.Signature }

// tokenize 简单分词（用于 BM25）。
// 按非字母数字字符分割，转小写。
func tokenize(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r >= 0x80) // 保留非 ASCII（中文）
	})
}

// splitCamelCase 驼峰分词（提升符号名检索效果）。
// "getUserInfo" → ["get", "user", "info"]
func splitCamelCase(s string) []string {
	re := regexp.MustCompile(`[A-Z][a-z]*|[a-z]+|[0-9]+`)
	return re.FindAllString(s, -1)
}
