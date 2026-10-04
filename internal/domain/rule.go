package domain

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// InterlockRule is one immutable version of the safety interlock rule set.
// Versions form an append-only chain: a version's EffectiveTo is the next
// version's EffectiveFrom, and the latest version stays open-ended. A
// revision never rewrites a stored version; it only closes the previous
// window and appends a new one.
type InterlockRule struct {
	ID                string
	Sequence          int64
	MaxTargetAperture float64
	MaxApertureChange float64
	Reason            string
	IssuedBy          string
	EffectiveFrom     time.Time
	EffectiveTo       time.Time
	CreatedAt         time.Time
}

func NewInterlockRule(
	id string,
	maxTargetAperture float64,
	maxApertureChange float64,
	reason string,
	issuedBy string,
	effectiveFrom time.Time,
	now time.Time,
) (InterlockRule, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return InterlockRule{}, Invalid("invalid_rule_id", "rule id is required")
	}
	if maxTargetAperture < 0 || maxApertureChange < 0 {
		return InterlockRule{}, Invalid("invalid_rule_limit", "rule limits must not be negative")
	}

	reason = strings.TrimSpace(reason)
	if reason == "" {
		return InterlockRule{}, Invalid("invalid_rule_reason", "reason is required")
	}

	issuedBy = strings.TrimSpace(issuedBy)
	if issuedBy == "" {
		return InterlockRule{}, Invalid("invalid_rule_issuer", "issued_by is required")
	}

	if effectiveFrom.IsZero() {
		effectiveFrom = now
	}

	return InterlockRule{
		ID:                id,
		MaxTargetAperture: maxTargetAperture,
		MaxApertureChange: maxApertureChange,
		Reason:            reason,
		IssuedBy:          issuedBy,
		EffectiveFrom:     effectiveFrom.UTC(),
		CreatedAt:         now.UTC(),
	}, nil
}

func (r InterlockRule) EffectiveAt(t time.Time) bool {
	if t.Before(r.EffectiveFrom) {
		return false
	}
	return r.EffectiveTo.IsZero() || t.Before(r.EffectiveTo)
}

func (r *InterlockRule) CloseAt(boundary time.Time) error {
	if !r.EffectiveTo.IsZero() {
		return Conflict("rule_window_closed", "rule effective window is already closed")
	}
	if !boundary.After(r.EffectiveFrom) {
		return Conflict("rule_effective_from_regression", "effective boundary must move forward")
	}
	r.EffectiveTo = boundary.UTC()
	return nil
}

// Check verifies that a clear verdict is allowed under this rule version.
// A violation is a conflict: the command stays pending and nothing is written.
func (r InterlockRule) Check(command GateCommand) error {
	if r.MaxTargetAperture > 0 && command.TargetAperture > r.MaxTargetAperture {
		return Conflict(
			"interlock_rule_violated",
			fmt.Sprintf(
				"interlock rule v%d limits target aperture to %.4g but command %s targets %.4g",
				r.Sequence,
				r.MaxTargetAperture,
				command.ID,
				command.TargetAperture,
			),
		)
	}
	if r.MaxApertureChange > 0 {
		change := math.Abs(command.TargetAperture - command.StartingAperture)
		if change > r.MaxApertureChange {
			return Conflict(
				"interlock_rule_violated",
				fmt.Sprintf(
					"interlock rule v%d limits aperture change to %.4g but command %s changes by %.4g",
					r.Sequence,
					r.MaxApertureChange,
					command.ID,
					change,
				),
			)
		}
	}
	return nil
}
