package tools

import (
	"strings"
	"unicode/utf8"
)

// FuzzyMatchThreshold 自动应用 fuzzy 匹配的置信度门槛。
// 低于此值则不应用，返回错误并提示最近候选，避免误改。
const FuzzyMatchThreshold = 0.85

// FuzzyMaxWindow 限制 fuzzy 匹配的最大窗口行数。
// 超过此行数的 old_string 不做 fuzzy（性能保护 + 大块本就该精确匹配）。
const FuzzyMaxWindow = 50

// fuzzyResult 是 fuzzy 匹配的结果。
type fuzzyResult struct {
	// Matched 是原文中匹配到的精确子串（含原始空白）。
	Matched string
	// Confidence 置信度 0-1，1 = 完全相同。
	Confidence float64
	// StartLine 匹配起始行（1-based），用于错误提示。
	StartLine int
}

// fuzzyMatchLines 在 original 中寻找与 old 最相似的行窗口。
//
// 算法：
//  1. 把 original 和 old 按行切分
//  2. 在 original 上用 len(oldLines) 大小的滑动窗口
//  3. 对每个窗口计算逐行 Levenshtein 相似度均值
//  4. 返回置信度最高的窗口
//
// 性能保护：
//  - old 行数 > FuzzyMaxWindow 直接返回零值
//  - 窗口行长度差异 >50% 跳过（快速剪枝）
//  - Levenshtein 用空间优化版（两行 DP）
//
// 找不到（original 行数 < old 行数）返回零值 fuzzyResult。
func fuzzyMatchLines(original, old string) fuzzyResult {
	origLines := strings.Split(original, "\n")
	oldLines := strings.Split(old, "\n")

	if len(oldLines) == 0 || len(oldLines) > FuzzyMaxWindow {
		return fuzzyResult{}
	}
	if len(origLines) < len(oldLines) {
		return fuzzyResult{}
	}

	windowSize := len(oldLines)
	best := fuzzyResult{Confidence: 0}

	for start := 0; start <= len(origLines)-windowSize; start++ {
		end := start + windowSize
		// 快速剪枝：窗口首行长度差异 >50% 跳过
		if !lengthCompatible(origLines[start], oldLines[0]) {
			continue
		}

		totalSim := 0.0
		for i := 0; i < windowSize; i++ {
			totalSim += lineSimilarity(origLines[start+i], oldLines[i])
		}
		avgSim := totalSim / float64(windowSize)

		if avgSim > best.Confidence {
			best.Confidence = avgSim
			best.StartLine = start + 1
			best.Matched = strings.Join(origLines[start:end], "\n")
		}
	}

	return best
}

// lengthCompatible 快速判断两行长度是否可能匹配。
// 长度差异 >50% 返回 false，用于剪枝加速。
func lengthCompatible(a, b string) bool {
	la := len(a)
	lb := len(b)
	if la == 0 && lb == 0 {
		return true
	}
	if la == 0 || lb == 0 {
		return false
	}
	ratio := float64(la) / float64(lb)
	if ratio < 1 {
		ratio = 1 / ratio
	}
	return ratio <= 2.0 // 允许 2x 差异（剪枝阈值放宽，避免漏匹配）
}

// lineSimilarity 计算两行的相似度 0-1。
// 基于 Levenshtein 距离归一化：1 - distance/maxLen。
// 空行与空行相似度 1；空行与非空相似度 0。
func lineSimilarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	if a == "" || b == "" {
		return 0.0
	}
	// 用 rune 计数避免中文截断
	ra := []rune(a)
	rb := []rune(b)
	maxLen := len(ra)
	if len(rb) > maxLen {
		maxLen = len(rb)
	}
	if maxLen == 0 {
		return 1.0
	}
	dist := levenshtein(ra, rb)
	return 1.0 - float64(dist)/float64(maxLen)
}

// levenshtein 计算两个 rune 切片的编辑距离。
// 空间优化版：只用两行 DP 数组，O(m*n) 时间 O(min(m,n)) 空间。
func levenshtein(a, b []rune) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// 确保 b 是较短的那一行，节省空间
	if lb > la {
		a, b = b, a
		la, lb = lb, la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(
				prev[j]+1,      // 删除
				curr[j-1]+1,    // 插入
				prev[j-1]+cost, // 替换
			)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}

// utf8Len 返回字符串的 rune 数（用于长度判断）。
func utf8Len(s string) int {
	return utf8.RuneCountInString(s)
}
