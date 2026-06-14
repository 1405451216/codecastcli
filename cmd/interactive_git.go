package cmd

// interactive_git.go: Git 相关命令处理器（从 interactive.go 拆分）
//
// 包含：/review, /blame, /history, /diff 及辅助函数。

import (
	"fmt"
	"strings"

	"codecast/cli/internal/agent"
	"codecast/cli/internal/git"

	"github.com/fatih/color"
)

// handleReviewCommand 处理 /review 命令 — AI 审查代码变更
func handleReviewCommand(args string, ag *agent.CodecastAgent) {
	analyzer := git.NewAnalyzer(".")
	if !analyzer.IsGitRepo() {
		color.Red("当前目录不是 Git 仓库")
		return
	}

	outputJSON := strings.Contains(args, "--json")
	postPR := strings.Contains(args, "--pr")
	cleanArgs := strings.ReplaceAll(args, "--json", "")
	cleanArgs = strings.ReplaceAll(cleanArgs, "--pr", "")
	branch := strings.TrimSpace(cleanArgs)

	var diffText string
	var err error

	if branch != "" {
		diffText, err = analyzer.GetDiff(branch)
		if err != nil {
			color.Red("获取 diff 失败: %v", err)
			return
		}
	} else {
		staged, sErr := analyzer.StagedDiff()
		unstaged, uErr := analyzer.UnstagedDiff()
		if sErr != nil {
			color.Red("获取 staged diff 失败: %v", sErr)
			return
		}
		if uErr != nil {
			color.Red("获取 unstaged diff 失败: %v", uErr)
			return
		}
		diffText = staged + unstaged
	}

	if diffText == "" {
		color.Yellow("没有找到变更")
		return
	}

	blames := collectBlames(analyzer, diffText)
	prompt := git.FormatReviewPrompt(diffText, blames)

	ctx, cancel := acquireProcessingCtx()
	defer cancel()

	if outputJSON || postPR {
		result, err := ag.ProcessWithResult(ctx, prompt)
		if err != nil {
			color.Red("审查失败: %v", err)
			return
		}

		reviewResult := git.ParseReviewOutput(result.Content)

		if outputJSON {
			jsonStr, err := reviewResult.ToJSON()
			if err != nil {
				color.Red("JSON 序列化失败: %v", err)
				return
			}
			fmt.Println(jsonStr)
		}

		if postPR {
			owner, repo, prNum, err := git.GetPRInfo(analyzer)
			if err != nil {
				color.Yellow("PR 信息获取失败: %v", err)
				color.White("以文本方式输出审查结果：")
				printReviewResultText(reviewResult)
				return
			}
			if err := git.PostPRComments(reviewResult.Findings, owner, repo, prNum); err != nil {
				color.Red("发布 PR 评论失败: %v", err)
			}
		}

		if outputJSON && !postPR {
			printReviewSummary(reviewResult)
		}
	} else {
		if err := ag.StreamProcess(ctx, prompt); err != nil {
			color.Red("审查失败: %v", err)
		}
	}
}

// collectBlames gathers blame summaries for each file in the diff.
func collectBlames(analyzer *git.Analyzer, diff string) map[string]string {
	files := git.ExtractChangedFiles(diff)
	if len(files) == 0 {
		return nil
	}

	blames := make(map[string]string, len(files))
	for _, file := range files {
		summary := analyzer.BlameSummary(file)
		blames[file] = summary
	}
	return blames
}

// printReviewResultText prints a ReviewResult as formatted text.
func printReviewResultText(r *git.ReviewResult) {
	color.Cyan("📋 审查结果")
	fmt.Printf("评分: %d/10\n", r.OverallScore)
	fmt.Printf("摘要: %s\n", r.Summary)
	if len(r.Findings) > 0 {
		fmt.Println()
		for i, f := range r.Findings {
			loc := f.File
			if f.Line > 0 {
				loc = fmt.Sprintf("%s:%d", f.File, f.Line)
			}
			sevColor := severityColor(f.Severity)
			sevColor("  [%d] %s | %s — %s", i+1, f.Severity, f.Category, loc)
			fmt.Printf("      %s\n", f.Message)
			if f.Suggestion != "" {
				fmt.Printf("      💡 %s\n", f.Suggestion)
			}
		}
	}
}

// printReviewSummary prints a brief summary after JSON output.
func printReviewSummary(r *git.ReviewResult) {
	color.Cyan("\n📋 审查摘要")
	fmt.Printf("  评分: %d/10 | 发现: %d 项\n", r.OverallScore, len(r.Findings))
	critical := 0
	for _, f := range r.Findings {
		if f.Severity == "critical" {
			critical++
		}
	}
	if critical > 0 {
		color.Red("  ⚠ %d 个严重问题", critical)
	}
}

// severityColor returns a color-print function based on severity level.
func severityColor(severity string) func(string, ...interface{}) {
	switch severity {
	case "critical":
		return color.Red
	case "warning":
		return color.Yellow
	default:
		return color.White
	}
}

// handleBlameCommand 处理 /blame 命令 — 查看 Git Blame 信息
func handleBlameCommand(args string, ag *agent.CodecastAgent) {
	analyzer := git.NewAnalyzer(".")
	if !analyzer.IsGitRepo() {
		color.Red("当前目录不是 Git 仓库")
		return
	}

	args = strings.TrimSpace(args)
	if args == "" {
		color.Yellow("用法: /blame <file> [line]")
		return
	}

	parts := strings.Fields(args)
	file := parts[0]

	var output string
	var err error

	if len(parts) >= 2 {
		lineSpec := parts[1]
		if idx := strings.Index(lineSpec, "-"); idx > 0 {
			var start, end int
			if _, err := fmt.Sscanf(lineSpec, "%d-%d", &start, &end); err == nil && start > 0 && end >= start {
				output, err = analyzer.BlameContext(file, start, end)
			} else {
				color.Red("无效的行号范围: %s (格式: start-end)", lineSpec)
				return
			}
		} else {
			var line int
			if _, err := fmt.Sscanf(lineSpec, "%d", &line); err == nil && line > 0 {
				output, err = analyzer.BlameContext(file, line, line)
			} else {
				color.Red("无效的行号: %s", lineSpec)
				return
			}
		}
	} else {
		output, err = analyzer.BlameFile(file)
	}

	if err != nil {
		color.Red("获取 blame 信息失败: %v", err)
		return
	}

	if output == "" {
		color.Yellow("文件 %s 没有 blame 信息", file)
		return
	}

	color.Cyan("📝 Git Blame: %s", file)
	fmt.Println(output)
}

// handleHistoryCommand 处理 /history 命令 — 查看文件修改历史
func handleHistoryCommand(args string, ag *agent.CodecastAgent) {
	analyzer := git.NewAnalyzer(".")
	if !analyzer.IsGitRepo() {
		color.Red("当前目录不是 Git 仓库")
		return
	}

	file := strings.TrimSpace(args)
	if file == "" {
		color.Yellow("用法: /history <file>")
		return
	}

	output, err := analyzer.FileHistory(file, 20)
	if err != nil {
		color.Red("获取文件历史失败: %v", err)
		return
	}

	if output == "" {
		color.Yellow("文件 %s 没有提交历史", file)
		return
	}

	color.Cyan("📜 文件修改历史: %s", file)
	fmt.Println(output)
}

// handleDiffCommand 处理 /diff 命令 — 查看当前代码变更
func handleDiffCommand(args string, ag *agent.CodecastAgent) {
	analyzer := git.NewAnalyzer(".")
	if !analyzer.IsGitRepo() {
		color.Red("当前目录不是 Git 仓库")
		return
	}

	branch := strings.TrimSpace(args)

	if branch != "" {
		output, err := analyzer.GetDiff(branch)
		if err != nil {
			color.Red("获取 diff 失败: %v", err)
			return
		}
		if output == "" {
			color.Yellow("与 %s 分支没有差异", branch)
			return
		}
		color.Cyan("🔀 Diff: %s...HEAD", branch)
		fmt.Println(output)
		return
	}

	staged, sErr := analyzer.StagedDiff()
	if sErr != nil {
		color.Red("获取 staged diff 失败: %v", sErr)
		return
	}
	unstaged, uErr := analyzer.UnstagedDiff()
	if uErr != nil {
		color.Red("获取 unstaged diff 失败: %v", uErr)
		return
	}

	if staged == "" && unstaged == "" {
		color.Yellow("没有未提交的变更")
		return
	}

	if staged != "" {
		color.Cyan("📋 Staged Changes:")
		fmt.Println(staged)
	}
	if unstaged != "" {
		color.Cyan("📋 Unstaged Changes:")
		fmt.Println(unstaged)
	}
}
