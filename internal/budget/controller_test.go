package budget

import (
	"errors"
	"testing"
	"time"
)

func TestRecord_NoLimits(t *testing.T) {
	cfg := BudgetConfig{}
	c := NewController(cfg)

	usage := UsageRecord{
		Model:       "gpt-4o",
		Provider:    "openai",
		TotalTokens: 100,
		CostUSD:     0.01,
		Timestamp:   time.Now(),
		SessionID:   "sess_1",
	}

	if err := c.Record(usage); err != nil {
		t.Fatalf("Record() with no limits should not error, got: %v", err)
	}
}

func TestRecord_DailyLimitExceeded(t *testing.T) {
	cfg := BudgetConfig{
		DailyLimitUSD: 0.05,
	}
	c := NewController(cfg)

	usage := UsageRecord{
		Model:       "gpt-4o",
		Provider:    "openai",
		TotalTokens: 100,
		CostUSD:     0.03,
		Timestamp:   time.Now(),
		SessionID:   "sess_1",
	}

	// First record: under limit
	if err := c.Record(usage); err != nil {
		t.Fatalf("first Record() should not error, got: %v", err)
	}

	// Second record: exceeds limit
	err := c.Record(usage)
	if err == nil {
		t.Fatal("second Record() should return BudgetExceededError")
	}

	var budgetErr *BudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExceededError, got %T: %v", err, err)
	}
}

func TestRecord_SessionLimitExceeded(t *testing.T) {
	cfg := BudgetConfig{
		SessionLimitUSD: 0.04,
	}
	c := NewController(cfg)

	usage := UsageRecord{
		Model:       "gpt-4o",
		Provider:    "openai",
		TotalTokens: 100,
		CostUSD:     0.03,
		Timestamp:   time.Now(),
		SessionID:   "sess_1",
	}

	if err := c.Record(usage); err != nil {
		t.Fatalf("first Record() should not error, got: %v", err)
	}

	err := c.Record(usage)
	if err == nil {
		t.Fatal("second Record() should return BudgetExceededError")
	}

	var budgetErr *BudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExceededError, got %T: %v", err, err)
	}
}

func TestRecord_DailyTokenLimitExceeded(t *testing.T) {
	cfg := BudgetConfig{
		DailyTokenLimit: 200,
	}
	c := NewController(cfg)

	usage := UsageRecord{
		Model:       "gpt-4o",
		Provider:    "openai",
		TotalTokens: 150,
		CostUSD:     0.01,
		Timestamp:   time.Now(),
		SessionID:   "sess_1",
	}

	if err := c.Record(usage); err != nil {
		t.Fatalf("first Record() should not error, got: %v", err)
	}

	err := c.Record(usage)
	if err == nil {
		t.Fatal("second Record() should return BudgetExceededError for daily token limit")
	}

	var budgetErr *BudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExceededError, got %T: %v", err, err)
	}
}

func TestRecord_SessionTokenLimitExceeded(t *testing.T) {
	cfg := BudgetConfig{
		SessionTokenLimit: 200,
	}
	c := NewController(cfg)

	usage := UsageRecord{
		Model:       "gpt-4o",
		Provider:    "openai",
		TotalTokens: 150,
		CostUSD:     0.01,
		Timestamp:   time.Now(),
		SessionID:   "sess_1",
	}

	if err := c.Record(usage); err != nil {
		t.Fatalf("first Record() should not error, got: %v", err)
	}

	err := c.Record(usage)
	if err == nil {
		t.Fatal("second Record() should return BudgetExceededError for session token limit")
	}

	var budgetErr *BudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExceededError, got %T: %v", err, err)
	}
}

func TestCheck(t *testing.T) {
	cfg := BudgetConfig{
		DailyLimitUSD:   1.0,
		SessionLimitUSD: 0.5,
	}
	c := NewController(cfg)

	usage := UsageRecord{
		Model:       "gpt-4o",
		Provider:    "openai",
		TotalTokens: 100,
		CostUSD:     0.3,
		Timestamp:   time.Now(),
		SessionID:   "sess_1",
	}
	_ = c.Record(usage)

	status := c.Check()

	if status.DailySpendUSD != 0.3 {
		t.Errorf("DailySpendUSD = %f, want 0.3", status.DailySpendUSD)
	}
	if status.SessionSpendUSD != 0.3 {
		t.Errorf("SessionSpendUSD = %f, want 0.3", status.SessionSpendUSD)
	}
	if status.DailyRemainingUSD != 0.7 {
		t.Errorf("DailyRemainingUSD = %f, want 0.7", status.DailyRemainingUSD)
	}
	if status.SessionRemainingUSD != 0.2 {
		t.Errorf("SessionRemainingUSD = %f, want 0.2", status.SessionRemainingUSD)
	}
	if status.DailyPercent != 30.0 {
		t.Errorf("DailyPercent = %f, want 30.0", status.DailyPercent)
	}
	if status.SessionPercent != 60.0 {
		t.Errorf("SessionPercent = %f, want 60.0", status.SessionPercent)
	}
	if status.IsOverBudget {
		t.Error("IsOverBudget should be false")
	}
}

func TestCheck_OverBudget(t *testing.T) {
	cfg := BudgetConfig{
		DailyLimitUSD: 0.05,
	}
	c := NewController(cfg)

	usage := UsageRecord{
		Model:       "gpt-4o",
		Provider:    "openai",
		TotalTokens: 100,
		CostUSD:     0.06,
		Timestamp:   time.Now(),
		SessionID:   "sess_1",
	}
	_ = c.Record(usage)

	status := c.Check()
	if !status.IsOverBudget {
		t.Error("IsOverBudget should be true")
	}
}

func TestBudgetExceededError(t *testing.T) {
	err := &BudgetExceededError{Message: "daily spend exceeded"}
	if err.Error() != "daily spend exceeded" {
		t.Errorf("Error() = %q, want %q", err.Error(), "daily spend exceeded")
	}
}

func TestResetSession(t *testing.T) {
	cfg := BudgetConfig{
		SessionLimitUSD: 1.0,
	}
	c := NewController(cfg)

	usage := UsageRecord{
		Model:       "gpt-4o",
		Provider:    "openai",
		TotalTokens: 100,
		CostUSD:     0.5,
		Timestamp:   time.Now(),
		SessionID:   "sess_1",
	}
	_ = c.Record(usage)

	status := c.Check()
	if status.SessionSpendUSD != 0.5 {
		t.Errorf("SessionSpendUSD before reset = %f, want 0.5", status.SessionSpendUSD)
	}

	c.ResetSession()

	status = c.Check()
	if status.SessionSpendUSD != 0 {
		t.Errorf("SessionSpendUSD after reset = %f, want 0", status.SessionSpendUSD)
	}

	// Daily spend should not be reset.
	if status.DailySpendUSD != 0.5 {
		t.Errorf("DailySpendUSD after session reset = %f, want 0.5", status.DailySpendUSD)
	}
}

func TestOnBudgetExceeded(t *testing.T) {
	cfg := BudgetConfig{
		DailyLimitUSD: 0.05,
	}
	c := NewController(cfg)

	var callbackMsg string
	c.OnBudgetExceeded(func(msg string) {
		callbackMsg = msg
	})

	usage := UsageRecord{
		Model:       "gpt-4o",
		Provider:    "openai",
		TotalTokens: 100,
		CostUSD:     0.06,
		Timestamp:   time.Now(),
		SessionID:   "sess_1",
	}
	_ = c.Record(usage)

	if callbackMsg == "" {
		t.Error("OnBudgetExceeded callback should have been called")
	}
}

func TestOnBudgetExceeded_WarningThreshold(t *testing.T) {
	cfg := BudgetConfig{
		DailyLimitUSD: 0.1,
		WarnThreshold: 0.8,
	}
	c := NewController(cfg)

	var warnings []string
	c.OnBudgetExceeded(func(msg string) {
		warnings = append(warnings, msg)
	})

	usage := UsageRecord{
		Model:       "gpt-4o",
		Provider:    "openai",
		TotalTokens: 100,
		CostUSD:     0.085,
		Timestamp:   time.Now(),
		SessionID:   "sess_1",
	}

	// 85% of 0.1 = 0.085, should trigger warning but not exceed.
	if err := c.Record(usage); err != nil {
		t.Fatalf("Record() at threshold should not error, got: %v", err)
	}

	if len(warnings) == 0 {
		t.Error("expected warning callback to be triggered at 80% threshold")
	}
}

func TestNewController_DefaultWarnThreshold(t *testing.T) {
	cfg := BudgetConfig{WarnThreshold: 0}
	c := NewController(cfg)
	if c.config.WarnThreshold != 0.8 {
		t.Errorf("default WarnThreshold = %f, want 0.8", c.config.WarnThreshold)
	}
}

func TestRecord_MultipleLimitsExceeded(t *testing.T) {
	cfg := BudgetConfig{
		DailyLimitUSD:   0.05,
		SessionLimitUSD: 0.05,
	}
	c := NewController(cfg)

	usage := UsageRecord{
		Model:       "gpt-4o",
		Provider:    "openai",
		TotalTokens: 100,
		CostUSD:     0.06,
		Timestamp:   time.Now(),
		SessionID:   "sess_1",
	}

	err := c.Record(usage)
	if err == nil {
		t.Fatal("expected BudgetExceededError")
	}

	var budgetErr *BudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExceededError, got %T", err)
	}

	// Both daily and session limits should be mentioned.
	if budgetErr.Message == "" {
		t.Error("BudgetExceededError Message should not be empty")
	}
}
