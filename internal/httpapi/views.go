package httpapi

import (
	"time"

	"example.com/spillway-gate-coordinator/internal/contracts"
	"example.com/spillway-gate-coordinator/internal/domain"
)

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
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
			Verdict:      string(command.Interlock.Verdict),
			Verifier:     command.Interlock.Verifier,
			Note:         command.Interlock.Note,
			RuleID:       command.Interlock.RuleID,
			RuleSequence: command.Interlock.RuleSequence,
			ReviewedAt:   formatTime(command.Interlock.ReviewedAt),
		}
	}
	return view
}

func ruleView(rule domain.InterlockRule) contracts.InterlockRuleView {
	view := contracts.InterlockRuleView{
		ID:                rule.ID,
		Sequence:          rule.Sequence,
		MaxTargetAperture: rule.MaxTargetAperture,
		MaxApertureChange: rule.MaxApertureChange,
		Reason:            rule.Reason,
		IssuedBy:          rule.IssuedBy,
		EffectiveFrom:     formatTime(rule.EffectiveFrom),
		CreatedAt:         formatTime(rule.CreatedAt),
	}
	if !rule.EffectiveTo.IsZero() {
		view.EffectiveTo = formatTime(rule.EffectiveTo)
	}
	return view
}
