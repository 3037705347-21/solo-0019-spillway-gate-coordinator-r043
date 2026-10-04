package coordination

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/spillway-gate-coordinator/internal/contracts"
	"example.com/spillway-gate-coordinator/internal/domain"
	"example.com/spillway-gate-coordinator/internal/storage"
)

func newTestService(t *testing.T, now time.Time) *Service {
	t.Helper()
	repository := storage.NewMemoryRepository()
	service := NewService(repository)
	service.now = func() time.Time { return now }
	service.newID = func(prefix string) (string, error) {
		return prefix + "-fixed", nil
	}
	if err := service.EnsureBaseline(context.Background()); err != nil {
		t.Fatalf("seed baseline: %v", err)
	}
	return service
}

func TestPublishRuleSetDefaultsBoundaryToNow(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	service := newTestService(t, now)

	published, err := service.PublishRuleSet(context.Background(), contracts.PublishRuleSetRequest{
		MaxAllowedAperture: 2,
		Season:             "flood",
		Note:               "tightened",
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if !published.EffectiveFrom.Equal(now) {
		t.Fatalf("default boundary: expected %v, got %v", now, published.EffectiveFrom)
	}
	if published.Revision != 2 {
		t.Fatalf("revision: expected 2, got %d", published.Revision)
	}

	resolved, err := service.CurrentRuleSet(context.Background())
	if err != nil {
		t.Fatalf("resolve current: %v", err)
	}
	if resolved.RuleSetID != published.RuleSetID {
		t.Fatalf("current revision: expected %q, got %q", published.RuleSetID, resolved.RuleSetID)
	}
}

func TestPublishRuleSetRejectsMalformedBoundary(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	service := newTestService(t, now)

	_, err := service.PublishRuleSet(context.Background(), contracts.PublishRuleSetRequest{
		MaxAllowedAperture: 2,
		Season:             "flood",
		Note:               "tightened",
		EffectiveFrom:      "not-a-timestamp",
	})
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != "invalid_effective_from" {
		t.Fatalf("expected invalid_effective_from, got %v", err)
	}
}

func TestPublishRuleSetRejectsRetroactiveBoundary(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	service := newTestService(t, now)

	_, err := service.PublishRuleSet(context.Background(), contracts.PublishRuleSetRequest{
		MaxAllowedAperture: 2,
		Season:             "flood",
		Note:               "tightened",
		EffectiveFrom:      time.Time{}.Format(time.RFC3339),
	})
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != "invalid_effective_from" {
		t.Fatalf("expected zero timestamp rejection, got %v", err)
	}

	// First publication takes effect now.
	if _, err := service.PublishRuleSet(context.Background(), contracts.PublishRuleSetRequest{
		MaxAllowedAperture: 2, Season: "flood", Note: "a",
	}); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	// A second publication with an explicit past boundary cannot move backward.
	_, err = service.PublishRuleSet(context.Background(), contracts.PublishRuleSetRequest{
		MaxAllowedAperture: 1, Season: "retro", Note: "b",
		EffectiveFrom: now.Add(-time.Hour).Format(time.RFC3339),
	})
	if !errors.As(err, &domainErr) || domainErr.Code != "rule_effective_boundary_conflict" {
		t.Fatalf("expected retroactive boundary conflict, got %v", err)
	}
}

func TestEnsureBaselineDoesNotReplaceChain(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	service := newTestService(t, now)

	published, err := service.PublishRuleSet(context.Background(), contracts.PublishRuleSetRequest{
		MaxAllowedAperture: 2, Season: "flood", Note: "a",
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := service.EnsureBaseline(context.Background()); err != nil {
		t.Fatalf("re-seed baseline: %v", err)
	}
	ruleSets, err := service.RuleSets(context.Background())
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if len(ruleSets) != 2 || ruleSets[1].RuleSetID != published.RuleSetID {
		t.Fatalf("rule chain must be preserved after re-seed, got %+v", ruleSets)
	}
}
