package tui

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// generateMarkdown generates a markdown document with approximately n lines.
func generateMarkdown(n int) string {
	var sb strings.Builder
	sb.WriteString("# Benchmark Document\n\n")
	for i := 2; i < n; i++ {
		switch {
		case i%20 == 0:
			sb.WriteString(fmt.Sprintf("## Section %d\n\n", i/20))
		case i%10 == 0:
			sb.WriteString(fmt.Sprintf("- List item %d\n", i))
		case i%5 == 0:
			sb.WriteString(fmt.Sprintf("Paragraph %d with **bold** and *italic* text.\n\n", i))
		default:
			sb.WriteString(fmt.Sprintf("Line %d of the benchmark document.\n", i))
		}
	}
	return sb.String()
}

// generateDiff generates a diff string with approximately n lines.
func generateDiff(n int) string {
	var sb strings.Builder
	sb.WriteString("--- a/benchmark.go\n")
	sb.WriteString("+++ b/benchmark.go\n")
	sb.WriteString("@@ -1,10 +1,10 @@\n")

	for i := 0; i < n-3; i++ {
		switch i % 5 {
		case 0:
			sb.WriteString(fmt.Sprintf("+added line %d\n", i))
		case 1:
			sb.WriteString(fmt.Sprintf("-removed line %d\n", i))
		default:
			sb.WriteString(fmt.Sprintf(" context line %d\n", i))
		}
	}
	return sb.String()
}

// BenchmarkRenderMarkdown benchmarks rendering a 100-line markdown document.
func BenchmarkRenderMarkdown(b *testing.B) {
	r := NewRenderer()
	md := generateMarkdown(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.RenderMarkdown(md)
	}
}

// BenchmarkRenderDiff benchmarks rendering a 50-line diff.
func BenchmarkRenderDiff(b *testing.B) {
	r := NewRenderer()
	diffText := generateDiff(50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.RenderDiff(diffText)
	}
}

// BenchmarkStreamPrinterWriteToken benchmarks writing 100 tokens.
func BenchmarkStreamPrinterWriteToken(b *testing.B) {
	var buf bytes.Buffer
	sp := &StreamPrinter{
		renderer:   NewRenderer(),
		writer:     &buf,
		liveRender: false,
	}

	tokens := make([]string, 100)
	for i := range tokens {
		tokens[i] = fmt.Sprintf("token%d ", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, tok := range tokens {
			sp.WriteToken(tok)
		}
		sp.Reset()
	}
}
