package cost

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/config"
	_ "modernc.org/sqlite"
)

// Record 单次调用成本记录
type Record struct {
	ID               int64     `json:"id"`
	Model            string    `json:"model"`
	Provider         string    `json:"provider"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	CostUSD          float64   `json:"cost_usd"`
	CostCNY          float64   `json:"cost_cny"`
	Timestamp        time.Time `json:"timestamp"`
	SessionID        string    `json:"session_id"`
	Command          string    `json:"command"`
	// PromptVariant 是本次调用使用的提示词变体（default / concise / safety-first / 用户自定义）
	// 用于 A/B 分析：哪个变体在同样任务上成本更低 / 完成率更高
	PromptVariant string `json:"prompt_variant,omitempty"`
}

// Summary 成本汇总
type Summary struct {
	TotalCostUSD      float64            `json:"total_cost_usd"`
	TotalCostCNY      float64            `json:"total_cost_cny"`
	TotalPromptTokens int64              `json:"total_prompt_tokens"`
	TotalCompTokens   int64              `json:"total_completion_tokens"`
	TotalTokens       int64              `json:"total_tokens"`
	CallCount         int                `json:"call_count"`
	ByModel           map[string]*Model  `json:"by_model"`
	ByDay             map[string]*Daily  `json:"by_day"`
}

// Model 单模型统计
type Model struct {
	Model            string  `json:"model"`
	Provider         string  `json:"provider"`
	CostUSD          float64 `json:"cost_usd"`
	CostCNY          float64 `json:"cost_cny"`
	Calls            int     `json:"calls"`
	Tokens           int64   `json:"tokens"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
}

// Daily 单日统计
type Daily struct {
	Day       string  `json:"day"`
	CostUSD   float64 `json:"cost_usd"`
	CostCNY   float64 `json:"cost_cny"`
	Calls     int     `json:"calls"`
	Tokens    int64   `json:"tokens"`
}

const usdToCNY = 7.2

// Tracker 成本追踪器
type Tracker struct {
	db   *sql.DB
	mu   sync.RWMutex
}

// NewTracker 创建成本追踪器
func NewTracker() (*Tracker, error) {
	configDir := config.GetConfigDir()
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("创建配置目录失败: %w", err)
	}

	dbPath := filepath.Join(configDir, "cost.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开成本数据库失败: %w", err)
	}

	if err := initSchema(db); err != nil {
		db.Close()
		return nil, err
	}

	return &Tracker{db: db}, nil
}

func initSchema(db *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS cost_records (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	model TEXT NOT NULL,
	provider TEXT NOT NULL,
	prompt_tokens INTEGER NOT NULL DEFAULT 0,
	completion_tokens INTEGER NOT NULL DEFAULT 0,
	total_tokens INTEGER NOT NULL DEFAULT 0,
	cost_usd REAL NOT NULL DEFAULT 0,
	cost_cny REAL NOT NULL DEFAULT 0,
	timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
	session_id TEXT,
	command TEXT
);
CREATE INDEX IF NOT EXISTS idx_cost_timestamp ON cost_records(timestamp);
CREATE INDEX IF NOT EXISTS idx_cost_session ON cost_records(session_id);
CREATE INDEX IF NOT EXISTS idx_cost_model ON cost_records(model);
`
	if _, err := db.Exec(schema); err != nil {
		return err
	}
	// v0.3.0 迁移：添加 prompt_variant 列（幂等）。
	// 老数据库 ALTER 会成功，新数据库列已存在会被 ALTER 报"duplicate column"，
	// 我们用 try-and-ignore 模式。
	if _, err := db.Exec(`ALTER TABLE cost_records ADD COLUMN prompt_variant TEXT`); err != nil {
		// 忽略 "duplicate column name" 错误
		if !isDuplicateColumnErr(err) {
			return fmt.Errorf("迁移 prompt_variant 列失败: %w", err)
		}
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_cost_variant ON cost_records(prompt_variant)`); err != nil {
		return fmt.Errorf("创建 idx_cost_variant 失败: %w", err)
	}
	return nil
}

// isDuplicateColumnErr 检测 SQLite "duplicate column" 错误。
// 兼容性写法：modernc/sqlite 返回的 error 字符串含 "duplicate column"。
func isDuplicateColumnErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column") || strings.Contains(msg, "already exists")
}

// Record 记录一次 LLM 调用（不携带 prompt variant，用于向后兼容）。
// F-12：使用 mu 锁包裹写入，避免并发场景下 modernc/sqlite 在繁忙时的
// SQLITE_BUSY 重试风暴（虽然驱动层会重试，但显式串行化可减少抖动）。
func (t *Tracker) Record(model, provider, sessionID, command string, usage ap.Usage) error {
	return t.RecordWithVariant(model, provider, sessionID, command, usage, "")
}

// RecordWithVariant 记录一次 LLM 调用 + 使用的 prompt variant。
// 变体名用于 A/B 分析：哪个变体在同样任务上成本更低 / 完成率更高。
func (t *Tracker) RecordWithVariant(model, provider, sessionID, command string, usage ap.Usage, promptVariant string) error {
	pricing := ap.DefaultPricingTable()
	costUSD := ap.EstimateCost(model, usage, pricing)
	costCNY := costUSD * usdToCNY

	t.mu.Lock()
	defer t.mu.Unlock()

	_, err := t.db.Exec(
		`INSERT INTO cost_records
		(model, provider, prompt_tokens, completion_tokens, total_tokens, cost_usd, cost_cny, session_id, command, prompt_variant)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		model, provider, usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens,
		costUSD, costCNY, sessionID, command, promptVariant,
	)
	if err != nil {
		return fmt.Errorf("记录成本失败: %w", err)
	}
	return nil
}

// VariantStat 单一变体的统计信息
type VariantStat struct {
	Variant         string  `json:"variant"`
	Calls           int64   `json:"calls"`
	TotalCostUSD    float64 `json:"total_cost_usd"`
	AvgCostUSD      float64 `json:"avg_cost_usd"`
	TotalTokens     int64   `json:"total_tokens"`
	AvgTokensPerCall float64 `json:"avg_tokens_per_call"`
}

// ReportByVariantText 返回人类可读的"按变体聚合"报告。
// 供 /ab 斜杠命令展示。
func (t *Tracker) ReportByVariantText() (string, error) {
	stats, err := t.SummaryByVariant()
	if err != nil {
		return "", err
	}
	if len(stats) == 0 {
		return "尚无成本记录", nil
	}
	var sb strings.Builder
	sb.WriteString("变体              调用     总费用(USD)   平均费用(USD)   平均 Token\n")
	sb.WriteString("--------------------------------------------------------\n")
	for _, s := range stats {
		sb.WriteString(fmt.Sprintf("%-18s  %4d   $%-10.4f   $%-10.4f   %.0f\n",
			s.Variant, s.Calls, s.TotalCostUSD, s.AvgCostUSD, s.AvgTokensPerCall))
	}
	return sb.String(), nil
}

// SummaryByVariant 按 prompt_variant 维度聚合成本。
// 用于 /cost summary --by-variant 或 A/B 分析仪表板。
func (t *Tracker) SummaryByVariant() ([]VariantStat, error) {
	rows, err := t.db.Query(`
		SELECT
			COALESCE(NULLIF(prompt_variant, ''), '(unset)') AS variant,
			COUNT(*) AS calls,
			SUM(cost_usd) AS total_cost_usd,
			SUM(total_tokens) AS total_tokens
		FROM cost_records
		GROUP BY prompt_variant
		ORDER BY total_cost_usd DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("按变体聚合失败: %w", err)
	}
	defer rows.Close()

	var out []VariantStat
	for rows.Next() {
		var s VariantStat
		if err := rows.Scan(&s.Variant, &s.Calls, &s.TotalCostUSD, &s.TotalTokens); err != nil {
			return nil, err
		}
		if s.Calls > 0 {
			s.AvgCostUSD = s.TotalCostUSD / float64(s.Calls)
			s.AvgTokensPerCall = float64(s.TotalTokens) / float64(s.Calls)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Summary 返回全部成本汇总
func (t *Tracker) Summary() (*Summary, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	rows, err := t.db.Query(`
		SELECT model, provider,
			SUM(prompt_tokens), SUM(completion_tokens), SUM(total_tokens),
			SUM(cost_usd), SUM(cost_cny), COUNT(*)
		FROM cost_records
		GROUP BY model, provider
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summary := &Summary{
		ByModel: make(map[string]*Model),
	}

	for rows.Next() {
		var m Model
		if err := rows.Scan(&m.Model, &m.Provider, &m.PromptTokens, &m.CompletionTokens, &m.Tokens, &m.CostUSD, &m.CostCNY, &m.Calls); err != nil {
			return nil, err
		}
		summary.ByModel[m.Model] = &m
		summary.TotalCostUSD += m.CostUSD
		summary.TotalCostCNY += m.CostCNY
		summary.TotalPromptTokens += m.PromptTokens
		summary.TotalCompTokens += m.CompletionTokens
		summary.TotalTokens += m.Tokens
		summary.CallCount += m.Calls
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return summary, nil
}

// DailySummary 返回最近 N 天的每日汇总
func (t *Tracker) DailySummary(days int) (map[string]*Daily, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	rows, err := t.db.Query(`
		SELECT DATE(timestamp) as day,
			SUM(cost_usd), SUM(cost_cny), COUNT(*), SUM(total_tokens)
		FROM cost_records
		WHERE timestamp >= DATE('now', ?)
		GROUP BY day
		ORDER BY day DESC
	`, fmt.Sprintf("-%d days", days))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]*Daily)
	for rows.Next() {
		var d Daily
		if err := rows.Scan(&d.Day, &d.CostUSD, &d.CostCNY, &d.Calls, &d.Tokens); err != nil {
			return nil, err
		}
		result[d.Day] = &d
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// RecentRecords 返回最近 N 条记录
func (t *Tracker) RecentRecords(limit int) ([]Record, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	rows, err := t.db.Query(`
		SELECT id, model, provider, prompt_tokens, completion_tokens, total_tokens,
			cost_usd, cost_cny, timestamp, session_id, command, prompt_variant
		FROM cost_records
		ORDER BY timestamp DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.ID, &r.Model, &r.Provider, &r.PromptTokens, &r.CompletionTokens,
			&r.TotalTokens, &r.CostUSD, &r.CostCNY, &r.Timestamp, &r.SessionID, &r.Command, &r.PromptVariant); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}

// Clear 清空所有记录
func (t *Tracker) Clear() error {
	_, err := t.db.Exec(`DELETE FROM cost_records`)
	return err
}

// Close 关闭数据库
func (t *Tracker) Close() error {
	return t.db.Close()
}
