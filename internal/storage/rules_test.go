package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/spillway-gate-coordinator/internal/domain"
)

func testContext() context.Context {
	return context.Background()
}

func seededRepository(t *testing.T, now time.Time) *MemoryRepository {
	t.Helper()
	repository := NewMemoryRepository()
	if err := repository.SeedBaselineRules(testContext(), domain.BaselineRuleSet(now)); err != nil {
		t.Fatalf("seed baseline: %v", err)
	}
	return repository
}

func TestSeedBaselineRulesIsIdempotent(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	repository := seededRepository(t, now)

	replacement := domain.BaselineRuleSet(now.Add(time.Hour))
	if err := repository.SeedBaselineRules(testContext(), replacement); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	ruleSets, err := repository.RuleSets(testContext())
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(ruleSets) != 1 {
		t.Fatalf("expected one revision after re-seed, got %d", len(ruleSets))
	}
	if got := ruleSets[0].CreatedAt; !got.Equal(now.UTC()) {
		t.Fatalf("existing baseline must not be replaced: got created_at %v", got)
	}
}

func TestCreateRuleSetRejectsOutOfOrderRevision(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	repository := seededRepository(t, now)
	boundary := now.Add(time.Hour)

	first, err := domain.NewRuleSet("rule-flood", domain.BaselineRuleSet(now), 2, "flood", "note", boundary, now)
	if err != nil {
		t.Fatalf("build first revision: %v", err)
	}
	if err := repository.CreateRuleSet(testContext(), first); err != nil {
		t.Fatalf("create first revision: %v", err)
	}

	// Build a revision claiming to follow the baseline again.
	duplicate, err := domain.NewRuleSet("rule-other", domain.BaselineRuleSet(now), 1, "other", "note", boundary.Add(time.Hour), now)
	if err != nil {
		t.Fatalf("build duplicate revision: %v", err)
	}
	err = repository.CreateRuleSet(testContext(), duplicate)
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != "rule_revision_out_of_order" {
		t.Fatalf("expected rule_revision_out_of_order, got %v", err)
	}

	// A duplicate id is rejected independently.
	sameID, err := domain.NewRuleSet("rule-flood", first, 1, "other", "note", boundary.Add(2*time.Hour), now)
	if err != nil {
		t.Fatalf("build same-id revision: %v", err)
	}
	if err := repository.CreateRuleSet(testContext(), sameID); err == nil {
		t.Fatal("duplicate rule set id must be rejected")
	}
}

func TestEffectiveRuleSetResolvesBoundary(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	repository := seededRepository(t, now)
	boundary := now.Add(48 * time.Hour)
	seasonal, err := domain.NewRuleSet("rule-flood", domain.BaselineRuleSet(now), 2, "flood", "note", boundary, now)
	if err != nil {
		t.Fatalf("build seasonal revision: %v", err)
	}
	if err := repository.CreateRuleSet(testContext(), seasonal); err != nil {
		t.Fatalf("create seasonal revision: %v", err)
	}

	resolved, err := repository.EffectiveRuleSet(testContext(), boundary.Add(-time.Nanosecond))
	if err != nil {
		t.Fatalf("resolve before boundary: %v", err)
	}
	if resolved.Revision != 1 {
		t.Fatalf("expected baseline before boundary, got revision %d", resolved.Revision)
	}
	resolved, err = repository.EffectiveRuleSet(testContext(), boundary)
	if err != nil {
		t.Fatalf("resolve at boundary: %v", err)
	}
	if resolved.Revision != 2 {
		t.Fatalf("expected seasonal revision at boundary, got %d", resolved.Revision)
	}
	resolved, err = repository.EffectiveRuleSet(testContext(), boundary.Add(time.Hour))
	if err != nil {
		t.Fatalf("resolve after boundary: %v", err)
	}
	if resolved.Revision != 2 {
		t.Fatalf("expected seasonal revision after boundary, got %d", resolved.Revision)
	}
}

func setupReviewedCommand(t *testing.T, now time.Time) (*MemoryRepository, domain.GateCommand) {
	t.Helper()
	repository := seededRepository(t, now)
	gate, err := domain.NewGate("gate-1", "一号闸", 10, 0.5, now)
	if err != nil {
		t.Fatalf("new gate: %v", err)
	}
	if err := repository.CreateGate(testContext(), gate); err != nil {
		t.Fatalf("create gate: %v", err)
	}
	command, err := domain.NewGateCommand("cmd-1", gate.ID, 1.5, 0.5, gate.MaxAperture, "reason", "dispatcher", now)
	if err != nil {
		t.Fatalf("new command: %v", err)
	}
	if err := repository.CreateCommand(testContext(), command); err != nil {
		t.Fatalf("create command: %v", err)
	}
	return repository, command
}

func TestReviewRecordsEffectiveRuleBasis(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	repository, _ := setupReviewedCommand(t, now)

	decision, err := domain.NewInterlockDecision("clear", "safety-wu", "ok", now)
	if err != nil {
		t.Fatalf("new decision: %v", err)
	}
	command, _, err := repository.ReviewCommand(testContext(), "cmd-1", decision, "token-1", now)
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if command.Interlock == nil {
		t.Fatal("review must store the decision")
	}
	if command.Interlock.Basis.Revision != 1 {
		t.Fatalf("basis revision: expected 1, got %d", command.Interlock.Basis.Revision)
	}
	if command.Interlock.Basis.RuleSetID != domain.BaselineRuleID {
		t.Fatalf("basis id: expected baseline, got %q", command.Interlock.Basis.RuleSetID)
	}
}

func TestReviewClearViolationLeavesNoMutation(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	repository, _ := setupReviewedCommand(t, now)
	boundary := now.Add(time.Hour)
	seasonal, err := domain.NewRuleSet("rule-flood", domain.BaselineRuleSet(now), 1.0, "flood", "tight", boundary, now)
	if err != nil {
		t.Fatalf("build seasonal revision: %v", err)
	}
	if err := repository.CreateRuleSet(testContext(), seasonal); err != nil {
		t.Fatalf("publish revision: %v", err)
	}

	decision, err := domain.NewInterlockDecision("clear", "safety-wu", "ok", boundary)
	if err != nil {
		t.Fatalf("new decision: %v", err)
	}
	_, gate, err := repository.ReviewCommand(testContext(), "cmd-1", decision, "token-1", boundary)
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != "interlock_clear_forbidden_by_rules" {
		t.Fatalf("expected interlock_clear_forbidden_by_rules, got %v (gate %+v)", err, gate)
	}

	command, err := repository.Command(testContext(), "cmd-1")
	if err != nil {
		t.Fatalf("read command: %v", err)
	}
	if command.Status != domain.CommandPending {
		t.Fatalf("status after refusal: expected pending, got %q", command.Status)
	}
	if command.Interlock != nil {
		t.Fatal("refusal must not store a decision")
	}
	if command.ExecutionToken != "" {
		t.Fatal("refusal must not issue a token")
	}
	gate, err = repository.Gate(testContext(), "gate-1")
	if err != nil {
		t.Fatalf("read gate: %v", err)
	}
	if gate.Status != domain.GateIdle || gate.ActiveCommandID != "" {
		t.Fatalf("gate must remain idle after refusal, got status=%q active=%q", gate.Status, gate.ActiveCommandID)
	}

	// After the refusal the verifier can block the same pending command.
	blocked, err := domain.NewInterlockDecision("blocked", "safety-wu", "over limit", boundary)
	if err != nil {
		t.Fatalf("new blocked decision: %v", err)
	}
	command, _, err = repository.ReviewCommand(testContext(), "cmd-1", blocked, "", boundary)
	if err != nil {
		t.Fatalf("block review: %v", err)
	}
	if command.Status != domain.CommandRejected {
		t.Fatalf("status: expected rejected, got %q", command.Status)
	}
	if command.Interlock.Basis.Revision != 2 {
		t.Fatalf("basis revision: expected 2, got %d", command.Interlock.Basis.Revision)
	}
}

func TestLaterRevisionDoesNotRewriteHistoricalBasis(t *testing.T) {
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	repository, _ := setupReviewedCommand(t, start)
	decision, err := domain.NewInterlockDecision("clear", "safety-wu", "ok", start)
	if err != nil {
		t.Fatalf("new decision: %v", err)
	}
	approved, _, err := repository.ReviewCommand(testContext(), "cmd-1", decision, "token-1", start)
	if err != nil {
		t.Fatalf("review: %v", err)
	}

	boundary := start.Add(time.Hour)
	seasonal, err := domain.NewRuleSet("rule-flood", domain.BaselineRuleSet(start), 1.0, "flood", "tight", boundary, start)
	if err != nil {
		t.Fatalf("build seasonal revision: %v", err)
	}
	if err := repository.CreateRuleSet(testContext(), seasonal); err != nil {
		t.Fatalf("publish revision: %v", err)
	}

	// The approved command executes under its historical basis; the tighter
	// revision cannot block an approval it did not review.
	completed, gate, err := repository.ExecuteCommand(testContext(), "cmd-1", "token-1", "operator", false, boundary)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if completed.Status != domain.CommandCompleted {
		t.Fatalf("status: expected completed, got %q", completed.Status)
	}
	if completed.Interlock.Basis.RuleSetID != domain.BaselineRuleID {
		t.Fatalf("historical basis must remain baseline, got %q", completed.Interlock.Basis.RuleSetID)
	}
	if completed.Interlock.Basis.ContentHash != approved.Interlock.Basis.ContentHash {
		t.Fatal("historical basis hash must not change after a revision")
	}
	if gate.Status != domain.GateIdle || gate.Aperture != 1.5 {
		t.Fatalf("gate settlement mismatch: status=%q aperture=%v", gate.Status, gate.Aperture)
	}
}
