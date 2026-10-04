package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// BaselineRuleID identifies the seeded rule set that is in effect before any
// published revision. Its effective boundary is the zero time, so it is valid
// at every readable instant.
const BaselineRuleID = "rule-baseline"

// BaselineMaxAperture is the permissive bound used by the seeded baseline.
// Gate-level max aperture still applies independently.
const BaselineMaxAperture = 1000.0

// RuleBasis is the immutable snapshot of the rule set a review was decided
// against. It is stored on the interlock decision so that historical clear and
// blocked conclusions stay self-describing after later rule revisions.
type RuleBasis struct {
	RuleSetID          string
	Revision           int
	Season             string
	Note               string
	EffectiveFrom      time.Time
	MaxAllowedAperture float64
	ContentHash        string
}

// RuleSet is one published revision of the interlock rules. Revisions are
// append-only; each revision's EffectiveFrom boundary must be strictly later
// than the previous revision's boundary.
type RuleSet struct {
	RuleSetID          string
	Revision           int
	Season             string
	Note               string
	EffectiveFrom      time.Time
	MaxAllowedAperture float64
	ContentHash        string
	CreatedAt          time.Time
}

// BaselineRuleSet returns the seeded first revision.
func BaselineRuleSet(now time.Time) RuleSet {
	baseline := RuleSet{
		RuleSetID:          BaselineRuleID,
		Revision:           1,
		Season:             "baseline",
		Note:               "基线联锁规则，未发布季节性修订前持续有效",
		EffectiveFrom:      time.Time{},
		MaxAllowedAperture: BaselineMaxAperture,
		CreatedAt:          now.UTC(),
	}
	baseline.ContentHash = ruleContentHash(
		baseline.Revision,
		baseline.Season,
		baseline.Note,
		baseline.EffectiveFrom,
		baseline.MaxAllowedAperture,
	)
	return baseline
}

// NewRuleSet publishes a revision directly after previous. The effective
// boundary must move forward; revisions can never be inserted retroactively,
// which keeps the rule chain monotonic and keeps reviews reproducible.
func NewRuleSet(
	ruleSetID string,
	previous RuleSet,
	maxAllowedAperture float64,
	season string,
	note string,
	effectiveFrom time.Time,
	now time.Time,
) (RuleSet, error) {
	ruleSetID = strings.TrimSpace(ruleSetID)
	if ruleSetID == "" {
		return RuleSet{}, Invalid("invalid_rule_set_id", "rule set id is required")
	}
	if ruleSetID == BaselineRuleID {
		return RuleSet{}, Invalid("invalid_rule_set_id", "baseline rule set id is reserved")
	}

	if maxAllowedAperture <= 0 || maxAllowedAperture > 1000 {
		return RuleSet{}, Invalid("invalid_max_allowed_aperture", "max allowed aperture must be greater than zero")
	}

	season = strings.TrimSpace(season)
	if season == "" {
		return RuleSet{}, Invalid("invalid_rule_season", "season is required")
	}
	note = strings.TrimSpace(note)
	if note == "" {
		return RuleSet{}, Invalid("invalid_rule_note", "rule note is required")
	}

	if !effectiveFrom.After(previous.EffectiveFrom) {
		return RuleSet{}, Conflict(
			"rule_effective_boundary_conflict",
			"a new rule revision must start strictly after the previous effective boundary",
		)
	}

	revision := previous.Revision + 1
	ruleSet := RuleSet{
		RuleSetID:          ruleSetID,
		Revision:           revision,
		Season:             season,
		Note:               note,
		EffectiveFrom:      effectiveFrom.UTC(),
		MaxAllowedAperture: maxAllowedAperture,
		CreatedAt:          now.UTC(),
	}
	ruleSet.ContentHash = ruleContentHash(
		ruleSet.Revision,
		ruleSet.Season,
		ruleSet.Note,
		ruleSet.EffectiveFrom,
		ruleSet.MaxAllowedAperture,
	)
	return ruleSet, nil
}

// ValidAt reports whether the revision is readable and in effect at instant t.
func (r RuleSet) ValidAt(t time.Time) bool {
	return !r.EffectiveFrom.After(t)
}

// PermitsClear reports whether a clear verdict for the given target aperture is
// allowed under this revision. A blocked verdict is the verifier's decision and
// is never overridden by the rule set.
func (r RuleSet) PermitsClear(targetAperture float64) bool {
	return targetAperture <= r.MaxAllowedAperture
}

// Basis snapshots the revision for storage on an interlock decision.
func (r RuleSet) Basis() RuleBasis {
	return RuleBasis{
		RuleSetID:          r.RuleSetID,
		Revision:           r.Revision,
		Season:             r.Season,
		Note:               r.Note,
		EffectiveFrom:      r.EffectiveFrom,
		MaxAllowedAperture: r.MaxAllowedAperture,
		ContentHash:        r.ContentHash,
	}
}

// PermitsClear mirrors RuleSet.PermitsClear for a stored basis snapshot.
func (b RuleBasis) PermitsClear(targetAperture float64) bool {
	return targetAperture <= b.MaxAllowedAperture
}

func ruleContentHash(
	revision int,
	season string,
	note string,
	effectiveFrom time.Time,
	maxAllowedAperture float64,
) string {
	h := sha256.New()
	fmt.Fprintln(h, revision)
	fmt.Fprintln(h, season)
	fmt.Fprintln(h, note)
	fmt.Fprintln(h, effectiveFrom.UTC().Format(time.RFC3339Nano))
	fmt.Fprintln(h, strconv.FormatFloat(maxAllowedAperture, 'g', -1, 64))
	return hex.EncodeToString(h.Sum(nil))
}
