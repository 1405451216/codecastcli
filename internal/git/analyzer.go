package git

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Analyzer Git repository analyzer
type Analyzer struct {
	repoPath string
}

// NewAnalyzer creates a new Git analyzer
func NewAnalyzer(repoPath string) *Analyzer {
	return &Analyzer{repoPath: repoPath}
}

// IsGitRepo checks if the path is inside a git repository
func (a *Analyzer) IsGitRepo() bool {
	_, err := a.git("rev-parse", "--git-dir")
	return err == nil
}

// RecentChanges returns a summary of recent N commits
func (a *Analyzer) RecentChanges(n int) (string, error) {
	output, err := a.git("log", "--oneline", fmt.Sprintf("-n%d", n))
	if err != nil {
		return "", err
	}
	return output, nil
}

// GetDiff returns the diff between current branch and base branch
func (a *Analyzer) GetDiff(baseBranch string) (string, error) {
	output, err := a.git("diff", baseBranch+"...HEAD")
	if err != nil {
		return "", err
	}
	return output, nil
}

// BlameContext returns git blame info for a file range
func (a *Analyzer) BlameContext(file string, startLine, endLine int) (string, error) {
	args := []string{"blame", fmt.Sprintf("-L%d,%d", startLine, endLine), file}
	output, err := a.git(args...)
	if err != nil {
		return "", err
	}
	return output, nil
}

// BlameFile returns git blame info for an entire file
func (a *Analyzer) BlameFile(file string) (string, error) {
	output, err := a.git("blame", file)
	if err != nil {
		return "", err
	}
	return output, nil
}

// FileHistory returns the modification history summary for a file
func (a *Analyzer) FileHistory(file string, n int) (string, error) {
	output, err := a.git("log", "--oneline", fmt.Sprintf("-n%d", n), "--", file)
	if err != nil {
		return "", err
	}
	return output, nil
}

// CurrentBranch returns the current branch name
func (a *Analyzer) CurrentBranch() (string, error) {
	output, err := a.git("branch", "--show-current")
	return strings.TrimSpace(output), err
}

// StagedDiff returns the staged changes diff
func (a *Analyzer) StagedDiff() (string, error) {
	output, err := a.git("diff", "--cached")
	return output, err
}

// UnstagedDiff returns the unstaged changes diff
func (a *Analyzer) UnstagedDiff() (string, error) {
	output, err := a.git("diff")
	return output, err
}

// ExtractChangedFiles parses a unified diff and returns the list of changed file paths.
func ExtractChangedFiles(diff string) []string {
	re := regexp.MustCompile(`^--- a/(.+)$`)
	var files []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(diff, "\n") {
		if m := re.FindStringSubmatch(line); len(m) == 2 {
			f := m[1]
			if !seen[f] {
				seen[f] = true
				files = append(files, f)
			}
		}
	}
	return files
}

// BlameSummary returns a one-line summary per file: "file (last modified by Author N days ago)".
// It uses git blame --line-porcelain to extract the author and timestamp of the last changed line.
func (a *Analyzer) BlameSummary(file string) string {
	output, err := a.git("blame", "--line-porcelain", file)
	if err != nil {
		return file
	}

	var lastAuthor, lastTime string
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "author ") {
			lastAuthor = strings.TrimPrefix(line, "author ")
		}
		if strings.HasPrefix(line, "author-time ") {
			ts := strings.TrimPrefix(line, "author-time ")
			lastTime = ts
		}
	}

	if lastAuthor == "" {
		return file
	}
	if lastTime != "" {
		return fmt.Sprintf("%s (last modified by %s %s)", file, lastAuthor, formatTimeAgo(lastTime))
	}
	return fmt.Sprintf("%s (last modified by %s)", file, lastAuthor)
}

// formatTimeAgo converts a unix timestamp string to a human-readable "N days ago" form.
func formatTimeAgo(unixStr string) string {
	var ts int64
	if _, err := fmt.Sscanf(unixStr, "%d", &ts); err != nil {
		return ""
	}
	days := int(time.Since(time.Unix(ts, 0)).Hours() / 24)
	switch {
	case days <= 0:
		return "today"
	case days == 1:
		return "1 day ago"
	default:
		return fmt.Sprintf("%d days ago", days)
	}
}

// git runs a git command and returns its output
func (a *Analyzer) git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = a.repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), string(output))
	}
	return string(output), nil
}
