package agent

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codecast/cli/internal/indexer"
)

// 快照测试：固定输入下，buildSystemPrompt 的输出与 testdata/prompt_*.golden 文件比对。
// 用法：
//
//	go test ./internal/agent/                              # 比对快照
//	UPDATE_SNAPSHOTS=1 go test ./internal/agent/            # 更新快照

var updateSnapshots = flag.Bool("update-snapshots", false, "update golden snapshot files")

// snapshotTestCase 定义一个快照测试用例
type snapshotTestCase struct {
	name         string
	goos         string
	cwd          string
	projectRules string
	mode         string
	budgetUSD    float64
	// indexerFiles: 如果非空，创建一个临时目录并填充这些文件作为 indexer
	indexerFiles map[string]string
}

func TestBuildSystemPromptSnapshots(t *testing.T) {
	cases := []snapshotTestCase{
		{
			name:         "suggest_with_indexer",
			goos:         "linux",
			cwd:          "/home/user/project",
			projectRules: "使用 Tab 缩进\n所有函数必须有注释",
			mode:         "suggest",
			budgetUSD:    0,
			indexerFiles: map[string]string{
				"main.go":         "package main\n",
				"internal/x.go":   "package x\n",
				"internal/y.go":   "package y\n",
				"README.md":       "# Project\n",
			},
		},
		{
			name:         "full_auto_no_rules",
			goos:         "darwin",
			cwd:          "/Users/dev/workspace",
			projectRules: "",
			mode:         "full-auto",
			budgetUSD:    0,
			indexerFiles: nil,
		},
		{
			name:         "auto_edit_with_budget",
			goos:         "windows",
			cwd:          "C:\\work\\app",
			projectRules: "提交前必须跑测试",
			mode:         "auto-edit",
			budgetUSD:    5.50,
			indexerFiles: map[string]string{
				"go.mod":  "module app\n",
				"main.go": "package main\n",
			},
		},
		{
			name:         "suggest_no_indexer_no_rules",
			goos:         "linux",
			cwd:          "/tmp/empty",
			projectRules: "",
			mode:         "suggest",
			budgetUSD:    0,
			indexerFiles: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var idx *indexer.Indexer
			if len(tc.indexerFiles) > 0 {
				tmp := t.TempDir()
				for path, content := range tc.indexerFiles {
					full := filepath.Join(tmp, path)
					if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(full, []byte(content), 0644); err != nil {
						t.Fatal(err)
					}
				}
				idx = indexer.NewIndexer(tmp)
				if err := idx.Build(); err != nil {
					t.Fatal(err)
				}
			}

			prompt := buildSystemPrompt(tc.goos, tc.cwd, tc.projectRules, idx, tc.mode, tc.budgetUSD)
			// 归一化 indexer 输出段（不同 Go map iteration 顺序会让文件树顺序变化）
			prompt = normalizeIndexerSection(prompt)
			assertSnapshot(t, "prompt_"+tc.name+".golden", normalize(prompt))
		})
	}
}

// normalize 跨平台路径归一化（windows 反斜杠 → 正斜杠），
// 让 snapshot 在 win/linux/mac 上生成一致。
func normalize(s string) string {
	s = strings.ReplaceAll(s, "\\", "/")
	return s
}

// normalizeIndexerSection 把 indexer 的文件树部分替换为稳定占位符。
// 原因：indexer.GetFileTree() 内部用 Go map，迭代顺序随机；不同运行产生不同输出。
// 我们关心的是"提示词结构"，不关心具体文件排序。
func normalizeIndexerSection(s string) string {
	const start = "=== 代码库结构 ==="
	const end = "=== 代码库结构结束 ==="
	i := strings.Index(s, start)
	j := strings.Index(s, end)
	if i < 0 || j < 0 || j < i {
		return s
	}
	return s[:i+len(start)] + "\n[FILE_TREE_NORMALIZED]\n" + s[j:]
}

// assertSnapshot 读取 golden 文件并比对；若 UPDATE_SNAPSHOTS=1 则覆写。
func assertSnapshot(t *testing.T, name, actual string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateSnapshots || os.Getenv("UPDATE_SNAPSHOTS") == "1" {
		if err := os.MkdirAll("testdata", 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(actual), 0644); err != nil {
			t.Fatal(err)
		}
		t.Logf("updated snapshot: %s", path)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read snapshot %s: %v\n--- hint: re-run with UPDATE_SNAPSHOTS=1 to create it ---\nactual:\n%s", path, err, actual)
	}
	if string(want) != actual {
		t.Errorf("snapshot mismatch for %s\n--- diff hint: re-run with UPDATE_SNAPSHOTS=1 to accept ---\n", path)
		// 打印前 60 行差异让 CI 可读
		t.Logf("actual (first 60 lines):\n%s", truncate(actual, 60))
	}
}

func truncate(s string, lines int) string {
	out := strings.SplitN(s, "\n", lines+1)
	if len(out) > lines {
		out = append(out[:lines], "...(truncated)")
	}
	return strings.Join(out, "\n")
}
