package storage

import (
	"context"
	"sync"
	"time"

	"example.com/spillway-gate-coordinator/internal/domain"
)

type MemoryRepository struct {
	mu       sync.RWMutex
	gates    map[string]domain.Gate
	commands map[string]domain.GateCommand
	// ruleSets is the append-only revision chain in publication order. It is
	// guarded by the same mutex as reviews, so a revision publication cannot
	// interleave with a rule resolution during a review.
	ruleSets []domain.RuleSet
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		gates:    make(map[string]domain.Gate),
		commands: make(map[string]domain.GateCommand),
	}
}

func (m *MemoryRepository) CreateGate(_ context.Context, gate domain.Gate) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.gates[gate.ID]; exists {
		return domain.Conflict("duplicate_gate_id", "gate id already exists")
	}
	m.gates[gate.ID] = gate
	return nil
}

func (m *MemoryRepository) Gate(_ context.Context, id string) (domain.Gate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	gate, exists := m.gates[id]
	if !exists {
		return domain.Gate{}, domain.NotFound("gate_not_found", "gate id was not found")
	}
	return gate, nil
}

func (m *MemoryRepository) CreateCommand(_ context.Context, command domain.GateCommand) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	gate, exists := m.gates[command.GateID]
	if !exists {
		return domain.NotFound("gate_not_found", "gate id was not found")
	}
	if gate.Status == domain.GateFaulted {
		return domain.Conflict("gate_faulted", "faulted gates cannot receive new commands")
	}
	if command.StartingAperture != gate.Aperture {
		return domain.Conflict("gate_aperture_changed", "gate aperture changed before the command was stored")
	}
	if command.TargetAperture > gate.MaxAperture {
		return domain.Invalid("invalid_target_aperture", "target aperture is outside the gate limit")
	}
	if _, exists := m.commands[command.ID]; exists {
		return domain.Conflict("duplicate_command_id", "command id already exists")
	}

	m.commands[command.ID] = command
	return nil
}

func (m *MemoryRepository) Command(_ context.Context, id string) (domain.GateCommand, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	command, exists := m.commands[id]
	if !exists {
		return domain.GateCommand{}, domain.NotFound("command_not_found", "command id was not found")
	}
	return cloneCommand(command), nil
}

func (m *MemoryRepository) ReviewCommand(
	_ context.Context,
	commandID string,
	decision domain.InterlockDecision,
	token string,
	now time.Time,
) (domain.GateCommand, domain.Gate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	command, exists := m.commands[commandID]
	if !exists {
		return domain.GateCommand{}, domain.Gate{}, domain.NotFound("command_not_found", "command id was not found")
	}
	gate, exists := m.gates[command.GateID]
	if !exists {
		return domain.GateCommand{}, domain.Gate{}, domain.NotFound("gate_not_found", "gate id was not found")
	}

	// Resolve the rules readable and in effect at the review instant under the
	// write lock, then snapshot the basis onto the decision. A concurrent rule
	// publication cannot change the basis of this review.
	rules, err := m.effectiveRuleSetLocked(now)
	if err != nil {
		return domain.GateCommand{}, domain.Gate{}, err
	}
	basis := rules.Basis()

	if decision.IsClear() && !basis.PermitsClear(command.TargetAperture) {
		// The command stays pending: no occupation, token, or conclusion is
		// written, so the outcome is reproducible even if rules are revised
		// before the next review attempt.
		return domain.GateCommand{}, domain.Gate{}, domain.Conflict(
			"interlock_clear_forbidden_by_rules",
			"clear verdict violates interlock rule revision "+
				rules.RuleSetID+" for the requested target aperture",
		)
	}

	nextCommand := cloneCommand(command)
	if err := nextCommand.Review(decision.WithBasis(basis), token, now); err != nil {
		return domain.GateCommand{}, domain.Gate{}, err
	}

	nextGate := gate
	if decision.IsClear() {
		if err := nextGate.Reserve(nextCommand.ID, now); err != nil {
			return domain.GateCommand{}, domain.Gate{}, err
		}
	}

	m.commands[nextCommand.ID] = nextCommand
	m.gates[nextGate.ID] = nextGate
	return cloneCommand(nextCommand), nextGate, nil
}

func (m *MemoryRepository) ExecuteCommand(
	_ context.Context,
	commandID string,
	token string,
	operator string,
	simulateFault bool,
	now time.Time,
) (domain.GateCommand, domain.Gate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	command, exists := m.commands[commandID]
	if !exists {
		return domain.GateCommand{}, domain.Gate{}, domain.NotFound("command_not_found", "command id was not found")
	}
	gate, exists := m.gates[command.GateID]
	if !exists {
		return domain.GateCommand{}, domain.Gate{}, domain.NotFound("gate_not_found", "gate id was not found")
	}

	nextCommand := cloneCommand(command)
	if err := nextCommand.BeginExecution(token, operator, now); err != nil {
		return domain.GateCommand{}, domain.Gate{}, err
	}

	nextGate := gate
	if err := nextGate.StartMovement(nextCommand.ID, now); err != nil {
		return domain.GateCommand{}, domain.Gate{}, err
	}

	if simulateFault {
		if err := nextGate.Settle(nextCommand.TargetAperture, true, now); err != nil {
			return domain.GateCommand{}, domain.Gate{}, err
		}
		if err := nextCommand.Abort("simulated_gate_fault", now); err != nil {
			return domain.GateCommand{}, domain.Gate{}, err
		}
	} else {
		if err := nextGate.Settle(nextCommand.TargetAperture, false, now); err != nil {
			return domain.GateCommand{}, domain.Gate{}, err
		}
		if err := nextCommand.Complete(now); err != nil {
			return domain.GateCommand{}, domain.Gate{}, err
		}
	}

	m.commands[nextCommand.ID] = nextCommand
	m.gates[nextGate.ID] = nextGate
	return cloneCommand(nextCommand), nextGate, nil
}

func (m *MemoryRepository) SeedBaselineRules(_ context.Context, baseline domain.RuleSet) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.ruleSets) > 0 {
		return nil
	}
	m.ruleSets = []domain.RuleSet{baseline}
	return nil
}

func (m *MemoryRepository) CreateRuleSet(_ context.Context, ruleSet domain.RuleSet) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.ruleSets) == 0 {
		return domain.Internal("rule_chain_missing", "baseline rule revision must be seeded first")
	}
	for _, existing := range m.ruleSets {
		if existing.RuleSetID == ruleSet.RuleSetID {
			return domain.Conflict("duplicate_rule_set_id", "rule set id already exists")
		}
	}
	latest := m.ruleSets[len(m.ruleSets)-1]
	if ruleSet.Revision != latest.Revision+1 {
		return domain.Conflict("rule_revision_out_of_order", "rule revision must extend the current chain consecutively")
	}
	if !ruleSet.EffectiveFrom.After(latest.EffectiveFrom) {
		return domain.Conflict(
			"rule_effective_boundary_conflict",
			"a new rule revision must start strictly after the previous effective boundary",
		)
	}

	m.ruleSets = append(m.ruleSets, ruleSet)
	return nil
}

func (m *MemoryRepository) RuleSet(_ context.Context, ruleSetID string) (domain.RuleSet, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, ruleSet := range m.ruleSets {
		if ruleSet.RuleSetID == ruleSetID {
			return ruleSet, nil
		}
	}
	return domain.RuleSet{}, domain.NotFound("rule_set_not_found", "rule set revision was not found")
}

func (m *MemoryRepository) RuleSets(_ context.Context) ([]domain.RuleSet, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return append([]domain.RuleSet(nil), m.ruleSets...), nil
}

func (m *MemoryRepository) EffectiveRuleSet(_ context.Context, t time.Time) (domain.RuleSet, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.effectiveRuleSetLocked(t)
}

func (m *MemoryRepository) effectiveRuleSetLocked(t time.Time) (domain.RuleSet, error) {
	for i := len(m.ruleSets) - 1; i >= 0; i-- {
		candidate := m.ruleSets[i]
		if candidate.ValidAt(t) {
			return candidate, nil
		}
	}
	return domain.RuleSet{}, domain.Internal("no_effective_rule_set", "no readable interlock rule revision is in effect")
}

func cloneCommand(command domain.GateCommand) domain.GateCommand {
	if command.Interlock == nil {
		return command
	}
	decision := *command.Interlock
	command.Interlock = &decision
	return command
}
