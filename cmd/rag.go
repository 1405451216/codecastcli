package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/agent"
	"codecast/cli/internal/config"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// rootRagCmd 是 `codecast rag` 根命令。
//
// 在 v0.2.0 中，rag 子命令已迁移到交互模式的 `/rag` 斜杠命令。
var rootRagCmd = &cobra.Command{
	Use:   "rag",
	Short: "RAG 知识库管理（已迁移: 在交互模式中使用 /rag）",
	Long: `⚠️  codecast rag 子命令已在 v0.2.0 中迁移到交互模式。

请在交互模式（运行 codecast 后）中直接使用 /rag 斜杠命令：

  /rag index <path>            — 索引文档到知识库
  /rag query <query>           — 查询知识库
  /rag chat <query>            — 基于知识库对话`,
	Run: func(cmd *cobra.Command, args []string) {
		color.Yellow("⚠️  `codecast rag` 子命令已在 v0.2.0 中迁移到交互模式。")
		fmt.Println()
		color.Cyan("请在交互模式中使用 /rag 斜杠命令：")
		color.White("  /rag index <path>    — 索引文档到知识库")
		color.White("  /rag query <query>   — 查询知识库")
		color.White("  /rag chat <query>    — 基于知识库对话")
		fmt.Println()
		color.White("直接运行 `codecast` 进入交互模式，然后输入 /rag 即可。")
	},
}

// ============== 可复用函数 ==============

// ragRunIndex 索引文档到知识库
func ragRunIndex(path string, recursive bool) error {
	color.Yellow("正在索引文档: %s", path)
	if recursive {
		color.White("模式: 递归索引")
	}

	files, err := listRAGFiles(path, recursive)
	if err != nil {
		return fmt.Errorf("读取文件失败: %w", err)
	}

	color.Green("找到 %d 个文件", len(files))

	ragStore, err := createRAGStore()
	if err != nil {
		return fmt.Errorf("创建 RAG Store 失败: %w", err)
	}

	ctx := context.Background()
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			color.Yellow("  跳过 %s: %v", file, err)
			continue
		}

		episode := &ap.Episode{
			ID:      file,
			Content: string(content),
			Role:    "document",
		}
		if err := ragStore.Add(ctx, episode); err != nil {
			color.Yellow("  索引失败 %s: %v", file, err)
			continue
		}
		color.Green("  ✓ %s", file)
	}

	color.Green("\n索引完成!")
	return nil
}

// ragRunQuery 查询知识库
func ragRunQuery(query string, topK int) error {
	color.Yellow("查询: %s", query)

	ragStore, err := createRAGStore()
	if err != nil {
		return fmt.Errorf("创建 RAG Store 失败: %w", err)
	}

	ctx := context.Background()
	results, err := ragStore.Query(ctx, query, topK)
	if err != nil {
		return fmt.Errorf("查询失败: %w", err)
	}

	if len(results) == 0 {
		color.Yellow("未找到相关结果")
		return nil
	}

	color.Green("找到 %d 个相关结果:", len(results))
	for i, r := range results {
		fmt.Printf("\n[%d] 相关度: %.2f\n", i+1, r.Score)
		fmt.Printf("来源: %v\n", r.Sources)
		content := r.Episode.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		fmt.Printf("内容: %s\n", content)
	}
	return nil
}

// ragRunChat 基于知识库对话
func ragRunChat(query string) error {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		return err
	}

	codecastAgent, err := createCodecastAgentForRAG(cfg)
	if err != nil {
		return fmt.Errorf("初始化 Agent 失败: %w", err)
	}
	defer codecastAgent.Close()

	ctx := context.Background()
	return codecastAgent.StreamProcess(ctx, query)
}

// listRAGFiles 列出指定路径下的文件
func listRAGFiles(path string, recursive bool) ([]string, error) {
	var files []string
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		return []string{path}, nil
	}

	if recursive {
		err = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			files = append(files, p)
			return nil
		})
	} else {
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				files = append(files, filepath.Join(path, entry.Name()))
			}
		}
	}

	return files, err
}

// createCodecastAgentForRAG 创建 codecast agent（用于 RAG chat）
func createCodecastAgentForRAG(cfg *config.Config) (*agent.CodecastAgent, error) {
	return agent.New(cfg)
}

// createRAGStore 创建 RAG Store
func createRAGStore() (*ap.RAGStore, error) {
	memPath := filepath.Join(config.GetConfigDir(), "rag_memory.db")
	memory, err := ap.NewSQLiteStore(memPath)
	if err != nil {
		return nil, fmt.Errorf("创建记忆存储失败: %w", err)
	}

	ragStore := ap.NewRAGStore(memory, nil)
	return ragStore, nil
}

// parseRAGQuery 合并多参数为单一 query 字符串
func parseRAGQuery(args []string) string {
	return strings.Join(args, " ")
}

func init() {
	// 仅保留 rootRagCmd，不再注册子命令
	rootCmd.AddCommand(rootRagCmd)
}
