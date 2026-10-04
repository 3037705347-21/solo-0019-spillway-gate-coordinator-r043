package coordination

import (
	"context"
	"strings"
	"time"

	"example.com/spillway-gate-coordinator/internal/contracts"
	"example.com/spillway-gate-coordinator/internal/domain"
)

func (s *Service) PublishRule(
	ctx context.Context,
	request contracts.PublishInterlockRuleRequest,
) (domain.InterlockRule, error) {
	now := s.now()

	effectiveFrom := now
	if raw := strings.TrimSpace(request.EffectiveFrom); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return domain.InterlockRule{}, domain.Invalid(
				"invalid_effective_from",
				"effective_from must be an RFC3339 timestamp",
			)
		}
		effectiveFrom = parsed.UTC()
	}
	// A revision must not backdate its effective boundary: decisions already
	// recorded would otherwise point at a window that no longer covers them.
	if effectiveFrom.Before(now) {
		return domain.InterlockRule{}, domain.Invalid(
			"invalid_effective_from",
			"effective_from must not be in the past",
		)
	}

	ruleID, err := s.newID("rule")
	if err != nil {
		return domain.InterlockRule{}, domain.Internal("rule_id_generation_failed", "rule id could not be generated")
	}

	rule, err := domain.NewInterlockRule(
		ruleID,
		request.MaxTargetAperture,
		request.MaxApertureChange,
		request.Reason,
		request.IssuedBy,
		effectiveFrom,
		now,
	)
	if err != nil {
		return domain.InterlockRule{}, err
	}
	return s.repository.PublishRule(ctx, rule)
}

func (s *Service) Rules(ctx context.Context) ([]domain.InterlockRule, error) {
	return s.repository.Rules(ctx)
}

func (s *Service) CurrentRule(ctx context.Context) (domain.InterlockRule, error) {
	return s.repository.RuleEffectiveAt(ctx, s.now())
}
