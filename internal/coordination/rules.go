package coordination

import (
	"context"
	"time"

	"example.com/spillway-gate-coordinator/internal/contracts"
	"example.com/spillway-gate-coordinator/internal/domain"
)

// EnsureBaseline seeds the always-effective baseline rule revision when the
// rule chain has not been initialized. Existing revisions are never replaced.
func (s *Service) EnsureBaseline(ctx context.Context) error {
	return s.repository.SeedBaselineRules(ctx, domain.BaselineRuleSet(s.now()))
}

func (s *Service) PublishRuleSet(
	ctx context.Context,
	request contracts.PublishRuleSetRequest,
) (domain.RuleSet, error) {
	effectiveFrom, err := resolveEffectiveFrom(request.EffectiveFrom, s.now())
	if err != nil {
		return domain.RuleSet{}, err
	}

	existing, err := s.repository.RuleSets(ctx)
	if err != nil {
		return domain.RuleSet{}, err
	}
	if len(existing) == 0 {
		return domain.RuleSet{}, domain.Internal("rule_chain_missing", "baseline rule revision must be seeded first")
	}
	previous := existing[len(existing)-1]

	ruleSetID, err := s.newID("rule")
	if err != nil {
		return domain.RuleSet{}, domain.Internal("rule_set_id_generation_failed", "rule set id could not be generated")
	}

	ruleSet, err := domain.NewRuleSet(
		ruleSetID,
		previous,
		request.MaxAllowedAperture,
		request.Season,
		request.Note,
		effectiveFrom,
		s.now(),
	)
	if err != nil {
		return domain.RuleSet{}, err
	}
	if err := s.repository.CreateRuleSet(ctx, ruleSet); err != nil {
		return domain.RuleSet{}, err
	}
	return ruleSet, nil
}

func (s *Service) RuleSet(ctx context.Context, ruleSetID string) (domain.RuleSet, error) {
	return s.repository.RuleSet(ctx, ruleSetID)
}

// RuleSets returns the full append-only revision chain. Each revision's
// effective boundary ends where the next revision starts; the last revision is
// open-ended.
func (s *Service) RuleSets(ctx context.Context) ([]domain.RuleSet, error) {
	return s.repository.RuleSets(ctx)
}

// EffectiveRuleSet resolves the revision readable and in effect at at.
func (s *Service) EffectiveRuleSet(ctx context.Context, at time.Time) (domain.RuleSet, error) {
	return s.repository.EffectiveRuleSet(ctx, at)
}

// CurrentRuleSet resolves the revision readable and in effect now.
func (s *Service) CurrentRuleSet(ctx context.Context) (domain.RuleSet, error) {
	return s.repository.EffectiveRuleSet(ctx, s.now())
}

// resolveEffectiveFrom parses an optional boundary. A missing boundary means
// "take effect now"; a zero instant from a caller is rejected because it would
// overlap the baseline boundary.
func resolveEffectiveFrom(raw string, now time.Time) (time.Time, error) {
	if raw == "" {
		return now.UTC(), nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, domain.Invalid("invalid_effective_from", "effective_from must be an RFC3339 timestamp")
	}
	if parsed.IsZero() {
		return time.Time{}, domain.Invalid("invalid_effective_from", "effective_from cannot be the zero timestamp")
	}
	return parsed.UTC(), nil
}
