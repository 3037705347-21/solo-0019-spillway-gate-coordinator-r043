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
	rules    []domain.InterlockRule
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

// PublishRule appends a new rule version and closes the previous effective
// window in the same critical section. Stored versions are never mutated
// beyond closing the open window, so historical decisions keep a readable
// basis. Gates and commands are untouched: a revision cannot rewrite
// existing reservations, terminal states, or recorded conclusions.
func (m *MemoryRepository) PublishRule(
	_ context.Context,
	rule domain.InterlockRule,
) (domain.InterlockRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.rules) > 0 {
		latest := m.rules[len(m.rules)-1]
		if err := latest.CloseAt(rule.EffectiveFrom); err != nil {
			return domain.InterlockRule{}, err
		}
		m.rules[len(m.rules)-1] = latest
	}

	rule.Sequence = int64(len(m.rules)) + 1
	m.rules = append(m.rules, rule)
	return rule, nil
}

func (m *MemoryRepository) Rules(_ context.Context) ([]domain.InterlockRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rules := make([]domain.InterlockRule, len(m.rules))
	copy(rules, m.rules)
	return rules, nil
}

func (m *MemoryRepository) RuleEffectiveAt(
	_ context.Context,
	now time.Time,
) (domain.InterlockRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rule, effective := m.ruleEffectiveAtLocked(now)
	if !effective {
		return domain.InterlockRule{}, domain.NotFound(
			"no_effective_interlock_rule",
			"no interlock rule is effective at the requested time",
		)
	}
	return rule, nil
}

func (m *MemoryRepository) ruleEffectiveAtLocked(now time.Time) (domain.InterlockRule, bool) {
	for i := len(m.rules) - 1; i >= 0; i-- {
		if m.rules[i].EffectiveAt(now) {
			return m.rules[i], true
		}
	}
	return domain.InterlockRule{}, false
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

	// The review must be based on the rule version effective at review time;
	// the basis is recorded on the decision so it stays explainable after
	// later revisions.
	rule, effective := m.ruleEffectiveAtLocked(now)
	if !effective {
		return domain.GateCommand{}, domain.Gate{}, domain.Conflict(
			"interlock_rule_unavailable",
			"no interlock rule is effective at review time",
		)
	}
	decision.RuleID = rule.ID
	decision.RuleSequence = rule.Sequence

	nextCommand := cloneCommand(command)
	if err := nextCommand.Review(decision, token, now); err != nil {
		return domain.GateCommand{}, domain.Gate{}, err
	}

	nextGate := gate
	if decision.IsClear() {
		if err := rule.Check(nextCommand); err != nil {
			return domain.GateCommand{}, domain.Gate{}, err
		}
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

func cloneCommand(command domain.GateCommand) domain.GateCommand {
	if command.Interlock == nil {
		return command
	}
	decision := *command.Interlock
	command.Interlock = &decision
	return command
}
