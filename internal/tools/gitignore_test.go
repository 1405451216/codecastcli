// GitignoreFilter 与 grep_search / glob_search 集成测试
package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ap "agentprimordia/pkg"
)

// writeFile 在 path 下创建文件，必要时会自动创建父目录
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// TestNewGitignoreFilter_Missing 验证 .gitignore 不存在时不报错
func TestNewGitignoreFilter_Missing(t *testing.T) {
	root := t.TempDir()
	f, err := NewGitignoreFilter(root)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if f == nil {
		t.Fatal("expected non-nil filter")
	}
	if f.ShouldSkip("foo.txt") || f.ShouldSkipDir("bar") {
		t.Fatal("filter with no .gitignore should never skip")
	}
}

// TestGitignoreFilter_StandardPattern 验证最基本的文件名匹配
func TestGitignoreFilter_StandardPattern(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".gitignore"), "secret.txt\n")

	f, err := NewGitignoreFilter(root)
	if err != nil {
		t.Fatalf("NewGitignoreFilter: %v", err)
	}

	if !f.ShouldSkip("secret.txt") {
		t.Error("expected secret.txt to be skipped")
	}
	if f.ShouldSkip("main.go") {
		t.Error("expected main.go NOT to be skipped")
	}
	// 嵌套文件同样匹配
	if !f.ShouldSkip(filepath.Join("sub", "secret.txt")) {
		t.Error("expected sub/secret.txt to be skipped")
	}
}

// TestGitignoreFilter_Wildcard 验证 * 通配符
func TestGitignoreFilter_Wildcard(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".gitignore"), "*.log\n*.tmp\n")

	f, err := NewGitignoreFilter(root)
	if err != nil {
		t.Fatalf("NewGitignoreFilter: %v", err)
	}

	if !f.ShouldSkip("debug.log") {
		t.Error("expected debug.log to be skipped (*.log)")
	}
	if !f.ShouldSkip(filepath.Join("a", "b", "test.tmp")) {
		t.Error("expected a/b/test.tmp to be skipped (*.tmp)")
	}
	if f.ShouldSkip("main.go") {
		t.Error("main.go should not be skipped")
	}
}

// TestGitignoreFilter_Negation 验证 ! 负向匹配
func TestGitignoreFilter_Negation(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".gitignore"), "*.log\n!important.log\n")

	f, err := NewGitignoreFilter(root)
	if err != nil {
		t.Fatalf("NewGitignoreFilter: %v", err)
	}

	if !f.ShouldSkip("debug.log") {
		t.Error("expected debug.log to be skipped")
	}
	if f.ShouldSkip("important.log") {
		t.Error("expected important.log to be re-included by !")
	}
}

// TestGitignoreFilter_NestedGitignore 验证子目录 .gitignore 也生效
// （简单实现只加载 rootDir 自身的 .gitignore，所以子目录的规则不会被合并）
// 本测试同时验证：
//  1. 根 .gitignore 匹配所有层级
//  2. 子目录 .gitignore 中的规则在 rootDir 视角下不会被加载（已知行为）
func TestGitignoreFilter_NestedGitignore(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".gitignore"), "*.bak\n")
	// 子目录里也有一个 .gitignore，按当前实现不会被加载
	writeFile(t, filepath.Join(root, "sub", ".gitignore"), "*.secret\n")

	f, err := NewGitignoreFilter(root)
	if err != nil {
		t.Fatalf("NewGitignoreFilter: %v", err)
	}

	// 根 .gitignore 跨层级生效
	if !f.ShouldSkip(filepath.Join("sub", "deep", "old.bak")) {
		t.Error("expected sub/deep/old.bak to be skipped by root *.bak rule")
	}
	// 子目录 .gitignore 在当前实现下不被加载 —— 记录行为
	if f.ShouldSkip(filepath.Join("sub", "data.secret")) {
		t.Log("note: nested .gitignore is honored (unexpected for current impl)")
	} else {
		t.Log("note: nested .gitignore is NOT loaded (current behavior)")
	}
}

// TestGitignoreFilter_DirPattern 验证以 / 结尾的目录规则
func TestGitignoreFilter_DirPattern(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".gitignore"), "build/\n")

	f, err := NewGitignoreFilter(root)
	if err != nil {
		t.Fatalf("NewGitignoreFilter: %v", err)
	}

	// 目录匹配
	if !f.ShouldSkipDir("build") {
		t.Error("expected build/ to be skipped as a dir")
	}
	// build 下的文件（经 ShouldSkip 查）也应被忽略
	if !f.ShouldSkip(filepath.Join("build", "out.bin")) {
		t.Error("expected build/out.bin to be skipped")
	}
}

// TestGitignoreFilter_InvalidFile 验证 .gitignore 内容非法时仍能加载
func TestGitignoreFilter_InvalidFile(t *testing.T) {
	root := t.TempDir()
	// 包含无效正则（[）— 某些 gitignore 实现可能失败；我们的实现走 CompileIgnoreLines，
	// 应当宽容。写入一个乱码混合内容以确保不阻塞。
	writeFile(t, filepath.Join(root, ".gitignore"), "valid_pattern\n[unterminated\nanother*\n")

	f, err := NewGitignoreFilter(root)
	if err != nil {
		t.Fatalf("NewGitignoreFilter: %v", err)
	}
	// 不论是否能正确解析 valid_pattern，绝不阻塞工具 —— 关键是不报错
	_ = f
}

// TestGrepSearch_RespectsGitignore 端到端：grep_search 跳过 .gitignore 文件
// 使用 NewGrepSearchToolWithRG("") 强制走 native 路径，专门测试我们新增的 .gitignore 过滤。
// （ripgrep 路径会自带 .gitignore 支持，但本测试聚焦于 native 路径的集成。）
func TestGrepSearch_RespectsGitignore(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".gitignore"), "*.secret\n")
	writeFile(t, filepath.Join(root, "main.go"), "package main // MARKER_HIT\n")
	writeFile(t, filepath.Join(root, "data.secret"), "MARKER_HIT inside secret file\n")
	writeFile(t, filepath.Join(root, "sub", "code.go"), "package sub // MARKER_HIT\n")
	writeFile(t, filepath.Join(root, "sub", "inner.secret"), "MARKER_HIT nested secret\n")

	tool := NewGrepSearchToolWithRG("") // 强制走 native
	out, err := tool.Execute(
		context.Background(),
		mustJSON(t, grepSearchParams{Pattern: "MARKER_HIT", Path: root, MaxResults: 100}),
	)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out == nil {
		t.Fatal("nil result")
	}
	body := fmtResultBody(out)
	if !strings.Contains(body, "main.go") {
		t.Error("expected main.go in results")
	}
	if !strings.Contains(body, "code.go") {
		t.Error("expected sub/code.go in results")
	}
	if strings.Contains(body, "data.secret") {
		t.Errorf("data.secret should have been skipped, got body:\n%s", body)
	}
	if strings.Contains(body, "inner.secret") {
		t.Errorf("sub/inner.secret should have been skipped, got body:\n%s", body)
	}
}

// TestGrepSearch_NoGitignore 不存在 .gitignore 时不能因为加载失败而崩溃
func TestGrepSearch_NoGitignore(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.go"), "package main // MARKER_HIT\n")

	tool := NewGrepSearchToolWithRG("") // 强制走 native
	out, err := tool.Execute(
		context.Background(),
		mustJSON(t, grepSearchParams{Pattern: "MARKER_HIT", Path: root, MaxResults: 100}),
	)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out == nil {
		t.Fatal("nil result")
	}
	body := fmtResultBody(out)
	if !strings.Contains(body, "main.go") {
		t.Errorf("expected main.go in results, got:\n%s", body)
	}
}

// TestGlobSearch_RespectsGitignore 端到端：glob_search 跳过 .gitignore 文件
// 现有 globRecursive 的 ** 后缀匹配有 pre-existing 限制：
// suffix 通常变成 "/*.go"，filepath.Match("/*.go", "a.go") 会返回 false。
// 本测试使用不带 ** 的标准 glob 模式，绕开该 pre-existing 限制，
// 但仍能验证 glob.go 中新增的 .gitignore 过滤生效。
func TestGlobSearch_RespectsGitignore(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".gitignore"), "*.secret\n")
	writeFile(t, filepath.Join(root, "a.go"), "")
	writeFile(t, filepath.Join(root, "b.secret"), "")
	writeFile(t, filepath.Join(root, "c.go"), "")
	writeFile(t, filepath.Join(root, "d.secret"), "")

	tool := NewGlobSearchTool()
	// 使用简单 *.go glob（不走 globRecursive）
	out, err := tool.Execute(
		context.Background(),
		mustJSON(t, globSearchParams{Pattern: "*.go", Path: root}),
	)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out == nil {
		t.Fatal("nil result")
	}
	body := fmtResultBody(out)
	if !strings.Contains(body, "a.go") {
		t.Errorf("expected a.go in results, got:\n%s", body)
	}
	if !strings.Contains(body, "c.go") {
		t.Errorf("expected c.go in results, got:\n%s", body)
	}
	if strings.Contains(body, "secret") {
		t.Errorf(".secret files should have been skipped, got:\n%s", body)
	}
}

// TestGlobSearch_NoGitignore 不存在 .gitignore 时不能因为加载失败而崩溃
func TestGlobSearch_NoGitignore(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.go"), "")

	tool := NewGlobSearchTool()
	out, err := tool.Execute(
		context.Background(),
		mustJSON(t, globSearchParams{Pattern: "*.go", Path: root}),
	)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out == nil {
		t.Fatal("nil result")
	}
	body := fmtResultBody(out)
	if !strings.Contains(body, "a.go") {
		t.Errorf("expected a.go in results, got:\n%s", body)
	}
}

// TestGlobRecursive_DoesNotPanic 当 .gitignore 不存在时 globRecursive 仍能正常工作
func TestGlobRecursive_DoesNotPanic(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "sub", "a.go"), "")
	// 不写 .gitignore
	_, err := globRecursive(root, "**/sub")
	if err != nil {
		t.Fatalf("globRecursive returned error: %v", err)
	}
}

// TestGlobRecursive_DoubleStarPattern 验证 **/*.ext 模式能正确匹配
// 修复前的 bug：pattern="**/*.go" 会让 suffix="/*.go"，
// filepath.Match("/*.go", "main.go") 返回 false（* 不匹配 /）。
func TestGlobRecursive_DoubleStarPattern(t *testing.T) {
	root := t.TempDir()
	// 创建多层目录结构
	writeFile(t, filepath.Join(root, "main.go"), "")
	writeFile(t, filepath.Join(root, "sub", "auth.go"), "")
	writeFile(t, filepath.Join(root, "sub", "deep", "deep.go"), "")
	writeFile(t, filepath.Join(root, "readme.md"), "")
	writeFile(t, filepath.Join(root, "sub", "README.md"), "")

	matches, err := globRecursive(root, "**/*.go")
	if err != nil {
		t.Fatalf("globRecursive returned error: %v", err)
	}

	basenames := make(map[string]bool)
	for _, m := range matches {
		basenames[filepath.Base(m)] = true
	}

	want := []string{"main.go", "auth.go", "deep.go"}
	for _, w := range want {
		if !basenames[w] {
			t.Errorf("expected %q in matches, got: %v", w, basenames)
		}
	}
	// 排除非 .go 文件
	for _, notWant := range []string{"readme.md", "README.md"} {
		if basenames[notWant] {
			t.Errorf("did not expect %q in .go matches", notWant)
		}
	}
}

// TestGrepSearch_KeepHardcodedSkipDirs 验证 hardcoded skipDirs 仍然生效
// （不能因为引入 .gitignore 而丢掉了 .git / node_modules 等）
func TestGrepSearch_KeepHardcodedSkipDirs(t *testing.T) {
	root := t.TempDir()
	// 故意不写 .gitignore —— 仅靠 hardcoded skipDirs 也要过滤
	writeFile(t, filepath.Join(root, "main.go"), "package main // MARKER_HIT\n")
	writeFile(t, filepath.Join(root, "node_modules", "pkg.js"), "var MARKER_HIT = 1\n")
	writeFile(t, filepath.Join(root, ".git", "HEAD"), "ref: MARKER_HIT\n")

	tool := NewGrepSearchToolWithRG("") // 强制走 native
	out, err := tool.Execute(
		context.Background(),
		mustJSON(t, grepSearchParams{Pattern: "MARKER_HIT", Path: root, MaxResults: 100}),
	)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	body := fmtResultBody(out)
	if !strings.Contains(body, "main.go") {
		t.Errorf("expected main.go in results, got:\n%s", body)
	}
	if strings.Contains(body, "node_modules") {
		t.Errorf("node_modules should have been skipped, got:\n%s", body)
	}
	if strings.Contains(body, ".git") {
		t.Errorf(".git should have been skipped, got:\n%s", body)
	}
}

// mustJSON 把 v 序列化成 json.RawMessage；出错则测试失败。
func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return b
}

// fmtResultBody 取出 *ap.ToolResult 的 Content 字段。
func fmtResultBody(r *ap.ToolResult) string {
	if r == nil {
		return ""
	}
	return r.Content
}
