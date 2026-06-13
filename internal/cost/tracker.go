package cost

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
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
	_, err := db.Exec(schema)
	return err
}

// Record 记录一次 LLM 调用
// F-12：使用 mu 锁包裹写入，避免并发场景下 modernc/sqlite 在繁忙时的
// SQLITE_BUSY 重试风暴（虽然驱动层会重试，但显式串行化可减少抖动）。
func (t *Tracker) Record(model, provider, sessionID, command string, usage ap.Usage) error {
	pricing := ap.DefaultPricingTable()
	costUSD := ap.EstimateCost(model, usage, pricing)
	costCNY := costUSD * usdToCNY

	t.mu.Lock()
	defer t.mu.Unlock()

	_, err := t.db.Exec(
		`INSERT INTO cost_records
		(model, provider, prompt_tokens, completion_tokens, total_tokens, cost_usd, cost_cny, session_id, command)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		model, provider, usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens,
		costUSD, costCNY, sessionID, command,
	)
	if err != nil {
		return fmt.Errorf("记录成本失败: %w", err)
	}
	return nil
}

// Summary 返回全部成本汇总
func (t *Tracker) Summary() (*Summary, error) {
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

	return summary, nil
}

// DailySummary 返回最近 N 天的每日汇总
func (t *Tracker) DailySummary(days int) (map[string]*Daily, error) {
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
	return result, nil
}

// RecentRecords 返回最近 N 条记录
func (t *Tracker) RecentRecords(limit int) ([]Record, error) {
	rows, err := t.db.Query(`
		SELECT id, model, provider, prompt_tokens, completion_tokens, total_tokens,
			cost_usd, cost_cny, timestamp, session_id, command
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
			&r.TotalTokens, &r.CostUSD, &r.CostCNY, &r.Timestamp, &r.SessionID, &r.Command); err != nil {
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
