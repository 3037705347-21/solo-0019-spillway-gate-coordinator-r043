package main

import (
	"fmt"
	"net/http"
	"time"
)

func runInterlockRuleVersioning(client *apiClient) error {
	// The baseline revision is seeded at startup and stays effective until a
	// published revision takes over.
	baselineList, err := client.get("/rule-sets")
	if err != nil {
		return err
	}
	if err := requireStatus(baselineList, http.StatusOK, "list seeded rule revisions"); err != nil {
		return err
	}
	baselineRevisions, err := objectSliceField(baselineList.Body, "rule_sets")
	if err != nil {
		return err
	}
	if len(baselineRevisions) != 1 {
		return fmt.Errorf("expected one seeded revision, got %d", len(baselineRevisions))
	}
	baselineRule := baselineRevisions[0]
	if id, _ := stringField(baselineRule, "rule_set_id"); id != "rule-baseline" {
		return fmt.Errorf("baseline rule id: expected rule-baseline, got %q", id)
	}
	if revision, _ := intField(baselineRule, "revision"); revision != 1 {
		return fmt.Errorf("baseline revision: expected 1, got %d", revision)
	}
	if active, _ := boolField(baselineRule, "active"); !active {
		return fmt.Errorf("baseline revision must be active before any publication")
	}
	baselineHash, err := stringField(baselineRule, "content_hash")
	if err != nil {
		return err
	}
	if baselineHash == "" {
		return fmt.Errorf("baseline revision must carry a content hash")
	}

	effectiveNow, err := client.get("/rule-sets/effective")
	if err != nil {
		return err
	}
	if err := requireStatus(effectiveNow, http.StatusOK, "resolve effective baseline rules"); err != nil {
		return err
	}
	effectiveRule, err := objectField(effectiveNow.Body, "rule_set")
	if err != nil {
		return err
	}
	if id, _ := stringField(effectiveRule, "rule_set_id"); id != "rule-baseline" {
		return fmt.Errorf("current effective revision: expected baseline, got %q", id)
	}

	if _, err := fixtureGate(client, "gate-season", "汛期泄洪闸", 10, 0.5); err != nil {
		return err
	}

	// A first command is cleared under the permissive baseline before the
	// seasonal revision takes effect.
	oldCommand, err := fixtureCommand(client, "gate-season", 1.5, "修订前放行命令", "dispatcher-qin")
	if err != nil {
		return err
	}
	oldID, err := stringField(oldCommand, "id")
	if err != nil {
		return err
	}
	oldApproved, err := fixtureClearReview(client, oldID, "safety-wu")
	if err != nil {
		return err
	}
	if basis, err := nestedObject(oldApproved, "interlock", "basis"); err != nil {
		return err
	} else if revision, _ := intField(basis, "revision"); revision != 1 {
		return fmt.Errorf("pre-revision clear basis: expected revision 1, got %d", revision)
	}

	// Publish the flood-season revision effective immediately; it tightens the
	// allowed aperture to 2.0.
	seasonal, err := client.post("/rule-sets", map[string]any{
		"max_allowed_aperture": 2.0,
		"season":               "flood-season",
		"note":                 "汛期收紧开度",
	})
	if err != nil {
		return err
	}
	if err := requireStatus(seasonal, http.StatusCreated, "publish flood-season rules"); err != nil {
		return err
	}
	seasonalRule, err := objectField(seasonal.Body, "rule_set")
	if err != nil {
		return err
	}
	seasonalID, err := stringField(seasonalRule, "rule_set_id")
	if err != nil {
		return err
	}
	if revision, _ := intField(seasonalRule, "revision"); revision != 2 {
		return fmt.Errorf("seasonal revision number: expected 2, got %d", revision)
	}

	// A clear verdict above the new bound must be refused without writing a
	// conclusion, occupying the gate, or changing the pending command.
	overLimit, err := fixtureCommand(client, "gate-season", 3.0, "汛期超开度申请", "dispatcher-qin")
	if err != nil {
		return err
	}
	overLimitID, err := stringField(overLimit, "id")
	if err != nil {
		return err
	}
	refused, err := client.post("/commands/"+overLimitID+"/interlock", map[string]any{
		"verdict":  "clear",
		"verifier": "safety-wu",
		"note":     "尝试放行",
	})
	if err != nil {
		return err
	}
	if err := requireStatus(refused, http.StatusConflict, "reject clear verdict violating current rules"); err != nil {
		return err
	}
	if code, _ := stringField(refused.Body, "code"); code != "interlock_clear_forbidden_by_rules" {
		return fmt.Errorf("rule refusal code: expected interlock_clear_forbidden_by_rules, got %q", code)
	}
	pendingRead, err := client.get("/commands/" + overLimitID)
	if err != nil {
		return err
	}
	pendingCommand, err := objectField(pendingRead.Body, "command")
	if err != nil {
		return err
	}
	if status, _ := stringField(pendingCommand, "status"); status != "pending" {
		return fmt.Errorf("command must stay pending after a rule refusal, got %q", status)
	}
	if _, exists := pendingCommand["interlock"]; exists {
		return fmt.Errorf("rule refusal must not write an interlock conclusion")
	}

	// The verifier can still block the pending command; that conclusion records
	// revision 2 as its immutable basis.
	blocked, err := client.post("/commands/"+overLimitID+"/interlock", map[string]any{
		"verdict":  "blocked",
		"verifier": "safety-wu",
		"note":     "汛期开度超限，拒绝",
	})
	if err != nil {
		return err
	}
	if err := requireStatus(blocked, http.StatusOK, "block pending command under seasonal rules"); err != nil {
		return err
	}
	blockedCommand, err := objectField(blocked.Body, "command")
	if err != nil {
		return err
	}
	if status, _ := stringField(blockedCommand, "status"); status != "rejected" {
		return fmt.Errorf("blocked command status: expected rejected, got %q", status)
	}
	blockedBasis, err := nestedObject(blockedCommand, "interlock", "basis")
	if err != nil {
		return err
	}
	if revision, _ := intField(blockedBasis, "revision"); revision != 2 {
		return fmt.Errorf("blocked basis: expected revision 2, got %d", revision)
	}
	if id, _ := stringField(blockedBasis, "rule_set_id"); id != seasonalID {
		return fmt.Errorf("blocked basis id: expected %q, got %q", seasonalID, id)
	}

	// A future revision is scheduled with a later effective boundary.
	futureBoundary := time.Now().UTC().Add(time.Hour)
	scheduled, err := client.post("/rule-sets", map[string]any{
		"max_allowed_aperture": 1.0,
		"season":               "post-flood",
		"note":                 "汛后进一步收紧",
		"effective_from":       futureBoundary.Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	if err := requireStatus(scheduled, http.StatusCreated, "schedule post-flood rules"); err != nil {
		return err
	}
	scheduledRule, err := objectField(scheduled.Body, "rule_set")
	if err != nil {
		return err
	}
	if revision, _ := intField(scheduledRule, "revision"); revision != 3 {
		return fmt.Errorf("scheduled revision number: expected 3, got %d", revision)
	}

	futureEffective, err := client.get("/rule-sets/effective?at=" + futureBoundary.Format(time.RFC3339))
	if err != nil {
		return err
	}
	if err := requireStatus(futureEffective, http.StatusOK, "resolve rules at future boundary"); err != nil {
		return err
	}
	futureRule, err := objectField(futureEffective.Body, "rule_set")
	if err != nil {
		return err
	}
	if revision, _ := intField(futureRule, "revision"); revision != 3 {
		return fmt.Errorf("future effective revision: expected 3, got %d", revision)
	}

	currentEffective, err := client.get("/rule-sets/effective")
	if err != nil {
		return err
	}
	if err := requireStatus(currentEffective, http.StatusOK, "resolve rules still seasonal now"); err != nil {
		return err
	}
	currentRule, err := objectField(currentEffective.Body, "rule_set")
	if err != nil {
		return err
	}
	if revision, _ := intField(currentRule, "revision"); revision != 2 {
		return fmt.Errorf("current effective revision before boundary: expected 2, got %d", revision)
	}

	// The revision chain exposes stable, non-overlapping boundaries.
	chains, err := client.get("/rule-sets")
	if err != nil {
		return err
	}
	if err := requireStatus(chains, http.StatusOK, "list full rule chain"); err != nil {
		return err
	}
	chain, err := objectSliceField(chains.Body, "rule_sets")
	if err != nil {
		return err
	}
	if len(chain) != 3 {
		return fmt.Errorf("rule chain length: expected 3, got %d", len(chain))
	}
	if to, _ := stringField(chain[0], "effective_to"); to == "" {
		return fmt.Errorf("baseline boundary should end when revision 2 starts")
	}
	if active, _ := boolField(chain[1], "active"); !active {
		return fmt.Errorf("revision 2 must be marked active now")
	}
	if active, _ := boolField(chain[2], "active"); active {
		return fmt.Errorf("scheduled revision 3 must not be active yet")
	}

	// Rule publication refuses to move a boundary backward: revisions are
	// append-only and never overwrite prior conclusions.
	stale, err := client.post("/rule-sets", map[string]any{
		"max_allowed_aperture": 3.0,
		"season":               "retroactive",
		"note":                 "试图回插生效边界",
		"effective_from":       time.Now().Add(-time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	if err := requireStatus(stale, http.StatusConflict, "reject retroactive rule boundary"); err != nil {
		return err
	}
	if code, _ := stringField(stale.Body, "code"); code != "rule_effective_boundary_conflict" {
		return fmt.Errorf("boundary conflict code mismatch: got %q", code)
	}
	invalid, err := client.post("/rule-sets", map[string]any{
		"max_allowed_aperture": 0,
		"season":               "broken",
		"note":                 "非法开度",
	})
	if err != nil {
		return err
	}
	if err := requireStatus(invalid, http.StatusBadRequest, "reject invalid rule content"); err != nil {
		return err
	}

	// Executing the command approved under the baseline is unaffected by later
	// revisions; its historical basis snapshot still names revision 1.
	oldToken, err := stringField(oldApproved, "execution_token")
	if err != nil {
		return err
	}
	executed, err := client.post("/commands/"+oldID+"/execute", map[string]any{
		"operator":       "operator-chen",
		"token":          oldToken,
		"simulate_fault": false,
	})
	if err != nil {
		return err
	}
	if err := requireStatus(executed, http.StatusOK, "execute command approved before revisions"); err != nil {
		return err
	}
	executedCommand, err := objectField(executed.Body, "command")
	if err != nil {
		return err
	}
	executedBasis, err := nestedObject(executedCommand, "interlock", "basis")
	if err != nil {
		return err
	}
	if revision, _ := intField(executedBasis, "revision"); revision != 1 {
		return fmt.Errorf("historical basis must still name revision 1 after execution, got %d", revision)
	}

	// A revision stays readable directly by id.
	single, err := client.get("/rule-sets/" + seasonalID)
	if err != nil {
		return err
	}
	if err := requireStatus(single, http.StatusOK, "read seasonal revision by id"); err != nil {
		return err
	}

	fmt.Println("interlock-rule-versioning: append-only rule revisions bound reviews reproducibly")
	return nil
}

func nestedObject(object map[string]any, parent, child string) (map[string]any, error) {
	nested, err := objectField(object, parent)
	if err != nil {
		return nil, err
	}
	return objectField(nested, child)
}

func objectSliceField(body map[string]any, name string) ([]map[string]any, error) {
	value, exists := body[name]
	if !exists {
		return nil, fmt.Errorf("response is missing array field %q", name)
	}
	raw, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("response field %q is not an array", name)
	}
	items := make([]map[string]any, 0, len(raw))
	for _, entry := range raw {
		object, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("response field %q contains a non-object entry", name)
		}
		items = append(items, object)
	}
	return items, nil
}

func intField(object map[string]any, name string) (int, error) {
	value, exists := object[name]
	if !exists {
		return 0, fmt.Errorf("object is missing numeric field %q", name)
	}
	number, ok := value.(float64)
	if !ok {
		return 0, fmt.Errorf("object field %q is not a number", name)
	}
	return int(number), nil
}

func boolField(object map[string]any, name string) (bool, error) {
	value, exists := object[name]
	if !exists {
		return false, fmt.Errorf("object is missing boolean field %q", name)
	}
	flag, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("object field %q is not a boolean", name)
	}
	return flag, nil
}
