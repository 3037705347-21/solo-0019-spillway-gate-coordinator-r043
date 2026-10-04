package contracts

type RegisterGateRequest struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	MaxAperture     float64 `json:"max_aperture"`
	InitialAperture float64 `json:"initial_aperture"`
}

type RequestCommandRequest struct {
	GateID         string  `json:"gate_id"`
	TargetAperture float64 `json:"target_aperture"`
	Reason         string  `json:"reason"`
	RequestedBy    string  `json:"requested_by"`
}

type ReviewInterlockRequest struct {
	Verdict  string `json:"verdict"`
	Verifier string `json:"verifier"`
	Note     string `json:"note"`
}

type ExecuteCommandRequest struct {
	Operator      string `json:"operator"`
	Token         string `json:"token"`
	SimulateFault bool   `json:"simulate_fault"`
}

// PublishRuleSetRequest appends a new interlock rule revision. EffectiveFrom is
// an optional RFC3339 timestamp; when omitted the revision takes effect now.
type PublishRuleSetRequest struct {
	MaxAllowedAperture float64 `json:"max_allowed_aperture"`
	Season             string  `json:"season"`
	Note               string  `json:"note"`
	EffectiveFrom      string  `json:"effective_from"`
}
