package assembly

import "log/slog"

// BudgetLevel indicates the current budget pressure.
type BudgetLevel int

const (
	BudgetOK       BudgetLevel = iota // below soft warning threshold
	BudgetSoft                        // above 80% — agent should plan to wrap up
	BudgetHard                        // above 95% — compression starting
)

// TokenBudget tracks token usage against the session budget.
// The agent sees both thresholds so it can plan around them.
type TokenBudget struct {
	total     int
	softPct   float64 // default 0.80
	hardPct   float64 // default 0.95
	used      int
	hardFloor int // absolute minimum tokens to keep in reserve
}

// NewTokenBudget creates a budget tracker with the given total and hard floor.
func NewTokenBudget(total, hardFloor int) *TokenBudget {
	return &TokenBudget{
		total:     total,
		softPct:   0.80,
		hardPct:   0.95,
		hardFloor: hardFloor,
	}
}

// Add records token consumption and returns the new budget level.
func (b *TokenBudget) Add(prompt, completion int) BudgetLevel {
	b.used += prompt + completion
	level := b.level()
	if level == BudgetSoft {
		slog.Warn("budget: soft threshold reached",
			"used", b.used, "total", b.total,
			"remaining", b.Remaining(),
		)
	} else if level == BudgetHard {
		slog.Warn("budget: hard threshold reached — session ending",
			"used", b.used, "total", b.total,
		)
	}
	return level
}

// Remaining returns the number of tokens still available.
func (b *TokenBudget) Remaining() int {
	r := b.total - b.used
	if r < 0 {
		return 0
	}
	return r
}

// Level returns the current pressure level without consuming tokens.
func (b *TokenBudget) Level() BudgetLevel {
	return b.level()
}

func (b *TokenBudget) level() BudgetLevel {
	if b.total == 0 {
		return BudgetOK
	}
	frac := float64(b.used) / float64(b.total)
	remaining := b.total - b.used
	if frac >= b.hardPct || remaining <= b.hardFloor {
		return BudgetHard
	}
	if frac >= b.softPct {
		return BudgetSoft
	}
	return BudgetOK
}
