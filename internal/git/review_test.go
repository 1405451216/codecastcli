package git

import (
	"strings"
	"testing"
)

func TestFormatReviewPrompt(t *testing.T) {
	diff := `--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
 
+import "fmt"
 func main() {
`
	blames := map[string]string{
		"main.go": "main.go (last modified by Alice 2 days ago)",
	}

	prompt := FormatReviewPrompt(diff, blames)

	// Verify prompt includes diff
	if !strings.Contains(prompt, "```diff") {
		t.Error("prompt should contain diff block")
	}
	if !strings.Contains(prompt, diff) {
		t.Error("prompt should contain the actual diff content")
	}

	// Verify prompt includes blame context
	if !strings.Contains(prompt, "Blame Context") {
		t.Error("prompt should contain Blame Context section")
	}
	if !strings.Contains(prompt, "Alice") {
		t.Error("prompt should contain blame author info")
	}

	// Verify prompt includes structured output instructions
	if !strings.Contains(prompt, "Review Summary") {
		t.Error("prompt should contain Review Summary instruction")
	}
	if !strings.Contains(prompt, "Overall Score") {
		t.Error("prompt should contain Overall Score instruction")
	}
	if !strings.Contains(prompt, "Severity") {
		t.Error("prompt should contain Severity instruction")
	}
	if !strings.Contains(prompt, "Category") {
		t.Error("prompt should contain Category instruction")
	}
}

func TestFormatReviewPrompt_NoBlame(t *testing.T) {
	diff := "--- a/a.go\n+++ b/a.go\n@@ +1 @@\n+hello\n"
	prompt := FormatReviewPrompt(diff, nil)

	if strings.Contains(prompt, "Blame Context") {
		t.Error("prompt should not contain Blame Context section when no blames provided")
	}
	if !strings.Contains(prompt, "```diff") {
		t.Error("prompt should still contain diff block")
	}
}

func TestParseReviewOutput(t *testing.T) {
	output := `### Review Summary

This change adds a new import statement. The import appears unused and may cause a compilation error.

### Overall Score: 7/10

### Findings

- **File**: main.go
- **Line**: 3
- **Severity**: warning
- **Category**: style
- **Message**: Unused import "fmt"
- **Suggestion**: Remove the unused import or add code that uses it.
---

- **File**: main.go
- **Line**: 1
- **Severity**: info
- **Category**: style
- **Message**: Package comment is missing
- **Suggestion**: Add a package-level doc comment.
---
`

	result := ParseReviewOutput(output)

	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
	if !strings.Contains(result.Summary, "import") {
		t.Errorf("summary = %q, want to mention import", result.Summary)
	}
	if result.OverallScore != 7 {
		t.Errorf("OverallScore = %d, want 7", result.OverallScore)
	}
	if len(result.Findings) != 2 {
		t.Fatalf("len(Findings) = %d, want 2", len(result.Findings))
	}

	f1 := result.Findings[0]
	if f1.File != "main.go" {
		t.Errorf("Findings[0].File = %q, want main.go", f1.File)
	}
	if f1.Line != 3 {
		t.Errorf("Findings[0].Line = %d, want 3", f1.Line)
	}
	if f1.Severity != "warning" {
		t.Errorf("Findings[0].Severity = %q, want warning", f1.Severity)
	}
	if f1.Category != "style" {
		t.Errorf("Findings[0].Category = %q, want style", f1.Category)
	}
	if !strings.Contains(f1.Message, "Unused") {
		t.Errorf("Findings[0].Message = %q, want to contain 'Unused'", f1.Message)
	}

	f2 := result.Findings[1]
	if f2.Severity != "info" {
		t.Errorf("Findings[1].Severity = %q, want info", f2.Severity)
	}
}

func TestParseReviewOutput_Empty(t *testing.T) {
	result := ParseReviewOutput("")
	if result.Summary != "" {
		t.Errorf("expected empty summary, got %q", result.Summary)
	}
	if result.OverallScore != 0 {
		t.Errorf("expected 0 score, got %d", result.OverallScore)
	}
	if len(result.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(result.Findings))
	}
}

func TestReviewFindingSeverity(t *testing.T) {
	tests := []struct {
		input    string
		valid    bool
		expected string
	}{
		{"critical", true, "critical"},
		{"warning", true, "warning"},
		{"info", true, "info"},
		{"CRITICAL", true, "critical"}, // case-insensitive
		{"unknown", false, ""},
		{"error", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lower := strings.ToLower(tt.input)
			ok := ValidSeverities[lower]
			if ok != tt.valid {
				t.Errorf("ValidSeverities[%q] = %v, want %v", lower, ok, tt.valid)
			}
		})
	}
}

func TestReviewFindingCategory(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"security", true},
		{"performance", true},
		{"style", true},
		{"bug", true},
		{"unknown", false},
		{"typo", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if ValidCategories[tt.input] != tt.valid {
				t.Errorf("ValidCategories[%q] = %v, want %v", tt.input, ValidCategories[tt.input], tt.valid)
			}
		})
	}
}

func TestReviewResult_ToJSON(t *testing.T) {
	result := &ReviewResult{
		Summary:      "Test summary",
		OverallScore: 8,
		Findings: []ReviewFinding{
			{
				File:       "main.go",
				Line:       10,
				Severity:   "warning",
				Category:   "security",
				Message:    "Potential SQL injection",
				Suggestion: "Use parameterized queries",
			},
		},
	}

	jsonStr, err := result.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON error: %v", err)
	}
	if !strings.Contains(jsonStr, `"summary"`) {
		t.Error("JSON should contain summary field")
	}
	if !strings.Contains(jsonStr, `"overall_score"`) {
		t.Error("JSON should contain overall_score field")
	}
	if !strings.Contains(jsonStr, `"findings"`) {
		t.Error("JSON should contain findings field")
	}
	if !strings.Contains(jsonStr, "SQL injection") {
		t.Error("JSON should contain the finding message")
	}
}

func TestExtractChangedFiles(t *testing.T) {
	diff := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
 
+import "fmt"
diff --git a/util.go b/util.go
--- a/util.go
+++ b/util.go
@@ -1,2 +1,3 @@
 package main
+
+func helper() {}
`
	files := ExtractChangedFiles(diff)
	if len(files) != 2 {
		t.Fatalf("len(files) = %d, want 2", len(files))
	}
	if files[0] != "main.go" {
		t.Errorf("files[0] = %q, want main.go", files[0])
	}
	if files[1] != "util.go" {
		t.Errorf("files[1] = %q, want util.go", files[1])
	}
}

func TestExtractChangedFiles_Dedup(t *testing.T) {
	diff := `--- a/main.go
+++ b/main.go
@@ -1 +1 @@
-old
+new
--- a/main.go
+++ b/main.go
@@ -5 +5 @@
-old2
+new2
`
	files := ExtractChangedFiles(diff)
	if len(files) != 1 {
		t.Errorf("len(files) = %d, want 1 (dedup)", len(files))
	}
}

func TestParseRemoteURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantOwner string
		wantRepo  string
	}{
		{"ssh", "git@github.com:owner/repo.git", "owner", "repo"},
		{"ssh_no_git", "git@github.com:owner/repo", "owner", "repo"},
		{"https", "https://github.com/owner/repo.git", "owner", "repo"},
		{"https_no_git", "https://github.com/owner/repo", "owner", "repo"},
		{"invalid", "not-a-url", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo := parseRemoteURL(tt.url)
			if owner != tt.wantOwner {
				t.Errorf("owner = %q, want %q", owner, tt.wantOwner)
			}
			if repo != tt.wantRepo {
				t.Errorf("repo = %q, want %q", repo, tt.wantRepo)
			}
		})
	}
}
