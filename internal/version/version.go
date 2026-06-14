package version

import "fmt"

// Version 语义化版本号，发布时由 CI 覆盖
var Version = "0.4.0"

// GitCommit 构建时的 Git commit hash，通过 -ldflags 注入
var GitCommit = "unknown"

// BuildDate 构建日期，通过 -ldflags 注入
var BuildDate = "unknown"

// FullVersion 返回完整版本信息，格式: codecast v0.1.0 (commit: xxx, built: xxx)
func FullVersion() string {
	return fmt.Sprintf("codecast v%s (commit: %s, built: %s)", Version, GitCommit, BuildDate)
}

// ShortVersion 返回简短版本号，格式: v0.1.0
func ShortVersion() string {
	return fmt.Sprintf("v%s", Version)
}
