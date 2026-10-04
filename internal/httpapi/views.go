package httpapi

import (
	"time"

	"example.com/spillway-gate-coordinator/internal/contracts"
	"example.com/spillway-gate-coordinator/internal/domain"
)

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func formatBoundary(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return formatTime(value)
}

func gateView(gate domain.Gate) contracts.GateView {
	return contracts.GateView{
		ID:              gate.ID,
		Name:            gate.Name,
		Status:          string(gate.Status),
		Aperture:        gate.Aperture,
		MaxAperture:     gate.MaxAperture,
		ActiveCommandID: gate.ActiveCommandID,
		UpdatedAt:       formatTime(gate.UpdatedAt),
		Version:         gate.Version,
	}
}

func ruleBasisView(basis domain.RuleBasis) contracts.RuleBasisView {
	return contracts.RuleBasisView{
		RuleSetID:          basis.RuleSetID,
		Revision:           basis.Revision,
		Season:             basis.Season,
		Note:               basis.Note,
		EffectiveFrom:      formatBoundary(basis.EffectiveFrom),
		MaxAllowedAperture: basis.MaxAllowedAperture,
		ContentHash:        basis.ContentHash,
	}
}

func ruleSetView(ruleSet domain.RuleSet, effectiveTo time.Time, active bool) contracts.RuleSetView {
	return contracts.RuleSetView{
		RuleSetID:          ruleSet.RuleSetID,
		Revision:           ruleSet.Revision,
		Season:             ruleSet.Season,
		Note:               ruleSet.Note,
		EffectiveFrom:      formatBoundary(ruleSet.EffectiveFrom),
		EffectiveTo:        formatBoundary(effectiveTo),
		MaxAllowedAperture: ruleSet.MaxAllowedAperture,
		ContentHash:        ruleSet.ContentHash,
		CreatedAt:          formatTime(ruleSet.CreatedAt),
		Active:             active,
		Basis:              ruleBasisView(ruleSet.Basis()),
	}
}

func commandView(command domain.GateCommand, includeToken bool) contracts.CommandView {
	view := contracts.CommandView{
		ID:               command.ID,
		GateID:           command.GateID,
		TargetAperture:   command.TargetAperture,
		StartingAperture: command.StartingAperture,
		Reason:           command.Reason,
		RequestedBy:      command.RequestedBy,
		Status:           string(command.Status),
		ExecutedBy:       command.ExecutedBy,
		TerminalReason:   command.TerminalReason,
		CreatedAt:        formatTime(command.CreatedAt),
		UpdatedAt:        formatTime(command.UpdatedAt),
		Version:          command.Version,
	}

	if includeToken && command.Status == domain.CommandApproved {
		view.ExecutionToken = command.ExecutionToken
	}
	if command.Interlock != nil {
		view.Interlock = &contracts.InterlockDecisionView{
			Verdict:    string(command.Interlock.Verdict),
			Verifier:   command.Interlock.Verifier,
			Note:       command.Interlock.Note,
			ReviewedAt: formatTime(command.Interlock.ReviewedAt),
			Basis:      ruleBasisView(command.Interlock.Basis),
		}
	}
	return view
}
