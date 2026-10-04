package contracts

type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type InterlockDecisionView struct {
	Verdict      string `json:"verdict"`
	Verifier     string `json:"verifier"`
	Note         string `json:"note"`
	RuleID       string `json:"rule_id"`
	RuleSequence int64  `json:"rule_sequence"`
	ReviewedAt   string `json:"reviewed_at"`
}

type InterlockRuleView struct {
	ID                string  `json:"id"`
	Sequence          int64   `json:"sequence"`
	MaxTargetAperture float64 `json:"max_target_aperture"`
	MaxApertureChange float64 `json:"max_aperture_change"`
	Reason            string  `json:"reason"`
	IssuedBy          string  `json:"issued_by"`
	EffectiveFrom     string  `json:"effective_from"`
	EffectiveTo       string  `json:"effective_to,omitempty"`
	CreatedAt         string  `json:"created_at"`
}

type GateView struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Status          string  `json:"status"`
	Aperture        float64 `json:"aperture"`
	MaxAperture     float64 `json:"max_aperture"`
	ActiveCommandID string  `json:"active_command_id,omitempty"`
	UpdatedAt       string  `json:"updated_at"`
	Version         int64   `json:"version"`
}

type CommandView struct {
	ID               string                 `json:"id"`
	GateID           string                 `json:"gate_id"`
	TargetAperture   float64                `json:"target_aperture"`
	StartingAperture float64                `json:"starting_aperture"`
	Reason           string                 `json:"reason"`
	RequestedBy      string                 `json:"requested_by"`
	Status           string                 `json:"status"`
	Interlock        *InterlockDecisionView `json:"interlock,omitempty"`
	ExecutionToken   string                 `json:"execution_token,omitempty"`
	ExecutedBy       string                 `json:"executed_by,omitempty"`
	TerminalReason   string                 `json:"terminal_reason,omitempty"`
	CreatedAt        string                 `json:"created_at"`
	UpdatedAt        string                 `json:"updated_at"`
	Version          int64                  `json:"version"`
}

type RegisterGateResponse struct {
	Gate GateView `json:"gate"`
}

type GateResponse struct {
	Gate GateView `json:"gate"`
}

type CommandResponse struct {
	Command CommandView `json:"command"`
}

type ReviewCommandResponse struct {
	Command CommandView `json:"command"`
	Gate    GateView    `json:"gate"`
}

type PublishInterlockRuleResponse struct {
	Rule InterlockRuleView `json:"rule"`
}

type InterlockRulesResponse struct {
	Rules []InterlockRuleView `json:"rules"`
}

type InterlockRuleResponse struct {
	Rule InterlockRuleView `json:"rule"`
}

type ExecuteCommandResponse struct {
	Command CommandView `json:"command"`
	Gate    GateView    `json:"gate"`
}
