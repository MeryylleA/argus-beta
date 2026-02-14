package agent

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDefaultBudget(t *testing.T) {
	b := DefaultBudget()
	if b.MaxTokens != 500_000 {
		t.Errorf("MaxTokens = %d, want 500000", b.MaxTokens)
	}
	if b.MaxCostUSD != 2.0 {
		t.Errorf("MaxCostUSD = %f, want 2.0", b.MaxCostUSD)
	}
	if b.MaxToolCalls != 200 {
		t.Errorf("MaxToolCalls = %d, want 200", b.MaxToolCalls)
	}
	if b.MaxDuration != 30*time.Minute {
		t.Errorf("MaxDuration = %s, want 30m", b.MaxDuration)
	}
	if b.MaxTurns != 50 {
		t.Errorf("MaxTurns = %d, want 50", b.MaxTurns)
	}
}

func TestBudgetTrackerNoLimitsExceeded(t *testing.T) {
	bt := NewBudgetTracker(DefaultBudget())
	bt.Record(1000, 500, 0.01, 2)

	if reason := bt.Exceeded(); reason != "" {
		t.Errorf("expected no limit exceeded, got %q", reason)
	}
}

func TestBudgetTrackerTokenLimit(t *testing.T) {
	bt := NewBudgetTracker(Budget{MaxTokens: 100})
	bt.Record(60, 50, 0.0, 0) // total=110, exceeds 100

	reason := bt.Exceeded()
	if reason == "" {
		t.Fatal("expected token limit exceeded")
	}
	if !strings.Contains(reason, "token limit") {
		t.Errorf("expected 'token limit' in reason, got %q", reason)
	}
}

func TestBudgetTrackerCostLimit(t *testing.T) {
	bt := NewBudgetTracker(Budget{MaxCostUSD: 0.05})
	bt.Record(100, 100, 0.03, 0)
	bt.Record(100, 100, 0.03, 0) // total=0.06, exceeds 0.05

	reason := bt.Exceeded()
	if reason == "" {
		t.Fatal("expected cost limit exceeded")
	}
	if !strings.Contains(reason, "cost limit") {
		t.Errorf("expected 'cost limit' in reason, got %q", reason)
	}
}

func TestBudgetTrackerToolCallLimit(t *testing.T) {
	bt := NewBudgetTracker(Budget{MaxToolCalls: 5})
	bt.Record(0, 0, 0, 3)
	bt.Record(0, 0, 0, 3) // total=6, exceeds 5

	reason := bt.Exceeded()
	if !strings.Contains(reason, "tool call limit") {
		t.Errorf("expected 'tool call limit' in reason, got %q", reason)
	}
}

func TestBudgetTrackerTurnLimit(t *testing.T) {
	bt := NewBudgetTracker(Budget{MaxTurns: 2})
	bt.Record(0, 0, 0, 0)
	bt.Record(0, 0, 0, 0) // turns=2, meets limit

	reason := bt.Exceeded()
	if !strings.Contains(reason, "turn limit") {
		t.Errorf("expected 'turn limit' in reason, got %q", reason)
	}
}

func TestBudgetTrackerDurationLimit(t *testing.T) {
	bt := NewBudgetTracker(Budget{MaxDuration: 1 * time.Millisecond})
	time.Sleep(5 * time.Millisecond)

	reason := bt.Exceeded()
	if !strings.Contains(reason, "duration limit") {
		t.Errorf("expected 'duration limit' in reason, got %q", reason)
	}
}

func TestBudgetTrackerTotalCost(t *testing.T) {
	bt := NewBudgetTracker(DefaultBudget())
	bt.Record(0, 0, 0.10, 0)
	bt.Record(0, 0, 0.25, 0)

	if cost := bt.TotalCost(); cost != 0.35 {
		t.Errorf("TotalCost = %f, want 0.35", cost)
	}
}

func TestBudgetTrackerSummary(t *testing.T) {
	bt := NewBudgetTracker(DefaultBudget())
	bt.Record(1000, 500, 0.05, 3)

	summary := bt.Summary()
	if !strings.Contains(summary, "Tokens: 1500") {
		t.Errorf("summary missing token info: %s", summary)
	}
	if !strings.Contains(summary, "$0.0500") {
		t.Errorf("summary missing cost info: %s", summary)
	}
	if !strings.Contains(summary, "Tool calls: 3") {
		t.Errorf("summary missing tool call info: %s", summary)
	}
	if !strings.Contains(summary, "Turns: 1") {
		t.Errorf("summary missing turn info: %s", summary)
	}
}

func TestBudgetTrackerConcurrency(t *testing.T) {
	bt := NewBudgetTracker(Budget{MaxTokens: 1_000_000})
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bt.Record(100, 100, 0.001, 1)
			bt.Exceeded()
			bt.Summary()
			bt.TotalCost()
		}()
	}
	wg.Wait()

	if cost := bt.TotalCost(); cost != 0.1 {
		t.Errorf("after 100 concurrent records, TotalCost = %f, want 0.1", cost)
	}
}

func TestBudgetTrackerZeroLimitsNeverExceed(t *testing.T) {
	// Zero-value limits mean "no limit" for that dimension.
	bt := NewBudgetTracker(Budget{})
	bt.Record(999999, 999999, 999.0, 999)

	if reason := bt.Exceeded(); reason != "" {
		t.Errorf("zero budget should mean no limits, got %q", reason)
	}
}
