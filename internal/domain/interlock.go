package domain

import (
	"strings"
	"time"
)

type InterlockVerdict string

const (
	InterlockClear   InterlockVerdict = "clear"
	InterlockBlocked InterlockVerdict = "blocked"
)

type InterlockDecision struct {
	Verdict    InterlockVerdict
	Verifier   string
	Note       string
	ReviewedAt time.Time
	// Basis records the rule revision that was readable and in effect when the
	// review was recorded. It is set at review time and never rewritten by
	// later rule revisions.
	Basis RuleBasis
}

func NewInterlockDecision(verdict, verifier, note string, now time.Time) (InterlockDecision, error) {
	normalized := InterlockVerdict(strings.ToLower(strings.TrimSpace(verdict)))
	if normalized != InterlockClear && normalized != InterlockBlocked {
		return InterlockDecision{}, Invalid("invalid_interlock_verdict", "verdict must be clear or blocked")
	}

	verifier = strings.TrimSpace(verifier)
	if verifier == "" {
		return InterlockDecision{}, Invalid("invalid_interlock_verifier", "verifier is required")
	}

	note = strings.TrimSpace(note)
	if normalized == InterlockBlocked && note == "" {
		return InterlockDecision{}, Invalid("invalid_interlock_note", "blocked decisions require a note")
	}

	return InterlockDecision{
		Verdict:    normalized,
		Verifier:   verifier,
		Note:       note,
		ReviewedAt: now.UTC(),
	}, nil
}

// WithBasis returns a copy of the decision annotated with the rule revision it
// was evaluated against.
func (d InterlockDecision) WithBasis(basis RuleBasis) InterlockDecision {
	annotated := d
	annotated.Basis = basis
	return annotated
}

func (d InterlockDecision) IsClear() bool {
	return d.Verdict == InterlockClear
}
