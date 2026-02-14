package agent

import (
	"fmt"
	"sync"
	"time"
)

// Budget defines resource limits for a session.
type Budget struct {
	MaxTokens    int           // total tokens (input+output) across all turns
	MaxCostUSD   float64       // maximum spend for this session
	MaxToolCalls int           // maximum tool executions
	MaxDuration  time.Duration // wall clock limit
	MaxTurns     int           // maximum LLM calls (each tool round = 1 turn)
}

// DefaultBudget returns conservative defaults.
func DefaultBudget() Budget {
	return Budget{
		MaxTokens:    500_000,
		MaxCostUSD:   2.0,
		MaxToolCalls: 200,
		MaxDuration:  30 * time.Minute,
		MaxTurns:     50,
	}
}

// BudgetTracker tracks usage against limits.
type BudgetTracker struct {
	mu        sync.Mutex
	budget    Budget
	tokens    int
	costUSD   float64
	toolCalls int
	turns     int
	startedAt time.Time
}

// NewBudgetTracker creates a tracker for the given budget.
func NewBudgetTracker(b Budget) *BudgetTracker {
	return &BudgetTracker{
		budget:    b,
		startedAt: time.Now(),
	}
}

// Record adds usage from one completed turn.
func (bt *BudgetTracker) Record(inputTokens, outputTokens int, costUSD float64, toolCallsThisTurn int) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	bt.tokens += inputTokens + outputTokens
	bt.costUSD += costUSD
	bt.toolCalls += toolCallsThisTurn
	bt.turns++
}

// Exceeded returns the reason if any limit is exceeded, empty string if within limits.
func (bt *BudgetTracker) Exceeded() string {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	if bt.budget.MaxTokens > 0 && bt.tokens >= bt.budget.MaxTokens {
		return fmt.Sprintf("token limit reached (%d/%d)", bt.tokens, bt.budget.MaxTokens)
	}
	if bt.budget.MaxCostUSD > 0 && bt.costUSD >= bt.budget.MaxCostUSD {
		return fmt.Sprintf("cost limit reached ($%.4f/$%.2f)", bt.costUSD, bt.budget.MaxCostUSD)
	}
	if bt.budget.MaxToolCalls > 0 && bt.toolCalls >= bt.budget.MaxToolCalls {
		return fmt.Sprintf("tool call limit reached (%d/%d)", bt.toolCalls, bt.budget.MaxToolCalls)
	}
	if bt.budget.MaxDuration > 0 && time.Since(bt.startedAt) >= bt.budget.MaxDuration {
		return fmt.Sprintf("duration limit reached (%s/%s)", time.Since(bt.startedAt).Round(time.Second), bt.budget.MaxDuration)
	}
	if bt.budget.MaxTurns > 0 && bt.turns >= bt.budget.MaxTurns {
		return fmt.Sprintf("turn limit reached (%d/%d)", bt.turns, bt.budget.MaxTurns)
	}
	return ""
}

// Summary returns a human-readable usage summary.
func (bt *BudgetTracker) Summary() string {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	elapsed := time.Since(bt.startedAt).Round(time.Second)
	return fmt.Sprintf(
		"Tokens: %d | Cost: $%.4f | Tool calls: %d | Turns: %d | Duration: %s",
		bt.tokens, bt.costUSD, bt.toolCalls, bt.turns, elapsed,
	)
}

// TotalCost returns the accumulated cost.
func (bt *BudgetTracker) TotalCost() float64 {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	return bt.costUSD
}
