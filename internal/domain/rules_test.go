package domain

import (
	"errors"
	"testing"
	"time"
)

func testNow() time.Time {
	return time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
}

func TestBaselineRuleSetIsEffectiveAtEveryReadableInstant(t *testing.T) {
	baseline := BaselineRuleSet(testNow())

	if baseline.RuleSetID != BaselineRuleID {
		t.Fatalf("baseline id: expected %q, got %q", BaselineRuleID, baseline.RuleSetID)
	}
	if !baseline.ValidAt(time.Time{}) {
		t.Fatal("baseline must be valid at the zero instant")
	}
	if !baseline.ValidAt(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("baseline must be valid before publication date")
	}
	if baseline.Basis().RuleSetID != BaselineRuleID {
		t.Fatal("baseline basis must identify itself")
	}
}

func TestNewRuleSetRequiresForwardBoundary(t *testing.T) {
	now := testNow()
	baseline := BaselineRuleSet(now)

	cases := []struct {
		name          string
		effectiveFrom time.Time
	}{
		{"zero boundary", time.Time{}},
		{"equal boundary", baseline.EffectiveFrom},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewRuleSet("rule-flood", baseline, 2, "flood", "note", tc.effectiveFrom, now)
			if err == nil {
				t.Fatal("expected boundary conflict, got nil")
			}
			var domainErr *Error
			if !errors.As(err, &domainErr) || domainErr.Code != "rule_effective_boundary_conflict" {
				t.Fatalf("expected rule_effective_boundary_conflict, got %v", err)
			}
		})
	}
}

func TestNewRuleSetAdvancesRevisionAndHashesContent(t *testing.T) {
	now := testNow()
	baseline := BaselineRuleSet(now)
	boundary := now.Add(24 * time.Hour)

	first, err := NewRuleSet("rule-flood", baseline, 2, "flood", "tightened", boundary, now)
	if err != nil {
		t.Fatalf("publish first revision: %v", err)
	}
	if first.Revision != 2 {
		t.Fatalf("revision: expected 2, got %d", first.Revision)
	}
	if first.ContentHash == "" || first.ContentHash == baseline.ContentHash {
		t.Fatal("revision must carry its own content hash")
	}
	basis := first.Basis()
	if basis.ContentHash != first.ContentHash || basis.Revision != first.Revision {
		t.Fatal("basis snapshot must match the revision")
	}

	second, err := NewRuleSet("rule-post", first, 1, "post-flood", "tighter", boundary.Add(time.Hour), now)
	if err != nil {
		t.Fatalf("publish second revision: %v", err)
	}
	if second.Revision != 3 {
		t.Fatalf("revision: expected 3, got %d", second.Revision)
	}
}

func TestNewRuleSetValidatesContent(t *testing.T) {
	now := testNow()
	baseline := BaselineRuleSet(now)
	boundary := now.Add(time.Hour)

	if _, err := NewRuleSet("rule-flood", baseline, 0, "flood", "note", boundary, now); err == nil {
		t.Fatal("zero max aperture must be invalid")
	}
	if _, err := NewRuleSet("rule-flood", baseline, 2, " ", "note", boundary, now); err == nil {
		t.Fatal("blank season must be invalid")
	}
	if _, err := NewRuleSet("rule-flood", baseline, 2, "flood", " ", boundary, now); err == nil {
		t.Fatal("blank note must be invalid")
	}
	if _, err := NewRuleSet(BaselineRuleID, baseline, 2, "flood", "note", boundary, now); err == nil {
		t.Fatal("baseline id must be reserved")
	}
}

func TestRuleSetPermitsClearOnlyWithinBound(t *testing.T) {
	rules := RuleSet{MaxAllowedAperture: 2.0}
	if !rules.PermitsClear(2.0) {
		t.Fatal("target equal to the bound must clear")
	}
	if !rules.PermitsClear(0) {
		t.Fatal("zero target must clear")
	}
	if rules.PermitsClear(2.01) {
		t.Fatal("target above the bound must not clear")
	}
	basis := rules.Basis()
	if basis.PermitsClear(2.01) {
		t.Fatal("stored basis must enforce the same bound")
	}
}
