package httpapi

import (
	"context"
	"net/http"
	"time"

	"example.com/spillway-gate-coordinator/internal/contracts"
	"example.com/spillway-gate-coordinator/internal/domain"
)

func (h *handler) publishRuleSet(w http.ResponseWriter, r *http.Request) {
	var request contracts.PublishRuleSetRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeDomainError(w, err)
		return
	}

	ruleSet, err := h.service.PublishRuleSet(r.Context(), request)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	view, err := h.ruleSetView(r.Context(), ruleSet)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, contracts.RuleSetResponse{RuleSet: view})
}

func (h *handler) listRuleSets(w http.ResponseWriter, r *http.Request) {
	ruleSets, err := h.service.RuleSets(r.Context())
	if err != nil {
		writeDomainError(w, err)
		return
	}
	current, err := h.service.CurrentRuleSet(r.Context())
	if err != nil {
		writeDomainError(w, err)
		return
	}

	views := make([]contracts.RuleSetView, 0, len(ruleSets))
	for i, ruleSet := range ruleSets {
		effectiveTo := time.Time{}
		if i+1 < len(ruleSets) {
			effectiveTo = ruleSets[i+1].EffectiveFrom
		}
		views = append(views, ruleSetView(ruleSet, effectiveTo, ruleSet.RuleSetID == current.RuleSetID))
	}
	writeJSON(w, http.StatusOK, contracts.RuleSetListResponse{RuleSets: views})
}

func (h *handler) ruleSet(w http.ResponseWriter, r *http.Request) {
	ruleSet, err := h.service.RuleSet(r.Context(), r.PathValue("id"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	view, err := h.ruleSetView(r.Context(), ruleSet)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, contracts.RuleSetResponse{RuleSet: view})
}

func (h *handler) effectiveRuleSet(w http.ResponseWriter, r *http.Request) {
	at := time.Now().UTC()
	if raw := r.URL.Query().Get("at"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeDomainError(w, domain.Invalid("invalid_at", "at must be an RFC3339 timestamp"))
			return
		}
		at = parsed.UTC()
	}

	ruleSet, err := h.service.EffectiveRuleSet(r.Context(), at)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	current, err := h.service.CurrentRuleSet(r.Context())
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, contracts.RuleSetResponse{
		RuleSet: ruleSetView(ruleSet, time.Time{}, ruleSet.RuleSetID == current.RuleSetID),
	})
}

// ruleSetView enriches a single revision with the next revision's start as its
// effective upper boundary and the current active marker, so every rule view
// reports the same boundary semantics as the chain listing.
func (h *handler) ruleSetView(ctx context.Context, ruleSet domain.RuleSet) (contracts.RuleSetView, error) {
	chain, err := h.service.RuleSets(ctx)
	if err != nil {
		return contracts.RuleSetView{}, err
	}
	current, err := h.service.CurrentRuleSet(ctx)
	if err != nil {
		return contracts.RuleSetView{}, err
	}
	effectiveTo := time.Time{}
	for i, candidate := range chain {
		if candidate.RuleSetID != ruleSet.RuleSetID {
			continue
		}
		if i+1 < len(chain) {
			effectiveTo = chain[i+1].EffectiveFrom
		}
		break
	}
	return ruleSetView(ruleSet, effectiveTo, ruleSet.RuleSetID == current.RuleSetID), nil
}
