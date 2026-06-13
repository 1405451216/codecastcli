package cmd

// ab_export.go: /ab export <file.html> — 导出 A/B 报告为独立 HTML 页面。
//
// 报告内容：
//   - 收敛器状态表格（变体 / 采样 / 胜率 CI / 平均成本 / p50/p95 时延 / 评分 / 权重）
//   - 显著性结论（哪些变体显著优于其他）
//   - 成本按变体聚合（来自 cost.Tracker.SummaryByVariant）
//
// 输出文件为单页 HTML（无外链 CSS/JS），可邮件、Slack 分享。
// 不传文件名 → 写到 ~/.codecast/reports/ab_report_<ts>.html

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codecast/cli/internal/ab"
	"codecast/cli/internal/agent"
	"codecast/cli/internal/cost"

	"github.com/fatih/color"
)

func handleAbExport(args string, ag *agent.CodecastAgent) {
	abInt := abOf(ag)
	if abInt == nil {
		color.Yellow("A/B 收敛器未初始化")
		return
	}

	c, err := ab.Load(abStatePath())
	if err != nil {
		color.Red("✗ 加载状态失败: %v", err)
		return
	}
	available := availableVariantNames()

	outPath := strings.TrimSpace(args)
	if outPath == "" {
		home, _ := os.UserHomeDir()
		ts := time.Now().Format("20060102_150405")
		outPath = filepath.Join(home, ".codecast", "reports", fmt.Sprintf("ab_report_%s.html", ts))
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		color.Red("✗ 创建目录失败: %v", err)
		return
	}

	// 加载成本数据（可选；缺则跳过）
	var variantCosts []cost.VariantStat
	if t, err := cost.NewTracker(); err == nil {
		variantCosts, _ = t.SummaryByVariant()
		_ = t.Close()
	}

	// 用 ConvergerReport 结构化数据（从 Report() 解析或从新 ab 包 API 取）
	report := c.ReportWithLatency(available, abInt.Latency())
	rows := parseABReportRows(report)

	htmlContent := renderABReportHTML(rows, c, available, variantCosts, time.Now())
	if err := os.WriteFile(outPath, []byte(htmlContent), 0644); err != nil {
		color.Red("✗ 写入失败: %v", err)
		return
	}
	color.Green("✓ 已导出 A/B 报告: %s", outPath)
}

// abRow 是从 Report() 文本里解析出来的一行。
// 由于 Report() 是人类可读文本，正则解析可能脆；首选在 ab 包加结构化方法。
// 这里先用解析的方案，便于不破坏 ab 包的稳定性。
type abRow struct {
	Name     string
	Samples  int
	Success  string // "5/10" 或 "-"
	CI       string // "[25%, 75%]" 或 "-"
	AvgCost  string
	P50      string
	P95      string
	Score    string
	Weight   int
	IsWinner bool
}

// parseABReportRows 从 Converger.ReportWithLatency 文本里抓表行。
// 文本格式（latency=nil 时）：
//
//	变体          采样   胜率 (95%CI)            平均成本     评分  权重
//	------------------------------------------------------------------------
//	default       23   20/23 [68%, 96%]        $0.0120     8.345  10
//
// 文本格式（latency != nil）：
//
//	变体          采样   胜率 (95%CI)            平均成本     p50时延  p95时延  评分  权重
//	----------------------------------------------------------------------------------
//	default       23   20/23 [68%, 96%]        $0.0120     1200ms  2300ms  8.345  10
func parseABReportRows(report string) []abRow {
	rows := []abRow{}
	for _, line := range strings.Split(report, "\n") {
		// 跳过表头、分隔线、注释行
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "A/B") ||
			strings.HasPrefix(trimmed, "推荐") || strings.HasPrefix(trimmed, "变体") ||
			strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "结论") ||
			strings.HasPrefix(trimmed, "↑") || strings.HasPrefix(trimmed, "~") {
			continue
		}
		// 用连续两个及以上空格分列
		fields := strings.Fields(trimmed)
		if len(fields) < 4 {
			continue
		}
		// 必须以"采样"字段是纯数字开头
		var samples int
		if _, err := fmt.Sscanf(fields[1], "%d", &samples); err != nil {
			continue
		}
		row := abRow{Name: fields[0], Samples: samples}
		// 后续字段顺序按表头理解（无时延 / 有时延）
		if len(fields) >= 9 {
			// 有时延：name samples success CI cost p50 p95 score weight
			row.Success = fields[2]
			row.CI = fields[3]
			row.AvgCost = fields[4]
			row.P50 = fields[5]
			row.P95 = fields[6]
			row.Score = fields[7]
			fmt.Sscanf(fields[8], "%d", &row.Weight)
		} else if len(fields) >= 7 {
			// 无时延：name samples success CI cost score weight
			row.Success = fields[2]
			row.CI = fields[3]
			row.AvgCost = fields[4]
			row.Score = fields[5]
			fmt.Sscanf(fields[6], "%d", &row.Weight)
		}
		rows = append(rows, row)
	}
	return rows
}

// renderABReportHTML 生成自包含 HTML 报告。
// 样式 inline，无外链，可在任何浏览器/邮件客户端直接打开。
func renderABReportHTML(rows []abRow, c *ab.Converger, available []string, costs []cost.VariantStat, now time.Time) string {
	var sb strings.Builder
	cfg := c.Config()
	sug := c.Suggest(available)

	fmt.Fprintf(&sb, `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8">
<title>Codecast A/B 报告 — %s</title>
<style>
  body { font: 14px/1.5 -apple-system, "Segoe UI", sans-serif; max-width: 1080px; margin: 24px auto; padding: 0 16px; color: #1a1a1a; }
  h1 { font-size: 22px; border-bottom: 2px solid #2c5fc7; padding-bottom: 8px; }
  h2 { font-size: 17px; margin-top: 28px; color: #2c5fc7; }
  table { width: 100%%; border-collapse: collapse; margin: 8px 0 16px; font-size: 13px; }
  th, td { padding: 6px 10px; border-bottom: 1px solid #eee; text-align: right; }
  th:first-child, td:first-child { text-align: left; }
  th { background: #f4f6fb; color: #555; font-weight: 600; }
  tr:hover td { background: #fafbff; }
  .meta { color: #888; font-size: 12px; }
  code { background: #f4f4f4; padding: 1px 5px; border-radius: 3px; font-size: 12px; }
  .footer { margin-top: 40px; padding-top: 12px; border-top: 1px solid #eee; color: #999; font-size: 11px; }
</style></head><body>
`, now.Format("2006-01-02 15:04:05"))

	fmt.Fprintf(&sb, `<h1>Codecast A/B 评估报告</h1>
<p class="meta">生成时间: %s | epsilon=%.2f | MinSamples=%d</p>
<p>推荐下一个变体: <code>%s</code></p>
`, now.Format("2006-01-02 15:04:05"), cfg.Epsilon, cfg.MinSamples, html.EscapeString(sug.NextVariant))

	// 收敛状态表
	sb.WriteString(`<h2>收敛状态</h2>
<table>
  <thead><tr>
    <th>变体</th><th>采样</th><th>胜率</th><th>95% CI</th>
    <th>平均成本</th><th>p50 时延</th><th>p95 时延</th>
    <th>评分</th><th>权重</th>
  </tr></thead>
  <tbody>
`)
	if len(rows) == 0 {
		sb.WriteString(`  <tr><td colspan="9" style="text-align:center;color:#999">尚无数据</td></tr>`)
	}
	for _, r := range rows {
		fmt.Fprintf(&sb, "  <tr><td>%s</td><td>%d</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%d</td></tr>\n",
			html.EscapeString(r.Name), r.Samples,
			html.EscapeString(r.Success), html.EscapeString(r.CI),
			html.EscapeString(r.AvgCost),
			html.EscapeString(r.P50), html.EscapeString(r.P95),
			html.EscapeString(r.Score), r.Weight)
	}
	sb.WriteString("  </tbody>\n</table>\n")

	// 成本表
	if len(costs) > 0 {
		sb.WriteString(`<h2>成本按变体聚合（来自 cost.Tracker）</h2>
<table>
  <thead><tr>
    <th>变体</th><th>调用次数</th><th>总费用 (USD)</th>
    <th>平均费用 (USD)</th><th>平均 Token</th>
  </tr></thead>
  <tbody>
`)
		for _, s := range costs {
			fmt.Fprintf(&sb, "  <tr><td>%s</td><td>%d</td><td>$%.4f</td><td>$%.4f</td><td>%.0f</td></tr>\n",
				html.EscapeString(s.Variant), s.Calls, s.TotalCostUSD, s.AvgCostUSD, s.AvgTokensPerCall)
		}
		sb.WriteString("  </tbody>\n</table>\n")
	}

	// 显著性结论
	notes := buildSignificanceNotes(c, available)
	if len(notes) > 0 {
		sb.WriteString(`<h2>显著性结论</h2><ul>`)
		for _, n := range notes {
			fmt.Fprintf(&sb, "  <li>%s</li>\n", n)
		}
		sb.WriteString("</ul>\n")
	}

	fmt.Fprintf(&sb, `<div class="footer">由 Codecast CLI 生成 · state=%s</div>
</body></html>`, html.EscapeString(cfg.StatePath))
	return sb.String()
}

// buildSignificanceNotes 把收敛器的文字结论转成 HTML 列表项。
// 仅提取 "↑" / "~" 开头的行，去掉 ANSI 颜色。
func buildSignificanceNotes(c *ab.Converger, available []string) []string {
	report := c.Report(available)
	notes := []string{}
	for _, line := range strings.Split(report, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "↑") || strings.HasPrefix(trimmed, "~") {
			clean := stripAnsi(trimmed)
			notes = append(notes, html.EscapeString(clean))
		}
	}
	return notes
}

func stripAnsi(s string) string {
	var out strings.Builder
	inEsc := false
	for _, r := range s {
		if r == 0x1b {
			inEsc = true
			continue
		}
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}
