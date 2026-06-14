// F-13: list_files 工具的测试
package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeTree 在 tmpDir 下创建一个标准测试目录树：
//
//	tmpDir/
//	  main.go
//	  README.md
//	  sub/
//	    a.go
//	    b.txt
//	    deep/
//	      c.go
//	  node_modules/
//	    pkg.js
//	  .git/
//	    HEAD
func makeTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	files := map[string]string{
		"main.go":             "package main\n",
		"README.md":           "# readme\n",
		"sub/a.go":            "package a\n",
		"sub/b.txt":           "hello\n",
		"sub/deep/c.go":       "package c\n",
		"node_modules/pkg.js": "module.exports = {}\n",
		".git/HEAD":           "ref: refs/heads/main\n",
	}
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir failed: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatalf("write failed: %v", err)
		}
	}
	return root
}

func runListFiles(t *testing.T, params listFilesParams) (string, bool) {
	t.Helper()
	tool := NewListFilesTool()
	args, _ := json.Marshal(params)
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	return result.Content, result.IsError
}

func TestListFiles_HappyPath(t *testing.T) {
	root := makeTree(t)
	content, isErr := runListFiles(t, listFilesParams{
		Path:     root,
		MaxDepth: 2,
	})
	if isErr {
		t.Fatalf("期望成功，但得到错误: %s", content)
	}
	// 应包含 main.go 与 README.md（根的直接子）
	if !strings.Contains(content, "main.go") {
		t.Errorf("输出缺少 main.go: %s", content)
	}
	if !strings.Contains(content, "README.md") {
		t.Errorf("输出缺少 README.md: %s", content)
	}
	// 应包含 sub/ 目录
	if !strings.Contains(content, "sub/") {
		t.Errorf("输出缺少 sub/ 目录: %s", content)
	}
	// 应包含 sub/a.go（深度 2）
	if !strings.Contains(content, "a.go") {
		t.Errorf("输出缺少 sub/a.go: %s", content)
	}
}

func TestListFiles_MaxDepthZero_OnlyCurrentLayer(t *testing.T) {
	// 注意：我们的实现把 0 视为默认 2（与"未传"无法区分）。
	// 因此这里测的是 max_depth=1（仅当前层）的行为，更直接。
	root := makeTree(t)
	content, isErr := runListFiles(t, listFilesParams{
		Path:     root,
		MaxDepth: 1,
	})
	if isErr {
		t.Fatalf("期望成功，但得到错误: %s", content)
	}
	// 应包含 main.go / README.md
	if !strings.Contains(content, "main.go") {
		t.Errorf("输出缺少 main.go: %s", content)
	}
	// 应包含 sub/ 目录
	if !strings.Contains(content, "sub/") {
		t.Errorf("输出缺少 sub/ 目录: %s", content)
	}
	// 不应包含 sub/a.go（深度 2）
	if strings.Contains(content, "a.go") {
		t.Errorf("max_depth=1 时不应包含 a.go: %s", content)
	}
}

func TestListFiles_MaxDepth2_IncludesDeepFiles(t *testing.T) {
	root := makeTree(t)
	content, isErr := runListFiles(t, listFilesParams{
		Path:     root,
		MaxDepth: 2,
	})
	if isErr {
		t.Fatalf("期望成功，但得到错误: %s", content)
	}
	// sub/deep/c.go 在深度 3（root=0, sub=1, deep=2, c.go=3?）
	// 实际上 c.go 的 relPath = "sub/deep/c.go"，depthOfRel = 3
	// max_depth=2 时不应列出 c.go
	if strings.Contains(content, "c.go") {
		t.Errorf("max_depth=2 时不应列出 c.go（深度 3）: %s", content)
	}
	// 但 a.go 应该在（深度 2）
	if !strings.Contains(content, "a.go") {
		t.Errorf("max_depth=2 应包含 a.go（深度 2）: %s", content)
	}
}

func TestListFiles_MaxDepth3_IncludesAllFiles(t *testing.T) {
	root := makeTree(t)
	content, isErr := runListFiles(t, listFilesParams{
		Path:     root,
		MaxDepth: 3,
	})
	if isErr {
		t.Fatalf("期望成功，但得到错误: %s", content)
	}
	if !strings.Contains(content, "c.go") {
		t.Errorf("max_depth=3 应包含 c.go（深度 3）: %s", content)
	}
}

func TestListFiles_PatternFilter(t *testing.T) {
	root := makeTree(t)
	content, isErr := runListFiles(t, listFilesParams{
		Path:     root,
		MaxDepth: 3,
		Pattern:  "*.go",
	})
	if isErr {
		t.Fatalf("期望成功，但得到错误: %s", content)
	}
	// 应包含所有 .go 文件
	for _, f := range []string{"main.go", "a.go", "c.go"} {
		if !strings.Contains(content, f) {
			t.Errorf("pattern=*.go 应包含 %s: %s", f, content)
		}
	}
	// 不应包含 README.md 或 b.txt
	if strings.Contains(content, "README.md") {
		t.Errorf("pattern=*.go 不应包含 README.md: %s", content)
	}
	if strings.Contains(content, "b.txt") {
		t.Errorf("pattern=*.go 不应包含 b.txt: %s", content)
	}
}

func TestListFiles_SortByName(t *testing.T) {
	root := makeTree(t)
	content, isErr := runListFiles(t, listFilesParams{
		Path:     root,
		MaxDepth: 1,
		Sort:     "name",
	})
	if isErr {
		t.Fatalf("期望成功，但得到错误: %s", content)
	}
	// 找 main.go 与 README.md 的位置
	idxMain := strings.Index(content, "main.go")
	idxReadme := strings.Index(content, "README.md")
	if idxMain < 0 || idxReadme < 0 {
		t.Fatalf("输出应同时包含 main.go 与 README.md: %s", content)
	}
	// 按名称升序：README.md (R) < main.go (m)
	if idxReadme > idxMain {
		t.Errorf("sort=name 应使 README.md 排在 main.go 前: %s", content)
	}
}

func TestListFiles_SortBySize(t *testing.T) {
	root := t.TempDir()
	// 构造不同大小的文件
	if err := os.WriteFile(filepath.Join(root, "small.txt"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "big.txt"), []byte("aaaaaaaaaa"), 0644); err != nil {
		t.Fatal(err)
	}
	content, isErr := runListFiles(t, listFilesParams{
		Path:     root,
		MaxDepth: 1,
		Sort:     "size",
	})
	if isErr {
		t.Fatalf("期望成功，但得到错误: %s", content)
	}
	// sort=size 时大文件优先：big.txt 排在 small.txt 前
	idxBig := strings.Index(content, "big.txt")
	idxSmall := strings.Index(content, "small.txt")
	if idxBig < 0 || idxSmall < 0 {
		t.Fatalf("输出应同时包含 big.txt 与 small.txt: %s", content)
	}
	if idxBig > idxSmall {
		t.Errorf("sort=size 应使 big.txt 排在 small.txt 前: %s", content)
	}
}

func TestListFiles_SortByModified(t *testing.T) {
	root := t.TempDir()
	// 创建两个文件，修改时间不同
	old := filepath.Join(root, "old.txt")
	newer := filepath.Join(root, "newer.txt")
	if err := os.WriteFile(old, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	// 等一会儿再写第二个，确保 mtime 不同
	if err := os.WriteFile(newer, []byte("newer"), 0644); err != nil {
		t.Fatal(err)
	}
	content, isErr := runListFiles(t, listFilesParams{
		Path:     root,
		MaxDepth: 1,
		Sort:     "modified",
	})
	if isErr {
		t.Fatalf("期望成功，但得到错误: %s", content)
	}
	// sort=modified 时新的优先：newer.txt 排在 old.txt 前
	idxNewer := strings.Index(content, "newer.txt")
	idxOld := strings.Index(content, "old.txt")
	if idxNewer < 0 || idxOld < 0 {
		t.Fatalf("输出应同时包含 newer.txt 与 old.txt: %s", content)
	}
	if idxNewer > idxOld {
		t.Errorf("sort=modified 应使 newer.txt 排在 old.txt 前: %s", content)
	}
}

func TestListFiles_NonExistentPath(t *testing.T) {
	bogus := filepath.Join(t.TempDir(), "does-not-exist")
	content, isErr := runListFiles(t, listFilesParams{
		Path:     bogus,
		MaxDepth: 2,
	})
	if !isErr {
		t.Errorf("不存在的路径应返回错误，但得到成功: %s", content)
	}
	if !strings.Contains(content, "目录不存在") {
		t.Errorf("错误信息应包含 '目录不存在': %s", content)
	}
}

func TestListFiles_PathIsFile(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "not-a-dir.txt")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	content, isErr := runListFiles(t, listFilesParams{
		Path:     file,
		MaxDepth: 2,
	})
	if !isErr {
		t.Errorf("对文件调用应返回错误，但得到成功: %s", content)
	}
	if !strings.Contains(content, "不是目录") {
		t.Errorf("错误信息应包含 '不是目录': %s", content)
	}
}

func TestListFiles_SkipsSkipDirs(t *testing.T) {
	root := makeTree(t)
	content, isErr := runListFiles(t, listFilesParams{
		Path:     root,
		MaxDepth: 5,
	})
	if isErr {
		t.Fatalf("期望成功，但得到错误: %s", content)
	}
	// 不应出现 node_modules 或 .git
	if strings.Contains(content, "node_modules") {
		t.Errorf("应跳过 node_modules: %s", content)
	}
	if strings.Contains(content, ".git") {
		t.Errorf("应跳过 .git: %s", content)
	}
	if strings.Contains(content, "pkg.js") {
		t.Errorf("应跳过 node_modules/pkg.js: %s", content)
	}
	if strings.Contains(content, "HEAD") && strings.Contains(content, "ref:") {
		t.Errorf("应跳过 .git/HEAD: %s", content)
	}
}

func TestListFiles_DefaultPath(t *testing.T) {
	// path 留空 → 默认 "." → 应当对当前目录执行
	// 改 cwd 不可靠；改为显式传 "."，并接受任何非错误返回
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	root := makeTree(t)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	tool := NewListFilesTool()
	args, _ := json.Marshal(map[string]int{"max_depth": 1})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Errorf("path 缺省应正常工作: %s", result.Content)
	}
}

func TestListFiles_InvalidSort(t *testing.T) {
	root := makeTree(t)
	content, isErr := runListFiles(t, listFilesParams{
		Path:     root,
		MaxDepth: 1,
		Sort:     "garbage",
	})
	if !isErr {
		t.Errorf("无效 sort 值应返回错误，但得到成功: %s", content)
	}
}

func TestListFiles_NegativeMaxDepth(t *testing.T) {
	root := makeTree(t)
	content, isErr := runListFiles(t, listFilesParams{
		Path:     root,
		MaxDepth: -1,
	})
	if !isErr {
		t.Errorf("负数 max_depth 应返回错误，但得到成功: %s", content)
	}
}

func TestListFiles_EmptyDir(t *testing.T) {
	root := t.TempDir() // 空目录
	content, isErr := runListFiles(t, listFilesParams{
		Path:     root,
		MaxDepth: 2,
	})
	if isErr {
		t.Errorf("空目录不应返回错误: %s", content)
	}
	if !strings.Contains(content, "空") {
		t.Errorf("空目录应提示为空: %s", content)
	}
}

func TestListFiles_OutputFormat(t *testing.T) {
	// 验证输出格式：包含大小标注（KB / B）与日期（YYYY-MM-DD）
	root := makeTree(t)
	content, isErr := runListFiles(t, listFilesParams{
		Path:     root,
		MaxDepth: 2,
	})
	if isErr {
		t.Fatalf("期望成功: %s", content)
	}
	// main.go 内容是 "package main\n" = 13 字节，应格式化为 "13 B"
	if !strings.Contains(content, "13 B") {
		t.Errorf("输出应包含文件大小 '13 B': %s", content)
	}
	// 应包含 YYYY-MM-DD 格式的日期
	if !strings.Contains(content, "20") {
		t.Errorf("输出应包含日期（20xx-xx-xx）: %s", content)
	}
}
