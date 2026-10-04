package contracts

type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// RuleBasisView is the immutable rule reference stored on a decision.
type RuleBasisView struct {
	RuleSetID          string  `json:"rule_set_id"`
	Revision           int     `json:"revision"`
	Season             string  `json:"season"`
	Note               string  `json:"note"`
	EffectiveFrom      string  `json:"effective_from"`
	MaxAllowedAperture float64 `json:"max_allowed_aperture"`
	ContentHash        string  `json:"content_hash"`
}

// RuleSetView is one revision with its full effective boundary.
type RuleSetView struct {
	RuleSetID          string        `json:"rule_set_id"`
	Revision           int           `json:"revision"`
	Season             string        `json:"season"`
	Note               string        `json:"note"`
	EffectiveFrom      string        `json:"effective_from"`
	EffectiveTo        string        `json:"effective_to,omitempty"`
	MaxAllowedAperture float64       `json:"max_allowed_aperture"`
	ContentHash        string        `json:"content_hash"`
	CreatedAt          string        `json:"created_at"`
	Active             bool          `json:"active"`
	Basis              RuleBasisView `json:"basis"`
}

type InterlockDecisionView struct {
	Verdict    string        `json:"verdict"`
	Verifier   string        `json:"verifier"`
	Note       string        `json:"note"`
	ReviewedAt string        `json:"reviewed_at"`
	Basis      RuleBasisView `json:"basis"`
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

type ExecuteCommandResponse struct {
	Command CommandView `json:"command"`
	Gate    GateView    `json:"gate"`
}

type RuleSetResponse struct {
	RuleSet RuleSetView `json:"rule_set"`
}

type RuleSetListResponse struct {
	RuleSets []RuleSetView `json:"rule_sets"`
}
