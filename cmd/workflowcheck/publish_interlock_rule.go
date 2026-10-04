package main

import (
	"fmt"
	"net/http"
)

func runPublishInterlockRule(client *apiClient) error {
	if _, err := fixtureGate(client, "gate-central", "中央泄洪闸", 5, 0.5); err != nil {
		return err
	}

	baseline, err := client.get("/interlock-rules/current")
	if err != nil {
		return err
	}
	if err := requireStatus(baseline, http.StatusOK, "read baseline interlock rule"); err != nil {
		return err
	}
	baselineRule, err := objectField(baseline.Body, "rule")
	if err != nil {
		return err
	}
	baselineSequence, err := floatField(baselineRule, "sequence")
	if err != nil {
		return err
	}
	if baselineSequence != 1 {
		return fmt.Errorf("baseline rule sequence: expected 1, got %v", baselineSequence)
	}

	// A command approved under the baseline keeps its reservation, token, and
	// recorded rule basis across the revision.
	first, err := fixtureCommand(client, "gate-central", 2.0, "常规流量调整", "dispatcher-lin")
	if err != nil {
		return err
	}
	firstID, err := stringField(first, "id")
	if err != nil {
		return err
	}
	approved, err := fixtureClearReview(client, firstID, "safety-wu")
	if err != nil {
		return err
	}
	token, err := stringField(approved, "execution_token")
	if err != nil {
		return err
	}
	firstBasis, err := decisionRuleSequence(approved)
	if err != nil {
		return err
	}
	if firstBasis != 1 {
		return fmt.Errorf("first decision rule sequence: expected 1, got %v", firstBasis)
	}

	// Seasonal revision: flood-season limit takes effect immediately.
	revised, err := client.post("/interlock-rules", map[string]any{
		"max_target_aperture": 1.5,
		"reason":              "汛期水位上涨，收紧最大开度",
		"issued_by":           "safety-wu",
	})
	if err != nil {
		return err
	}
	if err := requireStatus(revised, http.StatusCreated, "publish revised interlock rule"); err != nil {
		return err
	}
	revisedRule, err := objectField(revised.Body, "rule")
	if err != nil {
		return err
	}
	revisedSequence, err := floatField(revisedRule, "sequence")
	if err != nil {
		return err
	}
	if revisedSequence != 2 {
		return fmt.Errorf("revised rule sequence: expected 2, got %v", revisedSequence)
	}
	revisedFrom, err := stringField(revisedRule, "effective_from")
	if err != nil {
		return err
	}

	// The revision must not rewrite the existing reservation or the recorded
	// historical conclusion.
	gateResult, err := client.get("/gates/gate-central")
	if err != nil {
		return err
	}
	if err := requireStatus(gateResult, http.StatusOK, "read gate after revision"); err != nil {
		return err
	}
	gate, err := objectField(gateResult.Body, "gate")
	if err != nil {
		return err
	}
	gateStatus, err := stringField(gate, "status")
	if err != nil {
		return err
	}
	activeCommandID, err := stringField(gate, "active_command_id")
	if err != nil {
		return err
	}
	if gateStatus != "reserved" || activeCommandID != firstID {
		return fmt.Errorf(
			"revision rewrote gate reservation: status=%q active_command_id=%q",
			gateStatus,
			activeCommandID,
		)
	}
	firstAfter, err := client.get("/commands/" + firstID)
	if err != nil {
		return err
	}
	if err := requireStatus(firstAfter, http.StatusOK, "read approved command after revision"); err != nil {
		return err
	}
	firstCommand, err := objectField(firstAfter.Body, "command")
	if err != nil {
		return err
	}
	keptBasis, err := decisionRuleSequence(firstCommand)
	if err != nil {
		return err
	}
	if keptBasis != 1 {
		return fmt.Errorf("revision rewrote historical basis: expected rule v1, got v%v", keptBasis)
	}

	// A pending command reviewed after the revision is judged by the revised
	// rule: a clear verdict that violates it is a stable conflict and the
	// command stays pending.
	second, err := fixtureCommand(client, "gate-central", 3.0, "待复核命令遇规则修订", "dispatcher-lin")
	if err != nil {
		return err
	}
	secondID, err := stringField(second, "id")
	if err != nil {
		return err
	}
	violation, err := client.post("/commands/"+secondID+"/interlock", map[string]any{
		"verdict":  "clear",
		"verifier": "safety-wu",
		"note":     "尝试按旧条件放行",
	})
	if err != nil {
		return err
	}
	if err := requireStatus(violation, http.StatusConflict, "reject clear violating revised rule"); err != nil {
		return err
	}
	violationCode, err := stringField(violation.Body, "code")
	if err != nil {
		return err
	}
	if violationCode != "interlock_rule_violated" {
		return fmt.Errorf("violation error code: expected interlock_rule_violated, got %q", violationCode)
	}
	stillPending, err := client.get("/commands/" + secondID)
	if err != nil {
		return err
	}
	pendingCommand, err := objectField(stillPending.Body, "command")
	if err != nil {
		return err
	}
	pendingStatus, err := stringField(pendingCommand, "status")
	if err != nil {
		return err
	}
	if pendingStatus != "pending" {
		return fmt.Errorf("violated command status: expected pending, got %q", pendingStatus)
	}

	// The blocked verdict is recorded against the revised rule version.
	blocked, err := client.post("/commands/"+secondID+"/interlock", map[string]any{
		"verdict":  "blocked",
		"verifier": "safety-wu",
		"note":     "超过汛期最大开度限制",
	})
	if err != nil {
		return err
	}
	if err := requireStatus(blocked, http.StatusOK, "block command under revised rule"); err != nil {
		return err
	}
	blockedCommand, err := objectField(blocked.Body, "command")
	if err != nil {
		return err
	}
	blockedBasis, err := decisionRuleSequence(blockedCommand)
	if err != nil {
		return err
	}
	if blockedBasis != 2 {
		return fmt.Errorf("blocked decision rule sequence: expected 2, got %v", blockedBasis)
	}

	// The rule chain stays readable: the baseline window is closed at the
	// revision boundary and the revised version is open-ended.
	rules, err := client.get("/interlock-rules")
	if err != nil {
		return err
	}
	if err := requireStatus(rules, http.StatusOK, "list interlock rules"); err != nil {
		return err
	}
	ruleList, ok := rules.Body["rules"].([]any)
	if !ok {
		return fmt.Errorf("rules response is missing the rules array")
	}
	if len(ruleList) != 2 {
		return fmt.Errorf("rule count: expected 2, got %d", len(ruleList))
	}
	firstRule, ok := ruleList[0].(map[string]any)
	if !ok {
		return fmt.Errorf("first rule entry is not an object")
	}
	closedTo, err := stringField(firstRule, "effective_to")
	if err != nil {
		return err
	}
	if closedTo != revisedFrom {
		return fmt.Errorf("baseline effective_to: expected %q, got %q", revisedFrom, closedTo)
	}

	// The command approved before the revision still executes with its token.
	executed, err := client.post("/commands/"+firstID+"/execute", map[string]any{
		"operator":       "operator-chen",
		"token":          token,
		"simulate_fault": false,
	})
	if err != nil {
		return err
	}
	if err := requireStatus(executed, http.StatusOK, "execute command approved before revision"); err != nil {
		return err
	}

	fmt.Println("publish-interlock-rule: revision bounded the baseline window, kept reservations and history, and judged pending commands by the revised rule")
	return nil
}

func decisionRuleSequence(command map[string]any) (float64, error) {
	interlock, err := objectField(command, "interlock")
	if err != nil {
		return 0, err
	}
	ruleID, err := stringField(interlock, "rule_id")
	if err != nil {
		return 0, err
	}
	if ruleID == "" {
		return 0, fmt.Errorf("decision did not record a rule id")
	}
	return floatField(interlock, "rule_sequence")
}
