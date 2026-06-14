package git

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// ReviewResult represents the structured output of a code review.
type ReviewResult struct {
	Summary     string          `json:"summary"`
	Findings    []ReviewFinding `json:"findings"`
	OverallScore int            `json:"overall_score"` // 1-10
}

// ReviewFinding represents a single finding from a code review.
type ReviewFinding struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	Severity   string `json:"severity"`   // critical, warning, info
	Category   string `json:"category"`   // security, performance, style, bug
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

// ValidSeverities is the set of allowed severity values.
var ValidSeverities = map[string]bool{
	"critical": true,
	"warning":  true,
	"info":     true,
}

// ValidCategories is the set of allowed category values.
var ValidCategories = map[string]bool{
	"security":    true,
	"performance": true,
	"style":       true,
	"bug":         true,
}

// FormatReviewPrompt creates a structured review prompt that includes both
// the diff and blame context for each changed file.
func FormatReviewPrompt(diff string, blames map[string]string) string {
	var sb strings.Builder

	sb.WriteString("请审查以下代码变更。请严格按照下面的格式输出审查结果，以便程序解析。\n\n")

	sb.WriteString("## 输出格式要求\n\n")
	sb.WriteString("请按以下 Markdown 格式输出：\n\n")
	sb.WriteString("### Review Summary\n")
	sb.WriteString("<一段总结性文字>\n\n")
	sb.WriteString("### Overall Score: N/10\n\n")
	sb.WriteString("### Findings\n\n")
	sb.WriteString("对每个发现，使用以下格式：\n\n")
	sb.WriteString("- **File**: 文件路径\n")
	sb.WriteString("- **Line**: 行号\n")
	sb.WriteString("- **Severity**: critical | warning | info\n")
	sb.WriteString("- **Category**: security | performance | style | bug\n")
	sb.WriteString("- **Message**: 问题描述\n")
	sb.WriteString("- **Suggestion**: 修改建议\n")
	sb.WriteString("---\n\n")

	if len(blames) > 0 {
		sb.WriteString("## Blame Context (代码归属信息)\n\n")
		sb.WriteString("以下是各变更文件的最近修改信息，帮助了解代码的上下文和作者：\n\n")
		for file, blame := range blames {
			sb.WriteString(fmt.Sprintf("- %s\n", blame))
			_ = file
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Diff\n\n")
	sb.WriteString("```diff\n")
	sb.WriteString(diff)
	sb.WriteString("\n```\n\n")

	sb.WriteString("请关注以下方面：\n")
	sb.WriteString("1. 安全性问题（SQL注入、XSS、敏感信息泄露等）\n")
	sb.WriteString("2. 性能问题（N+1查询、内存泄漏、不必要的计算等）\n")
	sb.WriteString("3. 代码风格（命名规范、代码组织、可读性等）\n")
	sb.WriteString("4. 潜在 Bug（空指针、边界条件、竞态条件等）\n")

	return sb.String()
}

// ParseReviewOutput parses the LLM's markdown response into a structured ReviewResult.
// It extracts the summary, overall score, and individual findings.
func ParseReviewOutput(output string) *ReviewResult {
	result := &ReviewResult{}

	// Extract summary: text between "### Review Summary" and the next "###"
	summaryRe := regexp.MustCompile(`(?s)### Review Summary\s*\n(.*?)###`)
	if m := summaryRe.FindStringSubmatch(output); len(m) > 1 {
		result.Summary = strings.TrimSpace(m[1])
	}

	// Extract overall score
	scoreRe := regexp.MustCompile(`### Overall Score:\s*(\d+)\s*/\s*10`)
	if m := scoreRe.FindStringSubmatch(output); len(m) > 1 {
		if score, err := strconv.Atoi(m[1]); err == nil {
			result.OverallScore = score
		}
	}

	// Extract findings section: everything after "### Findings"
	findingsIdx := strings.Index(output, "### Findings")
	findingsText := ""
	if findingsIdx >= 0 {
		findingsText = output[findingsIdx+len("### Findings"):]
	}

	// Split by "---" delimiter to get individual findings
	blocks := strings.Split(findingsText, "---")
	for _, block := range blocks {
		finding := parseFindingBlock(block)
		if finding != nil {
			result.Findings = append(result.Findings, *finding)
		}
	}

	return result
}

// parseFindingBlock parses a single finding block into a ReviewFinding.
func parseFindingBlock(block string) *ReviewFinding {
	f := &ReviewFinding{}

	fileRe := regexp.MustCompile(`\*\*File\*\*:\s*(.+)`)
	lineRe := regexp.MustCompile(`\*\*Line\*\*:\s*(\d+)`)
	severityRe := regexp.MustCompile(`\*\*Severity\*\*:\s*(\w+)`)
	categoryRe := regexp.MustCompile(`\*\*Category\*\*:\s*(\w+)`)
	messageRe := regexp.MustCompile(`\*\*Message\*\*:\s*(.+?)(?:\n|$)`)
	suggestionRe := regexp.MustCompile(`\*\*Suggestion\*\*:\s*(.+)`)

	if m := fileRe.FindStringSubmatch(block); len(m) > 1 {
		f.File = strings.TrimSpace(m[1])
	}
	if m := lineRe.FindStringSubmatch(block); len(m) > 1 {
		if n, err := strconv.Atoi(strings.TrimSpace(m[1])); err == nil {
			f.Line = n
		}
	}
	if m := severityRe.FindStringSubmatch(block); len(m) > 1 {
		s := strings.ToLower(strings.TrimSpace(m[1]))
		if ValidSeverities[s] {
			f.Severity = s
		}
	}
	if m := categoryRe.FindStringSubmatch(block); len(m) > 1 {
		c := strings.ToLower(strings.TrimSpace(m[1]))
		if ValidCategories[c] {
			f.Category = c
		}
	}
	if m := messageRe.FindStringSubmatch(block); len(m) > 1 {
		f.Message = strings.TrimSpace(m[1])
	}
	if m := suggestionRe.FindStringSubmatch(block); len(m) > 1 {
		f.Suggestion = strings.TrimSpace(m[1])
	}

	// Only return a finding if at least the message is present
	if f.Message == "" && f.File == "" {
		return nil
	}
	return f
}

// ToJSON serializes the ReviewResult to indented JSON.
func (r *ReviewResult) ToJSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal review result: %w", err)
	}
	return string(data), nil
}

// PostPRComments posts review findings as PR comments on GitHub using the gh CLI.
// If gh is not available, it prints the comments as text instead.
func PostPRComments(findings []ReviewFinding, repoOwner, repoName string, prNumber int) error {
	// Check if gh CLI is available
	if _, err := exec.LookPath("gh"); err != nil {
		fmt.Println("gh CLI 未安装，以文本方式输出审查结果：")
		printFindingsAsText(findings)
		return nil
	}

	if len(findings) == 0 {
		fmt.Println("没有发现需要评论的问题")
		return nil
	}

	posted := 0
	failed := 0
	for _, f := range findings {
		body := fmt.Sprintf("**[%s | %s]** %s\n\n💡 %s", f.Severity, f.Category, f.Message, f.Suggestion)
		if f.File != "" && f.Line > 0 {
			body = fmt.Sprintf("**[%s | %s]** `%s:%d`\n\n%s\n\n💡 %s",
				f.Severity, f.Category, f.File, f.Line, f.Message, f.Suggestion)
		}

		args := []string{
			"api",
			fmt.Sprintf("repos/%s/%s/pulls/%d/comments", repoOwner, repoName, prNumber),
			"-f", fmt.Sprintf("body=%s", body),
		}
		if f.File != "" && f.Line > 0 {
			args = append(args,
				"-f", fmt.Sprintf("path=%s", f.File),
				"-f", fmt.Sprintf("line=%d", f.Line),
				"-f", "side=RIGHT",
			)
		}

		cmd := exec.Command("gh", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			failed++
			fmt.Printf("  ✗ 评论失败 [%s:%d]: %s\n", f.File, f.Line, strings.TrimSpace(string(out)))
		} else {
			posted++
		}
	}

	fmt.Printf("PR 评论: %d 已发布, %d 失败\n", posted, failed)
	return nil
}

// printFindingsAsText prints findings as formatted text when gh is unavailable.
func printFindingsAsText(findings []ReviewFinding) {
	for i, f := range findings {
		loc := f.File
		if f.Line > 0 {
			loc = fmt.Sprintf("%s:%d", f.File, f.Line)
		}
		fmt.Printf("\n[%d] %s | %s — %s\n", i+1, f.Severity, f.Category, loc)
		fmt.Printf("    %s\n", f.Message)
		if f.Suggestion != "" {
			fmt.Printf("    💡 %s\n", f.Suggestion)
		}
	}
}

// GetPRInfo attempts to detect the current PR number from the git remote and branch.
// Returns (owner, repo, prNumber, error).
func GetPRInfo(analyzer *Analyzer) (string, string, int, error) {
	// Get remote URL to extract owner/repo
	remoteURL, err := analyzer.git("remote", "get-url", "origin")
	if err != nil {
		return "", "", 0, fmt.Errorf("获取 remote URL 失败: %w", err)
	}

	owner, repo := parseRemoteURL(remoteURL)
	if owner == "" || repo == "" {
		return "", "", 0, fmt.Errorf("无法从 remote URL 解析 owner/repo: %s", remoteURL)
	}

	// Try to get PR number from gh CLI
	cmd := exec.Command("gh", "pr", "view", "--json", "number", "-q", ".number")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", 0, fmt.Errorf("获取 PR 编号失败 (请确认当前分支关联了 PR): %w", err)
	}

	prNum, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return "", "", 0, fmt.Errorf("解析 PR 编号失败: %w", err)
	}

	return owner, repo, prNum, nil
}

// parseRemoteURL extracts owner and repo from a git remote URL.
// Supports both HTTPS and SSH formats.
func parseRemoteURL(raw string) (string, string) {
	raw = strings.TrimSpace(raw)

	// SSH: git@github.com:owner/repo.git
	sshRe := regexp.MustCompile(`git@[^:]+:([^/]+)/([^/]+?)(?:\.git)?$`)
	if m := sshRe.FindStringSubmatch(raw); len(m) == 3 {
		return m[1], m[2]
	}

	// HTTPS: https://github.com/owner/repo.git
	httpsRe := regexp.MustCompile(`https?://[^/]+/([^/]+)/([^/]+?)(?:\.git)?$`)
	if m := httpsRe.FindStringSubmatch(raw); len(m) == 3 {
		return m[1], m[2]
	}

	return "", ""
}
