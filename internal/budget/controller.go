package budget

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// BudgetConfig defines the budget limits and warning threshold.
type BudgetConfig struct {
	DailyLimitUSD    float64
	SessionLimitUSD  float64
	DailyTokenLimit  int
	SessionTokenLimit int
	WarnThreshold    float64 // default 0.8 = 80%
}

// UsageRecord represents a single usage event.
type UsageRecord struct {
	Model            string
	Provider         string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CostUSD          float64
	Timestamp        time.Time
	SessionID        string
}

// BudgetStatus represents the current budget status.
type BudgetStatus struct {
	DailySpendUSD      float64
	SessionSpendUSD    float64
	DailyRemainingUSD  float64
	SessionRemainingUSD float64
	DailyPercent       float64
	SessionPercent     float64
	IsOverBudget       bool
}

// BudgetExceededError is returned when a budget limit is exceeded.
type BudgetExceededError struct {
	Message string
}

func (e *BudgetExceededError) Error() string {
	return e.Message
}

// Controller manages cost budget tracking and enforcement.
type Controller struct {
	config       BudgetConfig
	dailySpend   float64
	sessionSpend float64
	dailyTokens  int
	sessionTokens int
	dailyDate    string
	mu           sync.Mutex
	onBudgetExceeded func(msg string)
}

// NewController creates a new budget controller.
func NewController(cfg BudgetConfig) *Controller {
	if cfg.WarnThreshold <= 0 || cfg.WarnThreshold > 1 {
		cfg.WarnThreshold = 0.8
	}
	return &Controller{
		config:    cfg,
		dailyDate: time.Now().Format("2006-01-02"),
	}
}

// Record records a usage event and checks budget limits.
// Returns BudgetExceededError if any limit is exceeded.
func (c *Controller) Record(usage UsageRecord) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Reset daily counters if the day has changed.
	today := time.Now().Format("2006-01-02")
	if today != c.dailyDate {
		c.dailyDate = today
		c.dailySpend = 0
		c.dailyTokens = 0
	}

	c.dailySpend += usage.CostUSD
	c.sessionSpend += usage.CostUSD
	c.dailyTokens += usage.TotalTokens
	c.sessionTokens += usage.TotalTokens

	// Check limits.
	var exceeded []string

	if c.config.DailyLimitUSD > 0 && c.dailySpend > c.config.DailyLimitUSD {
		exceeded = append(exceeded, fmt.Sprintf("daily spend $%.4f exceeds limit $%.4f", c.dailySpend, c.config.DailyLimitUSD))
	}
	if c.config.SessionLimitUSD > 0 && c.sessionSpend > c.config.SessionLimitUSD {
		exceeded = append(exceeded, fmt.Sprintf("session spend $%.4f exceeds limit $%.4f", c.sessionSpend, c.config.SessionLimitUSD))
	}
	if c.config.DailyTokenLimit > 0 && c.dailyTokens > c.config.DailyTokenLimit {
		exceeded = append(exceeded, fmt.Sprintf("daily tokens %d exceeds limit %d", c.dailyTokens, c.config.DailyTokenLimit))
	}
	if c.config.SessionTokenLimit > 0 && c.sessionTokens > c.config.SessionTokenLimit {
		exceeded = append(exceeded, fmt.Sprintf("session tokens %d exceeds limit %d", c.sessionTokens, c.config.SessionTokenLimit))
	}

	if len(exceeded) > 0 {
		msg := "budget exceeded: " + exceeded[0]
		for i := 1; i < len(exceeded); i++ {
			msg += "; " + exceeded[i]
		}
		if c.onBudgetExceeded != nil {
			c.onBudgetExceeded(msg)
		}
		return &BudgetExceededError{Message: msg}
	}

	// Check warning threshold.
	c.checkWarning()

	return nil
}

// checkWarning triggers the callback if spending has crossed the warn threshold.
// Must be called with c.mu held.
func (c *Controller) checkWarning() {
	if c.onBudgetExceeded == nil {
		return
	}
	if c.config.DailyLimitUSD > 0 {
		pct := c.dailySpend / c.config.DailyLimitUSD
		if pct >= c.config.WarnThreshold && pct < 1.0 {
			c.onBudgetExceeded(fmt.Sprintf("warning: daily spend at %.0f%% of limit ($%.4f / $%.4f)", pct*100, c.dailySpend, c.config.DailyLimitUSD))
		}
	}
	if c.config.SessionLimitUSD > 0 {
		pct := c.sessionSpend / c.config.SessionLimitUSD
		if pct >= c.config.WarnThreshold && pct < 1.0 {
			c.onBudgetExceeded(fmt.Sprintf("warning: session spend at %.0f%% of limit ($%.4f / $%.4f)", pct*100, c.sessionSpend, c.config.SessionLimitUSD))
		}
	}
}

// Check returns the current budget status.
func (c *Controller) Check() *BudgetStatus {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Reset daily counters if the day has changed.
	today := time.Now().Format("2006-01-02")
	if today != c.dailyDate {
		c.dailyDate = today
		c.dailySpend = 0
		c.dailyTokens = 0
	}

	status := &BudgetStatus{
		DailySpendUSD:   c.dailySpend,
		SessionSpendUSD: c.sessionSpend,
	}

	if c.config.DailyLimitUSD > 0 {
		status.DailyRemainingUSD = math.Max(0, c.config.DailyLimitUSD-c.dailySpend)
		status.DailyPercent = (c.dailySpend / c.config.DailyLimitUSD) * 100
	} else {
		status.DailyRemainingUSD = math.Inf(1)
		status.DailyPercent = 0
	}

	if c.config.SessionLimitUSD > 0 {
		status.SessionRemainingUSD = math.Max(0, c.config.SessionLimitUSD-c.sessionSpend)
		status.SessionPercent = (c.sessionSpend / c.config.SessionLimitUSD) * 100
	} else {
		status.SessionRemainingUSD = math.Inf(1)
		status.SessionPercent = 0
	}

	status.IsOverBudget = (c.config.DailyLimitUSD > 0 && c.dailySpend > c.config.DailyLimitUSD) ||
		(c.config.SessionLimitUSD > 0 && c.sessionSpend > c.config.SessionLimitUSD) ||
		(c.config.DailyTokenLimit > 0 && c.dailyTokens > c.config.DailyTokenLimit) ||
		(c.config.SessionTokenLimit > 0 && c.sessionTokens > c.config.SessionTokenLimit)

	return status
}

// OnBudgetExceeded sets a callback for budget exceeded notifications.
func (c *Controller) OnBudgetExceeded(fn func(msg string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onBudgetExceeded = fn
}

// ResetSession resets the session counters.
func (c *Controller) ResetSession() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionSpend = 0
	c.sessionTokens = 0
}
