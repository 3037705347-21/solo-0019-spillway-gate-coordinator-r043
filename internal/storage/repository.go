package storage

import (
	"context"
	"time"

	"example.com/spillway-gate-coordinator/internal/domain"
)

type Repository interface {
	CreateGate(ctx context.Context, gate domain.Gate) error
	Gate(ctx context.Context, id string) (domain.Gate, error)
	CreateCommand(ctx context.Context, command domain.GateCommand) error
	Command(ctx context.Context, id string) (domain.GateCommand, error)
	ReviewCommand(
		ctx context.Context,
		commandID string,
		decision domain.InterlockDecision,
		token string,
		now time.Time,
	) (domain.GateCommand, domain.Gate, error)
	ExecuteCommand(
		ctx context.Context,
		commandID string,
		token string,
		operator string,
		simulateFault bool,
		now time.Time,
	) (domain.GateCommand, domain.Gate, error)

	// SeedBaselineRules stores the baseline rule revision when no rule chain
	// exists. It is idempotent: an existing chain is left untouched.
	SeedBaselineRules(ctx context.Context, baseline domain.RuleSet) error
	// CreateRuleSet appends a new revision. The revision and its effective
	// boundary are checked against the current latest revision under the same
	// lock used by reviews, so a concurrent revision cannot move the boundary.
	CreateRuleSet(ctx context.Context, ruleSet domain.RuleSet) error
	// RuleSet reads a single stored revision by id.
	RuleSet(ctx context.Context, ruleSetID string) (domain.RuleSet, error)
	// RuleSets lists all stored revisions in publication order.
	RuleSets(ctx context.Context) ([]domain.RuleSet, error)
	// EffectiveRuleSet resolves the revision readable and in effect at t: the
	// latest revision whose effective boundary is not after t.
	EffectiveRuleSet(ctx context.Context, t time.Time) (domain.RuleSet, error)
}
